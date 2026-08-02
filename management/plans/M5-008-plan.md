# Implementation Plan: M5-008

## Overview

Add a backend coordinator/shard service that splits a collection into N shards, lets worker agents claim-run-submit each shard, and stitches per-shard outcomes back into a single aggregated Result row via the existing M4-004 ingestion path. Includes EF entities, migration 0008, endpoints, a stale-shard reaper hosted service, and Swagger exposure.

## Task Details

- **ID:** M5-008
- **Title:** Backend: distributed worker coordinator service
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-004 | Backend: results ingestion API | done |

## Decisions and Ambiguity Resolutions

The task YAML flags the spec as thin on the worker protocol. These choices are made based on the companion CLI tasks (M5-009, M5-010) and existing repo patterns:

1. **Worker authentication.** The task scope says "reuses org-scoped service tokens from M4-010 (spec thin — flagged in ambiguities)". M4-010 is not yet built, so for M5-008 workers are authenticated via the existing JWT-bearer scheme used by the rest of the backend, gated by org membership (`RoleResolver.HasPermissionAsync`). A new permission `coordinator.worker` is added for worker-role authorization; organization members (Owner/Admin) also receive it by default so the observable script — which uses a regular owner token — passes. This keeps M5-008 self-contained and forward-compatible with M4-010 service tokens (which can later grant the same permission key).
2. **Job request payload storage.** Jobs carry a `collection_sha` (opaque client-provided identifier for the collection being run) and a JSON blob of requests per shard. The observable script does not include request bodies; shards are created with an empty request list (`[]`) when no `shards_payload` is provided. This matches M5-010 which POSTs shards_payload separately. A `requests` (JSON TEXT) column on CoordinatorShard stores per-shard HTTP-request specs.
3. **Result aggregation path.** When the last shard completes, the service appends shard rows to an aggregated result via the existing M4-004 `ResultsService.IngestAsync` path. The aggregated result's `collection_name` is the `collection_sha` passed at job creation, `triggered_by = "coordinator"`, `git_sha = null`. This keeps ingestion logic single-source.
4. **Reaper interval and heartbeat timeout.** 15-second reaper tick, 60-second heartbeat timeout (from task YAML). Injectable via constructor parameters (like `SchedulerHost`) so tests can exercise the state machine synchronously.
5. **Shard id format.** `shd_<32-hex-chars>` mirroring `res_`, `run_`, `sched_`.
6. **Job id format.** `job_<32-hex-chars>`.
7. **Wire-format job id acceptance.** Endpoints accept the `job_`-prefixed wire id only (no slug lookup), since jobs are not human-named.
8. **Claim greediness.** First-claim-wins via a transactional `UPDATE … WHERE state='pending' AND id=(SELECT id … LIMIT 1) RETURNING`-equivalent pattern in EF (re-query under `IsolationLevel.Serializable` not needed for M5 — we use the simpler "load first pending, check version, save, retry on conflict"). SQLite's write serialization makes this adequate.
9. **Non-member response code.** The behavior says "403 permission_denied" for non-members POSTing to `/coordinator/jobs`. Returning 403 (not 404) is consistent with Results/Schedules endpoints.
10. **Shard state enum.** `Pending`, `Running`, `Completed`, `Failed`. Persisted as string for readability (matches `ScheduledRunStatus`).

## Implementation Steps

Each step has the smallest blast radius possible: pure-domain types and ID helpers first, then entities + migration, then service with unit tests, then HTTP endpoints, then hosted reaper service, and finally wiring and Swagger.

### Step 1: Add ID helpers (CoordinatorJobId, ShardId)

