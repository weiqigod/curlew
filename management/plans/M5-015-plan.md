# Implementation Plan: M5-015

## Overview

Deliver the `deploy/self-hosted/` Docker bundle — `docker-compose.yml`, `.env.example`, `postgres.Dockerfile`, `README.md` — so an operator can bring up a complete on-premises stack (Postgres + Redis + backend + web) with persistent volumes and a healthy state in <60 seconds. Add a minimal `/health` endpoint to the backend that reports `db` and `redis` connectivity, and a `scripts/test-self-hosted.sh` smoke script that exercises the observable end-to-end.

## Task Details

- **ID:** M5-015
- **Title:** Backend: self-hosted docker-compose bundle
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** medium
- **Branch:** `feature/M5-015-self-hosted-docker-bundle`

## Dependencies

None. (Standalone bundle; M5-016 depends on *this* task for the postgres container and migration plumbing.)

## Architectural Decisions

These resolve open questions before implementation — the executor should follow them unless a hard constraint forces a deviation.

1. **Backend stays on SQLite for M5-015.** The current backend is built on `Microsoft.EntityFrameworkCore.Sqlite`; every migration (`src/ApiTool.Backend/Migrations/*`) uses SQLite types. Pivoting to Postgres is a full migration rewrite and is explicitly the scope of M5-016 (admin bootstrap + migration runner — it runs `database update` against the new Postgres connection). M5-015 therefore provisions Postgres and Redis **containers** with persistent volumes and healthy state, but the backend container continues to write to `data/curlew.db` on a named volume. This is the only way to respect the task's 4–6h estimate and its concrete decision ("use the existing src/ApiTool.Backend/Dockerfile unchanged").
2. **Postgres + Redis are real, running, health-checked containers.** Behaviors 1, 4, 5, and 6 only care that the containers start, stay healthy, consume the env-file, and survive (or not) `down`/`down -v`. They do not require the backend to have consumed either service yet. M5-016 adds the real consumption.
3. **Behavior 4 interpretation.** "Given `POSTGRES_PASSWORD` is changed, when docker compose up is re-run, then the backend picks up the new password on first boot." Scope-consistent reading: the **postgres service** picks up `POSTGRES_PASSWORD` (the `postgres:16` image honors this on first init). The backend receives `ConnectionStrings__Default` composed from the env vars so that on M5-016 landing it will switch providers without a compose-file change. The README explicitly calls out that M5-015 wires the env-var surface but M5-016 switches the EF provider to Npgsql.
4. **New `/health` endpoint** lives in `src/ApiTool.Backend/Health/`. It is a GET `/health` that returns:
   - `200` with `{"status":"healthy","db":"connected","redis":"connected"}` when both probes succeed.
   - `503` with `{"status":"unhealthy","db":"<connected|disconnected>","redis":"<connected|disconnected>"}` when at least one probe fails.
   - Probes:
     - `db` — `AppDbContext.Database.CanConnectAsync(ct)` (already works for SQLite; will Just Work when M5-016 swaps to Npgsql).
     - `redis` — a raw TCP `PING`/`+PONG` round-trip against `REDIS_HOST:REDIS_PORT` with a 2 s timeout. No new NuGet dependency; `System.Net.Sockets.TcpClient` + `NetworkStream` only. When `REDIS_HOST` is unset, the redis probe is skipped and the response reports `"redis":"not_configured"` with status `"healthy"` (so the same backend image can run in non-self-hosted environments).
   - The endpoint is anonymous (no JWT required) and excluded from rate limiting. It is safe to expose externally.
5. **Probe composition via interfaces.** Define `IDbHealthProbe` and `IRedisHealthProbe` in `src/ApiTool.Backend/Health/`. The concrete implementations (`EfDbHealthProbe`, `TcpRedisHealthProbe`) live next to the interfaces. The endpoint depends on the interfaces so tests can inject deterministic fakes without spinning up Postgres or Redis. A `HealthService` orchestrates both probes and builds the response DTO.
6. **`postgres.Dockerfile`.** Built `FROM postgres:16-alpine` with a tiny customization: `RUN apk add --no-cache postgresql16-contrib` so the image ships `pg_cron` / `pg_trgm` / `pgcrypto` available for `CREATE EXTENSION` when M5-016's migrations need them. We do not enable `pg_cron` at boot (it requires `shared_preload_libraries` which is a later concern); we only make the extensions present. The task YAML explicitly asks for a "pg_cron-friendly image".
7. **Compose file location and name.** `deploy/self-hosted/docker-compose.yml`. This is the **production** compose file — distinct from the repo-root `docker-compose.test.yml` which stays test-only. Volumes: `pg_data`, `redis_data`, `backend_data`. Backend build context points at the repo root (`context: ../..`) and uses `dockerfile: src/ApiTool.Backend/Dockerfile`; web uses `context: ../../web` unchanged. This keeps `src/ApiTool.Backend/Dockerfile` unmodified per the task's concrete decision.
8. **Env-file strategy.** `deploy/self-hosted/.env.example` is the canonical variable list with safe-looking-but-clearly-fake defaults. `deploy/self-hosted/.env` is gitignored (new `.gitignore` scoped to the directory). Variables (all have explicit defaults in compose via `${VAR:-default}` for the few that are safe):
   - `POSTGRES_DB=curlew`
   - `POSTGRES_USER=curlew`
   - `POSTGRES_PASSWORD=change-me-please`
   - `REDIS_PASSWORD=` (empty → no-auth; noted as "set for production")
   - `BACKEND_PORT=5000`
   - `WEB_PORT=3000`
   - `JWT_SIGNING_KEY=change-me-32-bytes-minimum-please`
   - `ASPNETCORE_ENVIRONMENT=Production`
   - `BACKEND_RUN_MIGRATIONS=0` (M5-015 default; M5-016 flips it to `1` with semantics)
