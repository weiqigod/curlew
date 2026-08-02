# Implementation Plan: M4-006

## Overview
Add a scheduled-run trigger service to the backend: a `SchedulesController` (minimal API endpoints) for CRUD on cron-based schedules, plus a `SchedulerHost` background service that polls every 30 seconds for due schedules and enqueues run records. New EF entities `Schedule` and `ScheduledRun`, migration `0003_Schedules`. Cron parsing uses the Cronos NuGet library. Schedules are org-scoped and gated by the M4-003 RBAC service (admin-only for write operations).

## Task Details
- **ID:** M4-006
- **Title:** Backend: scheduled run trigger service
- **Phase:** M4: Team Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-003 | Backend: organization + seat RBAC data model and service | done |

## Implementation Steps

### Step 1: Add Cronos NuGet Package
**Rationale:** Smallest possible change — just adds the dependency. All subsequent steps rely on Cronos for cron parsing, so this must come first.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/ApiTool.Backend.csproj` | modify | Add `<PackageReference Include="Cronos" Version="0.8.4" />` |

#### Current Code
```xml
<ItemGroup>
    <PackageReference Include="Microsoft.AspNetCore.Authentication.JwtBearer" Version="9.0.0" />
    ...
    <PackageReference Include="Swashbuckle.AspNetCore" Version="7.3.1" />
</ItemGroup>
```

#### New Code
```xml
<ItemGroup>
    <PackageReference Include="Cronos" Version="0.8.4" />
    <PackageReference Include="Microsoft.AspNetCore.Authentication.JwtBearer" Version="9.0.0" />
    ...
    <PackageReference Include="Swashbuckle.AspNetCore" Version="7.3.1" />
</ItemGroup>
```

#### Tests to Write FIRST (RED phase)
None — this is a dependency addition verified by the build succeeding.

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 2: Add EF Entities (Schedule, ScheduledRun) and ScheduleId Helper
**Rationale:** Data model must exist before the service or endpoints can be written. Follows the entity-first pattern established by `Result`/`ResultItem`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/Schedule.cs` | create | New entity for persisted cron schedules |
| `src/ApiTool.Backend/Data/Entities/ScheduledRun.cs` | create | New entity for enqueued runs |
| `src/ApiTool.Backend/Data/Entities/ScheduledRunStatus.cs` | create | Enum: Queued, Running, Completed, Failed |
| `src/ApiTool.Backend/Schedules/ScheduleId.cs` | create | Wire-format id helper (`sched_<hex>`) |
| `src/ApiTool.Backend/Schedules/RunId.cs` | create | Wire-format id helper (`run_<hex>`) |

#### New Code

**Schedule.cs:**
```csharp
namespace ApiTool.Backend.Data.Entities;

/// <summary>A cron-scheduled test run configuration scoped to an organization.</summary>
public sealed class Schedule
{
    /// <summary>Primary key (serialized as <c>sched_&lt;hex&gt;</c>).</summary>
    public Guid Id { get; set; }

    /// <summary>Owning organization foreign key.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Human-readable schedule name (unique per org).</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>Standard 5-field cron expression (e.g. "0 2 * * *").</summary>
    public string CronExpression { get; set; } = string.Empty;

    /// <summary>Reference to the collection file to run (e.g. "smoke.yaml").</summary>
    public string CollectionRef { get; set; } = string.Empty;

    /// <summary>Whether this schedule is currently active.</summary>
    public bool Enabled { get; set; } = true;

    /// <summary>Computed next UTC time the schedule should fire.</summary>
    public DateTime? NextRunAt { get; set; }

    /// <summary>UTC time of the last completed/queued run, or null if never fired.</summary>
    public DateTime? LastRunAt { get; set; }

    /// <summary>Id of the user who created this schedule.</summary>
    public Guid CreatedBy { get; set; }

    /// <summary>UTC creation timestamp.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC last-updated timestamp.</summary>
    public DateTime UpdatedAt { get; set; }
}
```

