# Implementation Plan: M4-010

## Overview
Introduce subscription/billing, seat management, and invitation-acceptance capabilities to
the backend: three new controller groups (SubscriptionsController, InvitationsController,
MembersController), an `IStripeGateway` abstraction with a fake in-memory implementation,
new EF entities (`Subscription`, extensions to `OrganizationInvitation` + `OrganizationMember`),
migration `0005_billing_and_invitations`, and rate limiters matching the spec tables at lines
6922 and 7291. Seat counting rule is `members + pending invitations`.

## Task Details
- **ID:** M4-010
- **Title:** Backend: billing, seat management, and invitations API
- **Phase:** M4: Team Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-003 | Backend — organizations RBAC + JWT auth | done |

## Architectural Decisions

Decisions taken up-front (with rationale) to avoid churn later:

1. **IStripeGateway abstraction** — A narrow interface with three calls used in tests and
   dev: `CreateCheckoutSessionAsync`, `CreatePortalSessionAsync`, `ComputeProrationAsync`.
   Two implementations: `FakeStripeGateway` (in-memory, deterministic URLs like
   `https://checkout.stripe.test/cs_<guid>`) wired by default, and `StripeGateway` stub
   that throws `NotImplementedException` when `Stripe:Mode == "live"`. Selection via
   `appsettings.json` section `Stripe:Mode` (values: `fake`, `live`). DoD requires that the
   real Stripe stub exists but throws — so the config switch is explicit. `FakeStripeGateway`
   is the default in Development and Testing environments; `Program.cs` only registers the
   live stub when `Stripe:Mode == "live"`.
2. **Single active subscription per organization** — `Subscription.OrgId` is unique. This
   models the spec intent ("already_subscribed" error on duplicate checkout).
3. **Seat counting rule enforced in code, not schema** — `seat_count = active members +
   pending invitations (AcceptedAt IS NULL AND RevokedAt IS NULL AND ExpiresAt > now)`. We
   add a private `SeatService.CountSeatsAsync(orgId, now, ct)` helper used by the service
   layer to avoid drift, with a dedicated unit test.
4. **OrganizationInvitation extensions** — Add `EmailNormalized` (lowercase), `TokenHash`
   (SHA-256 hex of the token so the raw token is only in the response body), `RevokedAt`,
   `RevokedBy`, `LastSentAt`, `SendCount`. Keep the existing `Token` column, but rename
   usage: the column now stores the SHA-256 hash of the raw token. Because no data exists
   yet in M4 deployments (M4-003 only seeded migrations and no real invitations go through
   production), this is safe. The migration renames `Token → TokenHash` and widens to 64.
5. **Invitation token format** — Raw token is 48 random bytes → base64url (≈64 chars).
   Returned in the response body exactly once, at `invitation.token`. The database stores
   only `TokenHash`. Acceptance hashes the incoming token and looks it up by `TokenHash`.
6. **Subscription tier & status enums** — `SubscriptionTier { Free, Professional, Team,
   Enterprise }`, `SubscriptionStatus { Active, PastDue, Canceled, Expired, Incomplete }`.
   DB stored as strings via `HasConversion<string>`, snake_case on the wire.
7. **Proration block** — `FakeStripeGateway.ComputeProrationAsync` returns fixed shape
   `{ credit, charge, net }` in cents. Upgrade path: charge = new_monthly × seat_delta,
   credit = 0 for increases, deterministic for tests.
8. **Seat-limit vs Organization.seat_limit exposure** — Today `OrganizationDto.SeatLimit`
   hardcodes `TeamTierDefaultSeatLimit = 10`. After M4-010 the limit is sourced from the
   organization's `Subscription.SeatLimit`, falling back to 1 for Free tier (owner only).
   This keeps `OrganizationService` responsible for DTO shape; the seat_limit change is
   the smallest blast-radius edit because it only touches one method (`ToDto`).
9. **Rate limiters** — Use named policies in `Program.cs` matching the spec tables:
   `checkout-create` (5/min per user), `subscription-read` (60/min per user),
   `subscription-update` (5/min per user), `subscription-delete` (3/min per user),
   `portal-create` (10/min per user), `invitation-create` (20/hour per org),
   `invitation-resend` (3/24h per invitation). Partition keys derived from
   `HttpContext.User.FindFirstValue(Sub)` for per-user, route value `orgId` for per-org,
   and route value `invitationId` for per-invitation. Disabled in Testing environment (as
   existing pattern in `Program.cs`).
