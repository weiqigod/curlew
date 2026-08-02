# Implementation Plan: M4-003

## Overview
Bootstrap the ApiTool backend service (ASP.NET Core 9 minimal API + EF Core) and ship the organization + seat RBAC data model with the first three endpoints (`GET /organizations`, `POST /organizations`, `GET /organizations/{id}`) behind JWT bearer auth, plus a dev-mode token helper script.

## Task Details
- **ID:** M4-003
- **Title:** Backend: organization + seat RBAC data model and service
- **Phase:** M4: Team Tier
- **Priority:** 1
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| — | (none) | — |

## Architectural Decisions (for the record)

The task YAML and the `docs/TECH_CHOICES.md` layout diverge. TECH_CHOICES proposes a
four-project Clean Architecture split (`Api`/`Core`/`Infrastructure`/`Contracts` + four
test projects). The task YAML calls for exactly two projects: `src/ApiTool.Backend`
(ASP.NET Core minimal API) and `src/ApiTool.Backend.Tests` (xUnit). The task YAML wins
for this first slice because it is explicit, the observable curl commands exercise it,
and the always-runnable + smallest-blast-radius principles favour the minimal surface.
A later task may split the project into layers if the code demands it.

Additional decisions made here (not pinned by the task YAML) — chosen for the smallest
runnable slice and documented so reviewers know they were deliberate:

1. **Database provider:** EF Core with **SQLite**. Dev runtime uses a file at
   `data/apitool-dev.db` (auto-created, migrations applied on startup via
   `db.Database.Migrate()` when `ASPNETCORE_ENVIRONMENT=Development`). Tests use
   `Microsoft.Data.Sqlite` with `DataSource=:memory:` held open per test. Rationale:
   no external infra required — honours the always-runnable contract. Postgres can
   replace SQLite in a later task via the EF Core provider seam.
2. **JWT scheme:** HS256 symmetric key loaded from configuration
   (`Jwt:SigningKey`, `Jwt:Issuer`, `Jwt:Audience`). Dev default values live in
   `appsettings.Development.json`. `scripts/test-token.sh` uses `openssl` + `base64`
   to mint a signed token for `owner@example.com` (or any email arg) — no extra
   runtime dependency beyond what macOS/Linux already ship.
3. **User resolution:** JWT `sub` claim carries the user id (UUID). JWT `email` claim
   carries the email. On first request from a token whose `sub` is not yet in the
   `users` table, the auth middleware upserts a minimal user row (id + email). This
   keeps the curl demo flow a single shell command and removes "create user first"
   from the slice, while leaving the users table shape ready for a future user-management
   task. A minimal `users` table is introduced in this migration purely to satisfy
   the FK from `organizations.owner_id`.
4. **ID format on the wire:** store `Guid` primary keys in SQL; serialize organization
   IDs as `org_<hex-without-dashes>` strings in responses to match the spec examples.
   A small `OrgId` value object handles parse/format symmetrically.
5. **Error envelope:** not strict RFC 9457 — the task's behaviors require short
   codes (`unauthorized`, `invalid_slug`, `organization_slug_taken`,
   `organization_not_found`). Ship `ErrorResponse { code, message }` as the JSON body
   for 400/401/404/409, served via a single `Results.Json(...)` helper. A future task
   can widen to problem+json if clients demand it.
6. **Default seat limit:** 10 (matches spec §Team tier "up to 10 seats"). Hardcoded
   in the create handler with a `const` named `TeamTierDefaultSeatLimit`. Subscription
   wiring is deferred to M4-010+.
7. **Swashbuckle:** use `Swashbuckle.AspNetCore` (stable, well-known). Tech-choices
   only requires "generate OpenAPI automatically" — Swashbuckle satisfies it and the
   task YAML names it explicitly.
8. **Organization status default:** `active`. The spec's `creating → active` transition
   is subscription-driven; with no subscription yet we skip `creating` and create rows
   directly as `active`. This matches the observable's `role=owner` expectation on the
   immediate POST response.
