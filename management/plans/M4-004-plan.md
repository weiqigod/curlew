# Implementation Plan: M4-004

## Overview
Add a Results ingestion API to the ApiTool backend: `POST /api/v1/organizations/{orgId}/results` to ingest a CLI-shaped JUnit-like JSON payload, `GET .../results` for a newest-first list, and `GET /api/v1/results/{resultId}` for a per-test detail view. Introduces `Result`/`ResultItem` EF entities, migration `0002_results`, org-scoped RBAC via the existing `OrganizationMember` table, 5 MB payload cap, per-org fixed-window rate limiting at 60 req/min, and Swagger annotations.

## Task Details
- **ID:** M4-004
- **Title:** Backend: test results ingestion API
- **Phase:** M4: Team Tier
- **Priority:** 1
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-003 | Backend: organization + seat RBAC data model and service | done |

## Architectural Decisions

1. **Minimal APIs over MVC controllers.** The existing `OrganizationsEndpoints` uses a `MapGroup` pattern with static endpoint methods; we keep the same style in `ResultsEndpoints` even though the task YAML reads "ResultsController" — the CS code base has no MVC controllers wired in `Program.cs`. Introducing `AddControllers()` for a single feature would be inconsistent.
2. **Reuse RBAC pattern from M4-003.** Membership is resolved with a single query against `OrganizationMembers` joined to the caller's user id. Non-member → 403 `permission_denied` on write paths (the task explicitly requires 403, not 404, on POST). On read paths the task requires 404 `result_not_found` for cross-org detail access — matching the non-enumeration principle already used by `GetByIdForUserAsync` but keeping the code consistent with the task YAML.
3. **Two-table model.** `Result` (row per upload — aggregates) + `ResultItem` (row per test case). Per-test rows are only returned by the detail endpoint; list endpoint returns the header only.
4. **Wire-format id `res_<32-hex-chars>`** following the existing `OrgId` helper, implemented as a new `ResultId` static class.
5. **Payload limits.**
   - Size: `[RequestSizeLimit(5 * 1024 * 1024)]` on POST (applied via the minimal-API `WithMetadata` extension since attributes don't work on lambdas) → framework returns 413 before deserialization.
   - Schema: manually validated in `ResultsService.IngestAsync` (400 `invalid_result_schema` with JSON pointer to the first missing required field). We *do not* rely on `[Required]` attributes because snake_case + minimal APIs + our existing error-shape contract need a structured `ErrorResponse` body.
6. **Rate limiting.** Use built-in `Microsoft.AspNetCore.RateLimiting` (ASP.NET Core 8+ built-in). Register a named policy `"results-ingest"` as fixed-window 60 per minute partitioned on the `{orgId}` route value. Apply via `.RequireRateLimiting("results-ingest")` on the POST endpoint. Skip the middleware in `Testing` environment to keep existing collection fixtures deterministic, then add one dedicated test that enables it and asserts 429.
7. **Domain-event seam for M4-008.** Define `public interface IResultIngestedNotifier { Task NotifyAsync(Guid orgId, Guid resultId, CancellationToken ct); }` with a default `NoopResultIngestedNotifier` implementation, registered in `Program.cs`. M4-008 replaces the default with its dispatcher. No MediatR dependency — YAGNI.
8. **Migration name.** EF tooling will create `AddResults` on disk; we do not force the `0002_results` prefix because EF prepends a timestamp. We do place the migration file under `Migrations/` and explicitly name the class `AddResults` so the intent is obvious. The task's wording "0002_results" is satisfied by this being the second migration chronologically.
9. **Testdata fixture.** `testdata/backend/sample-result-upload.json` holds a 3-test passing payload consumed both by the observable curl in this slice and (per task YAML) by the future M4-012 e2e.
10. **Open question resolution (payload schema shape).** The task YAML references `pass_count`, `fail_count`, `duration_ms`, `run_at`, and "per-test rows". Neither the SPEC nor M4-005/M4-007 nail down the exact field set. We lock the schema now so M4-005/M4-007/M4-012 can depend on it. Chosen shape (snake_case, snake_case matches global JSON policy):

    ```json
    {
      "collection_name": "smoke-tests",
      "run_at": "2026-04-15T12:00:00Z",
      "duration_ms": 1234,
      "pass_count": 3,
      "fail_count": 0,
      "skipped_count": 0,
      "triggered_by": "cli",
      "git_sha": "abc123",
      "items": [
        {
          "name": "GET /users",
          "status": "passed",
          "duration_ms": 120,
          "message": null
        }
      ]
    }
    ```

    Required fields: `collection_name`, `run_at`, `duration_ms`, `pass_count`, `fail_count`, `items` (array — may be empty). Optional: `skipped_count`, `triggered_by`, `git_sha`, per-item `message`.

## Implementation Steps

### Step 1: Add `Result` and `ResultItem` entities, `AppDbContext` config, and migration

**Rationale:** Smallest blast radius — pure data model additions. No existing code paths break. All subsequent steps depend on these types.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/Result.cs` | create | `Result` entity (header row). |
| `src/ApiTool.Backend/Data/Entities/ResultItem.cs` | create | `ResultItem` entity (per-test row). |
| `src/ApiTool.Backend/Data/Entities/ResultStatus.cs` | create | Enum `Passed \| Failed \| Skipped \| Error`. |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Register the two new `DbSet`s and fluent config. |
| `src/ApiTool.Backend/Migrations/<timestamp>_AddResults.cs` | create (via `dotnet ef migrations add`) | Creates `results` + `result_items` tables. |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | update (via EF tooling) | Reflects new model. |
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | modify | Extend `[InlineData]` to cover `results` + `result_items`. |

#### New Code — `Result.cs`
```csharp
namespace ApiTool.Backend.Data.Entities;

/// <summary>Aggregate header row for a single uploaded test run.</summary>
public sealed class Result
{
    /// <summary>Primary key (serialized as <c>res_&lt;hex&gt;</c>).</summary>
    public Guid Id { get; set; }

    /// <summary>Owning organization foreign key.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Id of the user who uploaded this run.</summary>
    public Guid UploadedBy { get; set; }

    /// <summary>Collection/file name reported by the CLI.</summary>
    public string CollectionName { get; set; } = string.Empty;

    /// <summary>UTC timestamp the run started (client-reported).</summary>
    public DateTime RunAt { get; set; }

    /// <summary>Total duration in milliseconds (client-reported).</summary>
    public long DurationMs { get; set; }

    /// <summary>Number of passing tests.</summary>
    public int PassCount { get; set; }

    /// <summary>Number of failing tests.</summary>
    public int FailCount { get; set; }

    /// <summary>Number of skipped tests.</summary>
    public int SkippedCount { get; set; }

    /// <summary>Free-form triggered-by label (e.g. "cli", "scheduled", "ci").</summary>
    public string? TriggeredBy { get; set; }

    /// <summary>Optional git commit SHA associated with the run.</summary>
    public string? GitSha { get; set; }

    /// <summary>Server-side ingestion timestamp (UTC).</summary>
    public DateTime CreatedAt { get; set; }
}
```

#### New Code — `ResultItem.cs`
```csharp
namespace ApiTool.Backend.Data.Entities;

/// <summary>Per-test row for a <see cref="Result"/>.</summary>
public sealed class ResultItem
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>Owning result foreign key.</summary>
    public Guid ResultId { get; set; }

    /// <summary>Zero-based ordering index for stable display.</summary>
    public int Ordinal { get; set; }

    /// <summary>Test case name.</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>Outcome.</summary>
    public ResultStatus Status { get; set; }

    /// <summary>Per-test duration in milliseconds.</summary>
    public long DurationMs { get; set; }

    /// <summary>Error/assertion message when <see cref="Status"/> is not <c>Passed</c>.</summary>
    public string? Message { get; set; }
}
```

#### New Code — `AppDbContext.cs` additions
```csharp
/// <summary>Uploaded test run headers.</summary>
public DbSet<Result> Results => Set<Result>();