**ScheduledRun.cs:**
```csharp
namespace ApiTool.Backend.Data.Entities;

/// <summary>A single enqueued run triggered by the scheduler or a manual run-now.</summary>
public sealed class ScheduledRun
{
    /// <summary>Primary key (serialized as <c>run_&lt;hex&gt;</c>).</summary>
    public Guid Id { get; set; }

    /// <summary>Foreign key to the schedule that triggered this run.</summary>
    public Guid ScheduleId { get; set; }

    /// <summary>Current lifecycle status.</summary>
    public ScheduledRunStatus Status { get; set; } = ScheduledRunStatus.Queued;

    /// <summary>UTC timestamp when this run was enqueued.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp when the run started executing (null if still queued).</summary>
    public DateTime? StartedAt { get; set; }

    /// <summary>UTC timestamp when the run completed (null if not yet done).</summary>
    public DateTime? CompletedAt { get; set; }
}
```

**ScheduledRunStatus.cs:**
```csharp
namespace ApiTool.Backend.Data.Entities;

/// <summary>Lifecycle status of a scheduled run.</summary>
public enum ScheduledRunStatus
{
    Queued,
    Running,
    Completed,
    Failed,
}
```

**ScheduleId.cs:**
```csharp
namespace ApiTool.Backend.Schedules;

/// <summary>Wire-format id helper (<c>sched_&lt;32-hex-chars&gt;</c>).</summary>
public static class ScheduleId
{
    private const string Prefix = "sched_";

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

**RunId.cs:**
```csharp
namespace ApiTool.Backend.Schedules;

/// <summary>Wire-format id helper (<c>run_&lt;32-hex-chars&gt;</c>).</summary>
public static class RunId
{
    private const string Prefix = "run_";

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

#### Tests to Write FIRST (RED phase)

```csharp
// ScheduleIdTests.cs
public sealed class ScheduleIdTests
{
    [Fact] public void Format_returns_prefixed_hex();
    [Fact] public void TryParse_round_trips();
    [Fact] public void TryParse_returns_false_for_wrong_prefix();
    [Fact] public void TryParse_returns_false_for_null();
}

// RunIdTests.cs
public sealed class RunIdTests
{
    [Fact] public void Format_returns_prefixed_hex();
    [Fact] public void TryParse_round_trips();
    [Fact] public void TryParse_returns_false_for_wrong_prefix();
    [Fact] public void TryParse_returns_false_for_null();
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 3: Update AppDbContext and Add Migration
**Rationale:** The DbContext must know about the new entities before the service layer can query them. Migration must be committed for the schema to be applied. Depends on Step 2 entities existing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Add `DbSet<Schedule>` and `DbSet<ScheduledRun>`, plus `OnModelCreating` configuration |
| `src/ApiTool.Backend/Migrations/YYYYMMDDHHMMSS_AddSchedules.cs` | create | EF migration (generated via `dotnet ef migrations add`) |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modify | Auto-updated by EF migration tool |

#### Current Code (AppDbContext — OnModelCreating, end of method)
```csharp
    b.Entity<ResultItem>(e =>
    {
        e.ToTable("result_items");
        // ...
    });
}
```

#### New Code (additions to AppDbContext)
```csharp
/// <summary>Cron schedules table.</summary>
public DbSet<Schedule> Schedules => Set<Schedule>();

/// <summary>Enqueued/completed run instances from schedules.</summary>
public DbSet<ScheduledRun> ScheduledRuns => Set<ScheduledRun>();
```

OnModelCreating additions:
```csharp
b.Entity<Schedule>(e =>
{
    e.ToTable("schedules");
    e.HasKey(x => x.Id);
    e.Property(x => x.Name).HasMaxLength(100).IsRequired();
    e.Property(x => x.CronExpression).HasMaxLength(100).IsRequired();
    e.Property(x => x.CollectionRef).HasMaxLength(500).IsRequired();
    e.HasIndex(x => new { x.OrgId, x.Name }).IsUnique();
    e.HasIndex(x => new { x.Enabled, x.NextRunAt });
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
    e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).OnDelete(DeleteBehavior.Restrict);
});