9. **Owner membership on POST:** creating an org inserts both an `organizations` row
   and an `organization_members` row with `role=owner` in the same transaction.
   `seat_count=1` in the response is computed from `organization_members` count, not
   stored — so future seat math stays honest.

## Implementation Steps

Ordering rationale: we build outward from pure data (entities, migration, DbContext)
to pure service logic (OrganizationService + validation) to the transport edge
(endpoints, auth, error mapping) and finally to the dev ergonomics script. Each step
builds on the previous without reaching backward.

### Step 1: Solution skeleton, csproj files, solution file

**Rationale:** Nothing else compiles without this. Smallest blast radius because it
creates new files only and touches no existing code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `ApiTool.Backend.sln` | create | Solution file at repo root pairing the two new projects |
| `src/ApiTool.Backend/ApiTool.Backend.csproj` | create | ASP.NET Core 9 web project (minimal API). References: `Microsoft.AspNetCore.Authentication.JwtBearer`, `Microsoft.EntityFrameworkCore`, `Microsoft.EntityFrameworkCore.Sqlite`, `Microsoft.EntityFrameworkCore.Design`, `Swashbuckle.AspNetCore`, `Meziantou.Analyzers`. Nullable + implicit usings + `TreatWarningsAsErrors=true`. |
| `src/ApiTool.Backend/Program.cs` | create | Minimal API bootstrap. Builds `WebApplication`, wires services, maps endpoints (real endpoints added in step 5). Exposes `partial class Program` so `WebApplicationFactory<Program>` can reach it. |
| `src/ApiTool.Backend/appsettings.json` | create | Empty defaults + `Logging` block. |
| `src/ApiTool.Backend/appsettings.Development.json` | create | Dev Jwt secret placeholder, `ConnectionStrings:Default=Data Source=data/apitool-dev.db`. |
| `src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | create | xUnit project. References: `Microsoft.NET.Test.Sdk`, `xunit`, `xunit.runner.visualstudio`, `FluentAssertions`, `Microsoft.AspNetCore.Mvc.Testing`, `Microsoft.EntityFrameworkCore.Sqlite`, `Meziantou.Analyzers`. ProjectReference to `ApiTool.Backend`. Nullable enabled. |
| `src/ApiTool.Backend.Tests/GlobalUsings.cs` | create | `global using Xunit; global using FluentAssertions;` |
| `.gitignore` | modify | Append `bin/`, `obj/`, `data/*.db*` if not already ignored. |

#### New Code

```csharp
// src/ApiTool.Backend/Program.cs
var builder = WebApplication.CreateBuilder(args);

// (services wired in Step 2/3/4)

var app = builder.Build();

app.MapGet("/", () => Results.Redirect("/swagger"));

app.Run();

public partial class Program;
```

```xml
<!-- src/ApiTool.Backend/ApiTool.Backend.csproj -->
<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <TargetFramework>net9.0</TargetFramework>
    <Nullable>enable</Nullable>
    <ImplicitUsings>enable</ImplicitUsings>
    <TreatWarningsAsErrors>true</TreatWarningsAsErrors>
    <RootNamespace>ApiTool.Backend</RootNamespace>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="Microsoft.AspNetCore.Authentication.JwtBearer" Version="9.0.0" />
    <PackageReference Include="Microsoft.EntityFrameworkCore" Version="9.0.0" />
    <PackageReference Include="Microsoft.EntityFrameworkCore.Sqlite" Version="9.0.0" />
    <PackageReference Include="Microsoft.EntityFrameworkCore.Design" Version="9.0.0">
      <PrivateAssets>all</PrivateAssets>
      <IncludeAssets>runtime; build; native; contentfiles; analyzers; buildtransitive</IncludeAssets>
    </PackageReference>
    <PackageReference Include="Swashbuckle.AspNetCore" Version="7.0.0" />
  </ItemGroup>
</Project>
```

> Note: .NET 10 SDK is installed on the dev machine (`dotnet --version` reports
> `10.0.100`) but the spec pins .NET 9. Target `net9.0` and rely on SDK roll-forward.
> If `net9.0` runtime isn't installed locally, execute Step 2 will install it before
> any test runs — captured as a risk below.

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/SmokeTests.cs
public sealed class SmokeTests
{
    [Fact]
    public void Program_type_is_discoverable_for_WebApplicationFactory()
    {
        typeof(Program).Should().NotBeNull();
    }
}
```

This single test proves the two projects are wired together (the
`ProjectReference` works, `partial class Program` is reachable). It fails RED before
the csproj changes land.

#### Impact on Existing Tests
- No existing tests affected (no C# code exists yet).

### Step 2: Domain entities + `AppDbContext` + EF Core migration `0001_organizations_rbac`

**Rationale:** Service logic and endpoints both depend on the DbContext. Adding the
EF layer before any service code keeps the diff reviewable in isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/AppDbContext.cs` | create | EF Core DbContext with four DbSets + fluent config for constraints/indexes |
| `src/ApiTool.Backend/Data/Entities/User.cs` | create | Minimal user entity (`Id Guid`, `Email string`, `CreatedAt DateTime`) |
| `src/ApiTool.Backend/Data/Entities/Organization.cs` | create | Entity matching `organizations` DDL (Id, Name, Slug, OwnerId, Status, CreatedAt, UpdatedAt, Settings JSON) |
| `src/ApiTool.Backend/Data/Entities/OrganizationMember.cs` | create | Entity with composite key (OrgId, UserId), Role, JoinedAt, InvitedBy |
| `src/ApiTool.Backend/Data/Entities/OrganizationInvitation.cs` | create | Shell entity for invitations (fields only; no behaviour in this task) |
| `src/ApiTool.Backend/Data/Entities/OrganizationAuditLogEntry.cs` | create | Shell entity for audit log (fields only; inserted on org.created in step 4) |
| `src/ApiTool.Backend/Data/Entities/OrgRole.cs` | create | `public enum OrgRole { Owner, Admin, Member }` with EF value conversion to lowercase strings |
| `src/ApiTool.Backend/Data/Entities/OrgStatus.cs` | create | `public enum OrgStatus { Creating, Active, PendingDeletion, Deleted }` |
| `src/ApiTool.Backend/Migrations/20260415000001_InitialOrganizationsRbac.cs` | create | Hand-written EF migration (or generated then committed) creating the four tables |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | create | EF model snapshot emitted by `dotnet ef migrations add` |
| `src/ApiTool.Backend/Program.cs` | modify | Register `AppDbContext` with sqlite connection string; call `Database.Migrate()` in Development |

#### New Code (shape)

```csharp
// src/ApiTool.Backend/Data/AppDbContext.cs
public sealed class AppDbContext(DbContextOptions<AppDbContext> options) : DbContext(options)
{
    public DbSet<User> Users => Set<User>();
    public DbSet<Organization> Organizations => Set<Organization>();
    public DbSet<OrganizationMember> OrganizationMembers => Set<OrganizationMember>();
    public DbSet<OrganizationInvitation> OrganizationInvitations => Set<OrganizationInvitation>();
    public DbSet<OrganizationAuditLogEntry> OrganizationAuditLog => Set<OrganizationAuditLogEntry>();

    protected override void OnModelCreating(ModelBuilder b)
    {
        b.Entity<User>(e =>
        {
            e.ToTable("users");
            e.HasKey(x => x.Id);
            e.Property(x => x.Email).HasMaxLength(255).IsRequired();
            e.HasIndex(x => x.Email).IsUnique();
        });

        b.Entity<Organization>(e =>
        {
            e.ToTable("organizations");
            e.HasKey(x => x.Id);
            e.Property(x => x.Name).HasMaxLength(100).IsRequired();
            e.Property(x => x.Slug).HasMaxLength(100).IsRequired();
            e.HasIndex(x => x.Slug).IsUnique();
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.SettingsJson).HasColumnName("settings").IsRequired();
            e.HasOne<User>().WithMany().HasForeignKey(x => x.OwnerId).OnDelete(DeleteBehavior.Restrict);
        });

        b.Entity<OrganizationMember>(e =>
        {
            e.ToTable("organization_members");
            e.HasKey(x => new { x.OrgId, x.UserId });
            e.Property(x => x.Role).HasConversion<string>().HasMaxLength(20);
            e.HasIndex(x => new { x.OrgId, x.Role });
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<OrganizationInvitation>(e => { /* table + pk + indexes */ });
        b.Entity<OrganizationAuditLogEntry>(e => { /* table + pk + indexes */ });
    }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs
public sealed class AppDbContextSchemaTests
{
    [Theory]
    [InlineData("organizations")]
    [InlineData("organization_members")]
    [InlineData("organization_invitations")]
    [InlineData("organization_audit_log")]
    public async Task Migration_creates_expected_table(string tableName)
    {
        await using var db = TestDb.CreateOpen();
        await db.Database.MigrateAsync();
        var exists = await TableExistsAsync(db, tableName);
        exists.Should().BeTrue();
    }

    [Fact]
    public async Task Organizations_slug_is_unique()
    {
        await using var db = TestDb.CreateOpen();
        await db.Database.MigrateAsync();
        // seed a user
        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = "a@b.c", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization { /* slug=acme */ });
        await db.SaveChangesAsync();
        db.Organizations.Add(new Organization { /* slug=acme */ });
        var act = async () => await db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>();
    }
}

// src/ApiTool.Backend.Tests/TestInfrastructure/TestDb.cs
internal static class TestDb
{
    public static AppDbContext CreateOpen()
    {
        var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        var opts = new DbContextOptionsBuilder<AppDbContext>().UseSqlite(conn).Options;
        return new AppDbContext(opts);
    }
}
```

#### Impact on Existing Tests
- `SmokeTests` from Step 1 still passes unchanged.

### Step 3: Slug validation + `OrganizationService` (pure logic)

**Rationale:** Service is pure C# over the DbContext — no HTTP concerns. Unit-testable
without `WebApplicationFactory`, which keeps tests fast and anchors the behaviours
before wiring the transport layer.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Organizations/SlugValidator.cs` | create | Static `TryValidate(slug, out error)` — regex `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, length 2–100 |
| `src/ApiTool.Backend/Organizations/OrganizationService.cs` | create | `ListForUserAsync`, `CreateAsync`, `GetByIdForUserAsync` — returns `Result<T, OrgError>` discriminated-union style |
| `src/ApiTool.Backend/Organizations/OrgError.cs` | create | `public enum OrgError { None, InvalidSlug, SlugTaken, NotFound }` |
| `src/ApiTool.Backend/Organizations/OrganizationDto.cs` | create | Response shape `{ id, name, slug, role, seat_count, seat_limit, status, created_at }` — snake_case via `System.Text.Json` naming policy |
| `src/ApiTool.Backend.Tests/Organizations/SlugValidatorTests.cs` | create | Table tests for valid/invalid slugs |
| `src/ApiTool.Backend.Tests/Organizations/OrganizationServiceTests.cs` | create | Behavior tests using in-memory SQLite |

#### Function Signatures

```csharp
public sealed class OrganizationService(AppDbContext db, TimeProvider clock)
{
    public const int TeamTierDefaultSeatLimit = 10;

