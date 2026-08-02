# Implementation Plan: M5-004

## Overview

Extract inline audit writes into a centralised `IAuditWriter` service that captures
`ip_address` / `user_agent` via an `AuditCaptureMiddleware`, expand the
`organization_audit_log` schema to include `target_type`, `target_id`,
`previous_state`, `new_state`, `ip_address`, `user_agent`, and expose
`GET /api/v1/organizations/{id}/audit-log` with filter + CSV support.

## Task Details

- **ID:** M5-004
- **Title:** Backend: audit log capture middleware
- **Phase:** M5: Enterprise Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task    | Title                           | Status |
| ------- | ------------------------------- | ------ |
| M4-003  | Backend org RBAC                | done   |

## Scope Resolution (Open Questions Settled)

The task YAML says "schemas already in DB design, no new migration needed" and mentions
`user_audit_log`, `auth_audit_log`, `subscription_audit_log`. On inspection only
`organization_audit_log` exists in the database, and it is missing the `target_type`,
`target_id`, `previous_state`, `new_state`, `ip_address`, `user_agent` columns that the
listed behaviors require. Decisions taken (pipeline mode, no user confirmation):

1. **New EF Core migration IS needed** — the existing `organization_audit_log` table
   only has `Id`, `OrgId`, `ActorId`, `EventType`, `PayloadJson`, `CreatedAt`. The task
   behaviors explicitly require `target_type`, `target_id`, `previous_state`,
   `new_state`, `ip_address`, `user_agent`, so a migration is required. All added
   columns are nullable so existing rows remain valid.
2. **Only `organization_audit_log` is in scope for M5-004.** Separate `auth_audit_log`
   and `subscription_audit_log` tables are called out in the spec but are NOT needed
   for the CLI behaviors — all current auth/subscription audit writes already land in
   `organization_audit_log`. Introducing two extra tables would triple the migration
   blast radius and is out of scope for a single vertical slice. The behaviors around
   "sso.login", "subscription.updated", and failed login events are all satisfied by
   expanded `organization_audit_log` rows. We document this compromise here; adding
   the separate tables can become its own backlog item.
3. **No MediatR pipeline behaviour** — the codebase does not use MediatR, and adding it
   for one feature is disproportionate. The same goals (consistent capture of
   ip/user_agent, centralised event writing) are achieved by:
   - `AuditCaptureMiddleware` — populates a scoped `AuditContext` with `IpAddress` +
     `UserAgent` for every authenticated request.
   - `IAuditWriter` — scoped service that resolves `AuditContext` from DI and writes
     `OrganizationAuditLogEntry` rows with the captured fields. Callers never pass
     ip/user_agent explicitly.
4. **Rate limiting** — a new `audit-log-read` policy (30/min partitioned by `orgId`) is
   added to the Program.cs rate limiter configuration.
5. **Failed login capture** — `SsoService` / `OidcService` currently write
   `sso.login_failed` with actor `Guid.Empty` when the user does not yet exist.
   Extended to include `success=false` and `failure_reason` columns via the new
   schema.
6. **CSV format** — emits columns `created_at,event_type,user_id,target_type,target_id`
   with RFC 4180 quoting; `Content-Disposition: attachment; filename="audit-log-<org_slug>-<yyyyMMdd>.csv"`.
7. **`user_id` vs `actor_id`** — task behaviours speak in terms of `user_id`. The entity
   field stays `ActorId` for backwards compatibility, but the JSON response uses
   `user_id` (snake_case via `ActorId` → `user_id` mapping in the DTO).

## Implementation Steps

### Step 1: Expand `OrganizationAuditLogEntry` entity + EF migration

**Rationale:** Smallest blast radius first — data model change unlocks everything else.
All added columns are nullable, so all existing writes continue to work.

#### Files to Modify