b.Entity<ScheduledRun>(e =>
{
    e.ToTable("scheduled_runs");
    e.HasKey(x => x.Id);
    e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
    e.HasIndex(x => new { x.ScheduleId, x.CreatedAt });
    e.HasOne<Schedule>().WithMany().HasForeignKey(x => x.ScheduleId).OnDelete(DeleteBehavior.Cascade);
});
```

#### Tests to Write FIRST (RED phase)

Add to `AppDbContextSchemaTests`:
```csharp
[InlineData("schedules")]
[InlineData("scheduled_runs")]
// Added to existing Migration_creates_expected_table theory data

[Fact]
public async Task Schedules_name_is_unique_per_org();

[Fact]
public async Task Scheduled_runs_cascade_delete_when_parent_schedule_is_removed();
```

#### Impact on Existing Tests
- `AppDbContextSchemaTests.Migration_creates_expected_table` — add `"schedules"` and `"scheduled_runs"` inline data. Non-breaking addition.
- No other test breakage expected.

---

### Step 4: Implement SchedulesService
**Rationale:** Business logic layer must exist before endpoints can call it. Follows the same pattern as `ResultsService` (constructor-injected `AppDbContext`, `TimeProvider`; result-tuple returns with error enum).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Schedules/ScheduleError.cs` | create | Error enum for schedule operations |
| `src/ApiTool.Backend/Schedules/ScheduleDto.cs` | create | Response DTO for schedule |
| `src/ApiTool.Backend/Schedules/ScheduledRunDto.cs` | create | Response DTO for a run |
| `src/ApiTool.Backend/Schedules/CreateScheduleRequest.cs` | create | Request body record |
| `src/ApiTool.Backend/Schedules/SchedulesService.cs` | create | Core business logic |

#### New Code

**ScheduleError.cs:**
```csharp
namespace ApiTool.Backend.Schedules;

public enum ScheduleError
{
    None,
    PermissionDenied,
    InvalidCron,
    ScheduleNameTaken,
    NotFound,
    InvalidName,
    InvalidCollectionRef,
}
```

**ScheduleDto.cs:**
```csharp
namespace ApiTool.Backend.Schedules;

public sealed record ScheduleDto(
    string Id,
    string Name,
    string CronExpression,
    string CollectionRef,
    bool Enabled,
    DateTime? NextRunAt,
    DateTime? LastRunAt,
    DateTime CreatedAt);
```

**ScheduledRunDto.cs:**
```csharp
namespace ApiTool.Backend.Schedules;

public sealed record ScheduledRunDto(
    string RunId,
    string Status,
    DateTime CreatedAt,
    DateTime? StartedAt,
    DateTime? CompletedAt);
```

**CreateScheduleRequest.cs:**
```csharp
namespace ApiTool.Backend.Schedules;

public sealed record CreateScheduleRequest(
    string? Name,
    string? Cron,
    string? CollectionRef);
```

**SchedulesService.cs — key methods:**
```csharp
namespace ApiTool.Backend.Schedules;

public sealed class SchedulesService(AppDbContext db, TimeProvider clock)
{
    public async Task<(ScheduleDto? dto, ScheduleError error, string? message)>
        CreateAsync(Guid userId, Guid orgId, CreateScheduleRequest? request, CancellationToken ct);

    public async Task<(IReadOnlyList<ScheduleDto> schedules, ScheduleError error)>
        ListAsync(Guid userId, Guid orgId, CancellationToken ct);

    public async Task<(ScheduleDto? dto, ScheduleError error)>
        GetByNameAsync(Guid userId, Guid orgId, string name, CancellationToken ct);

    public async Task<(ScheduledRunDto? dto, ScheduleError error)>
        RunNowAsync(Guid userId, Guid orgId, string scheduleName, CancellationToken ct);

    public async Task<(IReadOnlyList<ScheduledRunDto> runs, ScheduleError error)>
        ListRunsAsync(Guid userId, Guid orgId, string scheduleName, int limit, CancellationToken ct);

    /// <summary>Called by SchedulerHost — finds due schedules and enqueues runs.</summary>
    public async Task<int> EnqueueDueAsync(CancellationToken ct);
}
```

**RBAC logic:** `CreateAsync` and `RunNowAsync` require the user to be an `Owner` or `Admin` on the org (not just `Member`). `ListAsync`, `GetByNameAsync`, and `ListRunsAsync` require any org membership. This is enforced by a private helper `IsAdminAsync` that checks `OrgRole`.