/// <summary>Per-test rows for uploaded runs.</summary>
public DbSet<ResultItem> ResultItems => Set<ResultItem>();

// ... inside OnModelCreating, after the organization_audit_log entity:
b.Entity<Result>(e =>
{
    e.ToTable("results");
    e.HasKey(x => x.Id);
    e.Property(x => x.CollectionName).HasMaxLength(200).IsRequired();
    e.Property(x => x.TriggeredBy).HasMaxLength(50);
    e.Property(x => x.GitSha).HasMaxLength(64);
    e.HasIndex(x => new { x.OrgId, x.CreatedAt });  // newest-first paging
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
    e.HasOne<User>().WithMany().HasForeignKey(x => x.UploadedBy).OnDelete(DeleteBehavior.Restrict);
});

b.Entity<ResultItem>(e =>
{
    e.ToTable("result_items");
    e.HasKey(x => x.Id);
    e.Property(x => x.Name).HasMaxLength(500).IsRequired();
    e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
    e.Property(x => x.Message).HasMaxLength(4000);
    e.HasIndex(x => new { x.ResultId, x.Ordinal });
    e.HasOne<Result>().WithMany().HasForeignKey(x => x.ResultId).OnDelete(DeleteBehavior.Cascade);
});
```

#### Tests to Write FIRST (RED phase)
Extend `AppDbContextSchemaTests.Migration_creates_expected_table`:

```csharp
[Theory]
[InlineData("organizations")]
[InlineData("organization_members")]
[InlineData("organization_invitations")]
[InlineData("organization_audit_log")]
[InlineData("results")]          // NEW
[InlineData("result_items")]     // NEW
public async Task Migration_creates_expected_table(string tableName) { /* unchanged */ }
```

Add a new fact:
```csharp
[Fact]
public async Task Result_items_cascade_delete_when_parent_result_is_removed()
{
    // Arrange: insert org, user, result, 2 items. Delete result. Expect 0 items.
}
```

#### Impact on Existing Tests
- `AppDbContextSchemaTests.Migration_creates_expected_table` — new theory rows will fail until the migration is added. Expected (RED).
- No other existing tests touch `AppDbContext.Results`.

---

### Step 2: `ResultId` helper + `ResultDto`/`ResultDetailDto`/request contract records

**Rationale:** Pure value-type additions consumed by Step 3. Zero coupling to existing code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Results/ResultId.cs` | create | `res_<hex>` format helper mirroring `OrgId`. |
| `src/ApiTool.Backend/Results/ResultDto.cs` | create | List-item response record. |
| `src/ApiTool.Backend/Results/ResultDetailDto.cs` | create | Detail response record (header + items). |
| `src/ApiTool.Backend/Results/ResultItemDto.cs` | create | Per-item nested record. |
| `src/ApiTool.Backend/Results/UploadResultRequest.cs` | create | Request body shape. |
| `src/ApiTool.Backend/Results/UploadResultItemRequest.cs` | create | Per-item request shape. |
| `src/ApiTool.Backend/Results/ResultError.cs` | create | Sentinel error enum. |
| `src/ApiTool.Backend.Tests/Results/ResultIdTests.cs` | create | Round-trip and negative parse tests. |