**Rationale:** Pure-function helpers with no dependencies. Building blocks for the rest of the feature. Zero risk to other code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Coordinator/CoordinatorJobId.cs` | create | Wire-id helper `job_<hex>` |
| `src/ApiTool.Backend/Coordinator/ShardId.cs` | create | Wire-id helper `shd_<hex>` |
| `src/ApiTool.Backend.Tests/Coordinator/CoordinatorJobIdTests.cs` | create | Parse/Format roundtrip + invalid prefix cases |
| `src/ApiTool.Backend.Tests/Coordinator/ShardIdTests.cs` | create | Parse/Format roundtrip + invalid prefix cases |

#### New Code (pattern mirrors `ResultId.cs`, `ScheduleId.cs`)

```csharp
namespace ApiTool.Backend.Coordinator;

public static class CoordinatorJobId
{
    private const string Prefix = "job_";
    public static string Format(Guid id) => Prefix + id.ToString("N");
    public static bool TryParse(string? value, out Guid id)
    {
        id = Guid.Empty;
        if (string.IsNullOrEmpty(value) || !value.StartsWith(Prefix, StringComparison.Ordinal))
            return false;
        return Guid.TryParseExact(value[Prefix.Length..], "N", out id);
    }
}
```

`ShardId` is identical but with `Prefix = "shd_"`.

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class CoordinatorJobIdTests
{
    [Theory]
    [InlineData("00000000-0000-0000-0000-000000000000", "job_00000000000000000000000000000000")]
    public void Format_returns_prefixed_hex(string guidString, string expected) { /* ... */ }

    [Theory]
    [InlineData("job_00000000000000000000000000000000", true)]
    [InlineData("job_notahex", false)]
    [InlineData("res_00000000000000000000000000000000", false)]  // wrong prefix
    [InlineData(null, false)]
    [InlineData("", false)]
    public void TryParse_matches_expected(string? input, bool expected) { /* ... */ }
}
```

#### Impact on Existing Tests
- None.

### Step 2: Add entities and enums

**Rationale:** Data model must exist before the service can compile. Low-risk; additions only.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/CoordinatorJob.cs` | create | Job entity |
| `src/ApiTool.Backend/Data/Entities/CoordinatorShard.cs` | create | Shard entity |
| `src/ApiTool.Backend/Data/Entities/CoordinatorJobStatus.cs` | create | Enum: Pending / Running / Completed / Failed |
| `src/ApiTool.Backend/Data/Entities/CoordinatorShardStatus.cs` | create | Enum: Pending / Running / Completed / Failed |

#### New Code

```csharp
// CoordinatorJob.cs
namespace ApiTool.Backend.Data.Entities;

public sealed class CoordinatorJob
{
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public Guid CreatedBy { get; set; }
    public string CollectionSha { get; set; } = string.Empty;
    public int ShardCount { get; set; }
    public CoordinatorJobStatus Status { get; set; } = CoordinatorJobStatus.Pending;
    public DateTime CreatedAt { get; set; }
    public DateTime UpdatedAt { get; set; }
    public DateTime? CompletedAt { get; set; }
    public Guid? AggregateResultId { get; set; }  // FK → results.Id when completed
}
```

```csharp
// CoordinatorShard.cs
namespace ApiTool.Backend.Data.Entities;