**Cron logic:** Uses `Cronos.CronExpression.Parse(request.Cron, CronFormat.Standard)` — on parse failure, returns `ScheduleError.InvalidCron` with the exception message. On success, `NextRunAt` is computed via `expression.GetNextOccurrence(DateTimeOffset.UtcNow)`.

**EnqueueDueAsync:** Queries `db.Schedules.Where(s => s.Enabled && s.NextRunAt != null && s.NextRunAt <= now)`, creates a `ScheduledRun` per due schedule, updates `LastRunAt` and recomputes `NextRunAt`, returns count of enqueued runs.

#### Tests to Write FIRST (RED phase)

```csharp
// SchedulesServiceTests.cs
public sealed class SchedulesServiceTests
{
    [Fact] public async Task CreateAsync_persists_schedule_with_computed_next_run_at();
    [Fact] public async Task CreateAsync_returns_invalid_cron_for_malformed_expression();
    [Fact] public async Task CreateAsync_returns_schedule_name_taken_for_duplicate_in_same_org();
    [Fact] public async Task CreateAsync_allows_same_name_in_different_orgs();
    [Fact] public async Task CreateAsync_returns_permission_denied_for_member_role();
    [Fact] public async Task CreateAsync_returns_invalid_name_when_name_is_empty();
    [Fact] public async Task CreateAsync_returns_invalid_collection_ref_when_ref_is_empty();
    [Fact] public async Task ListAsync_returns_all_schedules_for_org_member();
    [Fact] public async Task GetByNameAsync_returns_schedule_for_org_member();
    [Fact] public async Task GetByNameAsync_returns_not_found_for_non_member();
    [Fact] public async Task RunNowAsync_creates_queued_run_and_returns_202_dto();
    [Fact] public async Task RunNowAsync_returns_permission_denied_for_member_role();
    [Fact] public async Task ListRunsAsync_returns_runs_newest_first();
    [Fact] public async Task EnqueueDueAsync_creates_runs_for_due_schedules();
    [Fact] public async Task EnqueueDueAsync_skips_disabled_schedules();
    [Fact] public async Task EnqueueDueAsync_recomputes_next_run_at_after_enqueue();
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 5: Implement SchedulerHost (Background Service)
**Rationale:** The hosted service polls for due schedules. It depends on `SchedulesService.EnqueueDueAsync` (Step 4). Implementing it after the service layer means we can unit-test the enqueue logic independently.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Schedules/SchedulerHost.cs` | create | `BackgroundService` that polls every 30s |

#### New Code

```csharp
namespace ApiTool.Backend.Schedules;

/// <summary>
/// Background service that polls for due schedules every 30 seconds and enqueues runs.
/// </summary>
public sealed class SchedulerHost(
    IServiceScopeFactory scopeFactory,
    ILogger<SchedulerHost> logger) : BackgroundService
{
    private static readonly TimeSpan TickInterval = TimeSpan.FromSeconds(30);

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation("SchedulerHost started, polling every {Interval}s", TickInterval.TotalSeconds);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var svc = scope.ServiceProvider.GetRequiredService<SchedulesService>();
                var count = await svc.EnqueueDueAsync(stoppingToken);

                if (count > 0)
                    logger.LogInformation("scheduler tick {Count} schedules due", count);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "SchedulerHost tick failed");
            }

            await Task.Delay(TickInterval, stoppingToken);
        }

        logger.LogInformation("SchedulerHost stopped");
    }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
// SchedulerHostTests.cs
public sealed class SchedulerHostTests
{
    [Fact] public async Task ExecuteAsync_calls_enqueue_due_on_tick();
    [Fact] public async Task ExecuteAsync_logs_info_when_schedules_are_due();
    [Fact] public async Task ExecuteAsync_survives_transient_exceptions();
}
```

