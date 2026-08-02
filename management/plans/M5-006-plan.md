# Implementation Plan: M5-006

## Overview
Add custom roles and granular permissions to the backend: new `organization_custom_roles`
table, four CRUD endpoints under `/api/v1/organizations/{id}/roles`, a permission
catalogue, a `role_id` FK column on `organization_members`, and a `RoleResolver` that
produces an effective permission set (built-in role permissions ∪ custom role permissions
∪ direct overrides). Permission-gated endpoints check the resolver instead of matching on
`OrgRole` directly, exposing custom roles through the existing member-role assignment flow.

## Task Details
- **ID:** M5-006
- **Title:** Backend: custom roles and granular permissions
- **Phase:** M5: Enterprise Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task   | Title                                          | Status |
|--------|------------------------------------------------|--------|
| M4-003 | Backend: organization + seat RBAC data model   | done   |

## Architectural Decisions

These decisions resolve open questions up front to keep `/execute` unblocked:

1. **Permission catalogue lives in a single static C# type.** `Permissions` is a static
   class that exposes (a) string constants for every permission key in the spec matrix
   (§Roles and Permissions, lines 7085–7115 of `docs/SPECIFICATION.md`), (b) an
   immutable `IReadOnlySet<string> All` for validation, and (c) three immutable
   `IReadOnlySet<string>` sets (`BuiltInOwner`, `BuiltInAdmin`, `BuiltInMember`) that
   mirror the spec matrix checkmarks. No enum — permission keys are dotted strings on
   the wire (`results.upload`), and C# enum casing hurts more than it helps here.
2. **Built-in roles are virtual, not DB rows.** The `GET /roles` endpoint synthesizes
   three hardcoded entries (`owner`, `admin`, `member`) with `is_builtin=true` and
   prepends them to the DB-backed custom roles. This keeps the seat-counting
   and ownership-transfer logic untouched; `OrganizationMember.Role` still decides
   whether a user is an owner/admin/member; `RoleId` is an *additional* pointer to a
   custom role whose permissions *replace* the built-in defaults when set.
   The observable explicitly requires only `role_name_taken` checks within an org,
   so custom role names collide only with *other custom roles*, not with built-ins.
   (We still reject `owner`, `admin`, `member` as custom names to avoid confusion.)
3. **Effective permissions = custom role ∪ direct overrides (when `RoleId` set),
   otherwise the built-in role's set ∪ direct overrides.** The scope says "union of
   role permissions + direct overrides" — direct overrides remain additive on top of
   whichever role the member holds. We keep the existing
   `OrganizationMember.PermissionsJson` column as the direct-override list and
   continue to parse it as a JSON string array.
4. **DELETE blocks when members reference the custom role.** Observable #6 requires
   `409 role_in_use` with `member_count`. We count
   `OrganizationMembers.Where(m => m.RoleId == roleId)` and return 409 if > 0. No
   cascade, no "force" flag in this slice.
5. **PATCH /members/{id} accepts `role_id`.** The existing endpoint currently takes
   only `{ "role": "admin" }`. We extend `UpdateMemberRequest` with optional
   `RoleId string?` (wire field `role_id`). When present, we set `RoleId` FK on the
   membership; `Role` (built-in) is left untouched. When absent, the endpoint works
   exactly as before. The audit event emitted is `role.changed` with payload
   `{ role_id: "role_..." }` per observable #8. We keep the existing
   `member.role_changed` event for built-in-role changes (separate code path).
