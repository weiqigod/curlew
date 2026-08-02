# Improvement Report: M5-016 (Iteration 2)

**Task:** Backend: admin bootstrap + migration runner
**Date:** 2026-04-20
**Review:** management/reviews/M5-016-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `SchemaGuardMiddleware.InvokeAsync` catch block (lines 30–38) had 0% line coverage — the HTTP 503 response path was never hit by any test. Only the static `IsMissingSchemaException` helper was exercised. | Added `SchemaGuardMiddlewareHttpTests` integration test class that creates a `WebApplicationFactory<Program>` with a schema-less SQLite DB (schema deliberately not created), calls `POST /api/v1/auth/login` (AllowAnonymous, queries `db.Users`), and asserts HTTP 503 with `service_unavailable` and `BACKEND_RUN_MIGRATIONS=1` in the body. Lines 30–38 now show 1 hit each. | ✓ tests pass |
| 2 | Medium | `AdminBootstrap.RunAsync` transaction rollback catch block (lines 98–101) had 0 hits — no test exercised the save failure path. | Added `SaveFailureInterceptor` (EF Core `SaveChangesInterceptor` that always throws) and `FailOnSaveScopeFactory` helper to `TestDoubles.cs`. Added `RunAsync_rolls_back_transaction_when_save_fails` test: uses the shared SQLite connection approach (schema created on clean context, failing context uses same connection), calls bootstrap, verifies exception is thrown and `Users.CountAsync() == 0`. | ✓ tests pass |
| 3 | Low | `AdminBootstrapConfig.FromConfiguration` success return (line 47) had 0 hits — the valid-pair success path was never tested through `FromConfiguration`. | Added `FromConfiguration_valid_pair_returns_config_with_lowercased_email` test: builds in-memory config with `Admin@Example.COM` and `ChangeMe!Password`, calls `FromConfiguration`, asserts `err == null`, config non-null, email lowercased to `admin@example.com`. | ✓ tests pass |

## Out of Scope (Deferred)

| # | Severity | Finding | Rationale |
|---|----------|---------|-----------|
| — | — | `IsNpgsqlUndefinedTable` lines 73–75 remain at 0 hits | These lines check a Npgsql `PostgresException` via reflection. Exercising them would require a live Postgres connection or complex mocking of a non-public exception type. This is pre-existing behavior not changed in this iteration. Overall backend coverage (93.2%) is well above the 80% threshold and all task-specific behaviors are tested. |

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `dotnet build ApiTool.Backend` | PASS |
| `dotnet test ApiTool.Backend.Tests` (617 tests) | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (overall backend) | 93.2% |
| `SchemaGuardMiddleware.InvokeAsync` catch block (lines 30–38) | 1 hit each — covered |
| `AdminBootstrap.RunAsync` rollback path (lines 98–101) | covered |
| `AdminBootstrapConfig.FromConfiguration` success path (line 47) | covered |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 2e0bf34 | test(bootstrap): cover SchemaGuardMiddleware HTTP path, AdminBootstrap rollback, and FromConfiguration success path | #1, #2, #3 |

## Summary
3/3 findings resolved. 1 pre-existing coverage gap deferred with rationale (Npgsql reflection path in `IsNpgsqlUndefinedTable`). Overall backend coverage 93.2% (well above 80% threshold). All 617 tests pass.