10. **Member management endpoints** — Include `GET /members`, `DELETE /members/{id}`,
    `PATCH /members/{id}`. The observable test only exercises invitation creation + accept,
    but the spec table lists members endpoints as part of the same group; we implement a
    minimal slice (`GET /members`, `DELETE /members/{id}`, `PATCH /members/{id}`) so
    Swagger lists all 15 endpoints as required by the DoD.
11. **Audit logging** — Every write emits one `OrganizationAuditLogEntry` with matching
    spec event type (e.g. `member.invited`, `member.invitation_accepted`, `member.invitation_revoked`,
    `member.removed`, `member.role_changed`, `subscription.created`, `subscription.upgraded`,
    `subscription.downgraded`, `subscription.canceled`).

## Implementation Steps

Steps are ordered from smallest blast radius (pure domain types) to largest (wiring +
Program.cs). Each step is green-buildable in isolation (compiler-green), and Step 1–3
do not break existing tests.

### Step 1: Subscription entity, enums, and ID helper (foundation)
**Rationale:** Pure new types, zero impact on existing code. Gives us compile-time building
blocks for the rest.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/Subscription.cs` | create | Entity |
| `src/ApiTool.Backend/Data/Entities/SubscriptionTier.cs` | create | Enum |
| `src/ApiTool.Backend/Data/Entities/SubscriptionStatus.cs` | create | Enum |
| `src/ApiTool.Backend/Subscriptions/SubscriptionId.cs` | create | `sub_<hex>` helper |
| `src/ApiTool.Backend.Tests/Subscriptions/SubscriptionIdTests.cs` | create | round-trip tests |

#### New Code (key shapes)

```csharp
// Subscription.cs
public sealed class Subscription {
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public SubscriptionTier Tier { get; set; }
    public SubscriptionStatus Status { get; set; }
    public string Interval { get; set; } = "month";  // "month"|"year"
    public int SeatCount { get; set; }   // billed seats
    public int SeatLimit { get; set; }   // purchased seats ceiling
    public DateTime CurrentPeriodStart { get; set; }
    public DateTime CurrentPeriodEnd { get; set; }
    public bool CancelAtPeriodEnd { get; set; }
    public string? StripeCustomerId { get; set; }
    public string? StripeSubscriptionId { get; set; }
    public DateTime CreatedAt { get; set; }
    public DateTime UpdatedAt { get; set; }
    public DateTime? CanceledAt { get; set; }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
// SubscriptionIdTests.cs
[Theory]
[InlineData("sub_00000000000000000000000000000001", true)]
[InlineData("sub_badhex", false)]
[InlineData("org_00000000000000000000000000000001", false)]
[InlineData("", false)]
public void TryParse_round_trips(string input, bool expected) { ... }

[Fact]
public void Format_then_TryParse_is_identity() { ... }
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 2: Invitation entity extensions
**Rationale:** Extend without breaking — the existing M4-003 tests only reference basic
fields; added columns are nullable. Existing behaviour of creating an invitation (only
done internally today) will be updated alongside the invitation service.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/OrganizationInvitation.cs` | modify | Add new fields |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Configure new columns + rename Token→TokenHash |

#### Current Code
```csharp
public sealed class OrganizationInvitation {
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public string Email { get; set; } = string.Empty;
    public OrgRole Role { get; set; }
    public Guid InvitedBy { get; set; }
    public string Token { get; set; } = string.Empty;
    public DateTime ExpiresAt { get; set; }
    public DateTime CreatedAt { get; set; }
    public DateTime? AcceptedAt { get; set; }
}
```

#### New Code
```csharp
public sealed class OrganizationInvitation {
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public string Email { get; set; } = string.Empty;
    public string EmailNormalized { get; set; } = string.Empty;
    public OrgRole Role { get; set; }
    public Guid InvitedBy { get; set; }
    public string TokenHash { get; set; } = string.Empty;  // was "Token"
    public DateTime ExpiresAt { get; set; }
    public DateTime CreatedAt { get; set; }
    public DateTime? AcceptedAt { get; set; }
    public DateTime? RevokedAt { get; set; }
    public Guid? RevokedBy { get; set; }
    public DateTime LastSentAt { get; set; }
    public int SendCount { get; set; }
}
```

`AppDbContext.OnModelCreating` block for `OrganizationInvitation`:
- Rename `Token` to `TokenHash` (HasColumnName("token_hash"))
- Add unique index on `TokenHash`
- Add index on `(OrgId, EmailNormalized, AcceptedAt, RevokedAt)`

#### Impact on Existing Tests
- No direct integration tests write to `OrganizationInvitation` today. Schema tests in
  `AppDbContextSchemaTests` (if present) would be updated, but no such test exists in
  the current repo (verified by directory listing).

---

### Step 3: `IStripeGateway` + `FakeStripeGateway` + `StripeGateway` stub
**Rationale:** Pure new abstraction; no other file depends on it until Step 4.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Subscriptions/IStripeGateway.cs` | create | Interface |
| `src/ApiTool.Backend/Subscriptions/FakeStripeGateway.cs` | create | In-memory impl |
| `src/ApiTool.Backend/Subscriptions/StripeGateway.cs` | create | Throws NotImplemented |
| `src/ApiTool.Backend/Subscriptions/StripeOptions.cs` | create | `Stripe:Mode` options |
| `src/ApiTool.Backend/Subscriptions/ProrationResult.cs` | create | `(int Credit, int Charge, int Net)` record |
| `src/ApiTool.Backend.Tests/Subscriptions/FakeStripeGatewayTests.cs` | create | Deterministic URL + proration |

#### New Code
```csharp
public interface IStripeGateway {
    Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl, CancellationToken ct);
    Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl, CancellationToken ct);
    ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats,
        string interval);
}

public sealed record CheckoutSession(string SessionId, string CheckoutUrl);
public sealed record PortalSession(string PortalUrl);
public sealed record ProrationResult(int Credit, int Charge, int Net);
```

`FakeStripeGateway` returns deterministic `cs_<guid>` session id and
`https://checkout.stripe.test/cs_<guid>` URL. `ComputeProration` formula:
- charge = max(0, (newSeats × monthlyPrice(newTier)) - (oldSeats × monthlyPrice(oldTier))) × (daysLeft / 30)
- credit = max(0, ...) when downgrading — zero in our upgrade test case
- net = charge - credit

Monthly prices: Professional = 1900 cents, Team = 4900 cents per seat.

#### Tests to Write FIRST (RED phase)
```csharp
[Fact]
public async Task CreateCheckoutSession_returns_deterministic_url_shape() { ... }

[Fact]
public void ComputeProration_upgrade_5_to_8_team_seats_includes_positive_net() { ... }

[Fact]
public void ComputeProration_seat_decrease_returns_nonpositive_net() { ... }
```

#### Impact on Existing Tests
- None.

---

### Step 4: `SubscriptionsService` + endpoints
**Rationale:** Add new feature surface. Tests can assert behaviour without touching
existing organization tests. One touch point in `Program.cs` (add group map).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Add `DbSet<Subscription> Subscriptions` + config |
| `src/ApiTool.Backend/Subscriptions/SubscriptionsService.cs` | create | Business logic |
| `src/ApiTool.Backend/Subscriptions/SubscriptionsEndpoints.cs` | create | `/api/v1/subscriptions*` |
| `src/ApiTool.Backend/Subscriptions/SubscriptionDto.cs` | create | Wire model |
| `src/ApiTool.Backend/Subscriptions/SubscriptionError.cs` | create | Error enum |
| `src/ApiTool.Backend/Subscriptions/CheckoutRequest.cs` | create | POST /checkout body |
| `src/ApiTool.Backend/Subscriptions/UpdateSubscriptionRequest.cs` | create | PATCH body |
| `src/ApiTool.Backend/Subscriptions/CancelSubscriptionRequest.cs` | create | DELETE body |
| `src/ApiTool.Backend/Subscriptions/PortalRequest.cs` | create | POST /portal body |
| `src/ApiTool.Backend/Program.cs` | modify | Register service + map endpoints |
| `src/ApiTool.Backend.Tests/Subscriptions/SubscriptionsEndpointsTests.cs` | create | Endpoint behaviour |
| `src/ApiTool.Backend.Tests/Subscriptions/SubscriptionsServiceTests.cs` | create | Service rules |

#### Endpoints (from spec lines 6738–6891)
- `POST /api/v1/subscriptions/checkout` → 200 `{checkout_url, session_id}`
- `GET /api/v1/subscriptions` → 200 `{subscription|null, tier}`
- `PATCH /api/v1/subscriptions/{id}` → 200 `{subscription, proration}`
- `DELETE /api/v1/subscriptions/{id}` → 200 `{subscription, message}`
- `POST /api/v1/subscriptions/{id}/reactivate` → 200 `{subscription}`
- `POST /api/v1/subscriptions/portal` → 200 `{portal_url}`

#### Service signature
```csharp
public sealed class SubscriptionsService(AppDbContext db, IStripeGateway stripe, TimeProvider clock) {
    Task<(string checkoutUrl, string sessionId, SubscriptionError err, string? msg)>
        CreateCheckoutAsync(Guid userId, Guid orgId, SubscriptionTier tier, string interval,
                            int seatCount, string successUrl, string cancelUrl, CancellationToken ct);
    Task<SubscriptionDto?> GetForUserAsync(Guid userId, Guid orgId, CancellationToken ct);
    Task<(SubscriptionDto? sub, ProrationResult? proration, SubscriptionError err, string? msg)>
        UpdateAsync(Guid userId, Guid subId, SubscriptionTier? tier, int? seatCount, string? interval, CancellationToken ct);
    Task<(SubscriptionDto? sub, SubscriptionError err)> CancelAsync(Guid userId, Guid subId, string? reason, CancellationToken ct);
    Task<(SubscriptionDto? sub, SubscriptionError err)> ReactivateAsync(Guid userId, Guid subId, CancellationToken ct);
    Task<(string portalUrl, SubscriptionError err)> CreatePortalAsync(Guid userId, Guid orgId, string returnUrl, CancellationToken ct);
}
```

#### Tests to Write FIRST
```csharp
public sealed class SubscriptionsEndpointsTests : IAsyncLifetime {
    [Fact] public async Task Checkout_returns_200_with_checkout_url_for_owner() { ... }
    [Fact] public async Task Checkout_returns_403_for_non_owner() { ... }
    [Fact] public async Task Get_returns_null_subscription_and_tier_free_when_no_sub() { ... }
    [Fact] public async Task Get_returns_active_subscription_after_checkout_and_activation() { ... }
    [Fact] public async Task Patch_increases_seats_and_returns_proration_block() { ... }
    [Fact] public async Task Patch_with_downgrade_below_active_members_returns_403_downgrade_blocked() { ... }
    [Fact] public async Task Delete_marks_cancel_at_period_end() { ... }
    [Fact] public async Task Portal_returns_portal_url() { ... }
}

public sealed class SubscriptionsServiceTests {
    [Fact] public async Task Seat_count_equals_members_plus_pending_invitations() { ... }
}
```

Helper in test class: seed an active subscription row directly via `AppDbContext` to avoid
going through Stripe webhook flow (out of scope for M4-010).

#### Impact on Existing Tests
- `OrganizationServiceTests.Create_inserts_org_member_and_returns_owner_role` asserts
  `dto.SeatLimit.Should().Be(TeamTierDefaultSeatLimit)` (= 10). Now that SeatLimit is
  sourced from an optional `Subscription`, a fresh org with no subscription should
  return `SeatLimit = 1` (Free tier owner-only). **Action:** Update the test to assert
  `SeatLimit == 1` and replace the public constant name with `FreeTierSeatLimit = 1`
  (keep `TeamTierDefaultSeatLimit` if referenced elsewhere).
- `OrganizationsEndpointsTests.Post_creates_org_and_returns_201_with_owner_role_and_default_seat_counts`
  asserts `seat_limit == 10`. **Action:** Update to `seat_limit == 1`.

---

### Step 5: `InvitationsService` + endpoints
**Rationale:** Depends on Step 2's entity extensions and on the seat counter. Independent
of subscription endpoints.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Invitations/InvitationsService.cs` | create | Business logic |
| `src/ApiTool.Backend/Invitations/InvitationsEndpoints.cs` | create | Endpoint registrations |
| `src/ApiTool.Backend/Invitations/InvitationDto.cs` | create | Wire model |
| `src/ApiTool.Backend/Invitations/InvitationId.cs` | create | `inv_<hex>` helper |
| `src/ApiTool.Backend/Invitations/InvitationError.cs` | create | Error enum |
| `src/ApiTool.Backend/Invitations/CreateInvitationRequest.cs` | create | POST body |
| `src/ApiTool.Backend/Invitations/AcceptInvitationRequest.cs` | create | POST /accept body |
| `src/ApiTool.Backend/Invitations/InvitationTokenGenerator.cs` | create | 48-byte random → base64url + SHA-256 hash |
| `src/ApiTool.Backend/Program.cs` | modify | Register service + map endpoints |
| `src/ApiTool.Backend.Tests/Invitations/InvitationsEndpointsTests.cs` | create | Endpoint behaviour |
| `src/ApiTool.Backend.Tests/Invitations/InvitationsServiceTests.cs` | create | Service rules incl. seat counting |

#### Endpoints (from spec 7215–7234)
- `POST /api/v1/organizations/{id}/invitations` → 201
- `GET /api/v1/organizations/{id}/invitations` → 200
- `POST /api/v1/organizations/{id}/invitations/{inviteId}/resend` → 200
- `DELETE /api/v1/organizations/{id}/invitations/{inviteId}` → 200
- `POST /api/v1/invitations/accept` → 200 (Bearer auth; token in body)

#### Service signature
```csharp
public sealed class InvitationsService(AppDbContext db, TimeProvider clock) {
    const int ExpiryDays = 7;
    const int MaxResends = 3;
    const int ResendCooldownHours = 24;

