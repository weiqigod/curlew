# Implementation Plan: M4-008

## Overview
Add a notification subsystem to the backend: EF entities for `notification_rules` + `notification_deliveries`, a RESTful controller for creating rules and listing deliveries, and an in-process dispatcher that fires Slack webhooks / emails when failing runs are ingested via the M4-004 endpoint. Slack delivery uses `IHttpClientFactory`; email uses a pluggable `ISmtpSender` with an in-memory fake registered when the environment is `Testing`. The dispatcher retries failed attempts twice with exponential backoff (500 ms / 2 s).

## Task Details
- **ID:** M4-008
- **Title:** Backend: notification dispatcher (Slack + email)
- **Phase:** M4: Team Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-004 | Test results ingestion API | done |

The existing `IResultIngestedNotifier` seam (registered as `NoopResultIngestedNotifier` in `Program.cs` line 46) is the integration point already provided by M4-004. M4-008 replaces the noop registration with a real dispatcher implementation.

## Architectural Decisions

1. **No MediatR.** The task scope line mentions "MediatR RunIngested domain event" but the codebase already exposes a simpler, already-wired seam: `IResultIngestedNotifier.NotifyAsync(orgId, resultId, ct)` invoked at the end of `ResultsService.IngestAsync`. Introducing MediatR only for one event is disproportionate — the repository principle is "Minimise external dependencies". We reuse the existing seam.
2. **In-process dispatcher invoked synchronously after save.** The notifier interface awaits before the controller returns 202. Keeping dispatch in-process avoids a background queue (out of scope for this milestone) and lets tests assert delivery rows immediately without polling. The dispatcher swallows non-fatal failures (webhook 5xx) after retries so they never fail the ingestion request; only its own delivery row records the failure.
3. **Retries inside `NotificationsDispatcher`, not via Polly.** Polly isn't a dependency yet. A hand-rolled loop over `[500ms, 2s]` satisfies the DoD with no extra packages. We inject a `TimeProvider`-backed delay hook so tests can run retries instantly.
4. **Testing environment uses a `TimeSpan.Zero` retry delay** (controlled via an options object) so the dispatcher retry test completes synchronously.
5. **Rule event names live in a strongly-typed enum** `NotificationEvent { RunFailed, Flaky }` serialised as snake_case strings to match the wire format `run_failed`, `flaky`. `NotificationChannel { Slack, Email }` follows the same pattern.
6. **`on` column persisted as a pipe-separated string.** SQLite has no array type; following the precedent of `PayloadJson`/`SettingsJson` text columns, we store the events as e.g. `"run_failed|flaky"`. A private helper splits/joins.
7. **Rule / delivery ids use the `nrule_` / `ndel_` wire prefixes** mirroring `ResultId.Format` / `ScheduleId.Format` — consistent with existing wire formatting.
8. **DELETE and GET-list rules are out of scope for M4-008** (only POST + GET deliveries appear in behaviors). M4-009 will add them when the UI needs them. The service layer is designed so adding them is a 5-line follow-up.
9. **Ingestion detection of "failing" runs** is `FailCount > 0 OR Any(item.Status == Error)`. The `flaky` event is deferred (no behavior covers it for dispatch — only that the rule can be stored with `on=flaky`).
10. **Webhook posting uses `IHttpClientFactory` named client "notifications"** with a 10-second timeout so tests can inject a stub `DelegatingHandler`.

## Implementation Steps

### Step 1: EF entities + migration 0004_notifications

**Rationale:** Database schema must exist before services can query it. Smallest blast radius — pure additive migration, no other tables touched.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/NotificationRule.cs` | create | Entity for a rule |
| `src/ApiTool.Backend/Data/Entities/NotificationDelivery.cs` | create | Entity for a delivery attempt |
| `src/ApiTool.Backend/Data/Entities/NotificationChannel.cs` | create | Enum: Slack, Email |
| `src/ApiTool.Backend/Data/Entities/NotificationDeliveryStatus.cs` | create | Enum: Pending, Delivered, Failed |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Register new DbSets + model config |
| `src/ApiTool.Backend/Migrations/20260417NNNNNN_AddNotifications.cs` | generate via `dotnet ef migrations add` | Creates `notification_rules`, `notification_deliveries` |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | regenerate | Updated by EF tooling |

#### New Entity Shape

```csharp
// NotificationRule.cs
public sealed class NotificationRule
{
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public NotificationChannel Channel { get; set; }
    public string Target { get; set; } = string.Empty;   // URL for slack, email for smtp
    public string OnEvents { get; set; } = string.Empty; // "run_failed|flaky"
    public Guid CreatedBy { get; set; }
    public DateTime CreatedAt { get; set; }
}