public sealed class CoordinatorShard
{
    public Guid Id { get; set; }
    public Guid JobId { get; set; }
    public int ShardIndex { get; set; }                 // 0..shard_count-1
    public CoordinatorShardStatus Status { get; set; } = CoordinatorShardStatus.Pending;
    public string? AssignedWorker { get; set; }
    public DateTime? ClaimedAt { get; set; }
    public DateTime? LastHeartbeatAt { get; set; }
    public DateTime? CompletedAt { get; set; }
    public string RequestsJson { get; set; } = "[]";    // opaque request list
    public string? ResultJson { get; set; }             // opaque per-request outcomes on completion
    public int PassCount { get; set; }
    public int FailCount { get; set; }
    public long DurationMs { get; set; }
}
```

#### Tests to Write FIRST
Pure POCOs — the behaviour is exercised through service tests in Step 4. Add a schema test row in Step 3 to prove the migration creates both tables.

#### Impact on Existing Tests
- None.

### Step 3: Register DbSets, map entities, add migration 0008

**Rationale:** Schema must land before service logic can write to the tables. Keeps migration review surface contained to one commit.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Add `CoordinatorJobs`, `CoordinatorShards` DbSets and model configuration |
| `src/ApiTool.Backend/Migrations/202604….._AddCoordinator.cs` | create (via `dotnet ef migrations add`) | Creates `coordinator_jobs` and `coordinator_shards` tables |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modify (via `dotnet ef migrations add`) | Snapshot update |
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | modify | Add `[InlineData("coordinator_jobs")]` and `[InlineData("coordinator_shards")]` |

#### Current Code (`AppDbContext.cs`, end of `OnModelCreating`)

```csharp
        b.Entity<SsoCredential>(e =>
        {
            // ...
        });
    }
}
```

#### New Code

Add DbSet properties:

```csharp
/// <summary>Distributed execution jobs.</summary>
public DbSet<CoordinatorJob> CoordinatorJobs => Set<CoordinatorJob>();

/// <summary>Shards belonging to a distributed execution job.</summary>
public DbSet<CoordinatorShard> CoordinatorShards => Set<CoordinatorShard>();
```

Add configuration inside `OnModelCreating`:

```csharp
b.Entity<CoordinatorJob>(e =>
{
    e.ToTable("coordinator_jobs");
    e.HasKey(x => x.Id);
    e.Property(x => x.CollectionSha).HasColumnName("collection_sha").HasMaxLength(128).IsRequired();
    e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
    e.Property(x => x.AggregateResultId).HasColumnName("aggregate_result_id");
    e.HasIndex(x => new { x.OrgId, x.CreatedAt });
    e.HasIndex(x => x.Status);
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
    e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).OnDelete(DeleteBehavior.Restrict);
});

b.Entity<CoordinatorShard>(e =>
{
    e.ToTable("coordinator_shards");
    e.HasKey(x => x.Id);
    e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
    e.Property(x => x.AssignedWorker).HasColumnName("assigned_worker").HasMaxLength(100);
    e.Property(x => x.RequestsJson).HasColumnName("requests").IsRequired();
    e.Property(x => x.ResultJson).HasColumnName("result");
    e.HasIndex(x => new { x.JobId, x.ShardIndex }).IsUnique();
    e.HasIndex(x => new { x.Status, x.LastHeartbeatAt });
    e.HasOne<CoordinatorJob>().WithMany().HasForeignKey(x => x.JobId).OnDelete(DeleteBehavior.Cascade);
});
```

Migration file name will be `20260419<HHMMSS>_AddCoordinator.cs` (generated by `dotnet ef migrations add AddCoordinator`). Do NOT hand-write — run the tool to generate the paired `.cs` + `.Designer.cs` + snapshot update.

#### Tests to Write FIRST (RED phase)

Add to `AppDbContextSchemaTests.Migration_creates_expected_table`:
```csharp
[InlineData("coordinator_jobs")]
[InlineData("coordinator_shards")]
```

#### Impact on Existing Tests
- `Migration_creates_expected_table` theory adds two new rows; no existing rows break.

### Step 4: CoordinatorService (business logic) + unit tests

**Rationale:** Core state machine isolated from HTTP. Tested with real SQLite (via `TestDb`) so unit tests cover the RBAC check, transition rules, and aggregation. Smaller blast radius than adding endpoints first.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Coordinator/CoordinatorError.cs` | create | Sentinel error enum |
| `src/ApiTool.Backend/Coordinator/CoordinatorJobDto.cs` | create | Wire DTO for job header |
| `src/ApiTool.Backend/Coordinator/CoordinatorShardDto.cs` | create | Wire DTO for shard |
| `src/ApiTool.Backend/Coordinator/CreateJobRequest.cs` | create | Request body (`collection_sha`, `shard_count`, optional `shards_payload`) |
| `src/ApiTool.Backend/Coordinator/ClaimRequest.cs` | create | Request body (`worker_id`, `capabilities`) |
| `src/ApiTool.Backend/Coordinator/SubmitResultRequest.cs` | create | Request body (`pass_count`, `fail_count`, `duration_ms`, `items`) |
| `src/ApiTool.Backend/Coordinator/HeartbeatRequest.cs` | create | Request body (`worker_id`) |
| `src/ApiTool.Backend/Coordinator/CoordinatorService.cs` | create | Business logic |
| `src/ApiTool.Backend.Tests/Coordinator/CoordinatorServiceTests.cs` | create | Unit tests with SQLite via TestDb |