    public Task<IReadOnlyList<OrganizationDto>> ListForUserAsync(
        Guid userId, CancellationToken ct);

    public Task<(OrganizationDto? dto, OrgError error, string? message)> CreateAsync(
        Guid userId, string name, string slug, CancellationToken ct);

    public Task<(OrganizationDto? dto, OrgError error)> GetByIdForUserAsync(
        Guid userId, Guid orgId, CancellationToken ct);
}
```

`CreateAsync` sequence:
1. Validate slug (return `InvalidSlug` + message if bad).
2. Query `organizations` for slug (return `SlugTaken` if taken).
3. Insert `Organization` row (status=active).
4. Insert `OrganizationMember` row (role=owner).
5. Insert `organization_audit_log` row (event_type=`org.created`).
6. SaveChanges in a single transaction (EF SaveChanges is transactional).
7. Return computed DTO with `role=owner`, `seat_count=1`, `seat_limit=10`.

`GetByIdForUserAsync` returns `NotFound` for both "row doesn't exist" and
"user is not a member" — matches the 404-no-enumeration behaviour.

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class SlugValidatorTests
{
    [Theory]
    [InlineData("acme", true)]
    [InlineData("ac", true)]
    [InlineData("acme-corp", true)]
    [InlineData("ab12", true)]
    [InlineData("Acme", false)]          // uppercase
    [InlineData("-acme", false)]         // leading hyphen
    [InlineData("acme-", false)]         // trailing hyphen
    [InlineData("a", false)]             // too short
    [InlineData("acme_corp", false)]     // underscore
    [InlineData("", false)]              // empty
    public void Validates_slug_format(string slug, bool expected) { /* ... */ }
}

public sealed class OrganizationServiceTests
{
    [Fact] public async Task Create_inserts_org_member_and_returns_owner_role() { /* ... */ }
    [Fact] public async Task Create_rejects_uppercase_slug_with_invalid_slug_error() { /* ... */ }
    [Fact] public async Task Create_rejects_duplicate_slug_with_slug_taken_error() { /* ... */ }
    [Fact] public async Task Create_persists_audit_log_entry_org_created() { /* ... */ }
    [Fact] public async Task List_returns_empty_when_user_has_no_memberships() { /* ... */ }
    [Fact] public async Task List_returns_orgs_with_role_and_seat_count() { /* ... */ }
    [Fact] public async Task GetById_returns_not_found_when_user_is_not_member() { /* ... */ }
    [Fact] public async Task GetById_returns_org_for_member() { /* ... */ }
}
```