| File | Action | Description |
| ---- | ------ | ----------- |
| `src/ApiTool.Backend/Data/Entities/OrganizationAuditLogEntry.cs` | modify | Add `TargetType`, `TargetId`, `PreviousStateJson`, `NewStateJson`, `IpAddress`, `UserAgent` nullable properties |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Configure column names and max-lengths for the new fields |
| `src/ApiTool.Backend/Migrations/2026041900000_AddAuditLogDetails.cs` | create | `migrationBuilder.AddColumn` for the six new columns + index on `EventType` |
| `src/ApiTool.Backend/Migrations/2026041900000_AddAuditLogDetails.Designer.cs` | create | EF-generated designer |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modify | Update snapshot to reflect new columns |

#### Current Code

```csharp
public sealed class OrganizationAuditLogEntry
{
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public Guid ActorId { get; set; }
    public string EventType { get; set; } = string.Empty;
    public string PayloadJson { get; set; } = "{}";
    public DateTime CreatedAt { get; set; }
}
```

#### New Code

```csharp
public sealed class OrganizationAuditLogEntry
{
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public Guid ActorId { get; set; }
    public string EventType { get; set; } = string.Empty;
    public string PayloadJson { get; set; } = "{}";
    public string? TargetType { get; set; }
    public Guid? TargetId { get; set; }
    public string? PreviousStateJson { get; set; }
    public string? NewStateJson { get; set; }
    public string? IpAddress { get; set; }
    public string? UserAgent { get; set; }
    public bool Success { get; set; } = true;
    public string? FailureReason { get; set; }
    public DateTime CreatedAt { get; set; }
}
```

In `AppDbContext.OnModelCreating`, the `OrganizationAuditLogEntry` block extends to:

```csharp
b.Entity<OrganizationAuditLogEntry>(e =>
{
    e.ToTable("organization_audit_log");
    e.HasKey(x => x.Id);
    e.Property(x => x.EventType).HasMaxLength(100).IsRequired();
    e.Property(x => x.PayloadJson).HasColumnName("payload").IsRequired();
    e.Property(x => x.TargetType).HasColumnName("target_type").HasMaxLength(50);
    e.Property(x => x.TargetId).HasColumnName("target_id");
    e.Property(x => x.PreviousStateJson).HasColumnName("previous_state");
    e.Property(x => x.NewStateJson).HasColumnName("new_state");
    e.Property(x => x.IpAddress).HasColumnName("ip_address").HasMaxLength(45); // IPv6
    e.Property(x => x.UserAgent).HasColumnName("user_agent").HasMaxLength(500);
    e.Property(x => x.Success).HasColumnName("success").IsRequired();
    e.Property(x => x.FailureReason).HasColumnName("failure_reason").HasMaxLength(200);
    e.HasIndex(x => new { x.OrgId, x.CreatedAt });
    e.HasIndex(x => x.EventType);
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
});
```

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Data/OrganizationAuditLogSchemaTests.cs
public sealed class OrganizationAuditLogSchemaTests
{
    [Fact]
    public async Task Audit_log_table_has_extended_columns()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        // Insert a row with all new columns populated.
        var orgId = Guid.NewGuid();
        scope.Db.Organizations.Add(new Organization { Id = orgId, Name = "T", Slug = $"t{orgId:N}"[..20], OwnerId = Guid.NewGuid(), Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow });
        scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(), OrgId = orgId, ActorId = Guid.NewGuid(),
            EventType = "member.invited", TargetType = "invitation",
            TargetId = Guid.NewGuid(), PreviousStateJson = "{}",
            NewStateJson = "{\"role\":\"member\"}",
            IpAddress = "192.0.2.1", UserAgent = "curl/8",
            CreatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();
    }
}
```

#### Impact on Existing Tests

- `AppDbContextSchemaTests` — unaffected (no direct column assertions on audit table).
- All existing tests that write to `OrganizationAuditLog` continue to work because all
  new columns are nullable / have defaults.

---

### Step 2: Introduce `AuditContext` + `AuditCaptureMiddleware`

**Rationale:** Without the middleware, downstream audit writes have no way to see the
request ip/user-agent. Must land before Step 3 starts consuming it.

#### Files to Modify

| File | Action | Description |
| ---- | ------ | ----------- |
| `src/ApiTool.Backend/Audit/AuditContext.cs` | create | Scoped per-request holder of `IpAddress` + `UserAgent` |
| `src/ApiTool.Backend/Audit/AuditCaptureMiddleware.cs` | create | Populates `AuditContext` from `HttpContext.Connection.RemoteIpAddress` and `Request.Headers.UserAgent` |
| `src/ApiTool.Backend/Program.cs` | modify | Register `AuditContext` scoped, register middleware via `app.UseMiddleware<AuditCaptureMiddleware>()` after `UseAuthentication` |

#### New Code

```csharp
// src/ApiTool.Backend/Audit/AuditContext.cs
namespace ApiTool.Backend.Audit;