// NotificationDelivery.cs
public sealed class NotificationDelivery
{
    public Guid Id { get; set; }
    public Guid RuleId { get; set; }
    public Guid OrgId { get; set; }       // denormalised for fast org-scoped GET
    public Guid? ResultId { get; set; }   // which result triggered it
    public NotificationChannel Channel { get; set; }
    public NotificationDeliveryStatus Status { get; set; }
    public int? ResponseCode { get; set; }
    public int AttemptCount { get; set; }
    public string? ErrorMessage { get; set; }
    public DateTime AttemptedAt { get; set; }
}
```

#### AppDbContext additions

```csharp
public DbSet<NotificationRule> NotificationRules => Set<NotificationRule>();
public DbSet<NotificationDelivery> NotificationDeliveries => Set<NotificationDelivery>();

// in OnModelCreating:
b.Entity<NotificationRule>(e =>
{
    e.ToTable("notification_rules");
    e.HasKey(x => x.Id);
    e.Property(x => x.Channel).HasConversion<string>().HasMaxLength(20);
    e.Property(x => x.Target).HasMaxLength(500).IsRequired();
    e.Property(x => x.OnEvents).HasMaxLength(200).IsRequired();
    e.HasIndex(x => new { x.OrgId, x.Channel });
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
    e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).OnDelete(DeleteBehavior.Restrict);
});

b.Entity<NotificationDelivery>(e =>
{
    e.ToTable("notification_deliveries");
    e.HasKey(x => x.Id);
    e.Property(x => x.Channel).HasConversion<string>().HasMaxLength(20);
    e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
    e.Property(x => x.ErrorMessage).HasMaxLength(1000);
    e.HasIndex(x => new { x.OrgId, x.AttemptedAt });
    e.HasOne<NotificationRule>().WithMany().HasForeignKey(x => x.RuleId).OnDelete(DeleteBehavior.Cascade);
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
});
```

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs (extend existing file)

[Fact]
public async Task NotificationRules_and_deliveries_tables_exist_with_expected_columns()
{
    await using var scope = TestDb.CreateOpen();
    await scope.Db.Database.MigrateAsync();

    var ruleColumns = await ListTableColumnsAsync(scope.Connection, "notification_rules");
    ruleColumns.Should().Contain(new[] { "Id", "OrgId", "Channel", "Target", "OnEvents",
                                         "CreatedBy", "CreatedAt" });

    var deliveryColumns = await ListTableColumnsAsync(scope.Connection, "notification_deliveries");
    deliveryColumns.Should().Contain(new[] { "Id", "RuleId", "OrgId", "ResultId",
                                             "Channel", "Status", "ResponseCode",
                                             "AttemptCount", "ErrorMessage", "AttemptedAt" });
}
```

#### Impact on Existing Tests
- `AppDbContextSchemaTests` — extended with one new `[Fact]`; existing tests unaffected.
- `ResultsServiceTests` uses `await db.Database.MigrateAsync()` — the new migration must run cleanly. The test will surface a failing migration immediately.

### Step 2: NotificationsService (rule creation, delivery listing, dispatch orchestration)

**Rationale:** Pure DB/business-logic layer with no HTTP surface. Depends only on Step 1. Establishes the exact contract the endpoints will call, so the endpoint layer is trivial once this is green.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Notifications/NotificationsService.cs` | create | Business logic: CreateRule, ListRules, ListDeliveries, DispatchAsync |
| `src/ApiTool.Backend/Notifications/NotificationError.cs` | create | Enum mirroring `ResultError` / `ScheduleError` |
| `src/ApiTool.Backend/Notifications/NotificationRuleDto.cs` | create | DTO for rules |
| `src/ApiTool.Backend/Notifications/NotificationDeliveryDto.cs` | create | DTO for deliveries |
| `src/ApiTool.Backend/Notifications/CreateNotificationRuleRequest.cs` | create | Request DTO |
| `src/ApiTool.Backend/Notifications/NotificationRuleId.cs` | create | Wire format `nrule_<32-hex>` |
| `src/ApiTool.Backend/Notifications/NotificationDeliveryId.cs` | create | Wire format `ndel_<32-hex>` |
| `src/ApiTool.Backend/Notifications/NotificationEvent.cs` | create | Enum `RunFailed, Flaky` + parse helpers |
| `src/ApiTool.Backend/Notifications/ISlackWebhookPoster.cs` | create | Seam for Slack HTTP call |
| `src/ApiTool.Backend/Notifications/ISmtpSender.cs` | create | Seam for email sending |
| `src/ApiTool.Backend/Notifications/SlackWebhookPoster.cs` | create | Prod impl using `IHttpClientFactory` |
| `src/ApiTool.Backend/Notifications/NoopSmtpSender.cs` | create | Dev default — logs, no-op |
| `src/ApiTool.Backend/Notifications/NotificationsDispatcherOptions.cs` | create | Retry delay settings |

#### Service Signatures

```csharp
public sealed class NotificationsService(AppDbContext db, TimeProvider clock)
{
    public async Task<(NotificationRuleDto? dto, NotificationError error, string? message)>
        CreateRuleAsync(Guid userId, Guid orgId, CreateNotificationRuleRequest? req, CancellationToken ct);