6. **Permission gating (observable #5).** We introduce a `PermissionChecker` service
   with `HasPermissionAsync(Guid userId, Guid orgId, string permissionKey, CancellationToken)`.
   The checker resolves the member, derives effective permissions, and returns
   `bool`. We demonstrate integration in exactly one place for this slice: the
   existing `ResultsEndpoints` `POST /results` endpoint already has a non-member
   403; we add a secondary gate that calls `PermissionChecker.HasPermissionAsync(…,
   "results.upload", …)` and returns `permission_denied` on false. That satisfies
   observable #5 without rewiring every feature in the codebase. Broader RBAC
   refactor is out of scope (existing behaviors keep their built-in-role gates).
7. **Role id format follows the project convention.** `role_<32-hex-chars>`. We add
   a `RoleId` helper in `Rbac/CustomRoles/RoleId.cs` mirroring `OrgId`.
8. **Tier gating is out of scope at the backend API level.** SSO and audit-log
   endpoints from M5-001..M5-005 do not gate on `SubscriptionTier.Enterprise` on
   the backend; the web UI performs the check. We follow that same pattern: the
   custom-roles endpoints accept calls from any tier and rely on the frontend
   to hide them for non-enterprise orgs. The spec calls it out as an Enterprise
   feature but the behaviors test suite does not assert a tier check, so we
   keep parity with existing practice. (Documented here so reviewers know it's
   deliberate.)
9. **Permissions JSON in the role row uses `TEXT` storage with a JSON-array value.**
   SQLite doesn't have a native JSONB column; the codebase already uses
   `TEXT NOT NULL` columns for JSON (`OrganizationMember.permissions`,
   `organizations.settings`). Task wording says "JSONB" — we follow the existing
   codebase idiom rather than the ticket verbatim.
10. **Created-by/created-at capture uses the injected `TimeProvider`** (matching
    every other service in the codebase — `OrganizationService`, `MembersService`).

## Implementation Steps

Steps are ordered **smallest blast radius first**: pure value types → pure catalogue →
entity/migration → resolver → service → endpoint → integration demonstration →
wiring. Each step leaves the tree green.

### Step 1: Permission catalogue (pure, no dependencies)
**Rationale:** Zero coupling. A constants class + three static sets. Everything else in
the slice references these keys. Trivial to unit-test in isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Rbac/Permissions.cs` | create | String constants + `All` + built-in sets. |
| `src/ApiTool.Backend.Tests/Rbac/PermissionsTests.cs` | create | Asserts spec matrix. |

#### New Code (shape only)

```csharp
namespace ApiTool.Backend.Rbac;

/// <summary>Canonical catalogue of organization permission keys.</summary>
public static class Permissions
{
    public const string MembersInvite = "members.invite";
    public const string MembersRemove = "members.remove";
    public const string MembersView = "members.view";
    public const string RolesChange = "roles.change";
    public const string RolesView = "roles.view";
    public const string BillingManage = "billing.manage";
    public const string BillingView = "billing.view";
    public const string SeatsAdd = "seats.add";
    public const string SeatsRemove = "seats.remove";
    public const string OrgSettingsManage = "org.settings.manage";
    public const string OrgSettingsView = "org.settings.view";
    public const string OrgDelete = "org.delete";
    public const string OrgTransfer = "org.transfer";
    public const string TokensCreate = "tokens.create";
    public const string TokensRevoke = "tokens.revoke";
    public const string TokensView = "tokens.view";
    public const string ResultsView = "results.view";
    public const string ResultsUpload = "results.upload";
    public const string ResultsDelete = "results.delete";
    public const string VaultConfigManage = "vault_config.manage";
    public const string VaultConfigView = "vault_config.view";
    public const string DashboardView = "dashboard.view";
    public const string DashboardExport = "dashboard.export";

    public static readonly IReadOnlySet<string> All;
    public static readonly IReadOnlySet<string> BuiltInOwner;
    public static readonly IReadOnlySet<string> BuiltInAdmin;
    public static readonly IReadOnlySet<string> BuiltInMember;

    static Permissions() { /* populate from the spec matrix */ }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class PermissionsTests
{
    [Theory]
    [InlineData("members.invite")]
    [InlineData("results.upload")]
    [InlineData("dashboard.view")]
    public void All_contains_spec_keys(string key) =>
        Permissions.All.Should().Contain(key);

    [Fact]
    public void All_has_23_entries_matching_spec_matrix() =>
        Permissions.All.Should().HaveCount(23);

    [Fact]
    public void Owner_set_is_superset_of_admin_set() =>
        Permissions.BuiltInOwner.Should().BeSupersetOf(Permissions.BuiltInAdmin);

    [Fact]
    public void Admin_has_members_invite_but_member_does_not()
    {
        Permissions.BuiltInAdmin.Should().Contain(Permissions.MembersInvite);
        Permissions.BuiltInMember.Should().NotContain(Permissions.MembersInvite);
    }

    [Fact]
    public void Owner_only_permissions_absent_from_admin()
    {
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.RolesChange);
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.BillingManage);
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.OrgDelete);
    }

    [Fact]
    public void Member_has_view_only_permissions()
    {
        Permissions.BuiltInMember.Should().Contain(Permissions.ResultsView);
        Permissions.BuiltInMember.Should().Contain(Permissions.DashboardView);
        Permissions.BuiltInMember.Should().NotContain(Permissions.ResultsDelete);
    }
}
```

#### Impact on Existing Tests
- None. New file.

---

### Step 2: CustomRole entity + RoleId helper + EF mapping
**Rationale:** Adds the DB surface. Still no behaviour changes because the
endpoint doesn't exist yet. Pure data model + schema.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/CustomRole.cs` | create | Entity class. |
| `src/ApiTool.Backend/Data/Entities/OrganizationMember.cs` | modify | Add `Guid? RoleId`. |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Add `DbSet<CustomRole>` + mapping. |
| `src/ApiTool.Backend/Rbac/CustomRoles/RoleId.cs` | create | Wire-format helper. |
| `src/ApiTool.Backend.Tests/Rbac/RoleIdTests.cs` | create | Round-trip + parse tests. |