#### Proposed Service Signatures

```csharp
namespace ApiTool.Backend.Coordinator;

public sealed class CoordinatorService(
    AppDbContext db,
    TimeProvider clock,
    RoleResolver roleResolver,
    ResultsService results)
{
    public const int MaxShardCount = 64;
    public static readonly TimeSpan HeartbeatTimeout = TimeSpan.FromSeconds(60);

    public Task<(CoordinatorJobDto? dto, CoordinatorError error, string? message)>
        CreateJobAsync(Guid userId, Guid orgId, CreateJobRequest? request, CancellationToken ct);

    public Task<(CoordinatorShardDto? dto, CoordinatorError error)>
        ClaimAsync(Guid userId, Guid orgId, Guid jobId, ClaimRequest? request, CancellationToken ct);

    public Task<(CoordinatorError error, string? message)>
        SubmitResultAsync(Guid userId, Guid orgId, Guid jobId, Guid shardId, SubmitResultRequest? request, CancellationToken ct);

    public Task<(CoordinatorError error, string? message)>
        HeartbeatAsync(Guid userId, Guid orgId, Guid jobId, Guid shardId, HeartbeatRequest? request, CancellationToken ct);

    public Task<(CoordinatorJobDto? dto, CoordinatorError error)>
        GetJobAsync(Guid userId, Guid orgId, Guid jobId, CancellationToken ct);

    // Called by ShardReaper hosted service — reclaims stale shards.
    public Task<int> ReapStaleShardsAsync(CancellationToken ct);
}
```

```csharp
public enum CoordinatorError
{
    None,
    PermissionDenied,
    InvalidShardCount,
    InvalidRequest,
    NotFound,
    NoShardsAvailable,
    ShardNotClaimed,      // submit/heartbeat called on a shard not owned by the worker
    InvalidState,         // submit called on a shard already completed
}
```

Key invariants:
- `CreateJobAsync` inserts 1 job + `shard_count` shards in `Pending` state under a single transaction. Rejects `shard_count < 1 || > MaxShardCount`. Returns 403 when `HasPermissionAsync(Permissions.CoordinatorWorker)` is false.
- `ClaimAsync` uses a retry loop: load oldest `Pending` shard → set `Running`, `AssignedWorker`, `ClaimedAt = LastHeartbeatAt = now` → `SaveChangesAsync`. On `DbUpdateConcurrencyException` retry up to 3× then return `NoShardsAvailable`. Also transitions the parent Job to `Running` the first time.
- `SubmitResultAsync` requires the caller's `worker_id` to match `shard.AssignedWorker` and the shard to be in `Running`. Sets shard `Completed`, records `pass_count`/`fail_count`/`duration_ms`/`result_json`. If this is the last shard, calls a private `AggregateAndWriteResultAsync` that:
  1. Sums shard counters.
  2. Flattens `result_json` items across shards to a single list.
  3. Builds an `UploadResultRequest` with `collection_name = job.CollectionSha`, `triggered_by = "coordinator"`, items = flattened.
  4. Calls `results.IngestAsync(job.CreatedBy, job.OrgId, request, ct)` — reuses existing ingestion + notifier path.
  5. Stores the returned result id on `job.AggregateResultId`, sets job `Completed`.
