# Improvement Report: M18-007

**Task:** Telemetry Phase 3 ingest: telemetry_events table, anonymous POST endpoint, daily aggregator, 90-day purge
**Date:** 2026-05-19
**Review:** management/reviews/M18-007-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `TelemetryAggregatorHost` and `TelemetryPurgeHost` registered unconditionally — braceless `if` only guarded the first line, leaking both hosts into the Testing environment | Replaced with a dedicated `if (!builder.Environment.IsEnvironment("Testing"))` block with braces, matching the pattern used by all other background hosts in the codebase | ✓ tests pass |
| 2 | Critical | Rate-limit `telemetry-ingest` policy partitioned on `HttpContext.Items["telemetry.install_id"]` but no code ever set that key — all traffic fell into the `"unknown"` bucket | Implemented `TelemetryInstallIdMiddleware` registered before `UseRateLimiter()` that buffers the body via `EnableBuffering()`, extracts `install_id`, and sets `HttpContext.Items["telemetry.install_id"]` before the rate-limiter middleware evaluates the partition | ✓ tests pass |
| 3 | Critical | Behavior #4 (per-`install_id` rate limit returning 429) had no functional test | Added `TelemetryRateLimitTests` with `TelemetryRateLimitFactory` (a custom `WebApplicationFactory<Program>` that uses `PostConfigure<RateLimiterOptions>` to replace the no-op testing policy with a real 3/min fixed-window limiter). Two tests: (a) 4th request from same `install_id` returns 429; (b) rate-limit buckets are per-`install_id`, not global | ✓ tests pass |
| 4 | High | `dynamic` dispatch for Postgres unique-constraint detection in `IsUniqueConstraintViolation` — fragile, bypasses null safety, risks `RuntimeBinderException` | Replaced with safe reflection: `GetProperty("SqlState").GetValue(ex.InnerException) as string` which fails explicitly rather than with a binder exception | ✓ tests pass |
| 5 | High | `OpenApi_path_has_no_security_requirement` had a silent always-pass `else` branch — test could not detect regressions where global Bearer security was incorrectly applied | Replaced the silent `else` with a real assertion (`postOp.TryGetProperty("security", out _).Should().BeFalse(...)`) so both branches contain observable assertions | ✓ tests pass |
| 6 | Medium | `TelemetryAggregatorOptions` and `TelemetryPurgeOptions` never wired to configuration — `Section` constants were dead code and operators could not override defaults | Added `builder.Services.Configure<TelemetryAggregatorOptions>(...)` and `builder.Services.Configure<TelemetryPurgeOptions>(...)` alongside the host registrations | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| `golangci-lint run` | PASS |
| Test count | 2222 passed, 15 skipped, 0 failed |
| Telemetry-scoped tests | 34 passed |
| `TelemetryIngestEndpoint` handler coverage | 90.1% |
| `TelemetryAggregatorHost` coverage | 93.1% |
| `TelemetryPurgeHost` coverage | 100% |
| `TelemetryInstallIdMiddleware` coverage | 100% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 095be09c | fix(telemetry): resolve all review findings for M18-007 | #1, #2, #3, #4, #5, #6 |

## Summary
6/6 findings resolved. 0 deferred.
