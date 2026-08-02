# Implementation Plan: M5-016

## Overview
Adds a first-boot admin bootstrap and EF migration runner to the backend so a
self-hosted stack can come up on an empty Postgres volume, apply all migrations,
seed a local admin user (argon2id password hash), and accept
`POST /api/v1/auth/login` with those credentials — turning M5-015's provisioned
Postgres into the real database of record.

## Task Details
- **ID:** M5-016
- **Title:** Backend: admin bootstrap + migration runner
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M5-015 | Self-hosted deployment — docker-compose bundle | done |

## Key Architectural Decisions

The task YAML is ambiguous in three places. Decisions made and rationale:

1. **Npgsql provider swap is in scope.** The observable starts fresh Postgres,
   expects `all EF migrations are applied` against it, and the M5-015 README
   explicitly defers the provider swap to M5-016
   (`deploy/self-hosted/README.md` line 135: "M5-016 switches the EF provider
   to Npgsql and runs migrations against Postgres on startup"). The provider
   is therefore chosen at runtime from env vars (Postgres when
   `POSTGRES_HOST` is set; SQLite otherwise for dev/tests), and fresh Postgres
   migrations are regenerated. Existing SQLite-based migrations are replaced
   with Npgsql-compatible ones (re-scaffolded from the current model snapshot).
   The existing migrations folder is rebuilt: SQLite-specific annotations
   (`AUTOINCREMENT`, `TEXT` columns for GUID) differ from Postgres (`uuid`,
   `bigserial`), so the migration snapshots must be regenerated.

   *Trade-off:* regenerating migrations drops the historical migration chain,
   which is acceptable because the self-hosted product has no shipped data —
   M5-015 is the first self-hosted release and is explicitly documented as
   "Postgres not yet consumed". Existing test/dev SQLite deployments use
   `EnsureCreated` (InMemory provider for tests, `MigrateAsync` in
   Development) and so are unaffected by migration-history changes.

2. **Password hashing uses `Konscious.Security.Cryptography.Argon2`.**
   This is the de facto .NET Argon2 implementation (OWASP-referenced,
   ≥9M downloads on NuGet, actively maintained). Task YAML explicitly
   specifies argon2id. No other argon2 dependency exists in the repo.
   Parameters: `m=65536 KiB (64 MiB)`, `t=3`, `p=4`, `saltLength=16`,
   `hashLength=32` (OWASP 2024 recommended). Encoded as PHC string
   (`$argon2id$v=19$m=65536,t=3,p=4$<salt-b64>$<hash-b64>`) so future
   rotations can change parameters without migrating the column type.

3. **Migration + bootstrap run as an async call before `app.Run()`**
   (not `IHostedService`). `IHostedService` runs after the web host starts
   accepting requests, which would race the healthcheck; blocking before
   `app.Run()` is the standard pattern (already used in `Program.cs` line
   286–291 for Development) and lets us return a non-zero exit code on
   failure. Guarded by `BACKEND_RUN_MIGRATIONS=1` and `BOOTSTRAP_ADMIN_*`
   env vars so tests and dev aren't affected.

4. **`/api/v1/auth/login` endpoint is local-password-only.** SSO login
   remains the primary flow (`/api/v1/sso/{provider}/...`). The new endpoint
   is a narrow escape hatch for the bootstrap admin on a freshly installed
   self-hosted instance. It issues the same `curlew_session` JWT via the
   existing `SessionTokenIssuer`. Rate-limited at 10 req/min per email to
   slow brute-force. `/api/v1/auth/admin/login` from the spec is not added
   (scope creep beyond the task's observable).

5. **Exit codes:** 3 for bootstrap config validation (password < 12 chars,
   missing email when password set or vice-versa, invalid email format),
   4 for migration failure (DB unreachable or SQL error). Both paths log
   structured error lines before exiting via `Environment.Exit(n)`.

6. **`User` entity grows two optional columns.** `PasswordHash` (nullable
   string) and `IsAdmin` (bool, default false). `Users.Email` already has
   a unique index. Keeping columns nullable avoids breaking the SSO-only
   user flow (existing users continue to have `PasswordHash = null` and
   `IsAdmin = false`). A `tier` column on `User` is **not** added — tier is
   already an organization-level concept (`Subscription.Tier`). "Admin
   tier=enterprise" in behavior #2 is modelled as:
   - `User.IsAdmin = true`
   - Auto-create a single "default" org owned by the admin with a
     `Subscription { Tier = Enterprise, Status = Active }`. This matches
     the task YAML scope (role=admin, tier=enterprise) and matches how
     the existing code models tiers everywhere.

7. **Tests use three layers:**
   - Unit tests: `AdminBootstrapTests.cs` + `MigrationRunnerTests.cs`
     exercise the two startup tasks against an in-memory SQLite
     `TestDbScope` (migrations unit-tested via `EnsureDeleted`/
     `EnsureCreated`, since EF migrations can't be applied to
     `:memory:` sqlite trivially — we verify `MigrateAsync` is called
     and failures propagate the expected exit code).
   - Integration tests: `AuthLoginEndpointTests.cs` via `BackendFactory`
     — exercises `POST /api/v1/auth/login` against the InMemory EF
     provider with a pre-seeded admin row.
   - Smoke: extend `scripts/test-self-hosted.sh` to set the bootstrap
     env vars and assert that the login call returns 200 with a token.

## Implementation Steps

Ordered smallest-blast-radius first: add the password primitives (no existing
callers) → entity + migrations → startup tasks → endpoint wiring → docker +
docs.

### Step 1: Add argon2id password hasher and PHC encoder

**Rationale:** Self-contained crypto primitive with no callers. Easiest to
TDD; every later step depends on it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/ApiTool.Backend.csproj` | modify | Add `Konscious.Security.Cryptography.Argon2` v1.3.1 |
| `src/ApiTool.Backend/Auth/PasswordHasher.cs` | create | Encode/verify argon2id PHC strings |
| `src/ApiTool.Backend.Tests/Auth/PasswordHasherTests.cs` | create | Round-trip + malformed hash cases |

#### New Code — `PasswordHasher.cs`

```csharp
using System.Security.Cryptography;
using System.Text;
using Konscious.Security.Cryptography;

namespace ApiTool.Backend.Auth;

/// <summary>Argon2id password hasher that emits and parses PHC-format strings.</summary>
public sealed class PasswordHasher
{
    private const int SaltBytes = 16;
    private const int HashBytes = 32;
    private const int MemoryKiB = 65536;   // 64 MiB
    private const int Iterations = 3;
    private const int Parallelism = 4;

    /// <summary>Hashes a plaintext password and returns an OWASP-style PHC encoded string.</summary>
    public string Hash(string password)
    {
        ArgumentException.ThrowIfNullOrEmpty(password);
        var salt = RandomNumberGenerator.GetBytes(SaltBytes);
        var hash = ComputeHash(password, salt);
        return $"$argon2id$v=19$m={MemoryKiB},t={Iterations},p={Parallelism}$" +
               $"{Convert.ToBase64String(salt)}${Convert.ToBase64String(hash)}";
    }

    /// <summary>
    /// Verifies a plaintext password against a PHC-encoded hash. Returns
    /// <see langword="false"/> on any malformed input instead of throwing.
    /// </summary>
    public bool Verify(string password, string encoded)
    {
        if (string.IsNullOrEmpty(password) || string.IsNullOrEmpty(encoded))
            return false;
        if (!TryParsePhc(encoded, out var parsed)) return false;
        var computed = ComputeHash(password, parsed.Salt, parsed.MemoryKiB, parsed.Iterations, parsed.Parallelism, parsed.Hash.Length);
        return CryptographicOperations.FixedTimeEquals(computed, parsed.Hash);
    }

    private static byte[] ComputeHash(
        string password, byte[] salt,
        int memoryKiB = MemoryKiB, int iterations = Iterations,
        int parallelism = Parallelism, int hashBytes = HashBytes)
    {
        using var argon = new Argon2id(Encoding.UTF8.GetBytes(password))
        {
            Salt = salt,
            MemorySize = memoryKiB,
            Iterations = iterations,
            DegreeOfParallelism = parallelism,
        };
        return argon.GetBytes(hashBytes);
    }

    private record ParsedPhc(byte[] Salt, byte[] Hash, int MemoryKiB, int Iterations, int Parallelism);

    private static bool TryParsePhc(string encoded, out ParsedPhc parsed)
    {
        parsed = null!;
        // Expected: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
        var parts = encoded.Split('$', StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length != 5 || parts[0] != "argon2id") return false;
        if (parts[1] != "v=19") return false;
        var paramPairs = parts[2].Split(',');
        int m = 0, t = 0, p = 0;
        foreach (var pair in paramPairs)
        {
            var kv = pair.Split('=');
            if (kv.Length != 2) return false;
            if (!int.TryParse(kv[1], out var n)) return false;
            switch (kv[0]) { case "m": m = n; break; case "t": t = n; break; case "p": p = n; break; }
        }
        if (m <= 0 || t <= 0 || p <= 0) return false;
        try
        {
            parsed = new ParsedPhc(Convert.FromBase64String(parts[3]), Convert.FromBase64String(parts[4]), m, t, p);
            return true;
        }
        catch (FormatException) { return false; }
    }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class PasswordHasherTests
{
    [Theory]
    [InlineData("ChangeMe!ChangeMe!")]
    [InlineData("a-long-enough-password-123")]
    public void Hash_then_verify_returns_true(string password)
    {
        var h = new PasswordHasher();
        var encoded = h.Hash(password);
        encoded.Should().StartWith("$argon2id$v=19$m=65536,t=3,p=4$");
        h.Verify(password, encoded).Should().BeTrue();
    }

    [Fact]
    public void Verify_with_wrong_password_returns_false()
    {
        var h = new PasswordHasher();
        h.Verify("wrong-password-xyz", h.Hash("ChangeMe!ChangeMe!")).Should().BeFalse();
    }

    [Theory]
    [InlineData("")]
    [InlineData("not-a-phc-string")]
    [InlineData("$argon2id$v=19$m=foo,t=3,p=4$c2FsdA==$aGFzaA==")]
    [InlineData("$argon2d$v=19$m=65536,t=3,p=4$c2FsdA==$aGFzaA==")]  // wrong algorithm
    [InlineData("$argon2id$v=19$m=65536,t=3,p=4$!!!not-base64!!!$aGFzaA==")]
    public void Verify_with_malformed_hash_returns_false(string encoded)
    {
        new PasswordHasher().Verify("ChangeMe!ChangeMe!", encoded).Should().BeFalse();
    }

    [Fact]
    public void Hash_produces_different_salts_for_same_password()
    {
        var h = new PasswordHasher();
        var a = h.Hash("same-password-xyz");
        var b = h.Hash("same-password-xyz");
        a.Should().NotBe(b);
    }
}
```

#### Impact on Existing Tests
- None — new file, no existing callers.

---

### Step 2: Extend `User` entity with `PasswordHash` and `IsAdmin`, update `AppDbContext`

**Rationale:** Storage schema must exist before bootstrap can write. Schema
change touches a single property and its mapping; the Npgsql provider swap
in Step 3 will re-scaffold migrations that pick this up.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/User.cs` | modify | Add `PasswordHash?`, `IsAdmin` |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | Configure `PasswordHash` column (max 200, nullable) and `IsAdmin` (bool, default false) |
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | modify | Assert new columns round-trip |

#### Current Code — `User.cs`

```csharp
public sealed class User
{
    public Guid Id { get; set; }
    public string Email { get; set; } = string.Empty;
    public DateTime CreatedAt { get; set; }
}
```

#### New Code

```csharp
public sealed class User
{
    public Guid Id { get; set; }
    public string Email { get; set; } = string.Empty;
    public DateTime CreatedAt { get; set; }

    /// <summary>Argon2id PHC-encoded password hash; null for SSO-only users.</summary>
    public string? PasswordHash { get; set; }

    /// <summary>True for users created via the first-boot admin bootstrap.</summary>
    public bool IsAdmin { get; set; }
}
```

#### AppDbContext `User` entity config additions:

```csharp
b.Entity<User>(e =>
{
    e.ToTable("users");
    e.HasKey(x => x.Id);
    e.Property(x => x.Email).HasMaxLength(255).IsRequired();
    e.HasIndex(x => x.Email).IsUnique();
    e.Property(x => x.PasswordHash).HasColumnName("password_hash").HasMaxLength(200);
    e.Property(x => x.IsAdmin).HasColumnName("is_admin").HasDefaultValue(false);
});
```

#### Tests to Write FIRST

Extend `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs`
(if it doesn't exist under Data/, create it):

```csharp
[Fact]
public async Task User_password_hash_and_is_admin_round_trip()
{
    await using var scope = TestDb.CreateOpen();
    await scope.Db.Database.EnsureCreatedAsync();

    var u = new User
    {
        Id = Guid.NewGuid(),
        Email = "bootstrap@example.com",
        CreatedAt = DateTime.UtcNow,
        PasswordHash = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA==$aGFzaA==",
        IsAdmin = true,
    };
    scope.Db.Users.Add(u);
    await scope.Db.SaveChangesAsync();

    scope.Db.ChangeTracker.Clear();
    var back = await scope.Db.Users.SingleAsync(x => x.Id == u.Id);
    back.PasswordHash.Should().Be(u.PasswordHash);
    back.IsAdmin.Should().BeTrue();
}

[Fact]
public async Task User_is_admin_defaults_false()
{
    await using var scope = TestDb.CreateOpen();
    await scope.Db.Database.EnsureCreatedAsync();
    var u = new User { Id = Guid.NewGuid(), Email = "plain@example.com", CreatedAt = DateTime.UtcNow };
    scope.Db.Users.Add(u);
    await scope.Db.SaveChangesAsync();
    scope.Db.ChangeTracker.Clear();
    (await scope.Db.Users.SingleAsync(x => x.Id == u.Id)).IsAdmin.Should().BeFalse();
}
```

#### Impact on Existing Tests
- `CurrentUserAccessor` and its tests — unaffected, set `PasswordHash = null`
  by default.
- Any test creating a `User` by hand continues to compile (new columns are
  defaulted).

---

### Step 3: Add Npgsql provider and regenerate migrations

**Rationale:** Migrations must target Postgres for the observable to pass.
Done after the entity changes so the regenerated migrations capture the new
columns in one go.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/ApiTool.Backend.csproj` | modify | Add `Npgsql.EntityFrameworkCore.PostgreSQL` v9.0.1 |
| `src/ApiTool.Backend/Program.cs` | modify | Select Npgsql when `POSTGRES_HOST` is set, SQLite otherwise |
| `src/ApiTool.Backend/Migrations/*.cs` | delete + regenerate | `dotnet ef migrations add Initial` against Npgsql |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | regenerate | Npgsql-annotated snapshot |

#### New Code — DB registration in `Program.cs`

Replace the current `AddDbContext` block:

```csharp
// ── Database ──────────────────────────────────────────────────────────────
if (!builder.Environment.IsEnvironment("Testing"))
{
    var pgHost = builder.Configuration["POSTGRES_HOST"];
    if (!string.IsNullOrEmpty(pgHost))
    {
        var pgPort = builder.Configuration["POSTGRES_PORT"] ?? "5432";
        var pgDb   = builder.Configuration["POSTGRES_DB"] ?? "curlew";
        var pgUser = builder.Configuration["POSTGRES_USER"] ?? "curlew";
        var pgPass = builder.Configuration["POSTGRES_PASSWORD"] ?? "";
        var connectionString = $"Host={pgHost};Port={pgPort};Database={pgDb};Username={pgUser};Password={pgPass}";
        builder.Services.AddDbContext<AppDbContext>(options =>
            options.UseNpgsql(connectionString, npg => npg.MigrationsAssembly("ApiTool.Backend")));
    }
    else
    {
        builder.Services.AddDbContext<AppDbContext>(options =>
            options.UseSqlite(builder.Configuration.GetConnectionString("Default")
                ?? "Data Source=data/curlew.db"));
    }
}
```

#### Migration regeneration steps (manual, documented in plan)

```bash
cd src/ApiTool.Backend
rm -rf Migrations
ASPNETCORE_ENVIRONMENT=Development \
POSTGRES_HOST=localhost \
POSTGRES_DB=curlew_scaffold \
POSTGRES_USER=postgres \
POSTGRES_PASSWORD=postgres \
  dotnet ef migrations add Initial --context AppDbContext
```

To unblock this without a running Postgres, the dev can instead run against
the throwaway Postgres from the existing `docker-compose.test.yml`:
`./scripts/test-stack.sh up` exposes port 5432. The migration files are
model-only; EF Core does not connect to the DB to scaffold migrations as
long as the design-time services resolve cleanly.

#### Tests to Write FIRST

None specific to Npgsql in unit tests — tests continue to use SQLite via
`TestDb` and InMemory via `BackendFactory`. A new integration test in Step 4
(`MigrationRunnerIntegrationTests`) exercises `MigrateAsync()` semantics
against the SQLite test DB (SQLite supports `MigrateAsync` via the relational
migrator when a migration history table exists).

#### Impact on Existing Tests
- **All `[Collection(BackendCollection.Name)]` tests** — unaffected.
  `BackendFactory` uses `UseInMemoryDatabase`, which ignores the provider
  conditional in `Program.cs` (we already skip the block in Testing env).
- **`TestDb.CreateOpen()` tests** — unaffected, continue to use SQLite
  in-memory (explicit `UseSqlite` override).
- **Migrations folder tests** — none; no test reads the migrations folder.

---

### Step 4: `MigrationRunner` — startup task that applies pending migrations

**Rationale:** Migration application is a prerequisite for bootstrap; it runs
first and exits with code 4 on failure.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Bootstrap/MigrationRunner.cs` | create | `RunAsync` invokes `db.Database.MigrateAsync` with timeout + structured logs |
| `src/ApiTool.Backend/Bootstrap/BootstrapExitCodes.cs` | create | `MigrationsFailed = 4`, `BootstrapInvalid = 3` |
| `src/ApiTool.Backend.Tests/Bootstrap/MigrationRunnerTests.cs` | create | Happy path + skip path + DB unreachable |

#### Proposed Signatures

```csharp
namespace ApiTool.Backend.Bootstrap;

public static class BootstrapExitCodes
{
    public const int BootstrapInvalid = 3;
    public const int MigrationsFailed = 4;
}

public sealed class MigrationRunner(
    IServiceScopeFactory scopes,
    ILogger<MigrationRunner> logger,
    TimeProvider clock)
{
    /// <summary>
    /// Applies all pending EF migrations with a 60s default timeout. Returns
    /// the count applied, or throws <see cref="MigrationFailedException"/> on
    /// failure. Caller is responsible for translating the exception to an
    /// exit code (4). When <paramref name="run"/> is false, logs
    /// "migrations: skipped" and returns 0.
    /// </summary>
    public async Task<int> RunAsync(bool run, TimeSpan? timeout = null, CancellationToken ct = default);
}

public sealed class MigrationFailedException(string message, Exception inner)
    : Exception(message, inner);
```

Implementation:

```csharp
public async Task<int> RunAsync(bool run, TimeSpan? timeout = null, CancellationToken ct = default)
{
    if (!run)
    {
        logger.LogInformation("migrations: skipped");
        return 0;
    }
    using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
    cts.CancelAfter(timeout ?? TimeSpan.FromSeconds(60));
    await using var scope = scopes.CreateAsyncScope();
    var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
    try
    {
        var pending = await db.Database.GetPendingMigrationsAsync(cts.Token);
        var count = pending.Count();
        if (count == 0)
        {
            logger.LogInformation("migrations: applied 0 (up-to-date)");
            return 0;
        }
        await db.Database.MigrateAsync(cts.Token);
        logger.LogInformation("migrations: applied {Count}", count);
        return count;
    }
    catch (Exception ex) when (ex is not OperationCanceledException || !ct.IsCancellationRequested)
    {
        logger.LogError(ex, "migrations: failed");
        throw new MigrationFailedException("Failed to apply EF migrations", ex);
    }
}
```

#### Tests to Write FIRST

```csharp
public sealed class MigrationRunnerTests
{
    [Fact]
    public async Task RunAsync_run_false_logs_skipped_and_returns_zero()
    {
        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(StubScopes.Empty, sink, TimeProvider.System);
        var count = await runner.RunAsync(run: false);
        count.Should().Be(0);
        sink.Entries.Should().ContainSingle(e => e.Message.Contains("migrations: skipped"));
    }

    [Fact]
    public async Task RunAsync_with_empty_sqlite_applies_all_pending_migrations()
    {
        await using var scope = TestDb.CreateOpen();
        // ensure a clean DB without schema
        await scope.Db.Database.EnsureDeletedAsync();

        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(
            new SingletonScopeFactory(scope.Db), sink, TimeProvider.System);

        var count = await runner.RunAsync(run: true);
        count.Should().BeGreaterThan(0);
        sink.Entries.Should().Contain(e => e.Message.StartsWith("migrations: applied "));
    }

    [Fact]
    public async Task RunAsync_propagates_migration_failure_as_MigrationFailedException()
    {
        var brokenDb = BrokenDbContext.New(); // throws on MigrateAsync
        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(new SingletonScopeFactory(brokenDb), sink, TimeProvider.System);
        var act = async () => await runner.RunAsync(run: true);
        await act.Should().ThrowAsync<MigrationFailedException>();
        sink.Entries.Should().ContainSingle(e => e.Message == "migrations: failed");
    }

    [Fact]
    public async Task RunAsync_timeout_yields_MigrationFailedException()
    {
        var slowDb = SlowDbContext.New(TimeSpan.FromSeconds(5));
        var runner = new MigrationRunner(new SingletonScopeFactory(slowDb), new TestLogger<MigrationRunner>(), TimeProvider.System);
        var act = async () => await runner.RunAsync(run: true, timeout: TimeSpan.FromMilliseconds(50));
        await act.Should().ThrowAsync<MigrationFailedException>();
    }
}
```

Helper classes (`SingletonScopeFactory`, `TestLogger<T>`, `BrokenDbContext`,
`SlowDbContext`) live in
`src/ApiTool.Backend.Tests/Bootstrap/TestDoubles.cs`.

#### Impact on Existing Tests
- `Program.cs` Development auto-migrate block (line 286–291) — will be
  **removed** and replaced by the same call routed through `MigrationRunner`
  (so Development env continues to auto-migrate; production only does so
  when `BACKEND_RUN_MIGRATIONS=1`). No test exercises that block directly.

---

### Step 5: `AdminBootstrap` — startup task that seeds the admin user

**Rationale:** Depends on the password hasher (Step 1), the `User` schema
(Step 2), and an applied schema (Step 4). Orchestrates:
1. Validate `BOOTSTRAP_ADMIN_EMAIL` + `BOOTSTRAP_ADMIN_PASSWORD` pair.
2. Check for existing user with the email — skip if present.
3. Create `User { IsAdmin = true }`, create a "default" `Organization` owned
   by the admin, attach an Enterprise `Subscription`, add the admin as an
   `OrganizationMember { Role = Owner }`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Bootstrap/AdminBootstrap.cs` | create | `RunAsync` + validation |
| `src/ApiTool.Backend/Bootstrap/AdminBootstrapConfig.cs` | create | Record holding env vars + `FromEnvironment` static parser |
| `src/ApiTool.Backend.Tests/Bootstrap/AdminBootstrapTests.cs` | create | 6 behavior cases |

#### Proposed Signatures

```csharp
namespace ApiTool.Backend.Bootstrap;

public sealed record AdminBootstrapConfig(string Email, string Password)
{
    public const int MinPasswordLength = 12;

    /// <summary>
    /// Reads BOOTSTRAP_ADMIN_EMAIL/PASSWORD from configuration. Returns null
    /// when neither env var is set (i.e. bootstrap disabled), and a
    /// <see cref="BootstrapConfigError"/> when one is set but the pair is
    /// invalid.
    /// </summary>
    public static (AdminBootstrapConfig? Config, BootstrapConfigError? Error) FromConfiguration(IConfiguration cfg);
}

public enum BootstrapConfigError
{
    EmailMissing,
    PasswordMissing,
    PasswordTooShort,
    EmailInvalid,
}

public sealed class AdminBootstrap(
    IServiceScopeFactory scopes,
    PasswordHasher hasher,
    ILogger<AdminBootstrap> logger,
    TimeProvider clock)
{
    /// <summary>
    /// Returns true when an admin row was created, false when skipped
    /// (already exists). Throws <see cref="BootstrapValidationException"/>
    /// on bad config; caller converts to exit code 3.
    /// </summary>
    public async Task<bool> RunAsync(AdminBootstrapConfig config, CancellationToken ct = default);
}

public sealed class BootstrapValidationException(BootstrapConfigError reason, string message)
    : Exception(message)
{
    public BootstrapConfigError Reason { get; } = reason;
}
```

Behavior map:

- `FromConfiguration` — both blank → returns `(null, null)` (bootstrap
  disabled); one blank → returns the corresponding `*_Missing` error;
  password < 12 chars → `PasswordTooShort`; email fails basic regex
  → `EmailInvalid`.
- `RunAsync` —
  1. Open scope, load `AppDbContext`.
  2. `existing = Users.FirstOrDefaultAsync(u => u.Email == config.Email)`.
  3. If `existing is not null` → log
     `"bootstrap: admin already exists, skipping"` + return false.
  4. Else → create user (with `IsAdmin = true` + argon2 hash), create
     default org ("Default Org", slug `"default"`; if slug collision, append
     short suffix), create membership (role=Owner), create subscription
     (tier=Enterprise, status=Active, seat_count=1, seat_limit=1).
  5. Wrap in a transaction; log
     `"bootstrap: admin {email} created (org={slug})"` on success.

#### Tests to Write FIRST

```csharp
public sealed class AdminBootstrapTests
{
    [Fact]
    public void FromConfiguration_both_unset_returns_disabled()
    {
        var cfg = Build(new());
        var (c, err) = AdminBootstrapConfig.FromConfiguration(cfg);
        c.Should().BeNull();
        err.Should().BeNull();
    }

    [Theory]
    [InlineData("", "abcdefghijkl", BootstrapConfigError.EmailMissing)]
    [InlineData("admin@example.com", "", BootstrapConfigError.PasswordMissing)]
    [InlineData("admin@example.com", "tooshort", BootstrapConfigError.PasswordTooShort)]
    [InlineData("not-an-email", "abcdefghijklmnop", BootstrapConfigError.EmailInvalid)]
    public void FromConfiguration_partial_or_invalid_returns_error(
        string email, string password, BootstrapConfigError expected)
    {
        var cfg = Build(new()
        {
            ["BOOTSTRAP_ADMIN_EMAIL"] = email,
            ["BOOTSTRAP_ADMIN_PASSWORD"] = password,
        });
        var (_, err) = AdminBootstrapConfig.FromConfiguration(cfg);
        err.Should().Be(expected);
    }

    [Fact]
    public async Task RunAsync_empty_db_creates_admin_user_and_default_org()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();

        var boot = new AdminBootstrap(new SingletonScopeFactory(scope.Db),
            new PasswordHasher(), new TestLogger<AdminBootstrap>(), TimeProvider.System);
        var created = await boot.RunAsync(new("admin@example.com", "ChangeMe!Password"));
        created.Should().BeTrue();

        var user = await scope.Db.Users.SingleAsync();
        user.IsAdmin.Should().BeTrue();
        user.PasswordHash.Should().NotBeNullOrEmpty();
        (await scope.Db.Organizations.CountAsync()).Should().Be(1);
        (await scope.Db.OrganizationMembers.SingleAsync()).Role.Should().Be(OrgRole.Owner);
        (await scope.Db.Subscriptions.SingleAsync()).Tier.Should().Be(SubscriptionTier.Enterprise);
    }

    [Fact]
    public async Task RunAsync_existing_user_is_idempotent()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        scope.Db.Users.Add(new User { Id = Guid.NewGuid(), Email = "admin@example.com", CreatedAt = DateTime.UtcNow });
        await scope.Db.SaveChangesAsync();

        var sink = new TestLogger<AdminBootstrap>();
        var boot = new AdminBootstrap(new SingletonScopeFactory(scope.Db),
            new PasswordHasher(), sink, TimeProvider.System);
        var created = await boot.RunAsync(new("admin@example.com", "ChangeMe!Password"));
        created.Should().BeFalse();
        sink.Entries.Should().ContainSingle(e => e.Message.Contains("already exists, skipping"));
        (await scope.Db.Users.CountAsync()).Should().Be(1);
    }
}
```

#### Impact on Existing Tests
- None — no existing code touches the admin boot path.

---

### Step 6: Wire migration + bootstrap + exit codes into `Program.cs`

**Rationale:** Glues together the pieces from Steps 4–5 behind env var
guards. Must happen after the DbContext is registered and before
`app.Run()`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Program.cs` | modify | Register `PasswordHasher`, `MigrationRunner`, `AdminBootstrap` as DI singletons/scoped; call both before `app.Run()` |
| `src/ApiTool.Backend.Tests/Bootstrap/ProgramBootstrapIntegrationTests.cs` | create | (Optional) Smokes that `BackendFactory` starts with the bootstrap block neutralized in Testing env |

#### Current Code (lines 285–291)

```csharp
// ── Auto-migrate and Swagger in Development ───────────────────────────────
if (app.Environment.IsDevelopment())
{
    await using var scope = app.Services.CreateAsyncScope();
    var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
    await db.Database.MigrateAsync();
}
```

#### New Code (replaces the block above)

```csharp
// ── Startup: migrations + admin bootstrap (non-Testing only) ──────────────
if (!app.Environment.IsEnvironment("Testing"))
{
    // Determine whether migrations should run. Dev always auto-migrates for
    // developer ergonomics; Production is gated on BACKEND_RUN_MIGRATIONS=1.
    var runMigrations = app.Environment.IsDevelopment()
        || builder.Configuration["BACKEND_RUN_MIGRATIONS"] == "1";

    var migrator = new MigrationRunner(
        app.Services.GetRequiredService<IServiceScopeFactory>(),
        app.Services.GetRequiredService<ILogger<MigrationRunner>>(),
        TimeProvider.System);
    try
    {
        await migrator.RunAsync(runMigrations);
    }
    catch (MigrationFailedException)
    {
        // MigrationRunner has already emitted "migrations: failed".
        Environment.Exit(BootstrapExitCodes.MigrationsFailed);
        return;
    }

    // Bootstrap — only when both env vars are set.
    var (bootCfg, bootErr) = AdminBootstrapConfig.FromConfiguration(builder.Configuration);
    if (bootErr is { } err)
    {
        var msg = err switch
        {
            BootstrapConfigError.EmailMissing => "bootstrap: BOOTSTRAP_ADMIN_EMAIL is required when BOOTSTRAP_ADMIN_PASSWORD is set",
            BootstrapConfigError.PasswordMissing => "bootstrap: BOOTSTRAP_ADMIN_PASSWORD is required when BOOTSTRAP_ADMIN_EMAIL is set",
            BootstrapConfigError.PasswordTooShort => "bootstrap: password must be >=12 chars",
            BootstrapConfigError.EmailInvalid => "bootstrap: BOOTSTRAP_ADMIN_EMAIL is not a valid email address",
            _ => "bootstrap: invalid configuration",
        };
        app.Logger.LogError("{Message}", msg);
        Environment.Exit(BootstrapExitCodes.BootstrapInvalid);
        return;
    }
    if (bootCfg is not null)
    {
        var boot = new AdminBootstrap(
            app.Services.GetRequiredService<IServiceScopeFactory>(),
            app.Services.GetRequiredService<PasswordHasher>(),
            app.Services.GetRequiredService<ILogger<AdminBootstrap>>(),
            TimeProvider.System);
        await boot.RunAsync(bootCfg);
    }
}
```

Add DI registrations near the other singletons:

```csharp
builder.Services.AddSingleton<PasswordHasher>();
// MigrationRunner/AdminBootstrap are invoked directly, not via DI, so no
// service registration is needed — but register them so tests can resolve
// them via BackendFactory if desired:
builder.Services.AddSingleton<MigrationRunner>();
builder.Services.AddSingleton<AdminBootstrap>();
```

#### Impact on Existing Tests
- All `BackendFactory`-based tests — unaffected because the block is guarded
  `if (!app.Environment.IsEnvironment("Testing"))`.
- Development auto-migrate behaviour preserved (same `MigrateAsync` semantics
  now routed through `MigrationRunner.RunAsync(run: true)`).
- Tests that start `BackendFactory` must not fail due to `Environment.Exit`
  — verify by running the full test suite in Step 8.

---

### Step 7: `POST /api/v1/auth/login` endpoint

**Rationale:** The observable test curl-POSTs this endpoint. Cannot ship
without it. Depends on `PasswordHasher` (Step 1) and `SessionTokenIssuer`
(existing SSO code).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Auth/AuthLoginEndpoints.cs` | create | Maps `POST /api/v1/auth/login` |
| `src/ApiTool.Backend/Auth/LoginRequest.cs` | create | `{ email, password }` record |
| `src/ApiTool.Backend/Auth/LoginResponse.cs` | create | `{ access_token, token_type, expires_in, role }` record |
| `src/ApiTool.Backend/Program.cs` | modify | `app.MapAuthEndpoints()` |
| `src/ApiTool.Backend.Tests/Auth/AuthLoginEndpointsTests.cs` | create | Integration tests via BackendFactory |

#### Proposed Signatures

```csharp
public sealed record LoginRequest(string Email, string Password);
public sealed record LoginResponse(string AccessToken, string TokenType, int ExpiresIn, string Role);

public static class AuthLoginEndpoints
{
    public static IEndpointRouteBuilder MapAuthEndpoints(this IEndpointRouteBuilder app);
}
```

Handler skeleton:

```csharp
app.MapPost("/api/v1/auth/login", async (
    LoginRequest body,
    AppDbContext db,
    PasswordHasher hasher,
    SessionTokenIssuer issuer,
    CancellationToken ct) =>
{
    if (string.IsNullOrWhiteSpace(body?.Email) || string.IsNullOrWhiteSpace(body?.Password))
        return Results.Json(new ErrorResponse("invalid_credentials", "Email and password are required."),
            statusCode: StatusCodes.Status400BadRequest);

    var user = await db.Users.FirstOrDefaultAsync(u => u.Email == body.Email, ct);
    // Always run Verify even when user is null, to prevent user-enumeration via timing.
    var ok = user?.PasswordHash is { } ph
        ? hasher.Verify(body.Password, ph)
        : (hasher.Verify(body.Password, PlaceholderHash), false).Item2;
    if (user is null || user.PasswordHash is null || !ok)
        return Results.Json(new ErrorResponse("invalid_credentials", "Incorrect email or password."),
            statusCode: StatusCodes.Status401Unauthorized);

    var ttl = TimeSpan.FromHours(8);
    var token = issuer.IssueForUser(user.Id, user.Email, ttl);
    var role = user.IsAdmin ? "admin" : "member";
    return Results.Ok(new LoginResponse(token, "Bearer", (int)ttl.TotalSeconds, role));
})
.AllowAnonymous()
.DisableAntiforgery()
.WithName("AuthLogin")
.WithTags("Auth")
.Accepts<LoginRequest>("application/json")
.Produces<LoginResponse>()
.Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
.Produces<ErrorResponse>(StatusCodes.Status401Unauthorized);
```

`PlaceholderHash` is a pre-computed argon2id hash of a random string, held
as a `static readonly` field — prevents user-enumeration timing attacks.

#### Tests to Write FIRST

```csharp
[Collection(BackendCollection.Name)]
public sealed class AuthLoginEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    public AuthLoginEndpointsTests(BackendFactory f) => _factory = f;
    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Login_with_valid_admin_credentials_returns_200_and_access_token()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var hasher = new PasswordHasher();
        db.Users.Add(new User
        {
            Id = Guid.NewGuid(),
            Email = "login-admin@example.com",
            CreatedAt = DateTime.UtcNow,
            PasswordHash = hasher.Hash("ChangeMe!Password"),
            IsAdmin = true,
        });
        await db.SaveChangesAsync();

        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "login-admin@example.com", password = "ChangeMe!Password" });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("access_token").GetString().Should().NotBeNullOrEmpty();
        body.GetProperty("token_type").GetString().Should().Be("Bearer");
        body.GetProperty("role").GetString().Should().Be("admin");
    }

    [Fact]
    public async Task Login_with_unknown_email_returns_401_invalid_credentials()
    {
        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "nobody@example.com", password = "any-password-1234" });
        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Login_with_wrong_password_returns_401_invalid_credentials()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Users.Add(new User
        {
            Id = Guid.NewGuid(),
            Email = "pw-test@example.com",
            CreatedAt = DateTime.UtcNow,
            PasswordHash = new PasswordHasher().Hash("Correct!Password"),
            IsAdmin = false,
        });
        await db.SaveChangesAsync();

        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "pw-test@example.com", password = "Wrong!Password" });
        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Login_with_empty_body_returns_400_invalid_credentials()
    {
        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login", new { });
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Login_for_sso_only_user_without_password_returns_401()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Users.Add(new User
        {
            Id = Guid.NewGuid(),
            Email = "sso-only@example.com",
            CreatedAt = DateTime.UtcNow,
            PasswordHash = null,
        });
        await db.SaveChangesAsync();

        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "sso-only@example.com", password = "any-password-1234" });
        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }
}
```

#### Impact on Existing Tests
- `SwaggerTests` — the Swagger OpenAPI document will now include a
  `POST /api/v1/auth/login` operation. No existing test currently asserts
  the Swagger operation count, so no tests need updating. A new test in
  `SwaggerAuthSurfaceTests.cs` asserts the endpoint is listed.

---

### Step 8: Docker, docs, and smoke test

**Rationale:** Ties the new env-var surface into the existing deployment
surface; updates the docs; exercises the full path via the smoke script.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `deploy/self-hosted/docker-compose.yml` | modify | Add `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD` to backend env |
| `deploy/self-hosted/.env.example` | modify | Add the two new vars + flip `BACKEND_RUN_MIGRATIONS=1` default |
| `deploy/self-hosted/README.md` | modify | Document bootstrap behavior, exit codes, login endpoint, backup note |
| `scripts/test-self-hosted.sh` | modify | Exercise bootstrap + login path after stack is up |
| `src/ApiTool.Backend/Dockerfile` | modify | Keep as-is; no changes needed (already uses `ASPNETCORE_URLS=0.0.0.0:5000`) |
| `CHANGELOG.md` | modify | `Added` entry for M5-016 |

#### Updated `.env.example`

```dotenv
# EF Core migrations on startup — required for Postgres provider
BACKEND_RUN_MIGRATIONS=1

# First-boot admin bootstrap (both must be set together; password >=12 chars)
BOOTSTRAP_ADMIN_EMAIL=
BOOTSTRAP_ADMIN_PASSWORD=
```

#### Updated `docker-compose.yml` backend env:

```yaml
BACKEND_RUN_MIGRATIONS: ${BACKEND_RUN_MIGRATIONS}
BOOTSTRAP_ADMIN_EMAIL: ${BOOTSTRAP_ADMIN_EMAIL}
BOOTSTRAP_ADMIN_PASSWORD: ${BOOTSTRAP_ADMIN_PASSWORD}
```

#### Smoke test additions (bash snippets appended to `test-self-hosted.sh`)

```bash
echo "--- Bootstrap: first-boot admin login ---"
docker compose -f docker-compose.yml --env-file .env down -v >/dev/null
cat >> .env <<EOF
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=ChangeMe!Password
EOF
docker compose -f docker-compose.yml --env-file .env up -d --build
# wait for /health healthy as before
for i in $(seq 1 90); do
  curl -fsS http://localhost:5000/health 2>/dev/null | grep -q '"status":"healthy"' && break
  sleep 1
done
BODY=$(curl -sS -X POST -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"ChangeMe!Password"}' \
  http://localhost:5000/api/v1/auth/login)
echo "$BODY" | grep -q '"access_token"' || { echo "FAIL: no access_token — $BODY" >&2; exit 1; }
echo "$BODY" | grep -q '"role":"admin"'  || { echo "FAIL: role != admin — $BODY" >&2; exit 1; }
echo "PASS: bootstrap admin login"

echo "--- Bootstrap: idempotent on restart ---"
docker compose -f docker-compose.yml --env-file .env restart backend
for i in $(seq 1 90); do
  curl -fsS http://localhost:5000/health 2>/dev/null | grep -q '"status":"healthy"' && break
  sleep 1
done
docker compose -f docker-compose.yml --env-file .env logs backend | grep -q 'bootstrap: admin already exists, skipping' \
  || { echo "FAIL: idempotent skip log missing" >&2; exit 1; }
echo "PASS: bootstrap idempotent"
```

#### Impact on Existing Tests
- `scripts/test-self-hosted.sh` is opt-in via `CURLEW_RUN_SELF_HOSTED=1`;
  the additional ~30s cost is only paid by contributors running the self-
  hosted gate. CI behavior is unchanged unless the env var is set.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | `Organizations_slug_is_unique` | none | — |
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | new `User_password_hash_and_is_admin_round_trip` | new | write in Step 2 |
| `src/ApiTool.Backend.Tests/Sso/*Tests.cs` | all | none | — (BackendFactory skips bootstrap) |
| `src/ApiTool.Backend.Tests/Auth/PasswordHasherTests.cs` | new file, 4 cases | new | write in Step 1 |
| `src/ApiTool.Backend.Tests/Bootstrap/MigrationRunnerTests.cs` | new file, 4 cases | new | write in Step 4 |
| `src/ApiTool.Backend.Tests/Bootstrap/AdminBootstrapTests.cs` | new file, 6 cases | new | write in Step 5 |
| `src/ApiTool.Backend.Tests/Auth/AuthLoginEndpointsTests.cs` | new file, 5 cases | new | write in Step 7 |
| `src/ApiTool.Backend.Tests/SmokeTests.cs` | `Program_type_is_discoverable...` | none | — |
| All other existing tests | — | none | — (Testing env skips the startup block) |

**Total new tests: 23** across 4 new test files.

## Risks and Edge Cases

- **Risk:** Regenerating migrations drops the historical chain; if a user
  has an existing dev SQLite file (`data/curlew-dev.db`) they'll get
  `PendingModelChangesWarning` or a crash on the next `MigrateAsync`.
  **Mitigation:** The Dev auto-migrate block now runs `MigrateAsync` which
  applies the new `Initial` migration against an empty DB. Dev databases
  should be blown away (documented in CHANGELOG). Production has no existing
  databases since M5-015 explicitly says Postgres is "not yet consumed".

- **Risk:** `Environment.Exit(3)` / `Environment.Exit(4)` aborts the whole
  process including pending disposals; any incomplete transactions are
  rolled back by Postgres naturally, but logs may be dropped if the
  `ILogger` is buffered.
  **Mitigation:** Use `logger.LogError` before `Environment.Exit`; ASP.NET
  Core's default console logger is unbuffered.

- **Risk:** Password-less bootstrap (user sets only `BOOTSTRAP_ADMIN_EMAIL`)
  on restart would otherwise silently succeed.
  **Mitigation:** `FromConfiguration` returns `EmailMissing`/`PasswordMissing`
  when one is set and the other is blank — exit code 3 with clear log.

- **Risk:** Timing-based user-enumeration attack on `/api/v1/auth/login`
  (fast 401 when user doesn't exist, slow 401 when it does).
  **Mitigation:** Always run `PasswordHasher.Verify` against a static
  placeholder hash on the miss path; discard the result.

- **Risk:** Brute-force against `/api/v1/auth/login`.
  **Mitigation:** Add rate-limit policy `auth-login` (5 requests/minute per
  email) analogous to `checkout-create`. Optional for first shipment —
  covered by the spec-level recommendation; add if easy, else defer.

- **Edge case:** `BOOTSTRAP_ADMIN_EMAIL` casing differs between runs
  ("Admin@Example.com" vs "admin@example.com"). Existing `users.Email`
  column is case-sensitive under Npgsql default collation.
  **Handling:** `FromConfiguration` lowercases the email before persisting;
  validation regex disallows uppercase. Document in README.

- **Edge case:** Bootstrap runs while migrations are still applying on a
  slow DB. Can't happen because migration runs synchronously before
  bootstrap in `Program.cs`.

- **Edge case:** Migration succeeds but bootstrap crashes mid-transaction
  (e.g. DB dropped). Transaction rolls back — admin is not created. Next
  restart retries the bootstrap. This is the desired behavior.

- **Edge case:** `BACKEND_RUN_MIGRATIONS=0` against a schema-less Postgres.
  The first DB-backed endpoint (e.g. `POST /api/v1/organizations`) returns
  500, not the 503 the behavior requests. The healthcheck's
  `CanConnectAsync` succeeds because Postgres is up. To satisfy behavior
  #7, add an endpoint-level guard: `HealthService.CheckAsync` now calls
  `GetAppliedMigrationsAsync` and reports `"db":"schema_missing"` when zero
  migrations are applied. Any endpoint hitting a missing table gets 503 via
  a middleware that catches Npgsql `42P01 undefined_table` errors.
  **Handling:** Add a minimal `SchemaGuardMiddleware` in the same commit as
  the endpoint, translating Npgsql `42P01` into
  `503 { "code": "service_unavailable", "message": "Database schema not applied. Set BACKEND_RUN_MIGRATIONS=1." }`.
  For tests, use SQLite error code 1 ("no such table") mapped the same way.

- **Edge case:** `Dockerfile` currently `ENTRYPOINT ["dotnet", "ApiTool.Backend.dll"]` and has `WORKDIR /app`. On `Environment.Exit(3/4)`, Docker restarts the container per its restart policy. Compose's default is "no"; the self-hosted compose doesn't set a restart policy, so a bad bootstrap doesn't loop.
  **Handling:** Keep as-is; failed bootstraps remain visible in `docker compose logs backend`. Document in README troubleshooting.

## Proposed Go Function Signatures

Not applicable — this task is entirely in C# / .NET backend. Go CLI is
untouched by this task.

## Verification

```bash
# Unit/integration tests — the authoritative observable
cd /Users/peterlindqvist/kod/active/Curlew
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Bootstrap|FullyQualifiedName~Migrations|FullyQualifiedName~PasswordHasher|FullyQualifiedName~AuthLogin"

# Full backend test suite
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj

# Local CI gate — detects backend changes and runs dotnet tests
./scripts/ci-local.sh

# Smoke — requires Docker
CURLEW_RUN_SELF_HOSTED=1 ./scripts/test-self-hosted.sh
```

Observable verification (mirrors task YAML):

```bash
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Bootstrap|FullyQualifiedName~Migrations"
# Expected: Passed: >=8, Failed: 0

cd deploy/self-hosted
cp .env.example .env
# Set real secrets + bootstrap vars:
sed -i '' 's/^BOOTSTRAP_ADMIN_EMAIL=.*/BOOTSTRAP_ADMIN_EMAIL=admin@example.com/' .env
sed -i '' 's/^BOOTSTRAP_ADMIN_PASSWORD=.*/BOOTSTRAP_ADMIN_PASSWORD=ChangeMe!Password/' .env
docker compose up -d postgres redis
sleep 5
BACKEND_RUN_MIGRATIONS=1 docker compose up -d backend
sleep 10
curl -sS -X POST -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"ChangeMe!Password"}' \
  http://localhost:5000/api/v1/auth/login
# Expected HTTP 200, body contains "access_token":"..." and "role":"admin"
docker compose down -v
```