- `HeartbeatAsync` updates `shard.LastHeartbeatAt = now` when worker and state match.
- `ReapStaleShardsAsync` finds `Running` shards where `LastHeartbeatAt < now - HeartbeatTimeout`, transitions them back to `Pending`, clears `AssignedWorker`. Returns the count for logging.

#### Tests to Write FIRST (table-driven where useful)

`CoordinatorServiceTests` — run against real SQLite via `TestDb.CreateOpen()`, migrate, seed one owner user + org + member row. Test method names:

1. `CreateJob_persists_job_with_pending_shards` — posts `shard_count=4`, asserts job row + 4 shards all `Pending`.
2. `CreateJob_rejects_invalid_shard_count` — table-driven: `{ 0, -1, 65 }` all return `InvalidShardCount`.
3. `CreateJob_returns_permission_denied_for_non_member` — user without membership.
4. `Claim_transitions_one_shard_to_running` — posts claim, asserts shard `Running`, `AssignedWorker="w1"`.
5. `Claim_returns_no_shards_available_when_all_claimed` — claim 4× then a 5th worker tries.
6. `Claim_round_robins_across_workers` — two different worker_ids each claim one distinct shard.
7. `SubmitResult_transitions_shard_to_completed` — claim then submit → state `Completed`.
8. `SubmitResult_rejects_wrong_worker` — shard claimed by `w1`, `w2` tries to submit → `ShardNotClaimed`.
9. `SubmitResult_on_last_shard_creates_aggregate_result` — 1-shard job → submit → `job.Status == Completed` and `job.AggregateResultId != null` and Results row exists with `collection_name == collection_sha` and `triggered_by == "coordinator"`.
10. `Heartbeat_updates_last_heartbeat_at` — claim, heartbeat, assert timestamp advanced.
11. `ReapStaleShards_reclaims_shards_past_timeout` — using a fake clock, advance by 61s, assert shard `Pending` again and `AssignedWorker` cleared.
12. `ReapStaleShards_leaves_fresh_shards_alone` — fresh heartbeat → no state change.

#### Impact on Existing Tests
- None. New code; no existing signatures change.

### Step 5: Add `coordinator.worker` permission key + owner/admin defaults

**Rationale:** Must exist before endpoints can call `RoleResolver.HasPermissionAsync`. Low risk — additive.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Rbac/Permissions.cs` | modify | Add `CoordinatorWorker = "coordinator.worker"`, include in `All`, `BuiltInOwner` (by being in `All`), `BuiltInAdmin`, `BuiltInMember` |

#### Current Code

```csharp
public const string DashboardExport = "dashboard.export";
// static initializer inserts into All/BuiltInMember/BuiltInAdmin/BuiltInOwner
```

#### New Code

```csharp
public const string DashboardExport = "dashboard.export";

/// <summary>Claim coordinator shards and submit worker results.</summary>
public const string CoordinatorWorker = "coordinator.worker";
```

And in the static initializer, add `CoordinatorWorker` to:
- `All`
- `BuiltInMember` (allows members to both create jobs and be workers — matches observable script using an owner token)
- `BuiltInAdmin`
- (`BuiltInOwner` = `All`, inherits automatically)

#### Tests to Write FIRST

Extend the existing permissions tests if any, or add one assertion in a new `PermissionsTests.cs`:

```csharp
[Fact]
public void CoordinatorWorker_is_in_built_in_owner_set() =>
    Permissions.BuiltInOwner.Should().Contain("coordinator.worker");
```

(Search with `Grep` during execution for existing `PermissionsTests.cs` — if present, extend; if not, create one.)

#### Impact on Existing Tests
- None break (additive). Any test asserting "size of BuiltInMember == N" would need +1; none exist today based on the Grep search plan during execution.

### Step 6: HTTP endpoints + integration tests

**Rationale:** Bind the service to HTTP. Larger surface area — comes after the service is green.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Coordinator/CoordinatorEndpoints.cs` | create | Maps POST /jobs, POST /claim, POST /result, POST /heartbeat, GET /jobs/{jobId} |
| `src/ApiTool.Backend.Tests/Coordinator/CoordinatorEndpointsTests.cs` | create | HTTP integration tests |