#### New Code — `ResultId.cs`
```csharp
namespace ApiTool.Backend.Results;

/// <summary>Helpers for the wire-format result identifier (<c>res_&lt;32-hex-chars&gt;</c>).</summary>
public static class ResultId
{
    private const string Prefix = "res_";

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

#### New Code — `ResultError.cs`
```csharp
namespace ApiTool.Backend.Results;

/// <summary>Well-known error codes returned by <see cref="ResultsService"/>.</summary>
public enum ResultError
{
    None,
    PermissionDenied,       // caller is not an org member
    InvalidSchema,          // payload fails structural validation
    NotFound,               // result does not exist or belongs to another org
    PayloadTooLarge,        // body exceeded 5 MB (emitted by middleware, not service)
}
```

#### New Code — DTOs
```csharp
public sealed record ResultDto(
    string Id,
    string CollectionName,
    int PassCount,
    int FailCount,
    int SkippedCount,
    long DurationMs,
    DateTime RunAt,
    DateTime CreatedAt,
    string? TriggeredBy,
    string? GitSha);

public sealed record ResultDetailDto(
    string Id,
    string CollectionName,
    int PassCount,
    int FailCount,
    int SkippedCount,
    long DurationMs,
    DateTime RunAt,
    DateTime CreatedAt,
    string? TriggeredBy,
    string? GitSha,
    IReadOnlyList<ResultItemDto> Items);