Eight behaviour tests from the service plus the slug validator table — fifteen-plus
tests after the endpoint layer in Step 5 lands. Comfortably above the "≥12" floor.

#### Impact on Existing Tests
- None — this step only adds files.

### Step 4: JWT authentication middleware + current-user resolution

**Rationale:** Endpoints need an authenticated principal. Land auth before endpoints
so endpoint tests can inject a bearer token the same way the production code resolves
it. Smaller blast radius than going straight to endpoints — if auth config breaks,
failure is isolated here.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Auth/JwtOptions.cs` | create | `IOptions<JwtOptions>` with SigningKey, Issuer, Audience |
| `src/ApiTool.Backend/Auth/CurrentUserAccessor.cs` | create | Scoped service: reads `sub` + `email` from `HttpContext.User`, upserts user row, caches for request |
| `src/ApiTool.Backend/Auth/UnauthorizedResponseWriter.cs` | create | Overrides JwtBearer `OnChallenge` to return `{ "code":"unauthorized","message":"..." }` JSON with 401 |
| `src/ApiTool.Backend/Program.cs` | modify | `AddAuthentication().AddJwtBearer(...)`, `AddAuthorization()`, register `JwtOptions`, `CurrentUserAccessor`, `OrganizationService`, `TimeProvider.System`, Swashbuckle |
| `src/ApiTool.Backend/appsettings.Development.json` | modify | Add `Jwt` block with a deterministic dev secret (documented as dev-only) |
| `src/ApiTool.Backend.Tests/TestInfrastructure/BackendFactory.cs` | create | `WebApplicationFactory<Program>` that swaps DbContext for in-memory sqlite and injects deterministic JWT options |
| `src/ApiTool.Backend.Tests/TestInfrastructure/TestTokens.cs` | create | Helper that mints HS256 bearer tokens for tests using the same signing key |

#### Key snippets

```csharp
// JwtBearer config
options.TokenValidationParameters = new TokenValidationParameters
{
    ValidateIssuer = true,
    ValidIssuer = jwt.Issuer,
    ValidateAudience = true,
    ValidAudience = jwt.Audience,
    ValidateIssuerSigningKey = true,
    IssuerSigningKey = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(jwt.SigningKey)),
    ValidateLifetime = true,
    NameClaimType = JwtRegisteredClaimNames.Sub,
};
options.Events = new JwtBearerEvents
{
    OnChallenge = ctx => UnauthorizedResponseWriter.WriteAsync(ctx),
};
```

```csharp
// CurrentUserAccessor.ResolveAsync
var subClaim = httpContext.User.FindFirstValue(ClaimTypes.NameIdentifier)
               ?? httpContext.User.FindFirstValue(JwtRegisteredClaimNames.Sub);