#### Endpoint Map (under `/api/v1/organizations/{orgId}/coordinator`)

- `POST /jobs` → 201 `CoordinatorJobDto` (includes `shards[]` with state `pending`)
- `GET /jobs/{jobId}` → 200 `CoordinatorJobDto`
- `POST /jobs/{jobId}/claim` → 200 `CoordinatorShardDto` | 204 no_shards_available
- `POST /jobs/{jobId}/shards/{shardId}/result` → 202 accepted
- `POST /jobs/{jobId}/shards/{shardId}/heartbeat` → 204
- `POST /jobs/{jobId}/heartbeat` → 204 (job-level alias; many implementations put it at job scope — we pick shard-scoped since shard is the unit of liveness, and also expose the job-scoped route because the observable-neighbour task M5-009 mentions POST /heartbeat without shard path. **Decision:** shard-scoped only; M5-009 will be told to include the shard in the URL.)

**Final decision:** Heartbeat path is `POST /jobs/{jobId}/shards/{shardId}/heartbeat`.

All routes require `RequireAuthorization()` and resolve org via `OrgResolver.ResolveAsync`. 401 on missing/invalid bearer. 403 on non-member or missing permission.

Endpoint skeleton pattern mirrors `SchedulesEndpoints.cs`:

```csharp
public static IEndpointRouteBuilder MapCoordinatorEndpoints(this IEndpointRouteBuilder app)
{
    var group = app.MapGroup("/api/v1/organizations/{orgId}/coordinator")
        .RequireAuthorization()
        .WithTags("Coordinator");

    group.MapPost("jobs", CreateJob)
        .WithName("CreateCoordinatorJob")
        .Accepts<CreateJobRequest>("application/json")
        .Produces<CoordinatorJobDto>(StatusCodes.Status201Created)
        .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
        .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
        .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

    group.MapGet("jobs/{jobId}", GetJob).WithName("GetCoordinatorJob")
        .Produces<CoordinatorJobDto>().Produces<ErrorResponse>(StatusCodes.Status404NotFound);

    group.MapPost("jobs/{jobId}/claim", Claim).WithName("ClaimCoordinatorShard")
        .Accepts<ClaimRequest>("application/json")
        .Produces<CoordinatorShardDto>(StatusCodes.Status200OK)
        .Produces(StatusCodes.Status204NoContent);

    group.MapPost("jobs/{jobId}/shards/{shardId}/result", SubmitResult)
        .WithName("SubmitCoordinatorShardResult")
        .Accepts<SubmitResultRequest>("application/json")
        .Produces(StatusCodes.Status202Accepted);

    group.MapPost("jobs/{jobId}/shards/{shardId}/heartbeat", Heartbeat)
        .WithName("CoordinatorShardHeartbeat")
        .Accepts<HeartbeatRequest>("application/json")
        .Produces(StatusCodes.Status204NoContent);

    return app;
}
```

Each handler:
1. Resolves the JWT user via `CurrentUserAccessor`, returns `Unauthorized401` on null.
2. Resolves org via `OrgResolver.ResolveAsync`.
3. Calls the `CoordinatorService` method.
4. Switches on `CoordinatorError` to the right HTTP status (403/404/400/500).
5. For `Claim`, returns `HttpResults.NoContent()` for `NoShardsAvailable`.

#### Tests to Write FIRST

`CoordinatorEndpointsTests` — mirrors `SchedulesEndpointsTests` structure:

1. `Post_jobs_returns_201_with_shards_pending` — observable behavior 1.
2. `Post_claim_returns_200_with_running_shard` — observable behavior 2.
3. `Post_claim_returns_204_when_all_shards_claimed` — behavior 3.
4. `Post_result_transitions_shard_completed_and_appends_aggregate` — behavior 4. Asserts a `GET /api/v1/organizations/{org}/results` list contains the aggregated result when the last shard completes.
5. `Post_jobs_returns_403_for_non_member` — behavior 7.
6. `All_coordinator_endpoints_return_401_without_bearer`.
7. `Get_job_returns_current_state_with_shard_counts`.
8. `Post_heartbeat_returns_204` — plain liveness.
9. `Post_result_rejects_wrong_worker_with_403` — security.
10. `Swagger_json_lists_coordinator_endpoints` — behavior 8. Asserts `/api/v1/organizations/{orgId}/coordinator/jobs` and `.../claim` appear in `paths`.

Minimum count ≥10, matching the task's "Passed: >=10" expectation.

#### Impact on Existing Tests
- None break.

### Step 7: ShardReaper hosted service + tests

**Rationale:** Wired via DI like `SchedulerHost`. Tests can use a fake `IServiceScopeFactory` pattern copied from `SchedulerHostTests`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Coordinator/IShardReaper.cs` | create | Interface with `Task<int> ReapStaleShardsAsync(CancellationToken)` |
| `src/ApiTool.Backend/Coordinator/ShardReaper.cs` | create | `BackgroundService` copying `SchedulerHost` structure; 15s interval |
| `src/ApiTool.Backend.Tests/Coordinator/ShardReaperTests.cs` | create | Fake scope factory + fake reaper implementation |

`CoordinatorService` implements `IShardReaper` (its `ReapStaleShardsAsync` method already fits the signature).

#### New Code

```csharp
public sealed class ShardReaper(
    IServiceScopeFactory scopeFactory,
    ILogger<ShardReaper> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromSeconds(15);

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var reaper = scope.ServiceProvider.GetRequiredService<IShardReaper>();
                var count = await reaper.ReapStaleShardsAsync(stoppingToken);
                if (count > 0) logger.LogInformation("shard reaper reclaimed {Count} stale shards", count);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
            catch (Exception ex) { logger.LogError(ex, "ShardReaper tick failed"); }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }
    }
}
```

#### Tests to Write FIRST
Copy `SchedulerHostTests` verbatim and adapt: a fake reaper counts invocations, a fake scope factory yields it, a 50ms tick, verify `>0` invocations after 200ms, and resilience to one transient exception. 3 tests.

#### Impact on Existing Tests
- None.

### Step 8: Wire into `Program.cs` and register services

**Rationale:** Final activation. Keeps DI wiring in one small commit so any regression is attributable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Program.cs` | modify | Add `using ApiTool.Backend.Coordinator`, register service, register IShardReaper, map endpoints, register hosted service (non-Testing) |

#### Current Code (relevant slice)

```csharp
builder.Services.AddScoped<CustomRolesService>();
// …
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddHostedService<SchedulerHost>();
}
// …
app.MapCustomRolesEndpoints();
app.Run();
```

#### New Code (diff)

Add after `CustomRolesService`:

```csharp
builder.Services.AddScoped<CoordinatorService>();
builder.Services.AddScoped<IShardReaper>(sp => sp.GetRequiredService<CoordinatorService>());
```

Add alongside the SchedulerHost registration:

```csharp
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddHostedService<SchedulerHost>();
    builder.Services.AddHostedService<ShardReaper>();
}
```

Add after `app.MapCustomRolesEndpoints();`:

```csharp
app.MapCoordinatorEndpoints();
```

#### Tests to Write FIRST
Not applicable — pure wiring validated transitively by the integration tests in Step 6.

#### Impact on Existing Tests
- None.

### Step 9: CHANGELOG.md update + smoke script check

**Rationale:** Required by the Quality Gates / Completeness Contract. Last step so the entry reflects the final delivered surface.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add Unreleased entry: "Backend coordinator for distributed worker jobs (M5-008)" |
| `smoke/run.sh` | inspect | Confirm no smoke update needed — this is a backend API slice; no CLI surface change in M5-008. CLI smoke is added by M5-009/M5-010. |