public sealed record ResultItemDto(
    string Name,
    string Status,
    long DurationMs,
    string? Message);

public sealed record UploadResultRequest(
    string? CollectionName,
    DateTime? RunAt,
    long? DurationMs,
    int? PassCount,
    int? FailCount,
    int? SkippedCount,
    string? TriggeredBy,
    string? GitSha,
    IReadOnlyList<UploadResultItemRequest>? Items);

public sealed record UploadResultItemRequest(
    string? Name,
    string? Status,
    long? DurationMs,
    string? Message);
```
All request fields are nullable so that missing-field detection can point to the specific field (`/pass_count` etc.) rather than be swallowed by binding defaults.

#### Tests to Write FIRST (RED phase)
```csharp
public sealed class ResultIdTests
{
    [Theory]
    [InlineData("res_00000000000000000000000000000000", true)]
    [InlineData("res_abcdef0123456789abcdef0123456789", true)]
    [InlineData("", false)]
    [InlineData("res_", false)]
    [InlineData("org_00000000000000000000000000000000", false)]
    [InlineData("res_xyz", false)]
    public void TryParse_accepts_valid_ids_and_rejects_invalid(string input, bool expected)
    { /* ... */ }

    [Fact]
    public void Format_then_TryParse_roundtrips() { /* ... */ }
}
```

#### Impact on Existing Tests
- None.

---

### Step 3: `ResultsService` — ingest, list, detail with RBAC

**Rationale:** Business logic lives in a single testable class that endpoints delegate to, mirroring `OrganizationService`. Tests hit SQLite in-memory (via `TestDb`) so they cover both correctness and schema interaction without HTTP overhead.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Results/ResultsService.cs` | create | Business logic + RBAC check. |
| `src/ApiTool.Backend/Results/IResultIngestedNotifier.cs` | create | Seam interface for M4-008. |
| `src/ApiTool.Backend/Results/NoopResultIngestedNotifier.cs` | create | Default implementation (no-op). |
| `src/ApiTool.Backend.Tests/Results/ResultsServiceTests.cs` | create | 10+ SQLite-backed behavior tests. |

#### New Code — `IResultIngestedNotifier.cs`
```csharp
namespace ApiTool.Backend.Results;

/// <summary>
/// Seam for downstream dispatchers (e.g. M4-008 notifications) to react to a newly
/// ingested result. The default <see cref="NoopResultIngestedNotifier"/> is a no-op.
/// </summary>
public interface IResultIngestedNotifier
{
    Task NotifyAsync(Guid orgId, Guid resultId, CancellationToken ct);
}
```

#### New Code — `ResultsService.cs` (key signatures)
```csharp
public sealed class ResultsService(
    AppDbContext db,
    TimeProvider clock,
    IResultIngestedNotifier notifier)
{
    public const int MaxItemsPerRequest = 5000;
    public const int MaxMessageLength = 4000;

    public async Task<(ResultDto? dto, ResultError error, string? message, string? fieldPointer)>
        IngestAsync(Guid userId, Guid orgId, UploadResultRequest request, CancellationToken ct)
    { /* membership check → schema validation → insert result+items → notifier */ }

    public async Task<(IReadOnlyList<ResultDto> results, ResultError error)>
        ListAsync(Guid userId, Guid orgId, int limit, CancellationToken ct)
    { /* membership check → newest-first query with Take(limit) */ }

    public async Task<(ResultDetailDto? dto, ResultError error)>
        GetDetailAsync(Guid userId, Guid resultId, CancellationToken ct)
    { /* loads result → membership check → 404 if non-member */ }

    private async Task<bool> IsMemberAsync(Guid userId, Guid orgId, CancellationToken ct) =>
        await db.OrganizationMembers.AnyAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
}
```