#### New Code (`CustomRole.cs`)

```csharp
namespace ApiTool.Backend.Data.Entities;

/// <summary>A custom permission role defined by an organization.</summary>
public sealed class CustomRole
{
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public string Name { get; set; } = string.Empty;
    /// <summary>JSON array of permission key strings.</summary>
    public string PermissionsJson { get; set; } = "[]";
    public DateTime CreatedAt { get; set; }
    public Guid CreatedBy { get; set; }
}
```

#### Change to `OrganizationMember.cs`

```csharp
// Add this property alongside the existing ones:
/// <summary>
/// Optional FK to an <c>organization_custom_roles</c> row. When set, the member's
/// effective permissions come from the custom role (plus <see cref="PermissionsJson"/>
/// overrides); when null, the built-in <see cref="Role"/> applies.
/// </summary>
public Guid? RoleId { get; set; }
```

#### Change to `AppDbContext.OnModelCreating`

```csharp
// Add new DbSet
public DbSet<CustomRole> OrganizationCustomRoles => Set<CustomRole>();

// In OnModelCreating, update the OrganizationMember block to include RoleId mapping:
e.Property(x => x.RoleId).HasColumnName("role_id").IsRequired(false);

// And add a new block:
b.Entity<CustomRole>(e =>
{
    e.ToTable("organization_custom_roles");
    e.HasKey(x => x.Id);
    e.Property(x => x.Name).HasMaxLength(100).IsRequired();
    e.Property(x => x.PermissionsJson).HasColumnName("permissions").IsRequired();
    e.HasIndex(x => new { x.OrgId, x.Name }).IsUnique();
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
    e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).OnDelete(DeleteBehavior.Restrict);
});
```

#### New Code (`Rbac/CustomRoles/RoleId.cs`)

```csharp
namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Wire-format helpers for custom role identifiers (<c>role_&lt;32-hex&gt;</c>).</summary>
public static class RoleId
{
    private const string Prefix = "role_";

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
public sealed class RoleIdTests
{
    [Fact]
    public void Round_trip_format_then_parse() {/* Format(Guid) → TryParse → Guid match */}

    [Theory]
    [InlineData("role_")]
    [InlineData("org_11111111111111111111111111111111")]
    [InlineData("")]
    [InlineData(null)]
    public void TryParse_rejects_invalid(string? v) {/* returns false, id=Guid.Empty */}
}
```

#### Impact on Existing Tests
- None — `RoleId` is new, `OrganizationMember.RoleId` defaults to null, and existing
  EF queries don't touch the column.

---

### Step 3: EF migration `0007_custom_roles`
**Rationale:** Generate the actual SQL. Safe to apply independently — no code path
reads the new column yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Migrations/<timestamp>_AddCustomRoles.cs` | create | EF migration. |
| `src/ApiTool.Backend/Migrations/<timestamp>_AddCustomRoles.Designer.cs` | create | EF-generated snapshot. |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modify | EF-generated update. |

File name format matches the existing pattern: `20260419010000_AddCustomRoles.cs`
(spec label "0007" is a human-friendly tag; EF uses timestamps). The plan labels
this migration `AddCustomRoles` matching other migrations
(`AddSsoCredentials`, `AddAuditLogDetails`).

#### Migration shape

```csharp
public partial class AddCustomRoles : Migration
{
    protected override void Up(MigrationBuilder m)
    {
        m.AddColumn<Guid>(name: "role_id", table: "organization_members",
            type: "TEXT", nullable: true);

        m.CreateTable(
            name: "organization_custom_roles",
            columns: t => new
            {
                Id = t.Column<Guid>(type: "TEXT", nullable: false),
                OrgId = t.Column<Guid>(type: "TEXT", nullable: false),
                Name = t.Column<string>(type: "TEXT", maxLength: 100, nullable: false),
                permissions = t.Column<string>(type: "TEXT", nullable: false),
                CreatedAt = t.Column<DateTime>(type: "TEXT", nullable: false),
                CreatedBy = t.Column<Guid>(type: "TEXT", nullable: false),
            },
            constraints: t =>
            {
                t.PrimaryKey("PK_organization_custom_roles", x => x.Id);
                t.ForeignKey(name: "FK_organization_custom_roles_organizations_OrgId",
                    column: x => x.OrgId, principalTable: "organizations",
                    principalColumn: "Id", onDelete: ReferentialAction.Cascade);
                t.ForeignKey(name: "FK_organization_custom_roles_users_CreatedBy",
                    column: x => x.CreatedBy, principalTable: "users",
                    principalColumn: "Id", onDelete: ReferentialAction.Restrict);
            });

        m.CreateIndex(name: "IX_organization_custom_roles_OrgId_Name",
            table: "organization_custom_roles",
            columns: new[] { "OrgId", "Name" }, unique: true);

        m.CreateIndex(name: "IX_organization_custom_roles_CreatedBy",
            table: "organization_custom_roles",
            column: "CreatedBy");
    }