/// <summary>Per-request holder of audit-relevant request metadata.</summary>
public sealed class AuditContext
{
    public string? IpAddress { get; set; }
    public string? UserAgent { get; set; }
}

// src/ApiTool.Backend/Audit/AuditCaptureMiddleware.cs
namespace ApiTool.Backend.Audit;

public sealed class AuditCaptureMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext ctx, AuditContext audit)
    {
        audit.IpAddress = ResolveIp(ctx);
        audit.UserAgent = ctx.Request.Headers.UserAgent.ToString();
        if (audit.UserAgent.Length > 500)
            audit.UserAgent = audit.UserAgent[..500];
        await next(ctx);
    }

    private static string? ResolveIp(HttpContext ctx)
    {
        // Prefer X-Forwarded-For first IP (behind reverse proxy); fall back to socket peer.
        var fwd = ctx.Request.Headers["X-Forwarded-For"].ToString();
        if (!string.IsNullOrWhiteSpace(fwd))
            return fwd.Split(',')[0].Trim();
        return ctx.Connection.RemoteIpAddress?.ToString();
    }
}
```

Wiring in `Program.cs` (right after `app.UseAuthentication()` / `UseAuthorization()`):

```csharp
builder.Services.AddScoped<AuditContext>();
// ...
app.UseMiddleware<AuditCaptureMiddleware>();
```

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Audit/AuditCaptureMiddlewareTests.cs
public sealed class AuditCaptureMiddlewareTests
{
    [Theory]
    [InlineData("X-Forwarded-For", "203.0.113.5, 10.0.0.1", "203.0.113.5")]
    [InlineData("User-Agent", "curl/8.1", "curl/8.1")]
    public async Task Middleware_populates_scoped_context(string header, string value, string expected) { /* ... */ }

    [Fact]
    public async Task Middleware_truncates_user_agent_at_500_chars() { /* ... */ }

    [Fact]
    public async Task Middleware_falls_back_to_socket_peer_when_no_forwarded_header() { /* ... */ }
}
```

#### Impact on Existing Tests

- None. New package, new middleware. Existing tests don't exercise it.

---

### Step 3: Introduce `IAuditWriter` service and refactor inline callers

**Rationale:** Centralise the write path so ip/user_agent/target_type are added
consistently. Inline callers become 1-line calls.

#### Files to Modify