Schema validation (inside `IngestAsync`) walks the request in a fixed order and returns the *first* missing/invalid field as a JSON pointer:
- `request is null` → `/`
- `CollectionName` null/empty → `/collection_name`
- `RunAt` null → `/run_at`
- `DurationMs` null or < 0 → `/duration_ms`
- `PassCount` null or < 0 → `/pass_count`
- `FailCount` null or < 0 → `/fail_count`
- `Items` null → `/items`
- `Items.Count > MaxItemsPerRequest` → `/items`
- For each item at index i:
  - `Name` null/empty → `/items/{i}/name`
  - `Status` null/empty/unparseable → `/items/{i}/status`
  - `DurationMs` null or < 0 → `/items/{i}/duration_ms`

On success, `IngestAsync` returns a `ResultDto` built from the inserted row and fires `notifier.NotifyAsync` *after* `SaveChangesAsync`.

#### Tests to Write FIRST (RED phase)

`ResultsServiceTests.cs` — all backed by `TestDb.CreateOpen()`:

```csharp
public sealed class ResultsServiceTests
{
    // ── IngestAsync happy path + RBAC ──
    [Fact] public async Task IngestAsync_persists_result_and_items_for_org_member();
    [Fact] public async Task IngestAsync_returns_permission_denied_for_non_member_and_writes_nothing();
    [Fact] public async Task IngestAsync_invokes_notifier_after_save();

    // ── IngestAsync schema validation (table-driven) ──
    [Theory]
    [InlineData(null,          "col",  "/collection_name")]
    [InlineData("missing-run", null,   "/run_at")]
    // etc., parametrized over the required-field matrix above
    public async Task IngestAsync_returns_invalid_schema_with_pointer(/* ... */);

    [Fact] public async Task IngestAsync_rejects_negative_pass_count();
    [Fact] public async Task IngestAsync_rejects_item_with_unknown_status();
    [Fact] public async Task IngestAsync_rejects_more_than_max_items();

    // ── ListAsync ──
    [Fact] public async Task ListAsync_returns_newest_first_bounded_by_limit();
    [Fact] public async Task ListAsync_returns_permission_denied_for_non_member();
    [Fact] public async Task ListAsync_clamps_limit_between_1_and_100();

    // ── GetDetailAsync ──
    [Fact] public async Task GetDetailAsync_returns_detail_with_items_in_ordinal_order();
    [Fact] public async Task GetDetailAsync_returns_not_found_for_cross_org_caller();
    [Fact] public async Task GetDetailAsync_returns_not_found_for_missing_id();
}
```

Also define a tiny `FakeResultIngestedNotifier` in the test project that records calls for the notifier assertion.

#### Impact on Existing Tests
- None — `ResultsService` is a new class.

---

### Step 4: `ResultsEndpoints` + DI wiring + rate limiting + Swagger metadata

**Rationale:** Exposes the service over HTTP. This is where `Program.cs` is edited — the highest-blast-radius change — so it comes after steps 1–3 are green. Kept surgical: add `AddScoped<ResultsService>`, register `IResultIngestedNotifier`, map endpoints, add rate limiter.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Results/ResultsEndpoints.cs` | create | `MapResultsEndpoints()` extension. |
| `src/ApiTool.Backend/Program.cs` | modify | Register service, notifier, rate limiter; call `MapResultsEndpoints`. |
| `src/ApiTool.Backend.Tests/Results/ResultsEndpointsTests.cs` | create | Full HTTP integration tests (8+). |

#### New Code — `ResultsEndpoints.cs` (skeleton)
```csharp
public static class ResultsEndpoints
{
    private const long MaxRequestBytes = 5 * 1024 * 1024;

    public static IEndpointRouteBuilder MapResultsEndpoints(this IEndpointRouteBuilder app)
    {
        var orgGroup = app
            .MapGroup("/api/v1/organizations/{orgId}/results")
            .RequireAuthorization()
            .WithTags("Results");

        orgGroup.MapPost("", IngestResult)
            .WithName("IngestResult")
            .WithMetadata(new RequestSizeLimitAttribute(MaxRequestBytes))
            .RequireRateLimiting("results-ingest")
            .Produces<IngestResultResponse>(StatusCodes.Status202Accepted)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status413PayloadTooLarge)
            .Produces<ErrorResponse>(StatusCodes.Status429TooManyRequests);