    protected override void Down(MigrationBuilder m)
    {
        m.DropTable(name: "organization_custom_roles");
        m.DropColumn(name: "role_id", table: "organization_members");
    }
}
```

#### Tests to Write FIRST (RED phase)
No direct migration test — we run `db.Database.EnsureCreatedAsync()` in the backend
test collection, which picks up the new entity mapping automatically. Schema
correctness is covered by the endpoint tests (Step 6) and by a smoke
`dotnet ef database update` check in the `./smoke/run.sh` equivalent.

#### Impact on Existing Tests
- EF InMemory provider ignores migrations; `EnsureCreatedAsync` regenerates schema
  from the model. All existing tests remain green.

---

### Step 4: `RoleResolver` — effective permission set
**Rationale:** Pure logic in a single async method; separates "how do I compute the
effective permissions" from "how do I query the DB". Unit-testable end-to-end via a
seeded InMemory DbContext.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Rbac/CustomRoles/RoleResolver.cs` | create | Resolver service. |
| `src/ApiTool.Backend.Tests/Rbac/RoleResolverTests.cs` | create | Union + override tests. |

#### New Code (signature)

```csharp
namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>
/// Computes the effective permission set for a given organization member.
/// Returns an empty set if the user is not a member of the org.
/// </summary>
public sealed class RoleResolver(AppDbContext db)
{
    public async Task<IReadOnlySet<string>> GetEffectivePermissionsAsync(
        Guid userId, Guid orgId, CancellationToken ct)
    { /* see below */ }

    public async Task<bool> HasPermissionAsync(
        Guid userId, Guid orgId, string permissionKey, CancellationToken ct)
    {
        var perms = await GetEffectivePermissionsAsync(userId, orgId, ct);
        return perms.Contains(permissionKey);
    }
}
```

Algorithm:
1. Look up the `OrganizationMember` row; if null → return empty set.
2. If `RoleId` is non-null and the referenced `CustomRole` exists in the same org →
   deserialize `PermissionsJson` → base set.
3. Otherwise → pick built-in set from `Permissions.BuiltInOwner/Admin/Member` by the
   member's `Role`.
4. Union with direct overrides parsed from `OrganizationMember.PermissionsJson`.
5. Ignore unknown permission keys silently (the POST endpoint validates on write;
   reads must be forgiving of stale keys if the catalogue ever shrinks).

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class RoleResolverTests : IAsyncLifetime
{
    // Uses a private in-memory AppDbContext seeded per test.

    public static IEnumerable<object[]> EffectivePermissionsCases()
    {
        yield return new object[] { /* built-in owner → BuiltInOwner */ };
        yield return new object[] { /* built-in admin → BuiltInAdmin */ };
        yield return new object[] { /* built-in member → BuiltInMember */ };
        yield return new object[] { /* custom role with [results.upload, dashboard.view]
                                       → exactly those */ };
        yield return new object[] { /* custom role + override [members.view]
                                       → union */ };
        yield return new object[] { /* RoleId points to deleted custom role
                                       → falls back to built-in */ };
        yield return new object[] { /* non-member user → empty set */ };
    }

    [Theory, MemberData(nameof(EffectivePermissionsCases))]
    public async Task GetEffectivePermissions_matches_expected(/* ... */) { /* ... */ }

    [Fact]
    public async Task HasPermission_returns_true_for_member_of_custom_role_with_key() { /* ... */ }

    [Fact]
    public async Task HasPermission_returns_false_for_key_not_in_custom_role() { /* ... */ }
}
```

#### Impact on Existing Tests
- None — new class, new tests, no existing call sites.

---

### Step 5: `CustomRolesService` + errors + DTOs
**Rationale:** Business logic for the four CRUD operations. Mirrors the
existing `MembersService`/`OrganizationService` shape (constructor-injected
`AppDbContext`, `TimeProvider`, `IAuditWriter`; tuple return with enum error).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRolesService.cs` | create | Service. |
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRoleDto.cs` | create | Wire DTO. |
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRoleError.cs` | create | Sentinel enum. |
| `src/ApiTool.Backend/Rbac/CustomRoles/CreateRoleRequest.cs` | create | Request body. |
| `src/ApiTool.Backend.Tests/Rbac/CustomRolesServiceTests.cs` | create | Unit tests. |

