# Code Review: M18-007

**Task:** Telemetry Phase 3 ingest: telemetry_events table, anonymous POST endpoint, daily aggregator, 90-day purge
**Reviewer:** AI
**Date:** 2026-05-19
**Branch:** feature/M18-007-telemetry-phase3-ingest

## Verdict: PASS

## Findings

No findings. All six issues from the iteration-1 review have been resolved.

## Resolved Since Iteration 1

| # | Original Severity | Finding | Resolution |
|---|------------------|---------|-----------|
| 1 | Critical | `TelemetryAggregatorHost` and `TelemetryPurgeHost` registered unconditionally (braceless `if`) | Fixed: dedicated `if (!builder.Environment.IsEnvironment("Testing"))` block with braces at Program.cs:527–531 |
| 2 | Critical | Rate-limit partition key `HttpContext.Items["telemetry.install_id"]` never set | Fixed: `TelemetryInstallIdMiddleware` implemented, registered at Program.cs:828 before `UseRateLimiter()` at Program.cs:829 |
| 3 | Critical | Behavior #4 (429 per-install_id) had no functional test | Fixed: `TelemetryRateLimitTests` with `TelemetryRateLimitFactory` added; two tests confirm 429 on over-limit and per-partition isolation |
| 4 | High | `dynamic` dispatch for Postgres unique-constraint detection | Fixed: reflection-based cast via `GetProperty("SqlState").GetValue(...)` in `IsUniqueConstraintViolation` |
| 5 | High | `OpenApi_path_has_no_security_requirement` had a silent always-pass `else` branch | Fixed: `else` branch now asserts `postOp.TryGetProperty("security", out _).Should().BeFalse(...)` |
| 6 | Medium | Options classes never wired to configuration — `Section` constants were dead code | Fixed: `builder.Services.Configure<TelemetryAggregatorOptions>(...)` and `builder.Services.Configure<TelemetryPurgeOptions>(...)` added at Program.cs:522–525 |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Errors returned, not thrown. `DbUpdateException` caught and handled. JSON parse errors return 400. `OperationCanceledException` correctly guarded in both hosts. |
| Input Validation | PASS | `install_id` UUID validation, `event_type` length, `Idempotency-Key` header presence and length, body size cap, null body — all validated with descriptive Problem responses. |
| Naming | PASS | No stuttering; doc comments on all exported types and methods; package-level names are descriptive. |
| Code Organization | PASS | Single-responsibility per file; `internal/` pattern respected; no circular dependencies; `using`/`await using` used for cleanup. Background hosts registered outside Testing. |
| Correctness | PASS | Middleware-before-rate-limiter ordering correct (Program.cs:828–829). Idempotency pre-check + constraint-catch covers both in-memory and real DB providers. Body position reset after middleware peek (TelemetryInstallIdMiddleware:78). Migration has a complete `Down()`. |
| Test Quality | PASS | All 8 behaviors from the task YAML have functional test coverage. 34 telemetry-scoped tests. Rate-limit bucket isolation tested. Schema constraints tested. Aggregator and purge correctness tested with `FakeClock`. |

## Test Coverage

- **Test count:** 34 telemetry-scoped tests across 5 test files (exceeds the ≥14 DoD requirement)
- **Endpoint coverage:** 90.1% (TelemetryIngestEndpoint), 100% (TelemetryInstallIdMiddleware)
- **Host coverage:** 93.1% (TelemetryAggregatorHost), 100% (TelemetryPurgeHost)
- **Behaviors covered:** All 8 behaviors from the task YAML — schema, anonymous ingest, idempotency replay, per-install_id rate limit (429), 413 body cap, daily aggregation, 90-day purge, forward-compat unknown event_type
- **Missing coverage:** None

## Summary

All six findings from the iteration-1 review are resolved with no regressions. The middleware-first rate-limit partitioning is correctly wired, both background hosts are properly excluded from the Testing environment, and the 429 rate-limit behavior now has a functional test that exercises a real fixed-window policy via a custom `WebApplicationFactory` variant. The implementation is structurally sound, the schema migration is reversible, and the OpenAPI surface is fully documented.