    public async Task<(IReadOnlyList<NotificationRuleDto> rules, NotificationError error)>
        ListRulesAsync(Guid userId, Guid orgId, CancellationToken ct);

    public async Task<(IReadOnlyList<NotificationDeliveryDto> deliveries, NotificationError error)>
        ListDeliveriesAsync(Guid userId, Guid orgId, int limit, CancellationToken ct);

    // Called by the dispatcher — returns rules matching a fired event
    public async Task<IReadOnlyList<NotificationRule>>
        FindMatchingRulesAsync(Guid orgId, NotificationEvent evt, CancellationToken ct);

    public async Task RecordDeliveryAsync(NotificationDelivery delivery, CancellationToken ct);
}
```

`CreateRuleAsync` validates:
- Caller is `Owner` or `Admin` (403 otherwise → `PermissionDenied`).
- `channel` parses to `NotificationChannel` (else `InvalidChannel` → 400 `code:invalid_channel`).
- `target` non-empty; if channel=slack must start with `https://`; if channel=email must contain `@`.
- `on` non-empty, all values parse to `NotificationEvent`.

#### Tests to Write FIRST (RED phase)

```csharp
// NotificationsServiceTests.cs

[Fact] public async Task CreateRuleAsync_persists_slack_rule_for_admin();
[Fact] public async Task CreateRuleAsync_persists_email_rule_with_multiple_events();
[Fact] public async Task CreateRuleAsync_returns_invalid_channel_for_unknown_channel();
[Fact] public async Task CreateRuleAsync_returns_invalid_channel_for_non_https_slack_target();
[Fact] public async Task CreateRuleAsync_returns_permission_denied_for_member();
[Fact] public async Task ListRulesAsync_returns_rules_in_stable_order_for_member();
[Fact] public async Task ListDeliveriesAsync_returns_newest_first_bounded_by_limit();
[Fact] public async Task ListDeliveriesAsync_returns_permission_denied_for_non_member();
[Fact] public async Task FindMatchingRulesAsync_returns_only_rules_matching_event_and_org();
```

#### Impact on Existing Tests
- None; this is new code.

### Step 3: NotificationsDispatcher implementing `IResultIngestedNotifier`

**Rationale:** Connects the services layer to the existing ingestion seam. Must come after Step 2 because it calls `NotificationsService.FindMatchingRulesAsync` and `RecordDeliveryAsync`. Isolated test surface thanks to `ISlackWebhookPoster` / `ISmtpSender` seams.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Notifications/NotificationsDispatcher.cs` | create | Replaces `NoopResultIngestedNotifier`; dispatches to matching rules with retries |
| `src/ApiTool.Backend.Tests/Notifications/FakeSlackWebhookPoster.cs` | create | Records calls + scripts response codes |
| `src/ApiTool.Backend.Tests/Notifications/FakeSmtpSender.cs` | create | Records email sends |

#### Dispatcher Signature

```csharp
public sealed class NotificationsDispatcher(
    IServiceScopeFactory scopeFactory,
    ISlackWebhookPoster slack,
    ISmtpSender smtp,
    TimeProvider clock,
    IOptions<NotificationsDispatcherOptions> options,
    ILogger<NotificationsDispatcher> logger) : IResultIngestedNotifier
{
    public async Task NotifyAsync(Guid orgId, Guid resultId, CancellationToken ct);
}
```

Behaviour:
1. Open a DI scope; resolve `AppDbContext` + `NotificationsService`.
2. Load the `Result` by id; derive event (`RunFailed` if `FailCount > 0`, otherwise no dispatch — short-circuit).
3. Find matching rules via `FindMatchingRulesAsync`.
4. For each rule: call `AttemptDeliveryAsync(rule, resultId)`:
   - Up to 3 attempts total (initial + 2 retries).
   - Delays `500ms`, `2s` (from options, configurable, zero in tests).
   - On attempt success (Slack: 2xx response; Email: no exception): insert one `NotificationDelivery` row with `Status=Delivered`, `AttemptCount=N`, `ResponseCode=200`.
   - On final failure: insert one `NotificationDelivery` row with `Status=Failed`, `ResponseCode` = last response code or `null`, `ErrorMessage` populated.
5. Exceptions inside `NotifyAsync` are caught + logged; never propagated. Ingestion must never 500 because of a notification failure.

#### Tests to Write FIRST (RED phase)

```csharp
// NotificationsDispatcherTests.cs