Note: `SchedulerHostTests` will use a fake/mock `IServiceScopeFactory` to inject a test-scoped `SchedulesService` that verifies enqueue behavior, or will test through the integration host. Since `BackgroundService` testing can be complex, we may test via integration: start the host, insert a due schedule, wait, verify a run record exists.

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 6: Implement SchedulesEndpoints (Minimal API)
**Rationale:** HTTP endpoints are the final wiring layer. They depend on all previous steps (entities, service, host). Following the existing pattern from `ResultsEndpoints` and `OrganizationsEndpoints`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Schedules/SchedulesEndpoints.cs` | create | Minimal API endpoint registration |

#### New Code

```csharp
namespace ApiTool.Backend.Schedules;

public static class SchedulesEndpoints
{
    public static IEndpointRouteBuilder MapSchedulesEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}/schedules")
            .RequireAuthorization()
            .WithTags("Schedules");

        group.MapPost("", CreateSchedule)
            .WithName("CreateSchedule")
            .Produces<ScheduleDto>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict);

        group.MapGet("", ListSchedules)
            .WithName("ListSchedules")
            .Produces<ListSchedulesResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        group.MapGet("{name}", GetSchedule)
            .WithName("GetSchedule")
            .Produces<ScheduleDto>()
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapPost("{name}/run-now", RunNow)
            .WithName("RunNowSchedule")
            .Produces<RunNowResponse>(StatusCodes.Status202Accepted)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapGet("{name}/runs", ListRuns)
            .WithName("ListScheduleRuns")
            .Produces<ListRunsResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        return app;
    }
    // ... endpoint handler methods follow same pattern as ResultsEndpoints
}
```

#### Tests to Write FIRST (RED phase)

```csharp
// SchedulesEndpointsTests.cs — integration tests
[Collection(BackendCollection.Name)]
public sealed class SchedulesEndpointsTests : IAsyncLifetime
{
    // Happy path
    [Fact] public async Task Post_schedule_returns_201_with_next_run_at_in_future();
    [Fact] public async Task Post_schedule_returns_400_invalid_cron_for_bad_expression();
    [Fact] public async Task Post_schedule_returns_409_schedule_name_taken_for_duplicate();
    [Fact] public async Task Post_run_now_returns_202_with_queued_run();
    [Fact] public async Task Get_schedule_runs_returns_queued_run_after_run_now();

    // RBAC
    [Fact] public async Task Post_schedule_returns_403_for_member_non_admin();

    // Auth
    [Fact] public async Task All_schedule_endpoints_return_401_without_bearer();

    // Swagger
    [Fact] public async Task Swagger_json_lists_schedules_endpoints();
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 7: Wire Everything in Program.cs
**Rationale:** Final integration — register services and map endpoints. Must come last because all components must exist.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Program.cs` | modify | Register `SchedulesService`, `SchedulerHost`, and map endpoints |

#### Current Code
```csharp
builder.Services.AddScoped<ResultsService>();
builder.Services.AddSingleton<IResultIngestedNotifier, NoopResultIngestedNotifier>();
builder.Services.AddSingleton(TimeProvider.System);
```

#### New Code
```csharp
builder.Services.AddScoped<ResultsService>();
builder.Services.AddScoped<SchedulesService>();
builder.Services.AddSingleton<IResultIngestedNotifier, NoopResultIngestedNotifier>();
builder.Services.AddSingleton(TimeProvider.System);

// ── Scheduler background service ─────────────────────────────────────
// Only run the scheduler tick in non-Testing environments. Tests exercise
// EnqueueDueAsync directly through SchedulesService.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddHostedService<SchedulerHost>();
}
```

And in the endpoints section:
```csharp
app.MapOrganizationsEndpoints();
app.MapResultsEndpoints();
app.MapSchedulesEndpoints();
```

#### Tests to Write FIRST (RED phase)
No new tests in this step — covered by Step 6 integration tests.

#### Impact on Existing Tests
- No existing tests affected. The `SchedulerHost` is not registered in `Testing` environment, so existing test infrastructure (`BackendFactory`) works without changes.

---

### Step 8: Add RBAC Helper for Admin Check
**Rationale:** The schedules service needs to distinguish admin vs member roles for write operations. This requires a helper method that checks the `OrgRole` from the `OrganizationMembers` table.

Note: This is logically part of Step 4 (SchedulesService) but called out explicitly because it introduces a new access pattern on the `OrganizationMember` entity.

#### Files to Modify
This is built into `SchedulesService` in Step 4 as private helper methods:

```csharp
private async Task<bool> IsMemberAsync(Guid userId, Guid orgId, CancellationToken ct) =>
    await db.OrganizationMembers.AnyAsync(m => m.OrgId == orgId && m.UserId == userId, ct);