#### Signatures

```csharp
public sealed record CustomRoleDto(
    string Id,
    string Name,
    IReadOnlyList<string> Permissions,
    bool IsBuiltin,
    DateTime? CreatedAt);

public sealed record CreateRoleRequest(string Name, IReadOnlyList<string> Permissions);

public enum CustomRoleError
{
    None,
    PermissionDenied,
    RoleNameTaken,
    InvalidPermission,      // one or more unknown keys
    InvalidName,            // empty, too long, or collides with a built-in name
    RoleNotFound,
    RoleInUse,              // DELETE blocked because members reference it
    OrganizationNotFound,
}

public sealed class CustomRolesService(AppDbContext db, TimeProvider clock, IAuditWriter audit)
{
    public Task<(IReadOnlyList<CustomRoleDto> roles, CustomRoleError err)>
        ListAsync(Guid userId, Guid orgId, CancellationToken ct);

    public Task<(CustomRoleDto? dto, CustomRoleError err, string? message)>
        CreateAsync(Guid userId, Guid orgId, CreateRoleRequest req, CancellationToken ct);

    public Task<(CustomRoleError err, int memberCount)>
        DeleteAsync(Guid userId, Guid orgId, Guid roleId, CancellationToken ct);
}
```

Behavior:
- **ListAsync** returns the three built-in DTOs (`is_builtin=true`, `created_at=null`,
  permissions from the static sets) followed by DB custom roles
  (`is_builtin=false`, `created_at` set), newest-first by `CreatedAt`.
  Members, admins, and owners can all read.
- **CreateAsync** requires the requestor's role to be `Owner` (observable #7 —
  "non-owner calls POST /roles ... 403 permission_denied"). Admins cannot create.
  Validates: trimmed name non-empty, ≤ 100 chars, not `owner`/`admin`/`member`;
  every permission key in `Permissions.All`; no existing custom role with the same
  `(orgId, name)`. Emits audit event `role.created` with payload
  `{ name, permissions }`.
- **DeleteAsync** requires Owner. Counts members with `RoleId == roleId` in the org;
  if > 0 → returns `RoleInUse` with the count. Otherwise soft nothing (hard delete
  via `Remove`), emits `role.deleted` audit event.

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class CustomRolesServiceTests : IAsyncLifetime
{
    // Each test seeds a private AppDbContext via DbContextOptionsBuilder.UseInMemoryDatabase.

    [Fact]
    public async Task Create_with_valid_permissions_persists_and_returns_dto() { /* ... */ }

    [Fact]
    public async Task Create_rejects_unknown_permission_key_with_InvalidPermission() { /* ... */ }

    [Fact]
    public async Task Create_rejects_empty_name_with_InvalidName() { /* ... */ }

    [Fact]
    public async Task Create_rejects_built_in_name_with_InvalidName() { /* cases owner/admin/member */ }

    [Fact]
    public async Task Create_rejects_duplicate_name_with_RoleNameTaken() { /* ... */ }

    [Fact]
    public async Task Create_by_admin_returns_PermissionDenied() { /* ... */ }

    [Fact]
    public async Task Create_by_member_returns_PermissionDenied() { /* ... */ }

    [Fact]
    public async Task Create_by_non_member_returns_PermissionDenied() { /* ... */ }

    [Fact]
    public async Task List_contains_three_built_ins_plus_custom() { /* asserts is_builtin flags */ }

    [Fact]
    public async Task List_by_member_succeeds() { /* read permission */ }

    [Fact]
    public async Task Delete_blocks_when_members_reference_role_with_RoleInUse_count() { /* ... */ }

    [Fact]
    public async Task Delete_succeeds_when_no_members_reference_role() { /* ... */ }

    [Fact]
    public async Task Delete_by_admin_returns_PermissionDenied() { /* ... */ }

    [Fact]
    public async Task Create_emits_role_created_audit_event() { /* spy IAuditWriter */ }
}
```

#### Impact on Existing Tests
- None. New service, new tests.

---

### Step 6: `CustomRolesEndpoints` + registration
**Rationale:** Minimal-API wiring glues service errors to HTTP status codes.
Follows the `MembersEndpoints` / `AuditLogEndpoints` shape exactly.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRolesEndpoints.cs` | create | Route mapping. |
| `src/ApiTool.Backend/Program.cs` | modify | `MapCustomRolesEndpoints()` + DI. |
| `src/ApiTool.Backend.Tests/Rbac/CustomRolesEndpointsTests.cs` | create | Integration tests. |