        orgGroup.MapGet("", ListResults)
            .WithName("ListResults")
            .Produces<ListResultsResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        app.MapGet("/api/v1/results/{resultId}", GetResult)
            .RequireAuthorization()
            .WithTags("Results")
            .WithName("GetResult")
            .Produces<ResultDetailDto>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        return app;
    }

    // Handler signatures:
    // Task<IResult> IngestResult(string orgId, UploadResultRequest body, CurrentUserAccessor users, ResultsService svc, CancellationToken ct)
    // Task<IResult> ListResults(string orgId, int? limit, CurrentUserAccessor users, ResultsService svc, CancellationToken ct)
    // Task<IResult> GetResult(string resultId, CurrentUserAccessor users, ResultsService svc, CancellationToken ct)

    public sealed record IngestResultResponse(string ResultId, string Status);
    public sealed record ListResultsResponse(IReadOnlyList<ResultDto> Results);
}
```

Error-code mapping in handlers:
- `PermissionDenied` → 403 `{"code":"permission_denied","message":"..."}`
- `InvalidSchema` → 400 `{"code":"invalid_result_schema","message":"...","field":"/pass_count"}`
- `NotFound` → 404 `{"code":"result_not_found","message":"..."}`
- 413 is emitted by the framework before the handler runs — we add a minimal exception-handler mapping in `Program.cs` to reshape `BadHttpRequestException` (request body too large) into our `ErrorResponse` JSON. Simpler alternative: catch the exception at handler entry and return a manual `Results.Json(..., 413)`. We choose the former (middleware `UseExceptionHandler` with a tiny map) to keep handlers clean.

To emit the `field` pointer while still matching `ErrorResponse`, extend the record:
```csharp
public sealed record ErrorResponse(string Code, string Message, string? Field = null);
```
This is backwards-compatible — existing `new ErrorResponse("code","msg")` call sites still compile (`Field` defaults to null and is omitted from JSON via serialization options).

#### New Code — `Program.cs` additions (diff)
```csharp
// After existing service registrations:
builder.Services.AddScoped<ResultsService>();
builder.Services.AddSingleton<IResultIngestedNotifier, NoopResultIngestedNotifier>();

// Rate limiting — skip in the Testing environment to keep fixture runs deterministic.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddRateLimiter(options =>
    {
        options.RejectionStatusCode = StatusCodes.Status429TooManyRequests;
        options.AddPolicy("results-ingest", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions
                {
                    PermitLimit = 60,
                    Window = TimeSpan.FromMinutes(1),
                    QueueLimit = 0,
                }));
    });
}
else
{
    // No-op policy so `.RequireRateLimiting("results-ingest")` still binds.
    builder.Services.AddRateLimiter(options =>
    {
        options.AddPolicy("results-ingest", _ =>
            RateLimitPartition.GetNoLimiter("testing"));
    });
}

// After var app = builder.Build();
app.UseRateLimiter();

// Under existing app.MapOrganizationsEndpoints();
app.MapResultsEndpoints();
```

Also update the Swagger `OpenApiInfo` description to mention "v1 — Organizations + Results" and ensure `AddSwaggerGen` keeps working with the new endpoints (it will — minimal APIs use endpoint metadata automatically).

One existing detail: `ConfigureHttpJsonOptions` sets `SnakeCaseLower` naming. The `Field` property on the new `ErrorResponse` will serialize as `"field"`. To suppress null fields so old 400 responses remain `{code,message}` without `"field":null`, add:
```csharp
opts.SerializerOptions.DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull;
```
This is a safe global change — existing DTOs do not rely on null roundtripping.

#### Tests to Write FIRST (RED phase) — `ResultsEndpointsTests.cs`

Uses the same `[Collection(BackendCollection.Name)]` + `BackendFactory` pattern. Helper: seed an org via `POST /organizations` so every test starts from a known-good state.

```csharp
[Collection(BackendCollection.Name)]
public sealed class ResultsEndpointsTests : IAsyncLifetime
{
    // Happy path
    [Fact] public async Task Post_results_returns_202_and_persists_row();
    [Fact] public async Task Get_results_returns_newest_first_for_member();
    [Fact] public async Task Get_result_detail_returns_200_with_items_for_member();