[Fact] public async Task NotifyAsync_does_nothing_when_result_passes();
[Fact] public async Task NotifyAsync_posts_to_slack_webhook_when_rule_matches_and_run_failed();
[Fact] public async Task NotifyAsync_records_delivered_status_on_2xx();
[Fact] public async Task NotifyAsync_retries_twice_on_5xx_then_records_failed();
[Fact] public async Task NotifyAsync_records_failed_when_all_retries_500();
[Fact] public async Task NotifyAsync_payload_contains_result_id_collection_and_failure_counts();
[Fact] public async Task NotifyAsync_never_throws_when_http_throws();
```

The retry test uses a `FakeSlackWebhookPoster` that returns `(HttpStatusCode.InternalServerError, "boom")` for all three attempts and asserts the `DeliveriesTable.Count() == 1 && status == Failed && AttemptCount == 3`.

#### Impact on Existing Tests
- `ResultsServiceTests.IngestAsync_invokes_notifier_after_save` continues to use `FakeResultIngestedNotifier` — still passes, nothing changed at the seam level.
- No existing tests break.

### Step 4: HTTP endpoints + request/response shaping

**Rationale:** Trivial layer over the service. Last, because it's the smallest piece once the service contract is stable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Notifications/NotificationsEndpoints.cs` | create | Maps POST `/notification-rules`, GET `/notification-deliveries` |
| `src/ApiTool.Backend/Program.cs` | modify | Registers services + `MapNotificationsEndpoints()` |

#### Endpoints

```csharp
orgGroup.MapPost("/notification-rules", CreateRule)
    .WithName("CreateNotificationRule")
    .Accepts<CreateNotificationRuleRequest>("application/json")
    .Produces<NotificationRuleDto>(StatusCodes.Status201Created)
    .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
    .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

orgGroup.MapGet("/notification-deliveries", ListDeliveries)
    .WithName("ListNotificationDeliveries")
    .Produces<ListDeliveriesResponse>()
    .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);
```

Both live under `/api/v1/organizations/{orgId}` (`MapGroup` pattern identical to schedules).

Error mapping:
- `NotificationError.InvalidChannel` → 400 `{ "code": "invalid_channel" }`
- `NotificationError.InvalidTarget` → 400 `{ "code": "invalid_target" }`
- `NotificationError.InvalidEvents` → 400 `{ "code": "invalid_events" }`
- `NotificationError.PermissionDenied` → 403 `permission_denied`

Rule response example (snake_case):
```json
{
  "id": "nrule_abc...",
  "channel": "slack",
  "target": "https://hooks.slack.test/xyz",
  "on": ["run_failed"],
  "created_at": "..."
}
```

Delivery response (inside `deliveries` wrapper):
```json
{
  "id": "ndel_...",
  "rule_id": "nrule_...",
  "channel": "slack",
  "status": "delivered",
  "response_code": 200,
  "attempted_at": "..."
}
```

#### Tests to Write FIRST (RED phase)

```csharp
// NotificationsEndpointsTests.cs
[Fact] public async Task Post_notification_rules_returns_201_with_slack_rule();
[Fact] public async Task Post_notification_rules_returns_201_with_email_rule_multiple_events();
[Fact] public async Task Post_notification_rules_returns_400_invalid_channel();
[Fact] public async Task Post_notification_rules_returns_403_for_non_admin_member();
[Fact] public async Task Get_notification_deliveries_returns_newest_first();
[Fact] public async Task Ingesting_failing_run_triggers_slack_post_and_delivery_row();
[Fact] public async Task Ingesting_passing_run_records_no_delivery();
[Fact] public async Task Swagger_json_lists_notification_endpoints();
```