var userId = Guid.Parse(subClaim);
var email = httpContext.User.FindFirstValue(JwtRegisteredClaimNames.Email) ?? "";
var existing = await db.Users.FindAsync([userId], ct);
if (existing is null)
{
    db.Users.Add(new User { Id = userId, Email = email, CreatedAt = clock.GetUtcNow().UtcDateTime });
    await db.SaveChangesAsync(ct);
}
return userId;
```

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class JwtAuthenticationTests(BackendFactory factory) : IClassFixture<BackendFactory>
{
    [Fact]
    public async Task Requests_without_bearer_return_401_with_unauthorized_code() { /* ... */ }

    [Fact]
    public async Task Requests_with_invalid_signature_return_401_with_unauthorized_code() { /* ... */ }

    [Fact]
    public async Task Valid_token_upserts_missing_user_row() { /* ... */ }
}
```

#### Impact on Existing Tests
- `SmokeTests` continues to pass; it never hit `/organizations` so auth doesn't affect it.

### Step 5: `OrganizationsController` (minimal API group) + error mapping

**Rationale:** Final step for the happy/sad paths exposed to the outside world. Now
that auth and the service work, the endpoint layer is a thin adaptor.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Organizations/OrganizationsEndpoints.cs` | create | `public static IEndpointRouteBuilder MapOrganizationsEndpoints(this IEndpointRouteBuilder app)` — maps GET list / POST create / GET detail under `/api/v1/organizations`, requires auth |
| `src/ApiTool.Backend/Organizations/CreateOrganizationRequest.cs` | create | Input DTO `{ Name, Slug }` |
| `src/ApiTool.Backend/Organizations/ErrorResponse.cs` | create | `{ code, message }` JSON response |
| `src/ApiTool.Backend/Program.cs` | modify | Call `app.MapOrganizationsEndpoints()`; add Swagger generation and UI in Development |
| `src/ApiTool.Backend.Tests/Organizations/OrganizationsEndpointsTests.cs` | create | Full behavior suite against `BackendFactory` |

#### Endpoint wiring

```csharp
var group = app.MapGroup("/api/v1/organizations").RequireAuthorization();

group.MapGet("", async (CurrentUserAccessor users, OrganizationService svc, CancellationToken ct) =>
{
    var userId = await users.ResolveAsync(ct);
    var orgs = await svc.ListForUserAsync(userId, ct);
    return Results.Ok(new { organizations = orgs });
});

group.MapPost("", async (CreateOrganizationRequest body, CurrentUserAccessor users, OrganizationService svc, CancellationToken ct) =>
{
    var userId = await users.ResolveAsync(ct);
    var (dto, error, message) = await svc.CreateAsync(userId, body.Name, body.Slug, ct);
    return error switch
    {
        OrgError.None         => Results.Created($"/api/v1/organizations/{dto!.Id}", dto),
        OrgError.InvalidSlug  => Results.Json(new ErrorResponse("invalid_slug", message!), statusCode: 400),
        OrgError.SlugTaken    => Results.Json(new ErrorResponse("organization_slug_taken", message!), statusCode: 409),
        _                     => Results.StatusCode(500),
    };
});