    // RBAC
    [Fact] public async Task Post_results_returns_403_permission_denied_for_non_member();
    [Fact] public async Task Get_result_detail_returns_404_for_cross_org_caller();

    // Validation
    [Fact] public async Task Post_results_returns_400_invalid_result_schema_with_field_pointer_when_pass_count_missing();
    [Fact] public async Task Post_results_returns_400_when_item_status_is_unknown();

    // Size cap
    [Fact] public async Task Post_results_returns_413_when_body_exceeds_5mb();

    // Auth
    [Fact] public async Task All_results_endpoints_return_401_when_no_bearer_header();

    // Swagger
    [Fact] public async Task Swagger_json_lists_results_endpoints();
}
```

The 413 test constructs a `HttpContent` whose length header is > 5 MB by posting a large `items` array of repeated strings. The 429 case is deferred to a dedicated test that spins up a factory with rate limiting *enabled* (derivative of `BackendFactory` overriding the environment to `Production` for one test class).

Actually, to keep the plan simple and deterministic, **move the 429 assertion to a pure unit test** on the rate-limiter policy registration in a future slice. The task YAML DoD only requires that rate limiting is "configured for POST /results (60/min/org)" — it does not require an HTTP-level 429 test. We satisfy the DoD via:
1. A `RateLimiterConfigurationTests` unit test that asserts the policy exists with `PermitLimit == 60` and `Window == 1 minute`.
2. Code-level configuration review.

#### Impact on Existing Tests
- `OrganizationsEndpointsTests` — unchanged. The `ErrorResponse` shape gains an optional `Field` with `WhenWritingNull` so existing 400 body assertions (`code`/`message`) still match.
- `JwtAuthenticationTests` — unchanged.
- `SmokeTests` — unchanged. Double-check that `Program.cs` edits do not break the minimal host startup for `Environment.IsEnvironment("Testing")`.

---

### Step 5: Testdata fixture + scripts + docs

**Rationale:** Leaf artifacts consumed by the observable, the pending M4-012 e2e, and developer workflows. Added last so tests drive the schema definition, not the other way around.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/backend/sample-result-upload.json` | create | 3-test passing fixture used by the observable curl and future M4-012 e2e. |
| `CHANGELOG.md` | modify | Add M4-004 entry under unreleased. |

#### Fixture shape
```json
{
  "collection_name": "smoke-tests",
  "run_at": "2026-04-15T12:00:00Z",
  "duration_ms": 1234,
  "pass_count": 3,
  "fail_count": 0,
  "skipped_count": 0,
  "triggered_by": "cli",
  "git_sha": "abc1234",
  "items": [
    { "name": "GET /users",   "status": "passed", "duration_ms": 120, "message": null },
    { "name": "POST /users",  "status": "passed", "duration_ms": 250, "message": null },
    { "name": "DELETE /users","status": "passed", "duration_ms": 864, "message": null }
  ]
}
```

#### Tests to Write FIRST (RED phase)
A single sanity test:
```csharp
[Fact]
public async Task Posting_the_testdata_fixture_returns_202()
{
    var json = await File.ReadAllTextAsync(
        Path.Combine(AppContext.BaseDirectory, "../../../../../testdata/backend/sample-result-upload.json"));
    // POST and assert 202 + expected pass_count=3
}
```

The `ApiTool.Backend.Tests.csproj` gains a `<Content Include="../../testdata/backend/sample-result-upload.json" CopyToOutputDirectory="PreserveNewest" Link="testdata/sample-result-upload.json" />` so the relative path resolution is stable. Prefer a directly-resolved path from `AppContext.BaseDirectory` to avoid fragile working-directory assumptions.