The "ingesting failing run" test relies on a `FakeSlackWebhookPoster` registered by `BackendFactory` so it observes the request body.

#### Impact on Existing Tests
- `SmokeTests` (if present) — unaffected; the root `/swagger` path is untouched.
- No existing tests break.

### Step 5: DI registration + `BackendFactory` overrides

**Rationale:** Ties everything together and switches the production registration from noop to real dispatcher. Must come after Steps 2–4.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Program.cs` | modify | Register `NotificationsService`, `NotificationsDispatcher`, `SlackWebhookPoster`, `NoopSmtpSender`, `IHttpClientFactory` named client, and `MapNotificationsEndpoints()` |
| `src/ApiTool.Backend.Tests/TestInfrastructure/BackendFactory.cs` | modify | In `ConfigureServices`, replace `ISlackWebhookPoster` + `ISmtpSender` with fakes, and configure zero-delay retry options |

#### Program.cs changes

```csharp
builder.Services.AddHttpClient("notifications", c => c.Timeout = TimeSpan.FromSeconds(10));
builder.Services.AddScoped<NotificationsService>();
builder.Services.AddSingleton<ISlackWebhookPoster, SlackWebhookPoster>();
builder.Services.AddSingleton<ISmtpSender, NoopSmtpSender>();
builder.Services.Configure<NotificationsDispatcherOptions>(o =>
{
    o.RetryDelays = new[] { TimeSpan.FromMilliseconds(500), TimeSpan.FromSeconds(2) };
});

// Replace the existing noop registration:
// builder.Services.AddSingleton<IResultIngestedNotifier, NoopResultIngestedNotifier>();
builder.Services.AddScoped<IResultIngestedNotifier, NotificationsDispatcher>();
```

Note the lifetime change to `Scoped` — the dispatcher needs per-request scope so it can resolve a scoped DbContext for the recording step. The interface is invoked inside `ResultsService.IngestAsync` which already runs in a request scope, so `Scoped` is correct. We can remove `NoopResultIngestedNotifier` or leave it as the documented fallback.

Testing environment in `BackendFactory`:
```csharp
services.RemoveAll<ISlackWebhookPoster>();
services.AddSingleton<FakeSlackWebhookPoster>();
services.AddSingleton<ISlackWebhookPoster>(sp => sp.GetRequiredService<FakeSlackWebhookPoster>());
services.RemoveAll<ISmtpSender>();
services.AddSingleton<FakeSmtpSender>();
services.AddSingleton<ISmtpSender>(sp => sp.GetRequiredService<FakeSmtpSender>());
services.Configure<NotificationsDispatcherOptions>(o =>
    o.RetryDelays = new[] { TimeSpan.Zero, TimeSpan.Zero });