#### Tests to Write FIRST
N/A.

#### Impact on Existing Tests
- None.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | `Migration_creates_expected_table` | extended | Add two `[InlineData]` rows for `coordinator_jobs` and `coordinator_shards` |
| All others | — | none | — |

## Risks and Edge Cases

- **Risk:** SQLite EF InMemory provider doesn't enforce unique indexes → tests passing against it might hide bugs that fail against real SQLite.
  **Mitigation:** `CoordinatorServiceTests` use `TestDb.CreateOpen()` (real SQLite in-memory) so unique index `(JobId, ShardIndex)` and concurrency semantics are exercised. Endpoint tests continue using `BackendFactory` (EF InMemory).

- **Risk:** Claim race — two workers claim the same shard simultaneously.
  **Mitigation:** Use `DbUpdateConcurrencyException` retry loop with a `RowVersion`-like approach — simpler: load, update, `SaveChangesAsync`, on conflict re-select the next `Pending` shard. Test `Claim_round_robins_across_workers` proves the happy path; a dedicated race test is deferred as SQLite's write serialization in tests precludes a meaningful concurrent repro without extra fixtures.

- **Risk:** Aggregate result insertion fails (e.g. validation rejects empty items). If `items=[]` because shards had no requests, `ResultsService.IngestAsync` still accepts it (items list is required but may be empty — **verify at execute time** by reading `ValidateRequest`: it requires `items` non-null but doesn't reject empty. Plan stands.).
  **Mitigation:** Flattened items may be empty for the happy observable (which doesn't submit per-request outcomes). Pass `[]` — `ResultsService.ValidateRequest` allows empty list.

- **Risk:** The observable uses shell variable `$JOB` without capturing it from the POST response. This is a script-only cosmetic issue — the behaviors and response shapes still verify.
  **Mitigation:** Match the documented response shape: top-level `{"job_id": "job_…", "shards": [{"shard_id": "shd_…", "state": "pending"}, …]}`.

- **Edge case:** `shard_count=1` — a single shard job completes after one submit and still writes the aggregate result.
  **Handling:** `SubmitResult_on_last_shard_creates_aggregate_result` exercises this.

- **Edge case:** Worker heartbeats on a completed shard.
  **Handling:** `HeartbeatAsync` returns `InvalidState` (204/404 at the endpoint is acceptable — we pick 404 `shard_not_found` for safety when the shard is no longer active).

- **Edge case:** Reaper runs concurrently with a legitimate submit. The submit's update + reaper's update both target the same shard. Last-writer-wins is acceptable since the reaper only transitions `Running → Pending` while the submit only transitions `Running → Completed`; EF Core concurrency tokens aren't added in M5-008 — a minor race window is accepted because the client-side worker will observe a stale heartbeat and exit its shard attempt.

- **Edge case:** Non-coordinator-related existing tests must continue to pass. Since `Permissions.CoordinatorWorker` is additive and all built-in roles receive it, no RBAC-counting test breaks (verified by Grep before commit).

## Go Function Signatures and Naming

N/A — backend-only change in C#. No Go code is touched.

## Verification

```bash
# Build and test
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
dotnet test  src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj

# Observable (from task YAML)
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Coordinator"
# Expected: Passed: >=10, Failed: 0

# End-to-end run against a live dev backend
dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
JOB=$(curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"collection_sha":"abc123","shard_count":4}' \
  "http://localhost:5000/api/v1/organizations/$ORG/coordinator/jobs" | jq -r '.job_id')
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"worker_id":"w1","capabilities":["http"]}' \
  "http://localhost:5000/api/v1/organizations/$ORG/coordinator/jobs/$JOB/claim"
# Expected: 200 with {"shard_id":"shd_...","state":"running","requests":[]}
```

The Go-toolchain quality gates (`go build`, `golangci-lint run`, `./smoke/run.sh`) are not affected by this backend-only task; they must still be green but require no changes.