#### Impact on Existing Tests
- None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `Data/AppDbContextSchemaTests.cs` | `Migration_creates_expected_table` | extend theory rows | add `results`, `result_items` rows |
| `Data/AppDbContextSchemaTests.cs` | — | add new fact | `Result_items_cascade_delete_when_parent_result_is_removed` |
| `Organizations/OrganizationsEndpointsTests.cs` | all tests | none | — (ErrorResponse change is additive) |
| `Organizations/OrganizationServiceTests.cs` | all tests | none | — |
| `Organizations/JwtAuthenticationTests.cs` | all tests | none | — |
| `SmokeTests.cs` | all tests | none | — |
| `Results/ResultIdTests.cs` | — | **new file**, ~6 tests | create |
| `Results/ResultsServiceTests.cs` | — | **new file**, ~14 tests | create |
| `Results/ResultsEndpointsTests.cs` | — | **new file**, ~10 tests | create |
| `Results/RateLimiterConfigurationTests.cs` | — | **new file**, 1 test | create |

**Total new tests:** ~31 (≥10 tests matching Results filter required by the DoD — generously exceeded).

## Risks and Edge Cases

- **Risk: `RequestSizeLimitAttribute` may not bind on minimal-API lambdas.** → **Mitigation:** Verified via the ASP.NET Core 8 docs that `.WithMetadata(new RequestSizeLimitAttribute(...))` works; if `Kestrel` has already buffered the body the framework emits a `BadHttpRequestException`. Fallback path: configure `KestrelServerOptions.Limits.MaxRequestBodySize = 5MB` globally during startup and rely on the built-in 413.
- **Risk: `ErrorResponse` gains an optional field that might leak through as `field: null` in existing responses.** → **Mitigation:** Set `DefaultIgnoreCondition = WhenWritingNull` globally so the new field is omitted when absent.
- **Risk: `ConfigureHttpJsonOptions` snake_case naming applied to nullable `long?` and `int?` may still deserialize `"pass_count": null` as "missing".** → **Mitigation:** Request fields are all nullable; our validation checks `request.PassCount is null` (not default), so a literal `null` and a missing key are treated identically, which is the correct behavior for "required".
- **Risk: Rate limiter in `Testing` environment.** → **Mitigation:** Use `GetNoLimiter` in Testing so existing collection fixture tests cannot flake on the 60/min cap.
- **Risk: Large-payload test (413) pressing actual 5 MB through `HttpClient` in-process is slow.** → **Mitigation:** Send a payload just over 5 MB (say 5.1 MB) composed of repeated short items; on a laptop this runs in <200 ms.
- **Risk: EF migration generation requires `dotnet ef` tooling locally.** → **Mitigation:** `dotnet tool restore` is already part of the repo — verify `dotnet ef migrations add AddResults --project src/ApiTool.Backend` runs cleanly. If tooling is missing we fall back to manually-authored migration + `ModelSnapshot` diff.
- **Edge case: `limit` query param out of range.** → Clamp `limit` to `[1, 100]` in the service; default `10`.
- **Edge case: `run_at` in the far future or far past.** → No range validation; surface verbatim.
- **Edge case: zero-item run (all skipped).** → Allowed — an empty `items` array is valid; `pass_count`/`fail_count` can both be 0.
- **Edge case: org id route value is not a valid `org_<hex>`.** → Return 403 `permission_denied` (the user cannot be a member of a non-existent org — avoids leaking existence and matches the task's "non-member → 403" behavior).
- **Edge case: `GET /results?limit=0`.** → Clamp up to 1.
- **Edge case: detail lookup where the `resultId` is malformed.** → 404 `result_not_found`.

## Verification

```bash
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
```

Observable verification (from the task YAML):
```bash
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Results"
# Expected: Passed: >=10, Failed: 0

dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/backend/sample-result-upload.json \
  "http://localhost:5000/api/v1/organizations/$ORG/results"
# Expected HTTP 202, body: {"result_id":"res_...","status":"accepted"}
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://localhost:5000/api/v1/organizations/$ORG/results?limit=10"
# Expected HTTP 200, body: {"results":[{"id":"res_...","pass_count":3,"fail_count":0,...}]}
```

Note: the observable requires a seeded org for the token's user — the dev bootstrap needs to auto-create one, or the operator runs `POST /api/v1/organizations` first. Document this in the task comment and do not block on it.
