# Code Review: M5-015 (Iteration 2)

**Task:** Backend: self-hosted docker-compose bundle
**Reviewer:** AI
**Date:** 2026-04-20
**Branch:** feature/M5-015-self-hosted-docker-bundle

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `SafeAsync` in `HealthService` catches probe exceptions and logs at Warning. `EfDbHealthProbe` correctly links a 2-second timeout CTS. `TcpRedisHealthProbe` now correctly propagates `OperationCanceledException` (finding #4 from iteration 1 resolved) while catching all other exceptions. No swallowed errors. |
| Input Validation | PASS | `TcpRedisHealthProbe.IsConfigured` guards the null/empty `REDIS_HOST` case. `HealthService` short-circuits the redis probe when `!redis.IsConfigured`. Compose env vars all have explicit defaults in `.env.example`. |
| Naming | PASS | No stuttering. All exported types, methods, and properties have doc comments. Interfaces are clearly named. No package-naming issues (C# namespace conventions followed). |
| Code Organization | PASS | All new C# code lives under `src/ApiTool.Backend/Health/` and `src/ApiTool.Backend.Tests/Health/`. Interfaces abstract concrete probes cleanly. `EfDbHealthProbe` correctly uses `IServiceScopeFactory` to avoid the captive-scoped-dependency problem. Registration in `Program.cs` is in the correct section and clearly commented. No circular dependencies. |
| Correctness | PASS | `HealthService` starts both probe tasks before awaiting either (parallel start, sequential await). The redis-not-configured short-circuit is correct. JSON serialization uses the global `SnakeCaseLower` policy via `ConfigureHttpJsonOptions`, so `HealthReport` properties serialize as `status`, `db`, `redis`. Rate-limiting exclusion is implicit (no `.RequireRateLimiting()` means no limit — correct for the current codebase). Compose healthchecks, `depends_on`, volumes, and port bindings are all correct. `TcpRedisHealthProbe` uses `NoDelay = true` and a 2-second linked CTS. |
| Test Quality | PASS | All four endpoint integration tests now assert both HTTP status code and response body shape. The redis-down 503 path is covered at the endpoint layer (`Health_returns_503_when_redis_down`). Unit theory covers all five probe-combination cases plus the exception case. `scripts/test-self-hosted.sh` covers behaviors 1–3, 5, and 6 end-to-end. Behavior 4 (password rotation) is documented in README — acceptable per plan decision. |

## Test Coverage

- Go coverage: 86.7% (unchanged — no Go changes in this task)
- C# Health package: 11 tests pass (5 theory rows + 1 exception unit + 5 endpoint integration tests). All happy and unhealthy paths covered at both unit and integration levels.
- Web: `<title>curlew</title>` fix verified by smoke script end-to-end.
- Shell smoke: `scripts/test-self-hosted.sh` is gated behind `CURLEW_RUN_SELF_HOSTED=1` (opt-in per plan decision; documented in README and ci-local.sh).

## Resolved Findings (from Iteration 1)

| # | Previous Finding | Resolution Status |
|---|-----------------|-------------------|
| 1 | `management/tasks/M5-015.yaml` had `status: backlog` | Fixed — now `status: review` |
| 2 | `Health_returns_503_when_db_down` missing body assertions | Fixed — body assertions added |
| 3 | No endpoint test for redis-down 503 path | Fixed — `Health_returns_503_when_redis_down` added |
| 4 | Bare `catch` swallowed `OperationCanceledException` in `TcpRedisHealthProbe` | Fixed — `catch (OperationCanceledException) { throw; }` added before bare catch |

## Summary

All four findings from the first review are resolved. The implementation is architecturally sound and complete: interfaces are well-defined, DI wiring is correct, JSON serialisation matches the spec, the compose bundle is correct, and all 11 C# health tests pass. No new issues were found in the iteration-2 state.