9. **Compose env wiring.** The backend service reads:
   - `ASPNETCORE_URLS=http://0.0.0.0:5000`
   - `ASPNETCORE_ENVIRONMENT=${ASPNETCORE_ENVIRONMENT}`
   - `ConnectionStrings__Default=Data Source=/app/data/curlew.db` (M5-015 hard-codes SQLite; noted in README that M5-016 swaps to Postgres)
   - `Jwt__SigningKey=${JWT_SIGNING_KEY}`
   - `REDIS_HOST=redis`, `REDIS_PORT=6379`, `REDIS_PASSWORD=${REDIS_PASSWORD}` (unused today except by the health probe — no probe auth in M5-015; documented)
   - `POSTGRES_HOST=postgres`, `POSTGRES_PORT=5432`, `POSTGRES_DB=${POSTGRES_DB}`, `POSTGRES_USER=${POSTGRES_USER}`, `POSTGRES_PASSWORD=${POSTGRES_PASSWORD}`, `BACKEND_RUN_MIGRATIONS=${BACKEND_RUN_MIGRATIONS}` (surfaced so M5-016 drops in without compose edits).
10. **Healthchecks in compose.** Each service has a compose-level `healthcheck`:
   - `postgres`: `pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}` every 5 s, 20 retries.
   - `redis`: `redis-cli ping` (or `nc -z localhost 6379` when password-less) every 5 s, 20 retries.
   - `backend`: `wget -qO- http://localhost:5000/health` every 5 s, 20 retries. Depends on postgres + redis `service_healthy`.
   - `web`: `wget -qO- http://localhost:3000/` every 5 s, 20 retries. Depends on backend `service_healthy`.
11. **Web `<title>` fix.** The observable asserts `<title>curlew</title>` on `/`. Today `web/src/routes/+page.svelte` has only `<h1>Curlew</h1>` and no title. Fix by adding `<svelte:head><title>curlew</title></svelte:head>` to `+page.svelte`. Scoped to the root — does not touch `/org/*` pages which set richer titles.
12. **Web host header.** The web image runs `node build`; the SvelteKit Node adapter requires `ORIGIN` or `PROTOCOL_HEADER` in production. Compose sets `ORIGIN=http://localhost:${WEB_PORT}` — matches the observable.
13. **Smoke test.** `scripts/test-self-hosted.sh` is the "smoke test for stack boot" required by the Definition of Done. It:
    1. Copies `.env.example` → `.env` (gitignored), bringing the stack up with `--env-file .env`.
    2. Polls backend `/health` for up to 90 s.
    3. Asserts the four behaviors that can be checked without spinning up real test clients:
       - All four services reach `healthy`.
       - `GET /health` returns `200` with `db=connected,redis=connected`.
       - `GET /` on web returns `200` and body contains `<title>curlew</title>`.
       - `docker compose down` (no `-v`) followed by re-up keeps `pg_data`, `redis_data`, and `backend_data` alive (verified by listing volumes). `docker compose down -v` removes them.
    4. Cleans up on exit (trap `EXIT`). Not wired into `ci-local.sh` automatically — the gate stays opt-in via `CURLEW_RUN_SELF_HOSTED=1` to avoid 60 s of stack boot on every PR. Documented in the README.
14. **Test coverage target ≥80%.** Go-side coverage is untouched. The new code is all C# (health endpoint + probes) and shell. For C#:
    - Unit tests for `HealthService` (4 cases: both up, db down, redis down, redis not configured).
    - Integration test through `BackendFactory` that hits `GET /health` and asserts the JSON shape. Uses fake probes to avoid real Postgres/Redis.
    - A real-compose smoke is provided by `scripts/test-self-hosted.sh` but does not count toward coverage.