group.MapGet("{id}", async (string id, CurrentUserAccessor users, OrganizationService svc, CancellationToken ct) =>
{
    if (!OrgId.TryParse(id, out var orgGuid))
        return Results.Json(new ErrorResponse("organization_not_found", "Organization not found"), statusCode: 404);

    var userId = await users.ResolveAsync(ct);
    var (dto, error) = await svc.GetByIdForUserAsync(userId, orgGuid, ct);
    return error switch
    {
        OrgError.None     => Results.Ok(dto),
        OrgError.NotFound => Results.Json(new ErrorResponse("organization_not_found", "Organization not found"), statusCode: 404),
        _                 => Results.StatusCode(500),
    };
});
```

#### Tests to Write FIRST (RED phase)

Every behavior from the task YAML maps to a named test:

```csharp
public sealed class OrganizationsEndpointsTests(BackendFactory factory) : IClassFixture<BackendFactory>
{
    [Fact] public async Task Post_creates_org_and_returns_201_with_owner_role_and_default_seat_counts() { /* ... */ }
    [Fact] public async Task Post_with_duplicate_slug_returns_409_organization_slug_taken() { /* ... */ }
    [Fact] public async Task Post_with_uppercase_slug_returns_400_invalid_slug() { /* ... */ }
    [Fact] public async Task Post_with_too_short_slug_returns_400_invalid_slug() { /* ... */ }
    [Fact] public async Task Post_with_slug_containing_underscore_returns_400_invalid_slug() { /* ... */ }
    [Fact] public async Task Get_list_returns_empty_for_user_with_no_memberships() { /* ... */ }
    [Fact] public async Task Get_list_returns_orgs_for_member_user() { /* ... */ }
    [Fact] public async Task Get_detail_returns_200_with_role_and_seats_for_member() { /* ... */ }
    [Fact] public async Task Get_detail_returns_404_for_non_member_without_leaking_existence() { /* ... */ }
    [Fact] public async Task Get_detail_returns_404_for_malformed_id() { /* ... */ }
    [Fact] public async Task All_endpoints_return_401_when_no_bearer_header() { /* ... */ }
}
```

Combined with the slug-validator table (10 cases), the service suite (8 tests), the
schema suite (5 tests), and the auth suite (3 tests), the Organizations/Rbac filter
will report roughly 35 tests — well above the ≥12 floor.

#### Impact on Existing Tests
- Step 2's `AppDbContextSchemaTests` and Step 3's `OrganizationServiceTests` continue
  to pass because the service API is unchanged.
- Step 4's JWT auth tests continue to pass.

### Step 6: `scripts/test-token.sh` + smoke curl wiring

**Rationale:** Last step because the script only exists to drive the observable
curl demo once the service is up. No other code depends on it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `scripts/test-token.sh` | create | Bash script that takes an email arg and prints an HS256-signed JWT to stdout |
| `src/ApiTool.Backend/README.md` | create | One-screen how-to: `dotnet run`, `./scripts/test-token.sh`, sample curl commands |
| `CHANGELOG.md` | modify | Add entry under Unreleased: backend bootstrap + organizations endpoints |

#### Script sketch

```bash
#!/usr/bin/env bash
set -euo pipefail

EMAIL="${1:-owner@example.com}"
SECRET="${APITOOL_JWT_SIGNING_KEY:-development-signing-key-change-me-32-bytes-minimum}"
ISSUER="${APITOOL_JWT_ISSUER:-apitool-dev}"
AUDIENCE="${APITOOL_JWT_AUDIENCE:-apitool-dev}"
SUB="${APITOOL_TEST_USER_ID:-$(uuidgen | tr '[:upper:]' '[:lower:]')}"

now=$(date +%s)
exp=$((now + 3600))

b64url() { openssl base64 -e -A | tr '+/' '-_' | tr -d '='; }

header='{"alg":"HS256","typ":"JWT"}'
payload=$(printf '{"sub":"%s","email":"%s","iss":"%s","aud":"%s","iat":%d,"exp":%d}' \
  "$SUB" "$EMAIL" "$ISSUER" "$AUDIENCE" "$now" "$exp")