    Task<(InvitationDto? dto, string? rawToken, InvitationError err, string? msg)>
        CreateAsync(Guid inviterUserId, Guid orgId, string email, OrgRole role, CancellationToken ct);

    Task<(IReadOnlyList<InvitationDto> invitations, InvitationError err)>
        ListPendingAsync(Guid userId, Guid orgId, CancellationToken ct);

    Task<(InvitationDto? dto, InvitationError err, string? msg)>
        ResendAsync(Guid userId, Guid orgId, Guid invitationId, CancellationToken ct);

    Task<InvitationError> RevokeAsync(Guid userId, Guid orgId, Guid invitationId, CancellationToken ct);

    Task<(InvitationDto? dto, OrgRole? role, InvitationError err, string? msg)>
        AcceptAsync(Guid acceptingUserId, string rawToken, CancellationToken ct);

    Task<int> CountSeatsAsync(Guid orgId, DateTime now, CancellationToken ct);
}
```

Seat count = `members.Count(org) + invitations.Count(org, AcceptedAt=null AND RevokedAt=null AND ExpiresAt > now)`.

Errors covered: `seat_limit_reached` (409), `already_member` (409), `invitation_pending`
(409), `invitation_expired` (410), `resend_limit_reached` (429), `resend_cooldown` (429),
`permission_denied` (403), `organization_not_found` (404), `invitation_not_found` (404).

#### Tests to Write FIRST
```csharp
public sealed class InvitationsEndpointsTests : IAsyncLifetime {
    [Fact] public async Task Post_creates_invitation_and_returns_201_with_email_and_role() { ... }
    [Fact] public async Task Post_writes_member_invited_audit_row() { ... }
    [Fact] public async Task Post_duplicate_email_while_pending_returns_409_invitation_pending() { ... }
    [Fact] public async Task Post_beyond_seat_limit_returns_409_seat_limit_reached() { ... }
    [Fact] public async Task Post_as_member_returns_403_permission_denied() { ... }
    [Fact] public async Task Accept_with_valid_token_creates_member_and_marks_accepted() { ... }
    [Fact] public async Task Accept_with_expired_token_returns_410_invitation_expired() { ... }
    [Fact] public async Task Resend_beyond_3_sends_returns_429_resend_limit_reached() { ... }
    [Fact] public async Task Revoke_marks_invitation_revoked_and_frees_seat() { ... }
    [Fact] public async Task List_returns_only_pending_invitations_for_org() { ... }
}

public sealed class InvitationsServiceTests {
    [Fact] public async Task CountSeatsAsync_equals_members_plus_pending_invitations() { ... }
    [Fact] public async Task CountSeatsAsync_excludes_expired_and_revoked_invitations() { ... }
}
```

#### Impact on Existing Tests
- None (invitations were never exercised by existing tests; only the schema is touched).

---

### Step 6: `MembersService` + endpoints (minimal slice)
**Rationale:** Completes the endpoint table (so Swagger lists 15 endpoints per DoD). Only
list/remove/patch are exposed — transfer/leave are stubs returning 501-ish pattern is not
allowed (always-runnable). Instead, we implement `GET /members`, `DELETE /members/{id}`,
`PATCH /members/{id}` fully. `POST /transfer` and `POST /leave` are deferred to a follow-up
task (M4-011 or M4-012) — noted as a risk below; DoD accommodates "all 15 endpoints" so we
must implement them. Decision: implement all 15 endpoints at minimum viable depth.

Members endpoints implemented:
- `GET /api/v1/organizations/{id}/members` → 200 list of members
- `DELETE /api/v1/organizations/{id}/members/{memberId}` → 200 (admin/owner, not self if owner)
- `PATCH /api/v1/organizations/{id}/members/{memberId}` → 200 (owner only, change role)
- `POST /api/v1/organizations/{id}/transfer` → 200 (owner only)
- `POST /api/v1/organizations/{id}/leave` → 200 (non-owner members)
- `PATCH /api/v1/organizations/{id}` → 200 (owner/admin, updates name/settings)
- `DELETE /api/v1/organizations/{id}` → 200 (owner only, sets status=pending_deletion)
- `POST /api/v1/organizations/{id}/cancel-deletion` → 200 (owner only)

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Organizations/MembersService.cs` | create | Member list/remove/patch |
| `src/ApiTool.Backend/Organizations/MembersEndpoints.cs` | create | Members + transfer + leave + update/delete org |
| `src/ApiTool.Backend/Organizations/MemberDto.cs` | create | Wire model |
| `src/ApiTool.Backend/Organizations/MemberError.cs` | create | Error enum |
| `src/ApiTool.Backend/Organizations/UpdateOrganizationRequest.cs` | create | PATCH body |
| `src/ApiTool.Backend/Organizations/TransferOwnershipRequest.cs` | create | POST body |
| `src/ApiTool.Backend/Organizations/UpdateMemberRequest.cs` | create | PATCH body |
| `src/ApiTool.Backend/Program.cs` | modify | Register service + map endpoints |
| `src/ApiTool.Backend.Tests/Organizations/MembersEndpointsTests.cs` | create | Endpoint behaviour |

#### Tests to Write FIRST
```csharp
[Fact] public async Task Get_members_returns_owner_for_new_org() { ... }
[Fact] public async Task Patch_member_role_by_owner_changes_role() { ... }
[Fact] public async Task Patch_member_role_by_admin_returns_403_permission_denied() { ... }
[Fact] public async Task Delete_member_by_owner_removes_membership() { ... }
[Fact] public async Task Delete_member_cannot_remove_owner_returns_403() { ... }
[Fact] public async Task Transfer_to_admin_swaps_roles() { ... }
[Fact] public async Task Transfer_to_member_returns_422() { ... }
[Fact] public async Task Leave_by_owner_returns_403_owner_cannot_leave() { ... }
[Fact] public async Task Patch_org_name_by_owner_updates_and_writes_audit() { ... }
[Fact] public async Task Delete_org_marks_pending_deletion() { ... }
[Fact] public async Task Cancel_deletion_restores_active_status() { ... }
```

#### Impact on Existing Tests
- `OrganizationsEndpointsTests` does not touch these paths — no breakage.

---

### Step 7: EF migration `0005_billing_and_invitations`
**Rationale:** Migration must include every schema change from Steps 1/2/4. We author it
after the entity changes are settled so the generated SQL captures everything in one
migration. Do not split because SQLite cannot alter column names cleanly — a single
migration that recreates `organization_invitations` is the cleanest path.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Migrations/20260418xxxxxx_AddBillingAndInvitations.cs` | create (via `dotnet ef migrations add`) | Add subscriptions, extend invitations |
| `src/ApiTool.Backend/Migrations/20260418xxxxxx_AddBillingAndInvitations.Designer.cs` | create | auto |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modify | auto |

Migration steps inside:
1. Create `subscriptions` table (unique index on org_id).
2. Recreate `organization_invitations` with all new fields + indexes (sqlite-safe rebuild).
3. Add index on `(org_id, email_normalized)` and unique index on `token_hash`.

#### Tests to Write FIRST
None — migrations are verified by `TestDb.CreateOpen()` → `db.Database.MigrateAsync()`
succeeding. If it succeeds and tests pass, the migration is valid. We add a tiny schema
smoke test:

```csharp
// src/ApiTool.Backend.Tests/Subscriptions/SubscriptionsMigrationTests.cs
[Fact]
public async Task Migration_creates_subscriptions_table() {
    await using var scope = TestDb.CreateOpen();
    await scope.Db.Database.MigrateAsync();
    (await scope.Db.Subscriptions.CountAsync()).Should().Be(0);
}
```

#### Impact on Existing Tests
- `BackendFactory` uses `EnsureCreatedAsync` for InMemory DB — unaffected.
- Any test using `TestDb.CreateOpen` + `MigrateAsync` will pick up the new migration
  automatically. `OrganizationServiceTests` uses this pattern and will need to tolerate
  the new default SeatLimit (covered in Step 4).

---

### Step 8: Rate limiters, Program.cs wiring, and Swagger verification
**Rationale:** Final integration touching Program.cs once for all the new groups.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Program.cs` | modify | Register SubscriptionsService, InvitationsService, MembersService, StripeGateway, rate-limit policies, map endpoint groups |
| `src/ApiTool.Backend.Tests/Subscriptions/SwaggerSurfaceTests.cs` | create | Asserts Swagger lists the spec's 15 endpoints |

#### Wiring (key diff)
```csharp
// Stripe gateway selection
builder.Services.Configure<StripeOptions>(builder.Configuration.GetSection("Stripe"));
var stripeMode = builder.Configuration["Stripe:Mode"] ?? "fake";
if (stripeMode == "live")
    builder.Services.AddSingleton<IStripeGateway, StripeGateway>();
else
    builder.Services.AddSingleton<IStripeGateway, FakeStripeGateway>();

builder.Services.AddScoped<SubscriptionsService>();
builder.Services.AddScoped<InvitationsService>();
builder.Services.AddScoped<MembersService>();

// ... rate-limiter policies (inside the existing AddRateLimiter block for non-Testing env):
options.AddPolicy("checkout-create", ctx =>
    RateLimitPartition.GetFixedWindowLimiter(
        partitionKey: ctx.User.FindFirstValue(JwtRegisteredClaimNames.Sub) ?? "anonymous",
        factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 5, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));
// ... plus the 6 other policies listed in Decision 9.

// Endpoint registration
app.MapSubscriptionsEndpoints();
app.MapInvitationsEndpoints();
app.MapMembersEndpoints();
```

In the Testing branch of AddRateLimiter, add matching no-op policies so
`.RequireRateLimiting("checkout-create")` etc. binds cleanly under the test host.

#### Swagger test
Use `WebApplicationFactory.CreateClient().GetAsync("/swagger/v1/swagger.json")` and assert
the JSON document has path entries for all 15 endpoints from spec 7215–7234 plus the 6
from 6738–6891.

```csharp
var json = await client.GetStringAsync("/swagger/v1/swagger.json");
using var doc = JsonDocument.Parse(json);
var paths = doc.RootElement.GetProperty("paths");
paths.TryGetProperty("/api/v1/subscriptions/checkout", out _).Should().BeTrue();
// ... one check per spec'd endpoint
```

#### Impact on Existing Tests
- Existing rate-limiter policies remain (results-ingest). No breakage.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `OrganizationServiceTests.cs` | `Create_inserts_org_member_and_returns_owner_role` | changes | Update SeatLimit expectation from `TeamTierDefaultSeatLimit` (10) to `FreeTierSeatLimit` (1) |
| `OrganizationsEndpointsTests.cs` | `Post_creates_org_and_returns_201_with_owner_role_and_default_seat_counts` | changes | `seat_limit` expectation 10 → 1 |
| `OrganizationsEndpointsTests.cs` | `Get_detail_returns_200_with_role_and_seats_for_member` | unaffected | — |
| All other existing tests | — | none | — |
| `SubscriptionIdTests.cs` | new | add | Step 1 |
| `FakeStripeGatewayTests.cs` | new | add | Step 3 |
| `SubscriptionsMigrationTests.cs` | new | add | Step 7 |
| `SubscriptionsServiceTests.cs` | new (1–2 tests) | add | Step 4 |
| `SubscriptionsEndpointsTests.cs` | new (~8 tests) | add | Step 4 |
| `InvitationsServiceTests.cs` | new (~2 tests) | add | Step 5 |
| `InvitationsEndpointsTests.cs` | new (~10 tests) | add | Step 5 |
| `MembersEndpointsTests.cs` | new (~11 tests) | add | Step 6 |
| `SwaggerSurfaceTests.cs` | new (1 test) | add | Step 8 |

Total new tests: ~35 (comfortably above the DoD threshold of 15).

## Risks and Edge Cases

- **Risk — SQLite column rename:** EF Core on SQLite handles column renames by table
  rebuild; this must preserve row data. **Mitigation:** No production data exists for
  invitations yet, and the dev/test DBs are recreated from migrations. Verified by the
  migration smoke test in Step 7.
- **Risk — Seat count race conditions:** Two concurrent invites could both pass the
  seat-limit check and race. **Mitigation:** Single-connection SQLite serialises writes
  in tests; production will add a `BEGIN IMMEDIATE`-equivalent through EF transaction
  scope (`db.Database.BeginTransactionAsync(IsolationLevel.Serializable)`) around
  `CreateInvitation`. Added note in service comment; not separately tested.
- **Edge case — Accepting invitation when user already member:** Return
  `already_member` (409) and mark invitation accepted to avoid stuck pending rows.
- **Edge case — Accepting another org's invitation:** Invitation email must match or
  normalise to the authenticated user's email; mismatches return `permission_denied` (403).
- **Edge case — Empty `on_events` during invitation (N/A) vs. role validation:** `role`
  must be `admin` or `member` (never `owner`) — enforced by `TryParse<OrgRole>` + check.
- **Risk — Observable step uses fresh token user (no org):** The observable pipeline in
  the task YAML uses `test-token.sh owner@example.com` which mints a random user id and
  then greps organizations. The user has no orgs on a fresh DB. **Mitigation:** Updated
  observable interpretation — the plan assumes the verifier runs
  `./scripts/seed-test-data.sh` first (matching the existing M4-009 flow) or creates an
  org interactively. Leave YAML untouched (it's the spec); document the prerequisite in
  the plan's Verification section below.
- **Risk — Program.cs diff size:** Many new registrations. **Mitigation:** Keep each new
  subsystem self-contained with a `MapXxxEndpoints` extension method that mirrors the
  existing `MapOrganizationsEndpoints` style.

## Verification

```bash
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Subscriptions|FullyQualifiedName~Invitations|FullyQualifiedName~Seats|FullyQualifiedName~Members"
# Expected: Passed: >= 30, Failed: 0
```

Observable verification (requires seeded test data):

```bash
dotnet run --project src/ApiTool.Backend &
sleep 2
./scripts/seed-test-data.sh   # creates owner + acme org
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')

curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"tier":"team","seat_count":5,"success_url":"http://localhost/ok","cancel_url":"http://localhost/no"}' \
  http://localhost:5000/api/v1/subscriptions/checkout
# Expected 200 with "checkout_url":"https://checkout.stripe.test/cs_..."

curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email":"newuser@example.com","role":"member"}' \
  "http://localhost:5000/api/v1/organizations/$ORG/invitations"
# Expected 201 with "email":"newuser@example.com","role":"member"
```

Swagger listing verification:
```bash
curl -sS http://localhost:5000/swagger/v1/swagger.json | jq '.paths | keys[]' | grep -E 'subscriptions|invitations|members'
# Expected 15+ matches spanning the 6 subscription endpoints and 9 org/member endpoints.
```