```

#### Impact on Existing Tests
- `ResultsEndpointsTests.Post_results_returns_202_and_persists_row` — still green; dispatcher sees zero rules → no-op.
- `ResultsServiceTests.IngestAsync_invokes_notifier_after_save` — unchanged; this test constructs its own `ResultsService` with a `FakeResultIngestedNotifier` directly, bypassing DI.
- Swagger tests — unaffected; just more paths listed.

### Step 6: Testdata fixture + csproj Content update

**Rationale:** The observable command posts `testdata/backend/failing-result-upload.json` which does not exist yet. Also the csproj only copies `sample-result-upload.json`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/backend/failing-result-upload.json` | create | Payload with `fail_count=1` + one failing item |
| `src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | modify | Extra `<Content Include="..\..\testdata\backend\failing-result-upload.json" ...>` |

#### New fixture

```json
{
  "collection_name": "smoke-tests",
  "run_at": "2026-04-17T12:00:00Z",
  "duration_ms": 1200,
  "pass_count": 2,
  "fail_count": 1,
  "skipped_count": 0,
  "triggered_by": "cli",
  "git_sha": "def5678",
  "items": [
    { "name": "GET /users", "status": "passed", "duration_ms": 120, "message": null },
    { "name": "POST /users", "status": "passed", "duration_ms": 250, "message": null },
    { "name": "DELETE /users", "status": "failed", "duration_ms": 830, "message": "Expected 204 got 500" }
  ]
}
```

#### Impact on Existing Tests
- None. New file.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | new `NotificationRules_and_deliveries_tables_exist_with_expected_columns` | add | write RED test |
| `src/ApiTool.Backend.Tests/Notifications/NotificationsServiceTests.cs` | new (9 tests) | add | write all RED tests |
| `src/ApiTool.Backend.Tests/Notifications/NotificationsDispatcherTests.cs` | new (7 tests) | add | write all RED tests |
| `src/ApiTool.Backend.Tests/Notifications/NotificationsEndpointsTests.cs` | new (8 tests) | add | write all RED tests |
| `src/ApiTool.Backend.Tests/Results/ResultsEndpointsTests.cs` | all | none | keep green by adding fakes in `BackendFactory` |
| `src/ApiTool.Backend.Tests/Results/ResultsServiceTests.cs` | all | none | unit tests build their own `ResultsService`; unaffected |
| `src/ApiTool.Backend.Tests/Results/FixturePostingTests.cs` | all | none | posts the passing fixture only |

Total new tests: **~25** (well above the DoD's `>=10` Notifications filter).

## Risks and Edge Cases

- **Risk:** Dispatcher runs inside the request scope, so a slow webhook blocks the 202 response → **Mitigation:** 10-second `HttpClient.Timeout` (prod) / 0ms retry delays (tests). Future work (tracked in M5 backlog, not here) could move to a channel.
- **Risk:** `Scoped` lifetime for `IResultIngestedNotifier` clashes with the existing `Singleton` registration of `NoopResultIngestedNotifier` — DI throws `InvalidOperationException` if both are registered → **Mitigation:** replace the existing registration, not add a parallel one.
- **Risk:** EF Core InMemory provider (used by `BackendFactory`) does not support migrations or certain constraints → **Mitigation:** use `EnsureCreatedAsync()` in test bootstrap (already done). New migration is only exercised by `TestDb`-backed SQLite tests + prod.
- **Edge case:** `fail_count=0` but one item has `status=error` → treat as failing run. Handled by the dispatcher (`FailCount > 0 || result has any Error/Failed item`). Covered by a dispatcher test.
- **Edge case:** Concurrent rule creation with same target → permitted by design; no unique constraint on `(OrgId, Target)`. Dispatcher iterates all matching rules, so duplicates fire twice — acceptable for now.
- **Edge case:** `on=[]` (empty array) → `InvalidEvents` 400. Covered.
- **Edge case:** Slack webhook returns 429 Too Many Requests → counts as failure; retried like 5xx. (Spec says "500 returns failed" but generalising to any non-2xx is safer and still satisfies the behavior.)
- **Edge case:** Dispatcher fires but DbContext has been disposed because the outer request already completed → **Mitigation:** dispatcher resolves a fresh DI scope via `IServiceScopeFactory`, independent of the request scope.
- **Risk:** `TreatWarningsAsErrors` will fail the build if any generated migration code has unused usings → **Mitigation:** verify after running `dotnet ef migrations add`; follow the existing `AddResults` pattern.

## Verification

```bash
# Build + run all tests
dotnet build src/ApiTool.Backend.sln -warnaserror
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj

# Notifications-only filter (matches observable)
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Notifications"
# Expected: Passed: >=10, Failed: 0

# Migration applies cleanly
rm -f src/ApiTool.Backend/Data/apitool-dev.db
ASPNETCORE_ENVIRONMENT=Development dotnet run --project src/ApiTool.Backend &
sleep 3
sqlite3 src/ApiTool.Backend/Data/apitool-dev.db ".tables" \
  | grep -E "notification_rules|notification_deliveries"
```

### Observable verification (from task YAML)

```bash
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Notifications"

dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"channel":"slack","target":"https://hooks.slack.test/xyz","on":["run_failed"]}' \
  "http://localhost:5000/api/v1/organizations/$ORG/notification-rules"
# Expected: HTTP 201, body contains "id":"nrule_..."

curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/backend/failing-result-upload.json \
  "http://localhost:5000/api/v1/organizations/$ORG/results"
sleep 1
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://localhost:5000/api/v1/organizations/$ORG/notification-deliveries"
# Expected HTTP 200, first row status=delivered|failed (depending on remote reachability),
# channel=slack
```

Note: the observable's expectation of `status=delivered` assumes the Slack URL is reachable. In the local manual run the host `hooks.slack.test` won't resolve, so expect `status=failed` with retries exhausted — still a valid demonstration that the dispatcher fired. The automated test `Ingesting_failing_run_triggers_slack_post_and_delivery_row` uses the in-memory `FakeSlackWebhookPoster` so it deterministically produces `delivered`.