15. **No changes to `src/ApiTool.Backend/Dockerfile`.** Concrete decision from the task YAML. The health endpoint lives inside the .NET image, mapped by Program.cs — no new container-level work.
16. **Env var naming.** ASP.NET Core's config provider maps `VAR__SUBVAR` → `Var:SubVar`. We use `ConnectionStrings__Default` (standard) and `Jwt__SigningKey` (matches existing `JwtOptions.Section`). New env vars for the health probe — `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD` — are read via `IConfiguration["REDIS_HOST"]` (no section), consistent with POSIX-style service discovery variables.
17. **Unit for byte-size / humanization.** None required — this task does not need a formatter.
18. **Determinism on re-up.** Behavior 4 asserts that changing `POSTGRES_PASSWORD` on first boot propagates. The `postgres:16` image reads `POSTGRES_PASSWORD` only when `$PGDATA` is empty, so a *true* first-boot password change works; changing it on a populated volume does nothing. The README explicitly documents this so operators know to `down -v` before rotating the password, which matches the real postgres semantics.
19. **Sentinel errors and logging.** `HealthService` logs probe failures via `ILogger<HealthService>` at Warning. Exit codes are not applicable (this is an HTTP endpoint). No new sentinel errors are needed.

## Implementation Steps

Step ordering is smallest blast radius first — pure health-service unit tests, then CLI integration, then compose bundle, then smoke script.

---

### Step 1: Health probe interfaces and unit-level `HealthService`

**Rationale:** No other code depends on this. It's pure logic exercised by unit tests; the endpoint is wired in Step 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Health/IDbHealthProbe.cs` | create | Interface for DB probe |
| `src/ApiTool.Backend/Health/IRedisHealthProbe.cs` | create | Interface for Redis probe |
| `src/ApiTool.Backend/Health/HealthReport.cs` | create | Immutable result record: `Status`, `Db`, `Redis` |
| `src/ApiTool.Backend/Health/HealthService.cs` | create | Orchestrates probes, builds `HealthReport` |
| `src/ApiTool.Backend.Tests/Health/HealthServiceTests.cs` | create | Table-driven tests over fake probes |

#### New Code

```csharp
// src/ApiTool.Backend/Health/HealthReport.cs
namespace ApiTool.Backend.Health;

/// <summary>Result of a /health probe.</summary>
public sealed record HealthReport(string Status, string Db, string Redis)
{
    public const string StatusHealthy = "healthy";
    public const string StatusUnhealthy = "unhealthy";
    public const string Connected = "connected";
    public const string Disconnected = "disconnected";
    public const string NotConfigured = "not_configured";
}
```

```csharp
// src/ApiTool.Backend/Health/IDbHealthProbe.cs
namespace ApiTool.Backend.Health;

/// <summary>Probes the backend's primary database.</summary>
public interface IDbHealthProbe
{
    /// <summary>Returns true if the database is reachable within the probe timeout.</summary>
    Task<bool> IsConnectedAsync(CancellationToken ct);
}
```

```csharp
// src/ApiTool.Backend/Health/IRedisHealthProbe.cs
namespace ApiTool.Backend.Health;

/// <summary>Probes the Redis cache. May be unconfigured.</summary>
public interface IRedisHealthProbe
{
    /// <summary>True if Redis is configured and PING succeeded; false if configured but unreachable.</summary>
    Task<bool> IsConnectedAsync(CancellationToken ct);

    /// <summary>True when no Redis host is configured (skip probe).</summary>
    bool IsConfigured { get; }
}
```