#### Route shape

```
POST    /api/v1/organizations/{id}/roles              → 201 / 400 / 403 / 409
GET     /api/v1/organizations/{id}/roles              → 200 / 404
DELETE  /api/v1/organizations/{id}/roles/{roleId}     → 204 / 403 / 404 / 409
```

`PATCH /roles/{roleId}` is **out of scope** for this slice (not in the observable
or behaviors). We document the absence in the route summary comment so a future
task can add it without confusion.

#### Error mapping

| Error                      | HTTP | Body code                |
|----------------------------|------|--------------------------|
| `None`                     | 201/200/204 | —                 |
| `PermissionDenied`         | 403  | `permission_denied`      |
| `RoleNameTaken`            | 409  | `role_name_taken`        |
| `InvalidPermission`        | 400  | `invalid_permission` (with offending key in `field`) |
| `InvalidName`              | 400  | `invalid_name`           |
| `RoleNotFound`             | 404  | `role_not_found`         |
| `RoleInUse`                | 409  | `role_in_use` (body includes `member_count`) |
| `OrganizationNotFound`     | 404  | `organization_not_found` |

For `InvalidPermission`, the ErrorResponse's `Field` slot carries the bad key
(observable #3 — "offending key in the body"). For `RoleInUse`, we return a
bespoke body `{ code, message, member_count }` — we cannot squeeze it into the
generic `ErrorResponse` without adding a nullable int, so we emit an inline
anonymous record.

#### Tests to Write FIRST (RED phase)

The count of test methods here directly drives the observable's "Passed: >=12"
threshold. We target **15** — a safe margin above 12 and one per behavior plus
edge cases.

```csharp
// FullyQualifiedName~Rbac&FullyQualifiedName~CustomRoles
[Collection(BackendCollection.Name)]
public sealed class CustomRolesEndpointsTests : IAsyncLifetime
{
    // 1. POST happy path → 201, body.id matches role_... pattern
    // 2. POST + GET list → includes built-ins + custom with is_builtin flag
    // 3. POST duplicate name → 409 role_name_taken
    // 4. POST unknown permission key → 400 invalid_permission, field=<key>
    // 5. POST empty name → 400 invalid_name
    // 6. POST built-in name "owner"/"admin"/"member" → 400 invalid_name
    // 7. POST by admin → 403 permission_denied
    // 8. POST by non-member → 403 permission_denied  (or 404, see below)
    // 9. GET by member succeeds (read access)
    // 10. DELETE by owner succeeds when no members reference
    // 11. DELETE when members reference → 409 role_in_use with member_count
    // 12. DELETE non-existent role → 404 role_not_found
    // 13. DELETE by admin → 403 permission_denied
    // 14. Full assign+call flow — owner creates results-only role, assigns to member
    //     via PATCH /members, member POSTs to a results.upload-gated endpoint → 200;
    //     member POSTs to a members.invite-gated endpoint → 403 permission_denied
    //     (observable #5).
    // 15. PATCH /members/{id} with role_id writes role.changed audit row.
}
```

For non-member access (#8) we return `organization_not_found` (404) to match the
existing `MembersService.ListMembersAsync` convention that avoids org enumeration.
The test asserts exactly that.

Test #14 exercises `PermissionChecker` integration described in Step 8.

#### Impact on Existing Tests
- None before Step 7 & 8. The endpoint is new.

---

### Step 7: PATCH /members/{id} accepts `role_id`
**Rationale:** Observable #8 requires this field to flow through. Extends an
existing request shape — minimal change, but it ripples into `MembersService`
and its tests, so it lands late.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Organizations/UpdateMemberRequest.cs` | modify | Add optional `RoleId`. |
| `src/ApiTool.Backend/Organizations/MembersService.cs` | modify | Accept `roleId`, validate, emit `role.changed`. |
| `src/ApiTool.Backend/Organizations/MembersEndpoints.cs` | modify | Plumb the new field through. |
| `src/ApiTool.Backend.Tests/Organizations/MembersEndpointsTests.cs` | modify | Add PATCH-with-role_id test. |

#### Request diff

```csharp
// Before
public sealed record UpdateMemberRequest(string Role);

// After
public sealed record UpdateMemberRequest(string? Role = null, string? RoleId = null);
```

At least one of `role` / `role_id` must be present; both simultaneously is allowed
(built-in role + custom role id).

#### Service change

New signature alongside the existing:

```csharp
public async Task<(MemberDto? member, MemberError err)>
    UpdateMemberAsync(
        Guid requestorId,
        Guid orgId,
        Guid targetUserId,
        string? roleStr,
        Guid? roleId,
        CancellationToken ct)
```

We keep the old `UpdateMemberRoleAsync` as an instance method that forwards to
`UpdateMemberAsync(role, roleId: null)` to avoid rewriting every caller. When
`roleId` is set we also validate that it belongs to the same org, else return
`PermissionDenied` (keeps the error set small; we do not add new enum values yet).
The audit event emitted when `roleId` is assigned is `role.changed` with payload
`{ role_id: "role_..." }` per observable #8.

#### Tests to Write FIRST (RED phase)

```csharp
[Fact]
public async Task Patch_member_with_role_id_by_owner_sets_role_id_and_writes_audit()
{ /* ... */ }

[Fact]
public async Task Patch_member_with_role_id_for_other_org_returns_403()
{ /* ... */ }
```

#### Impact on Existing Tests
- `MembersEndpointsTests.Patch_member_role_by_owner_changes_role` → still passes
  (we use a nullable `Role` default, existing body `{ "role": "admin" }` still binds).
- Any external callers of `UpdateMemberRoleAsync` — the legacy method is
  preserved, so the call graph doesn't break.

---

### Step 8: `PermissionChecker` integration on `POST /results`
**Rationale:** Observable #5 requires a member with the custom role to succeed on
a `results.upload` endpoint. We need exactly one endpoint in the codebase to
consult the resolver. `POST /results` is the most natural demonstration.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Results/ResultsEndpoints.cs` | modify | Add resolver check before existing member gate. |
| `src/ApiTool.Backend/Results/ResultsService.cs` | modify (if gate lives in the service) — TBD after reading the file at execute time. |
| `src/ApiTool.Backend.Tests/Rbac/CustomRolesEndpointsTests.cs` | modify | Add the end-to-end test (#14 above). |

#### Logic change

Inside the results-upload handler, after resolving the `userId`:
```csharp
var resolver = ctx.RequestServices.GetRequiredService<RoleResolver>();
if (!await resolver.HasPermissionAsync(userId.Value, orgGuid, Permissions.ResultsUpload, ct))
    return HttpResults.Json(
        new ErrorResponse("permission_denied", "results.upload required."),
        statusCode: StatusCodes.Status403Forbidden);
```

This runs *before* the existing member-role check so that a member holding a
custom role with `results.upload` can still upload. Built-in members already
have `results.upload` per the spec matrix, so existing tests continue to pass.

#### Impact on Existing Tests
- Existing `ResultsEndpointsTests` use members and admins, both of which have
  `results.upload` in `BuiltInMember`/`BuiltInAdmin`. They continue to pass.
- The non-member 403 path is unchanged (resolver returns `false` for non-members
  because `GetEffectivePermissionsAsync` returns empty set).

---

### Step 9: DI registration + CHANGELOG entry
**Rationale:** Wires the new services into `Program.cs` and documents the
change. Trivial but required for the binary to run.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Program.cs` | modify | Add `AddScoped` for `RoleResolver` and `CustomRolesService`; call `MapCustomRolesEndpoints()`. |
| `CHANGELOG.md` | modify | Add an `[Unreleased]` bullet for M5-006. |

```csharp
// Program.cs additions
builder.Services.AddScoped<ApiTool.Backend.Rbac.CustomRoles.RoleResolver>();
builder.Services.AddScoped<ApiTool.Backend.Rbac.CustomRoles.CustomRolesService>();
// ... later:
app.MapCustomRolesEndpoints();
```

#### Impact on Existing Tests
- None directly — the new endpoints aren't asserted elsewhere.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|---------------|--------|-----------------|
| `MembersEndpointsTests.cs` | `Patch_member_role_by_owner_changes_role` | none | existing body still binds with nullable Role |
| `ResultsEndpointsTests.cs` | all upload tests | none | owners/admins/members have `results.upload` in their built-in sets |
| `AuditLogEndpointsTests.cs` | all | none | new audit events are filterable but not asserted here |
| `OrganizationServiceTests.cs` | all | none | no OrganizationMember-shape changes beyond a nullable addition |
| `InvitationsServiceTests.cs` | all | none | no membership contract change |
| _all other `*Tests.cs`_ | — | none | — |

New test files (TDD, RED-first):
- `src/ApiTool.Backend.Tests/Rbac/PermissionsTests.cs`
- `src/ApiTool.Backend.Tests/Rbac/RoleIdTests.cs`
- `src/ApiTool.Backend.Tests/Rbac/RoleResolverTests.cs`
- `src/ApiTool.Backend.Tests/Rbac/CustomRolesServiceTests.cs`
- `src/ApiTool.Backend.Tests/Rbac/CustomRolesEndpointsTests.cs`

## Risks and Edge Cases

- **Risk: EF InMemory provider ignores the new unique index on `(org_id, name)`.**
  → **Mitigation:** Service-level `AnyAsync` pre-check before `SaveChangesAsync`, same
  pattern as `OrganizationService.CreateAsync` uses for `organizations.slug`. Production
  SQLite still enforces the constraint; observable #2 ("duplicate role name within the
  same org → 409") is driven by the pre-check.
- **Risk: Orphaned `role_id` after a `CustomRole` DELETE slips through.** → **Mitigation:**
  `DeleteAsync` hard-blocks with `RoleInUse` whenever any member references the role, so
  we never reach an orphan state.
- **Risk: Permission catalogue drift between code and spec.** → **Mitigation:**
  `PermissionsTests.All_has_23_entries_matching_spec_matrix` pins the total, plus
  individual spot checks on the boundary cases (owner-only, admin-only, member view).
- **Risk: Spec says "JSONB" but SQLite doesn't have it.** → **Mitigation:** Documented
  in Architectural Decision #9. We follow the codebase idiom (`TEXT NOT NULL` with JSON
  string).
- **Risk: Built-in role names accepted as custom role names.** → **Mitigation:**
  `InvalidName` rejection with explicit allow-list check in `CustomRolesService.CreateAsync`.
- **Edge case: Empty `permissions` array on POST.** → **Handling:** Allowed — a role
  with zero permissions is a valid read-nothing role (the UI can still assign it).
  Test covers this path with `InvalidName` *not* being returned.
- **Edge case: Case mismatch on permission keys.** → **Handling:** The lookup against
  `Permissions.All` is `OrdinalIgnoreCase` **false** (exact match). Spec keys are
  lowercase with dots; any other casing is invalid. Test asserts `Results.Upload` is
  rejected.
- **Edge case: Member with `RoleId` pointing at a `CustomRole` in a different org.**
  → **Handling:** `RoleResolver` only looks up the role when `role.OrgId == member.OrgId`;
  mismatched rows fall through to built-in set. Covered by a resolver test.
- **Edge case: `PATCH /members/{id}` body with both `role` and `role_id`.** → **Handling:**
  Both applied: the built-in `role` is updated first, then `RoleId` is set. Audit
  event is `role.changed`. Test covers.

## Proposed Go (C#) Signatures Summary

```csharp
// Catalogue
public static class Permissions { public const string MembersInvite = "members.invite"; /* … */ }

// Entity
public sealed class CustomRole { public Guid Id; public Guid OrgId; public string Name; public string PermissionsJson; public DateTime CreatedAt; public Guid CreatedBy; }

// ID helper
public static class RoleId { public static string Format(Guid); public static bool TryParse(string?, out Guid); }

// Resolver
public sealed class RoleResolver(AppDbContext db) {
    public Task<IReadOnlySet<string>> GetEffectivePermissionsAsync(Guid userId, Guid orgId, CancellationToken ct);
    public Task<bool> HasPermissionAsync(Guid userId, Guid orgId, string permissionKey, CancellationToken ct);
}

// Service
public sealed class CustomRolesService(AppDbContext db, TimeProvider clock, IAuditWriter audit) {
    public Task<(IReadOnlyList<CustomRoleDto> roles, CustomRoleError err)> ListAsync(Guid userId, Guid orgId, CancellationToken ct);
    public Task<(CustomRoleDto? dto, CustomRoleError err, string? message)> CreateAsync(Guid userId, Guid orgId, CreateRoleRequest req, CancellationToken ct);
    public Task<(CustomRoleError err, int memberCount)> DeleteAsync(Guid userId, Guid orgId, Guid roleId, CancellationToken ct);
}

// Endpoints
public static class CustomRolesEndpoints { public static IEndpointRouteBuilder MapCustomRolesEndpoints(this IEndpointRouteBuilder app); }

// Extended Members request
public sealed record UpdateMemberRequest(string? Role = null, string? RoleId = null);
```

## Verification

```bash
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
    --filter "FullyQualifiedName~Rbac&FullyQualifiedName~CustomRoles"
# Expected: Passed: >=12, Failed: 0  (target 15)

dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"results-only","permissions":["results.view","results.upload","dashboard.view"]}' \
  "http://localhost:5000/api/v1/organizations/$ORG/roles"
# Expected HTTP 201, body contains "id":"role_..."
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://localhost:5000/api/v1/organizations/$ORG/roles"
# Expected HTTP 200, list includes the built-ins (owner/admin/member) + "results-only"
```

Swagger check: `http://localhost:5000/swagger` lists
`POST /organizations/{id}/roles`, `GET /organizations/{id}/roles`,
`DELETE /organizations/{id}/roles/{roleId}`.
