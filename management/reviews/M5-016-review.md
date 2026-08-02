# Code Review: M5-016

**Task:** Backend: admin bootstrap + migration runner
**Reviewer:** AI
**Date:** 2026-04-20
**Branch:** feature/M5-016-admin-bootstrap-migrations
**Review iteration:** 3 (post-improve iteration 2)

## Verdict: PASS

## Findings

_No findings._

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `MigrationFailedException` is correctly thrown and caught at the `Program.cs` level for exit code 4. `BootstrapConfigError` enum with `FromConfiguration` cleanly separates validation errors. `AdminBootstrap.RunAsync` wraps its transaction in try/catch and rolls back on failure. `PasswordHasher.Verify` returns false (never throws) on any malformed input. `CryptographicOperations.FixedTimeEquals` is used correctly for timing-safe comparison. |
| Input Validation | PASS | `AdminBootstrapConfig.FromConfiguration` validates email format via compiled regex, enforces minimum password length (12 chars), and handles partial-pair semantics (email-only, password-only). `PasswordHasher.Hash` uses `ArgumentException.ThrowIfNullOrEmpty`. Login endpoint validates null/whitespace body/fields before DB access. |
| Naming | PASS | All exported types, functions, properties, and enum members have XML doc comments. No stuttering. C# naming conventions followed throughout (`BootstrapExitCodes`, `MigrationFailedException`, `SchemaGuardMiddleware`, `AdminBootstrapConfig`). |
| Code Organization | PASS | `Bootstrap/` namespace is cohesive and correctly encapsulates all startup-task code. `Auth/` additions are narrow. `Program.cs` startup block is guarded by `IsEnvironment("Testing")` so existing tests are unaffected. `SchemaGuardMiddleware` is wired after authentication but before endpoint dispatch, which is the correct position. No circular dependencies. |
| Correctness | PASS | Timing-attack mitigation via `PlaceholderHash` static readonly field (precomputed once at class load). Transaction rollback uses `CancellationToken.None` (correct — avoids cancelling a rollback). Email lowercased before persisting. `MigrationRunner` catch clause correctly distinguishes timeout `OperationCanceledException` (wraps it) from outer-cancellation `OperationCanceledException` (rethrows). `SlowScopeFactory` / `FailOnSaveScopeFactory` test doubles are correct and verify the timeout and rollback paths. |
| Test Quality | PASS | All 8 behaviors from the task YAML are covered by at least one test. `SchemaGuardMiddleware.InvokeAsync` HTTP 503 response path is covered by `SchemaGuardMiddlewareHttpTests`. `AdminBootstrap.RunAsync` transaction rollback path is covered by `RunAsync_rolls_back_transaction_when_save_fails`. `AdminBootstrapConfig.FromConfiguration` success path is covered by `FromConfiguration_valid_pair_returns_config_with_lowercased_email`. `IsNpgsqlUndefinedTable` Npgsql reflection path (lines 73–75) remains at 0 hits — deferred with rationale (requires live Postgres; pre-existing; overall coverage 93.2% is well above 80% threshold). |

## Test Coverage

- **Overall backend:** 93.2% (617 tests pass)
- **New files coverage (iteration 3):**
  - `Auth/PasswordHasher.cs`: 100%
  - `Auth/AuthLoginEndpoints.cs`: 100%
  - `Auth/LoginRequest.cs`: 100%
  - `Auth/LoginResponse.cs`: 100%
  - `Bootstrap/AdminBootstrap.cs`: 100% (including rollback path)
  - `Bootstrap/AdminBootstrapConfig.cs`: 100% (including success return path)
  - `Bootstrap/MigrationRunner.cs`: 100%
  - `Bootstrap/SchemaGuardMiddleware.cs InvokeAsync`: 100% (HTTP path covered by integration test)
  - `Bootstrap/SchemaGuardMiddleware.cs IsMissingSchemaException`: covered for SQLite; Npgsql reflection path deferred
  - `Bootstrap/MigrationFailedException.cs`: 100%
  - `Bootstrap/BootstrapExitCodes.cs`: 100%

## Summary

All three findings from the iteration-2 review were resolved: the `SchemaGuardMiddleware.InvokeAsync` HTTP 503 path is now covered by a `WebApplicationFactory<Program>` integration test with a schema-less SQLite DB; the `AdminBootstrap.RunAsync` transaction rollback path is covered by a `SaveFailureInterceptor`-backed test; and `AdminBootstrapConfig.FromConfiguration` success path is covered with case-normalization assertion. All 617 backend tests pass, the Go gate passes (86.7% overall, `golangci-lint` 0 issues, smoke pass), and all 8 task behaviors have test coverage. The code is correct, well-organized, and production-ready for the self-hosted deployment target.