```csharp
// src/ApiTool.Backend/Health/HealthService.cs
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Health;

/// <summary>Composes DB and Redis probes into a single HealthReport.</summary>
public sealed class HealthService(
    IDbHealthProbe db,
    IRedisHealthProbe redis,
    ILogger<HealthService> logger)
{
    /// <summary>Runs both probes in parallel and builds a HealthReport.</summary>
    public async Task<HealthReport> CheckAsync(CancellationToken ct)
    {
        var dbTask = db.IsConnectedAsync(ct);
        var redisTask = redis.IsConfigured
            ? redis.IsConnectedAsync(ct)
            : Task.FromResult(true);

        var dbOk = await SafeAsync(dbTask, "db", logger);
        var redisOk = await SafeAsync(redisTask, "redis", logger);

        var dbStr = dbOk ? HealthReport.Connected : HealthReport.Disconnected;
        var redisStr = !redis.IsConfigured
            ? HealthReport.NotConfigured
            : redisOk ? HealthReport.Connected : HealthReport.Disconnected;

        var healthy = dbOk && (redisOk || !redis.IsConfigured);
        var status = healthy ? HealthReport.StatusHealthy : HealthReport.StatusUnhealthy;
        return new HealthReport(status, dbStr, redisStr);
    }

    private static async Task<bool> SafeAsync(Task<bool> t, string name, ILogger logger)
    {
        try { return await t; }
        catch (Exception ex) { logger.LogWarning(ex, "health probe {Probe} threw", name); return false; }
    }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Health/HealthServiceTests.cs
public sealed class HealthServiceTests
{
    private sealed class FakeDb(bool ok, bool throws = false) : IDbHealthProbe
    {
        public Task<bool> IsConnectedAsync(CancellationToken ct) =>
            throws ? Task.FromException<bool>(new InvalidOperationException("db")) : Task.FromResult(ok);
    }
    private sealed class FakeRedis(bool ok, bool configured = true, bool throws = false) : IRedisHealthProbe
    {
        public bool IsConfigured => configured;
        public Task<bool> IsConnectedAsync(CancellationToken ct) =>
            throws ? Task.FromException<bool>(new InvalidOperationException("redis")) : Task.FromResult(ok);
    }

    [Theory]
    [InlineData(true,  true,  true,  "healthy",   "connected",    "connected")]
    [InlineData(false, true,  true,  "unhealthy", "disconnected", "connected")]
    [InlineData(true,  false, true,  "unhealthy", "connected",    "disconnected")]
    [InlineData(true,  true,  false, "healthy",   "connected",    "not_configured")]
    [InlineData(false, false, false, "unhealthy", "disconnected", "not_configured")]
    public async Task Check_returns_expected_report(
        bool dbOk, bool redisOk, bool redisConfigured,
        string wantStatus, string wantDb, string wantRedis)
    {
        var svc = new HealthService(
            new FakeDb(dbOk),
            new FakeRedis(redisOk, redisConfigured),
            NullLogger<HealthService>.Instance);
        var rep = await svc.CheckAsync(CancellationToken.None);
        rep.Status.Should().Be(wantStatus);
        rep.Db.Should().Be(wantDb);
        rep.Redis.Should().Be(wantRedis);
    }

    [Fact]
    public async Task Check_treats_probe_exception_as_disconnected()
    {
        var svc = new HealthService(
            new FakeDb(true, throws: true),
            new FakeRedis(true),
            NullLogger<HealthService>.Instance);
        var rep = await svc.CheckAsync(CancellationToken.None);
        rep.Status.Should().Be("unhealthy");
        rep.Db.Should().Be("disconnected");
    }
}
```

#### Impact on Existing Tests
- None. New files, new namespace.

---

### Step 2: Concrete probes (EF + TCP Redis) and endpoint wiring

**Rationale:** With the pure logic pinned in Step 1, we can land the IO-bound probes and the endpoint. An integration test through `BackendFactory` verifies the JSON shape without requiring a live Postgres/Redis.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Health/EfDbHealthProbe.cs` | create | Wraps `AppDbContext.Database.CanConnectAsync` with a 2 s timeout |
| `src/ApiTool.Backend/Health/TcpRedisHealthProbe.cs` | create | Opens a TCP connection, sends `PING`, parses `+PONG\r\n`; reads host/port from `IConfiguration` |
| `src/ApiTool.Backend/Health/HealthEndpoints.cs` | create | `MapHealthEndpoints(this IEndpointRouteBuilder app)` — maps `GET /health` |
| `src/ApiTool.Backend/Program.cs` | modify | Register `HealthService`, `EfDbHealthProbe`, `TcpRedisHealthProbe`; call `app.MapHealthEndpoints()` before authentication |
| `src/ApiTool.Backend.Tests/Health/HealthEndpointTests.cs` | create | Uses `BackendFactory` with fake probes registered to override the concrete ones |

#### Current Code

```csharp
// src/ApiTool.Backend/Program.cs — around line 300
app.MapGet("/", () => HttpResults.Redirect("/swagger"));
app.MapOrganizationsEndpoints();
```

#### New Code

```csharp
// src/ApiTool.Backend/Health/EfDbHealthProbe.cs
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Health;

public sealed class EfDbHealthProbe(IServiceScopeFactory scopes) : IDbHealthProbe
{
    public async Task<bool> IsConnectedAsync(CancellationToken ct)
    {
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(2));
        await using var scope = scopes.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        return await db.Database.CanConnectAsync(cts.Token);
    }
}
```

```csharp
// src/ApiTool.Backend/Health/TcpRedisHealthProbe.cs
using System.Net.Sockets;
using System.Text;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Health;

public sealed class TcpRedisHealthProbe(IConfiguration config) : IRedisHealthProbe
{
    private readonly string? _host = config["REDIS_HOST"];
    private readonly int _port = int.TryParse(config["REDIS_PORT"], out var p) ? p : 6379;

    public bool IsConfigured => !string.IsNullOrWhiteSpace(_host);