private async Task<bool> IsAdminAsync(Guid userId, Guid orgId, CancellationToken ct)
{
    var member = await db.OrganizationMembers
        .SingleOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
    return member is not null && member.Role is OrgRole.Owner or OrgRole.Admin;
}
```

#### Impact on Existing Tests
- No existing tests affected.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `AppDbContextSchemaTests.cs` | `Migration_creates_expected_table` | Add `"schedules"` and `"scheduled_runs"` InlineData | Non-breaking addition |
| All other existing test files | All | none | none |

## New Test Files Summary

| Test File | Test Count | Description |
|-----------|-----------|-------------|
| `ScheduleIdTests.cs` | 4 | Wire-format id round-tripping |
| `RunIdTests.cs` | 4 | Wire-format id round-tripping |
| `SchedulesServiceTests.cs` | 16 | Service-layer unit tests (cron, RBAC, enqueue) |
| `SchedulerHostTests.cs` | 3 | Background service tick behavior |
| `SchedulesEndpointsTests.cs` | 8 | HTTP integration tests |
| **Total** | **35** | |

## Risks and Edge Cases

- **Risk:** Cronos library version mismatch or API changes. **Mitigation:** Pin to `0.8.4` which is the most recent stable release. Cronos is widely used and has a stable API (`CronExpression.Parse`, `GetNextOccurrence`).

- **Risk:** Race condition in `EnqueueDueAsync` — two ticks fire simultaneously and both see the same schedule as due, creating duplicate runs. **Mitigation:** Ensure `EnqueueDueAsync` updates `NextRunAt` atomically in the same transaction as creating the run. Since SQLite serializes writes, this is naturally safe in the current architecture. Document this as a concern for PostgreSQL migration.

- **Risk:** `SchedulerHost` background service failing silently and never recovering. **Mitigation:** Log all exceptions. Wrap each tick in try/catch and continue the loop. Add info-level logging for "N schedules due" as required by the DoD.

- **Edge case:** Cron expression "0 2 * * *" when the current time is exactly 02:00 UTC — `GetNextOccurrence` should return tomorrow's 02:00. **Handling:** Cronos returns the *next* occurrence strictly *after* the given time, so this is handled correctly by default.

- **Edge case:** Schedule with `Enabled = false` — must not fire. **Handling:** `EnqueueDueAsync` query includes `s.Enabled == true` filter.

- **Edge case:** Duplicate schedule name across different orgs — must be allowed. **Handling:** Unique index is on `(OrgId, Name)` not just `Name`.

- **Edge case:** Service restart with existing schedules — `NextRunAt` may be in the past. **Handling:** `EnqueueDueAsync` picks up any schedule where `NextRunAt <= now`, so past-due schedules are caught on the first tick after restart. This satisfies the "re-hydrates and recomputes" behavior.

- **Edge case:** Member (non-admin) calling POST /schedules — must return 403. **Handling:** `IsAdminAsync` check in service layer returns `ScheduleError.PermissionDenied`.

## Verification

```bash
cd src && dotnet build ApiTool.Backend/ApiTool.Backend.csproj
dotnet test ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj --filter "FullyQualifiedName~Schedules"
dotnet test ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj
```

Observable verification:
```bash
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Schedules"
# Expected: Passed: >=8, Failed: 0
dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"nightly","cron":"0 2 * * *","collection_ref":"smoke.yaml"}' \
  "http://localhost:5000/api/v1/organizations/$ORG/schedules"
# Expected HTTP 201, body contains "next_run_at":"...Z"
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  "http://localhost:5000/api/v1/organizations/$ORG/schedules/nightly/run-now"
# Expected HTTP 202, body: {"run_id":"run_...","status":"queued"}
```