h=$(printf '%s' "$header"  | b64url)
p=$(printf '%s' "$payload" | b64url)
sig=$(printf '%s.%s' "$h" "$p" | openssl dgst -sha256 -hmac "$SECRET" -binary | b64url)
printf '%s.%s.%s\n' "$h" "$p" "$sig"
```

The script is fully deterministic given `APITOOL_TEST_USER_ID`, so smoke runs can
reuse the same user id across invocations (important for GET-after-POST flows).

#### Tests to Write FIRST (RED phase)
- Manual verification only (the observable curl commands from the task YAML). No
  automated test — shell script integration would expand scope beyond this slice.

#### Impact on Existing Tests
- None.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| — | — | none (no pre-existing C# tests) | — |

## Risks and Edge Cases

- **Risk: .NET 9 runtime missing on macOS dev machine** (SDK 10.0.100 is installed but
  net9.0 target requires the 9.0 runtime). **Mitigation:** Step 2 runs `dotnet workload
  restore` + `dotnet build` before any tests. If the runtime isn't locally available
  we ship a note in Step 6's README pointing at `brew install dotnet@9`.
- **Risk: EF Core `MigrateAsync` on SQLite in-memory per test is slow.** **Mitigation:**
  `BackendFactory` caches a single open `SqliteConnection` per test class via
  `IClassFixture<>`; the in-memory DB survives for the fixture lifetime, not per test.
  Each test cleans its data with `db.Database.ExecuteSqlRawAsync("DELETE FROM ...")`.
- **Risk: Slug regex doesn't match spec in edge cases.** **Mitigation:** regex is
  `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` taken verbatim from the DDL. Length checked
  separately (2–100 inclusive).
- **Risk: `Results.Json` defaults to camelCase; task behaviours expect snake_case
  (`seat_count`, `seat_limit`).** **Mitigation:** set
  `JsonSerializerOptions.PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower` on
  the minimal API via `ConfigureHttpJsonOptions` so every endpoint renders snake_case
  by default. DTOs stay in PascalCase C# properties.
- **Risk: Status enum serialisation.** **Mitigation:** configure
  `JsonStringEnumConverter` with a `SnakeCaseLower` naming policy so
  `OrgStatus.Active` renders as `"active"`.
- **Edge case: Malformed `org_…` id in GET detail.** **Handling:** `OrgId.TryParse`
  returns false → endpoint returns 404 `organization_not_found`, matching
  non-enumeration behaviour.
- **Edge case: Same slug in two concurrent POSTs.** **Handling:** unique index on
  `organizations.slug` guarantees one insert wins; the loser catches
  `DbUpdateException` with SQLite error 19 (constraint violation) and surfaces
  `SlugTaken` from `CreateAsync`.
- **Edge case: JWT with non-GUID `sub` claim.** **Handling:** `CurrentUserAccessor`
  returns 401 via an explicit write of the `unauthorized` error envelope rather than
  crashing.
- **Edge case: `GET /api/v1/organizations/{id}` called with well-formed id that's not
  a UUID.** **Handling:** `OrgId.TryParse` → false → 404 `organization_not_found`.
- **Open-question decision: members permissions JSONB default.** **Resolution:**
  store `[]` as the default via a value converter serialising `string[]`. A
  follow-up task will populate granular permissions; this task only needs the column
  to exist.
- **Open-question decision: how many tests count under the "≥12" filter.** The test
  filter `FullyQualifiedName~Organizations|FullyQualifiedName~Rbac` catches every
  file under `ApiTool.Backend.Tests/Organizations/*` plus anything with `Rbac` in
  the type name. To be safe I'll namespace `JwtAuthenticationTests` under
  `ApiTool.Backend.Tests.Organizations` so it counts too, or add an `[Trait]`.
  Planned count: ≥30 tests — generous margin.

## Verification

```bash
# Build
dotnet build ApiTool.Backend.sln

# Unit + integration tests
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj

# Filtered run — matches the observable
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Organizations|FullyQualifiedName~Rbac"
```

Observable verification (exact commands from the task YAML):

```bash
dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations
# Expected HTTP 200, JSON: {"organizations":[]}

curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme","slug":"acme"}' \
  http://localhost:5000/api/v1/organizations
# Expected HTTP 201, body contains "role":"owner","seat_count":1,"seat_limit":10
```

Swagger verification:

```bash
curl -sS http://localhost:5000/swagger/v1/swagger.json | jq '.paths | keys'
# Expected: ["/api/v1/organizations","/api/v1/organizations/{id}"]
```