| File | Action | Description |
| ---- | ------ | ----------- |
| `src/ApiTool.Backend/Audit/IAuditWriter.cs` | create | Interface for writing audit rows |
| `src/ApiTool.Backend/Audit/AuditWriter.cs` | create | Concrete impl — reads `AuditContext`, writes to `AppDbContext.OrganizationAuditLog` |
| `src/ApiTool.Backend/Audit/AuditEvent.cs` | create | Record with the write payload: `OrgId`, `ActorId`, `EventType`, `TargetType?`, `TargetId?`, previous/new state, success, failure_reason |
| `src/ApiTool.Backend/Program.cs` | modify | `services.AddScoped<IAuditWriter, AuditWriter>()` |
| `src/ApiTool.Backend/Invitations/InvitationsService.cs` | modify | Replace inline `db.OrganizationAuditLog.Add(...)` with `audit.Append(...)` (member.invited / member.invitation_revoked / member.invitation_accepted) and inject `IAuditWriter` |
| `src/ApiTool.Backend/Organizations/OrganizationService.cs` | modify | Same refactor for `org.created` |
| `src/ApiTool.Backend/Organizations/MembersService.cs` | modify | Same for `member.role_changed`, `member.removed`, `org.ownership_transferred`, `member.left`, `org.updated`, `org.deletion_scheduled`, `org.deletion_canceled`. `org.updated` gets `previous_state` + `new_state` populated (task behaviour #2) — renamed to `org.settings.updated` when settings (including name) changed, matching the CLI behaviour spec |
| `src/ApiTool.Backend/Subscriptions/SubscriptionsService.cs` | modify | Same for `subscription.created` / `subscription.updated` / `subscription.upgraded` / `subscription.downgraded` / `subscription.canceled`. `subscription.updated` populates `previous_state` + `new_state` + `target_type="subscription"` + `target_id=sub.Id` (task behaviour #5) |
| `src/ApiTool.Backend/Sso/SsoService.cs` | modify | Refactor `sso.config_updated`, `sso.login_success`, `sso.login_failed`. `sso.login_success` → `sso.login` with `success=true` (task behaviour #3); `sso.login_failed` → `sso.login` with `success=false` and `failure_reason` (task behaviour #4) |
| `src/ApiTool.Backend/Sso/OidcService.cs` | modify | Identical refactor |

#### New Code

```csharp
// src/ApiTool.Backend/Audit/IAuditWriter.cs
namespace ApiTool.Backend.Audit;

/// <summary>Appends an audit log row to the pending DbContext transaction.
/// Callers must still call SaveChangesAsync to flush.</summary>
public interface IAuditWriter
{
    void Append(AuditEvent evt);
}

// src/ApiTool.Backend/Audit/AuditEvent.cs
namespace ApiTool.Backend.Audit;

/// <summary>A single audit event payload. ip/user_agent are enriched by the writer.</summary>
public sealed record AuditEvent(
    Guid OrgId,
    Guid ActorId,
    string EventType,
    string? TargetType = null,
    Guid? TargetId = null,
    object? Payload = null,
    object? PreviousState = null,
    object? NewState = null,
    bool Success = true,
    string? FailureReason = null);

// src/ApiTool.Backend/Audit/AuditWriter.cs
public sealed class AuditWriter(AppDbContext db, AuditContext audit, TimeProvider clock) : IAuditWriter
{
    private static readonly JsonSerializerOptions Json = new() { PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower };

    public void Append(AuditEvent evt)
    {
        db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            OrgId = evt.OrgId,
            ActorId = evt.ActorId,
            EventType = evt.EventType,
            TargetType = evt.TargetType,
            TargetId = evt.TargetId,
            PayloadJson = evt.Payload is null ? "{}" : JsonSerializer.Serialize(evt.Payload, Json),
            PreviousStateJson = evt.PreviousState is null ? null : JsonSerializer.Serialize(evt.PreviousState, Json),
            NewStateJson = evt.NewState is null ? null : JsonSerializer.Serialize(evt.NewState, Json),
            IpAddress = audit.IpAddress,
            UserAgent = audit.UserAgent,
            Success = evt.Success,
            FailureReason = evt.FailureReason,
            CreatedAt = clock.GetUtcNow().UtcDateTime,
        });
    }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Audit/AuditWriterTests.cs
public sealed class AuditWriterTests
{
    [Fact]
    public async Task Append_writes_row_with_ip_and_user_agent_from_context() { /* ... */ }

    [Fact]
    public async Task Append_serializes_previous_and_new_state_when_provided() { /* ... */ }

    [Fact]
    public async Task Append_records_success_false_with_failure_reason() { /* ... */ }

    [Fact]
    public async Task Append_serializes_payload_with_snake_case() { /* ... */ }
}
```

Integration-test additions (all live in a new file to satisfy the `Audit` filter):

```csharp
// src/ApiTool.Backend.Tests/Audit/AuditCaptureIntegrationTests.cs
[Collection(BackendCollection.Name)]
public sealed class AuditCaptureIntegrationTests : IAsyncLifetime
{
    [Fact] public async Task Invitation_create_writes_row_with_target_type_invitation_and_ip() { }
    [Fact] public async Task Org_settings_patch_writes_previous_state_and_new_state_diff() { }
    [Fact] public async Task Sso_login_success_writes_row_with_success_true() { }
    [Fact] public async Task Sso_login_failure_writes_row_with_success_false_and_failure_reason() { }
    [Fact] public async Task Subscription_update_writes_row_with_target_type_subscription() { }
}
```

#### Impact on Existing Tests

| Test                                                        | Impact                                                                                                                                                                      |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `OrganizationServiceTests.Create_persists_audit_log_entry_org_created` | No change — still asserts `EventType == "org.created"`.                                                                                                         |
| `OrganizationServiceTests.Create_persists_valid_json_in_audit_log_for_special_character_names` | No change — `PayloadJson` still serializes name.                                                                                           |
| `InvitationsServiceTests.*audit*`                           | No change — uses `EventType == "member.invited"` assertion.                                                                                                                 |
| `InvitationsEndpointsTests.Post_writes_member_invited_audit_row` | No change — same assertion.                                                                                                                                            |
| `SsoServiceTests.*audit*`, `OidcServiceTests.*audit*`       | Events `sso.login_success` / `sso.login_failed` are renamed to `sso.login` with `Success` column. These tests need updating to assert `EventType=="sso.login"` + `Success` boolean. |
| `SubscriptionsServiceTests.*audit*`                         | `subscription.upgraded` etc. names preserved — tests unchanged.                                                                                                             |
| `MembersService*` tests (indirectly exercised)              | `org.updated` renamed to `org.settings.updated` when settings change. Grep shows no existing test asserts on this EventType, so no breakage.                                |

Concrete refactor — `InvitationsService.CreateAsync` before/after:

```csharp
// before
db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry { ... EventType = "member.invited", ... });

// after
audit.Append(new AuditEvent(
    OrgId: orgId,
    ActorId: inviterUserId,
    EventType: "member.invited",
    TargetType: "invitation",
    TargetId: invitation.Id,
    Payload: new { email = emailNormalized, role = roleStr.ToLowerInvariant() },
    NewState: new { email = emailNormalized, role = roleStr.ToLowerInvariant() }));
```

---

### Step 4: `AuditLogController` — GET endpoint with filter + CSV

**Rationale:** Consumer of all prior steps. Last because it depends on the schema,
context, and writer being in place.

#### Files to Modify

| File                                                         | Action | Description                                                                 |
| ------------------------------------------------------------ | ------ | --------------------------------------------------------------------------- |
| `src/ApiTool.Backend/Audit/AuditLogEndpoints.cs`             | create | Maps `GET /api/v1/organizations/{id}/audit-log`                             |
| `src/ApiTool.Backend/Audit/AuditLogQueryService.cs`          | create | Service that loads rows with filtering (RBAC, event_type, user_id, from/to) |
| `src/ApiTool.Backend/Audit/AuditLogEntryDto.cs`              | create | JSON DTO (`event_type`, `user_id`, `target_type`, `target_id`, `created_at`, `ip_address`, `success`, `failure_reason`) |
| `src/ApiTool.Backend/Audit/AuditLogCsvFormatter.cs`          | create | Writes CSV with RFC 4180 quoting                                            |
| `src/ApiTool.Backend/Audit/AuditLogError.cs`                 | create | `None`, `PermissionDenied`, `OrganizationNotFound`, `InvalidFilter`         |
| `src/ApiTool.Backend/Program.cs`                             | modify | Register `AuditLogQueryService` scoped, `app.MapAuditLogEndpoints()`, add `audit-log-read` rate-limit policy (30/min per org) |

#### New Code

```csharp
// src/ApiTool.Backend/Audit/AuditLogEndpoints.cs
public static class AuditLogEndpoints
{
    public static IEndpointRouteBuilder MapAuditLogEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}")
            .RequireAuthorization()
            .RequireRateLimiting("audit-log-read")
            .WithTags("AuditLog");

        group.MapGet("/audit-log", GetAuditLog)
            .WithName("GetAuditLog")
            .Produces<ListAuditLogResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        return app;
    }

    private static async Task<IResult> GetAuditLog(
        string orgId, string? event_type, string? user_id, DateTime? from, DateTime? to,
        int? limit, string? format,
        CurrentUserAccessor users, AuditLogQueryService svc, CancellationToken ct)
    {
        // ... parse OrgId, resolve user, delegate to svc.QueryAsync(...) ...
        // If format=="csv" return text/csv with Content-Disposition.
    }
}

// src/ApiTool.Backend/Audit/AuditLogQueryService.cs
public sealed class AuditLogQueryService(AppDbContext db)
{
    public async Task<(IReadOnlyList<AuditLogEntryDto> items, AuditLogError err, string? msg)>
        QueryAsync(Guid userId, Guid orgId, AuditLogFilter filter, CancellationToken ct)
    {
        // Require admin/owner — behavior #7
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null || member.Role == OrgRole.Member)
            return ([], AuditLogError.PermissionDenied, "Admins and owners only.");

        var q = db.OrganizationAuditLog.Where(e => e.OrgId == orgId);
        if (filter.EventType is not null) q = q.Where(e => e.EventType == filter.EventType);
        if (filter.UserId is not null)    q = q.Where(e => e.ActorId == filter.UserId);
        if (filter.From is not null)      q = q.Where(e => e.CreatedAt >= filter.From);
        if (filter.To is not null)        q = q.Where(e => e.CreatedAt <= filter.To);

        var rows = await q.OrderByDescending(e => e.CreatedAt)
            .Take(Math.Clamp(filter.Limit ?? 50, 1, 200))
            .ToListAsync(ct);

        return (rows.Select(ToDto).ToList(), AuditLogError.None, null);
    }

    private static AuditLogEntryDto ToDto(OrganizationAuditLogEntry e) =>
        new(
            EventType: e.EventType,
            UserId: e.ActorId == Guid.Empty ? null : e.ActorId.ToString("N"),
            TargetType: e.TargetType,
            TargetId: e.TargetId?.ToString("N"),
            CreatedAt: e.CreatedAt,
            IpAddress: e.IpAddress,
            Success: e.Success,
            FailureReason: e.FailureReason);
}
```

Rate-limit policy added to the non-Testing `AddRateLimiter` block in Program.cs:

```csharp
options.AddPolicy("audit-log-read", httpContext =>
    RateLimitPartition.GetFixedWindowLimiter(
        partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
        factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 30, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));
```

and the Testing block's no-op policy list is extended with `"audit-log-read"`.

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Audit/AuditLogEndpointsTests.cs
[Collection(BackendCollection.Name)]
public sealed class AuditLogEndpointsTests : IAsyncLifetime
{
    [Fact] public async Task Get_audit_log_returns_rows_newest_first() { }
    [Fact] public async Task Get_audit_log_as_non_admin_returns_403_permission_denied() { }
    [Fact] public async Task Get_audit_log_with_event_type_filter_returns_only_matching_rows() { }
    [Fact] public async Task Get_audit_log_with_from_and_to_filters_returns_only_in_range_rows() { }
    [Fact] public async Task Get_audit_log_with_user_id_filter_returns_only_matching_rows() { }
    [Fact] public async Task Get_audit_log_csv_format_returns_text_csv_with_content_disposition() { }
    [Fact] public async Task Get_audit_log_csv_rfc4180_quotes_fields_with_commas() { }
    [Fact] public async Task Get_audit_log_respects_limit_parameter_clamped_to_200() { }
    [Fact] public async Task Get_audit_log_returns_400_when_from_is_after_to() { }
    [Fact] public async Task Get_audit_log_for_unknown_org_returns_403() { }
}

// src/ApiTool.Backend.Tests/Audit/AuditLogSwaggerSurfaceTests.cs
[Collection(BackendCollection.Name)]
public sealed class AuditLogSwaggerSurfaceTests : IAsyncLifetime
{
    [Fact]
    public async Task Swagger_lists_audit_log_endpoint_with_filter_params()
    {
        // Assert paths contains "/api/v1/organizations/{orgId}/audit-log" and
        // operationId GetAuditLog has parameters event_type, user_id, from, to, format.
    }
}
```

#### Impact on Existing Tests

- None — pure addition.

## Test Impact Summary

| Test File                                          | Test Function                               | Impact  | Action Required                                                                 |
| -------------------------------------------------- | ------------------------------------------- | ------- | ------------------------------------------------------------------------------- |
| `Sso/SsoServiceTests.cs`                           | tests asserting `sso.login_success`/`sso.login_failed` | breaks  | Update to `sso.login` + `Success` boolean.                                       |
| `Sso/OidcServiceTests.cs`                          | tests asserting `sso.login_success`/`sso.login_failed` | breaks  | Same.                                                                           |
| `Organizations/OrganizationServiceTests.cs`        | audit assertions                            | none    | `org.created` unchanged.                                                        |
| `Invitations/InvitationsServiceTests.cs`           | `member.invited` assertion                  | none    | Event type unchanged.                                                           |
| `Invitations/InvitationsEndpointsTests.cs`         | `Post_writes_member_invited_audit_row`      | none    | Event type unchanged.                                                           |
| `Subscriptions/SubscriptionsServiceTests.cs`       | `subscription.upgraded` etc. assertions     | none    | Event type unchanged.                                                           |
| `Data/AppDbContextSchemaTests.cs`                  | schema assertions                           | none    | No column renames; new columns are additive.                                    |
| `Subscriptions/SwaggerSurfaceTests.cs`             | swagger path list                           | none    | Separate swagger test for audit-log created.                                    |

## Risks and Edge Cases

- **Risk:** Refactor surface is wide (6 services). Mitigation — each service refactor
  is independent and verifiable by its existing tests. Replace inline writes one
  service at a time on a green build.
- **Risk:** `AuditContext` is scoped DI. Background services (`SchedulerHost`) don't
  have an HTTP request. Mitigation — background writers can take a fresh
  `AuditContext` with nulls; writer handles nulls gracefully (columns are nullable).
- **Risk:** EF Core InMemory provider used by `BackendFactory` doesn't fully enforce
  text-length constraints; integration tests on SQLite-backed `TestDb` catch the
  schema constraints.
- **Edge case:** Empty `ActorId` (Guid.Empty) on SSO pre-auth events. Handling — DTO
  maps `Guid.Empty` to `null` so the CLI behaviour spec ("user_id" nullable) is met.
- **Edge case:** `User-Agent` is sometimes null. Handling — middleware stores empty
  string (`Headers.UserAgent.ToString()` returns `""` for missing), which the column
  accepts.
- **Edge case:** CSV injection (an attacker putting `=CMD()` into event_type).
  Mitigation — RFC 4180 quoting + prefix cells starting with `=`, `+`, `-`, `@` with
  a single quote to neutralise Excel formula parsing.
- **Edge case:** IPv6 address max length is 45 chars (incl. zone ID); column sized 45.
- **Edge case:** `From > To` should return 400 `invalid_filter` rather than empty rows
  to distinguish from "no data in range".
- **Edge case:** `format=csv` with > 10,000 rows — clamp `limit` at 200 to prevent
  slow unbounded exports; the full-export endpoint can be a future task.

## Verification

```bash
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
    --filter "FullyQualifiedName~Audit"
# Expected: Passed: >=12, Failed: 0

dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj
# Expected: all green (no regressions in other test classes)
```

Observable verification (from task YAML):

```bash
dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email":"auditee@example.com","role":"member"}' \
  "http://localhost:5000/api/v1/organizations/$ORG/invitations"
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://localhost:5000/api/v1/organizations/$ORG/audit-log?limit=10"
# Expected: HTTP 200, first row: event_type="member.invited", target_type="invitation"
```