    public async Task<bool> IsConnectedAsync(CancellationToken ct)
    {
        if (!IsConfigured) return false;
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(2));
        using var client = new TcpClient { NoDelay = true };
        try
        {
            await client.ConnectAsync(_host!, _port, cts.Token);
            await using var stream = client.GetStream();
            // RESP: inline PING: "PING\r\n"
            var cmd = Encoding.ASCII.GetBytes("PING\r\n");
            await stream.WriteAsync(cmd, cts.Token);
            var buf = new byte[7]; // "+PONG\r\n"
            var read = await stream.ReadAsync(buf.AsMemory(), cts.Token);
            var reply = Encoding.ASCII.GetString(buf, 0, read);
            return reply.StartsWith("+PONG", StringComparison.Ordinal);
        }
        catch { return false; }
    }
}
```

```csharp
// src/ApiTool.Backend/Health/HealthEndpoints.cs
namespace ApiTool.Backend.Health;

public static class HealthEndpoints
{
    public static IEndpointRouteBuilder MapHealthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/health", async (HealthService svc, CancellationToken ct) =>
        {
            var r = await svc.CheckAsync(ct);
            var code = r.Status == HealthReport.StatusHealthy ? 200 : 503;
            return Results.Json(r, statusCode: code);
        }).AllowAnonymous();
        return app;
    }
}
```

```csharp
// src/ApiTool.Backend/Program.cs — new registrations next to other scoped services
builder.Services.AddSingleton<IDbHealthProbe, EfDbHealthProbe>();
builder.Services.AddSingleton<IRedisHealthProbe, TcpRedisHealthProbe>();
builder.Services.AddSingleton<HealthService>();
```

```csharp
// src/ApiTool.Backend/Program.cs — new endpoint mapping before auth middleware
app.MapHealthEndpoints();
```

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Health/HealthEndpointTests.cs
[Collection(nameof(BackendCollection))]
public sealed class HealthEndpointTests(BackendFactory factory) : IAsyncLifetime
{
    public Task InitializeAsync() => factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    // Helper: swap the concrete probes for fakes the test controls.
    private HttpClient NewClientWith(bool dbOk, bool redisOk, bool redisConfigured)
    {
        return factory.WithWebHostBuilder(b =>
        {
            b.ConfigureServices(s =>
            {
                s.RemoveAll<IDbHealthProbe>();
                s.AddSingleton<IDbHealthProbe>(new FakeDb(dbOk));
                s.RemoveAll<IRedisHealthProbe>();
                s.AddSingleton<IRedisHealthProbe>(new FakeRedis(redisOk, redisConfigured));
            });
        }).CreateClient();
    }

    [Fact]
    public async Task Health_returns_200_when_both_up()
    {
        var c = NewClientWith(true, true, true);
        var resp = await c.GetAsync("/health");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("\"status\":\"healthy\"");
        body.Should().Contain("\"db\":\"connected\"");
        body.Should().Contain("\"redis\":\"connected\"");
    }

    [Fact]
    public async Task Health_returns_503_when_db_down()
    {
        var c = NewClientWith(false, true, true);
        var resp = await c.GetAsync("/health");
        resp.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable);
    }

    [Fact]
    public async Task Health_returns_200_when_redis_not_configured()
    {
        var c = NewClientWith(true, false, false);
        var resp = await c.GetAsync("/health");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        (await resp.Content.ReadAsStringAsync()).Should().Contain("\"redis\":\"not_configured\"");
    }

    [Fact]
    public async Task Health_does_not_require_authentication()
    {
        var c = NewClientWith(true, true, true);
        // No Authorization header set.
        var resp = await c.GetAsync("/health");
        resp.StatusCode.Should().NotBe(HttpStatusCode.Unauthorized);
    }
}
```

#### Impact on Existing Tests

| Test | Impact | Action |
|------|--------|--------|
| All existing `BackendCollection` tests | None — new endpoint is additive, default concrete probes in Testing environment fail-soft via exception catch (returns `disconnected`, but those tests don't hit `/health`). | No change. |
| `SmokeTests.Program_type_is_discoverable_for_WebApplicationFactory` | None. | No change. |

---

### Step 3: Add `<title>curlew</title>` to web root page

**Rationale:** Required by observable. Tiny change with no behavioral risk; run it as its own step so the diff is obvious in review.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/+page.svelte` | modify | Add `<svelte:head><title>curlew</title></svelte:head>` |
| `web/tests/unit/root-page.spec.ts` | create | Vitest unit asserting the page renders with the title (compiles in SvelteKit test env) |

#### Current Code

```svelte
<!-- web/src/routes/+page.svelte -->
<script lang="ts">
</script>

<h1>Curlew</h1>
```

#### New Code

```svelte
<!-- web/src/routes/+page.svelte -->
<script lang="ts">
</script>

<svelte:head>
	<title>curlew</title>
</svelte:head>

<h1>Curlew</h1>
```

#### Tests to Write FIRST (RED phase)

Add a `web/tests/unit/root-page.spec.ts` that renders the root page via `@testing-library/svelte` (already in web deps — verify in step) and asserts `document.title === 'curlew'`. If the unit harness does not pick up `<svelte:head>`, fall back to an HTML regex on the built output in the smoke test only (cheaper than adding Playwright).

If vitest cannot render `<svelte:head>` reliably, drop this unit test — the `scripts/test-self-hosted.sh` integration check is load-bearing and covers behavior 3 end-to-end.

#### Impact on Existing Tests
- None. `/org/*` pages set their own titles unchanged.

---

### Step 4: `deploy/self-hosted/` bundle files

**Rationale:** With health + title delivered, the compose file has something real to probe. Write the bundle after the backend code is in place so the smoke script in Step 5 can verify everything end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `deploy/self-hosted/docker-compose.yml` | create | 4-service compose with volumes and healthchecks |
| `deploy/self-hosted/.env.example` | create | Canonical env-var template |
| `deploy/self-hosted/.gitignore` | create | `.env` |
| `deploy/self-hosted/postgres.Dockerfile` | create | `FROM postgres:16-alpine` + contrib extensions |
| `deploy/self-hosted/README.md` | create | Setup, upgrade, env vars, ports, volumes, known-limitations |

#### New Code

```yaml
# deploy/self-hosted/docker-compose.yml
name: curlew-self-hosted

services:
  postgres:
    build:
      context: .
      dockerfile: postgres.Dockerfile
    environment:
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - pg_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 5s
      timeout: 3s
      retries: 20
      start_period: 10s

  redis:
    image: redis:7-alpine
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20
      start_period: 5s

  backend:
    build:
      context: ../..
      dockerfile: src/ApiTool.Backend/Dockerfile
    environment:
      ASPNETCORE_URLS: http://0.0.0.0:5000
      ASPNETCORE_ENVIRONMENT: ${ASPNETCORE_ENVIRONMENT}
      ConnectionStrings__Default: "Data Source=/app/data/curlew.db"
      Jwt__SigningKey: ${JWT_SIGNING_KEY}
      REDIS_HOST: redis
      REDIS_PORT: "6379"
      REDIS_PASSWORD: ${REDIS_PASSWORD}
      POSTGRES_HOST: postgres
      POSTGRES_PORT: "5432"
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      BACKEND_RUN_MIGRATIONS: ${BACKEND_RUN_MIGRATIONS}
    ports:
      - "${BACKEND_PORT}:5000"
    volumes:
      - backend_data:/app/data
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:5000/health"]
      interval: 5s
      timeout: 3s
      retries: 20
      start_period: 15s

  web:
    build:
      context: ../../web
      dockerfile: Dockerfile
    environment:
      PUBLIC_API_URL: http://backend:5000
      ORIGIN: http://localhost:${WEB_PORT}
    ports:
      - "${WEB_PORT}:3000"
    depends_on:
      backend:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:3000/"]
      interval: 5s
      timeout: 3s
      retries: 20
      start_period: 10s

volumes:
  pg_data:
  redis_data:
  backend_data:
```

```dotenv
# deploy/self-hosted/.env.example
# Copy to .env and change EVERY secret before first boot.

# Ports
BACKEND_PORT=5000
WEB_PORT=3000

# Environment
ASPNETCORE_ENVIRONMENT=Production

# Postgres (reserved for M5-016; postgres service is up and healthy today)
POSTGRES_DB=curlew
POSTGRES_USER=curlew
POSTGRES_PASSWORD=change-me-please

# Redis (no password in the default local stack; set in production)
REDIS_PASSWORD=

# JWT signing key — MUST be at least 32 bytes
JWT_SIGNING_KEY=change-me-32-bytes-minimum-please

# EF migrations on startup (set to 1 after M5-016 ships)
BACKEND_RUN_MIGRATIONS=0
```

```dockerfile
# deploy/self-hosted/postgres.Dockerfile
FROM postgres:16-alpine

# Ship pg_cron-friendly extensions in the image. Enabling pg_cron itself
# requires shared_preload_libraries at runtime — deferred to a later task.
RUN apk add --no-cache postgresql16-contrib
```

```gitignore
# deploy/self-hosted/.gitignore
.env
```

README.md contains: setup (copy → edit → up), env var table, port table, volume table, upgrade path (`docker compose pull && docker compose up -d`), known limits (backend still SQLite until M5-016, data lives on `backend_data` volume), backup guidance (`docker run --rm -v curlew-self-hosted_pg_data:/source ...`), and troubleshooting (checking `docker compose logs backend`).

#### Impact on Existing Tests
- None. New directory. `docker-compose.test.yml` at the repo root is unaffected.

---

### Step 5: `scripts/test-self-hosted.sh` smoke test

**Rationale:** Definition of Done: "smoke test for stack boot under `scripts/test-self-hosted.sh`". This is the authoritative end-to-end proof for the observable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `scripts/test-self-hosted.sh` | create | Executable shell script, `set -euo pipefail` |

#### New Code (shell)

```bash
#!/usr/bin/env bash
# test-self-hosted.sh — Brings up deploy/self-hosted, verifies the bundle is
# healthy end-to-end, and tears it down. Opt-in from ci-local.sh via
# CURLEW_RUN_SELF_HOSTED=1 (stack boot is ~60 s and we don't want to pay that
# on every PR).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUNDLE="$REPO_ROOT/deploy/self-hosted"
cd "$BUNDLE"

# Guard: require docker.
command -v docker >/dev/null || { echo "docker is required" >&2; exit 2; }

trap 'docker compose -f docker-compose.yml --env-file .env down -v >/dev/null 2>&1 || true; rm -f .env' EXIT

cp .env.example .env

echo "=== Self-Hosted: up ==="
docker compose -f docker-compose.yml --env-file .env up -d --build

echo "--- Waiting for backend /health ---"
for i in $(seq 1 90); do
  if curl -fsS http://localhost:5000/health | grep -q '"status":"healthy"'; then
    echo "backend healthy after ${i}s"; break
  fi
  sleep 1
  if [ "$i" -eq 90 ]; then
    docker compose logs --tail 200 >&2
    echo "FAIL: backend did not reach healthy within 90s" >&2; exit 1
  fi
done

echo "--- /health reports db + redis connected ---"
BODY=$(curl -fsS http://localhost:5000/health)
echo "$BODY" | grep -q '"db":"connected"'    || { echo "FAIL: db not connected — $BODY" >&2; exit 1; }
echo "$BODY" | grep -q '"redis":"connected"' || { echo "FAIL: redis not connected — $BODY" >&2; exit 1; }
echo "PASS: /health healthy"

echo "--- web / returns title ---"
curl -fsS http://localhost:3000/ | grep -q '<title>curlew</title>' \
  || { echo "FAIL: web title missing" >&2; exit 1; }
echo "PASS: web title"

echo "--- Volume survives down (no -v) ---"
docker compose -f docker-compose.yml --env-file .env down
docker volume ls --format '{{.Name}}' | grep -q curlew-self-hosted_pg_data \
  || { echo "FAIL: pg_data missing after down" >&2; exit 1; }
docker volume ls --format '{{.Name}}' | grep -q curlew-self-hosted_redis_data \
  || { echo "FAIL: redis_data missing after down" >&2; exit 1; }
docker volume ls --format '{{.Name}}' | grep -q curlew-self-hosted_backend_data \
  || { echo "FAIL: backend_data missing after down" >&2; exit 1; }
echo "PASS: volumes survived"

echo "--- down -v removes volumes ---"
docker compose -f docker-compose.yml --env-file .env up -d
# Wait once more so down -v has something to remove.
sleep 5
docker compose -f docker-compose.yml --env-file .env down -v
! docker volume ls --format '{{.Name}}' | grep -q curlew-self-hosted_pg_data \
  || { echo "FAIL: pg_data still present after down -v" >&2; exit 1; }
echo "PASS: down -v removed volumes"

echo "=== Self-Hosted smoke PASS ==="
```

Add the script to `README.md` and, in a separate one-line change, to `scripts/ci-local.sh` behind the env-var gate:

```bash
if [ "${CURLEW_RUN_SELF_HOSTED:-0}" = "1" ]; then
  step "self-hosted smoke"
  ./scripts/test-self-hosted.sh
fi
```

#### Impact on Existing Tests
- None. Additive; gated behind env var so ci-local is not slower by default.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `src/ApiTool.Backend.Tests/Health/HealthServiceTests.cs` | new file | — | 6 new unit tests (1 theory w/ 5 rows + 1 exception case) |
| `src/ApiTool.Backend.Tests/Health/HealthEndpointTests.cs` | new file | — | 4 new integration tests |
| `web/tests/unit/root-page.spec.ts` | new file (optional) | — | 1 test for title; drop if vitest renders `<svelte:head>` unreliably |
| `src/ApiTool.Backend.Tests/SmokeTests.cs` | existing | none | no change |
| All other existing backend tests | existing | none — `/health` endpoint additive, no middleware changes that affect existing routes | no change |
| `docker-compose.test.yml` | existing | none | untouched |
| `scripts/ci-local.sh` | existing | additive gated section | 1-line addition |
| `scripts/test-self-hosted.sh` | new file | — | exercised opt-in via `CURLEW_RUN_SELF_HOSTED=1` |

## Risks and Edge Cases

- **Risk:** Adding `TcpClient` PING contends with real Redis handshake semantics. → **Mitigation:** RESP inline commands (`PING\r\n`) are valid without AUTH unless `requirepass` is set. In M5-015 the default compose has no Redis password; when operators add one they set `REDIS_PASSWORD` and the probe will fail until the probe is upgraded. Documented explicitly in the README's limitations section.
- **Risk:** `Microsoft.EntityFrameworkCore.InMemory` (used in tests) returns `CanConnectAsync()=true` even for broken DBs. → **Mitigation:** `HealthEndpointTests` uses fake probes directly; it does not test the EF concrete probe against the in-memory provider. A separate concrete-probe unit test is not needed for M5-015 — the real-compose smoke covers the SQLite case.
- **Risk:** Compose `healthcheck` for `redis` uses `redis-cli ping` which fails silently if the image strips the CLI. → **Mitigation:** `redis:7-alpine` ships `redis-cli`; verified in the smoke run. If it ever regresses, fallback is `nc -z localhost 6379`.
- **Risk:** `backend` depends_on `postgres:service_healthy` but backend today does not consume Postgres. → **Acceptable** — the dependency guarantees `postgres` is healthy when `backend` starts so M5-016's migration runner (which WILL consume Postgres) has a clean contract. In M5-015 it just makes the boot sequence deterministic.
- **Risk:** `BACKEND_PORT`/`WEB_PORT` substitution with empty `.env` yields bare `:5000` in the ports list, which compose accepts but binds to a random host port. → **Mitigation:** `.env.example` ships explicit defaults so compose sees populated values. Smoke asserts the fixed ports.
- **Edge case:** `docker compose` on some hosts interprets `name: curlew-self-hosted` as the project name. Smoke asserts volumes by their prefixed name (`curlew-self-hosted_pg_data`). If an operator runs compose with `-p other-name`, the volumes rename accordingly — README warns against overriding `-p`.
- **Edge case:** Port collision (host already uses 5000 or 3000). → **Handling:** Override via `BACKEND_PORT=8080` etc. in `.env`. README shows this.
- **Edge case:** `POSTGRES_PASSWORD=""` empty. → **Handling:** postgres image refuses to initialize. README calls this out; `.env.example` ships a clearly-fake-but-non-empty default.
- **Edge case:** `REDIS_PASSWORD` set in `.env` but not passed as a compose-level arg to redis. → **Handling:** M5-015 ships a no-auth redis. A follow-up task can wire `REDIS_PASSWORD` into both the `redis` container and the probe. Documented limitation.
- **Edge case:** Running the bundle and the test stack simultaneously. → **Handling:** They share ports 5000/3000. README tells operators to `docker compose down` one before bringing the other up.
- **Risk:** `scripts/ci-local.sh` runs against the test stack; accidental overlap could wipe self-hosted volumes via `docker compose down -v` in the test-stack trap. → **Mitigation:** The test-stack trap targets `docker-compose.test.yml` by path; self-hosted compose targets `deploy/self-hosted/docker-compose.yml`. No cross-contamination.
- **Risk:** ASP.NET config binding for the new `REDIS_HOST` env var clashes with a section called `Redis`. → **Check** (grep the codebase for any `"Redis"` config key): none exist today. Safe.
- **Risk:** Health endpoint leaks operational info to the public internet (whether DB/Redis are up). → **Acceptable** for M5-015 — this is on-premises by design. A later task can add `/health/live` (no internals) vs. `/health/ready` (with internals, restricted to intranet) per the spec (§Health Checks).
- **Edge case:** The smoke runs in an environment without outbound docker pulls (air-gapped CI). → **Handling:** Smoke is opt-in via `CURLEW_RUN_SELF_HOSTED=1`. Operators running offline need to pre-pull `postgres:16-alpine`, `redis:7-alpine`.
- **Risk:** SvelteKit's `<svelte:head>` content is injected at SSR; a bare `curl /` must see the title in the response body. → **Verified:** SvelteKit's Node adapter renders `<svelte:head>` into the document `<head>` server-side before sending the first byte; `curl /` will see `<title>curlew</title>`. Smoke script asserts this explicitly.

## Verification

```bash
# Go gate (unchanged; no Go code changes)
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run

# Backend gate
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj --filter "FullyQualifiedName~Health"

# Web gate
( cd web && npm run test:unit )

# Opt-in stack boot (this is the authoritative observable verification)
CURLEW_RUN_SELF_HOSTED=1 ./scripts/ci-local.sh
# or directly:
./scripts/test-self-hosted.sh
```

Observable verification (from task YAML):

```bash
cd deploy/self-hosted
cp .env.example .env
docker compose -f docker-compose.yml --env-file .env up -d --build
# Expected: postgres, redis, backend, web containers reach healthy state within 60s
sleep 30
curl -sS http://localhost:5000/health
# Expected HTTP 200, body: {"status":"healthy","db":"connected","redis":"connected"}
curl -sS http://localhost:3000/ | grep -o '<title>curlew</title>'
# Expected: <title>curlew</title>
docker compose down -v
# Expected: all containers stop and volumes removed
```
