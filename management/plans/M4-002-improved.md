# Improvement Report: M4-002

**Task:** CLI consumes shared vault template at runtime
**Date:** 2026-04-15
**Review:** management/reviews/M4-002-review.md

## Resolved Findings (Iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `visitBody` type-switch branches (string, map[string]any, map[string]string, []any) completely uncovered at 12.5% | Added `TestVisitBody` (5 sub-tests) and `TestCollectSecretReferences_BodyTypes` (4 sub-tests) in `internal/runner/runner_test.go`; coverage rose from 12.5% → 100% | ✓ tests pass |
| 2 | Medium | `TestRunCmd_Help_MentionsTeamTemplate` only checked exit code 0, not content | Replaced stub with os.Pipe stdout capture; asserts on `APITEST_TEAM_CONFIG`, `--env`, and `"shared vault"` strings in help output | ✓ tests pass |
| 3 | Low | `resolver.Resolve` error paths untested: BulkFetch error, missing path, ExtractField error | Added `TestSecretsResolver_BulkFetchError`, `TestSecretsResolver_MissingPath`, `TestSecretsResolver_FieldExtractionError` with purpose-built `errProvider`, `omitPathProvider`, and `fieldErrProvider` test helpers | ✓ tests pass |
| 4 | Low | `NewSecretsResolver` factory error path untested | Added `TestNewSecretsResolver_FactoryError`; NewSecretsResolver coverage now 100% | ✓ tests pass |

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 5 | Medium | Behavior #5 ("logs 'Resolved N secrets from shared template (<env>)' exactly once") had zero test coverage — the stderr log line was emitted but never captured or asserted | Added `TestRunCmd_TeamSecrets_LogsResolvedCount` in `cmd/apitest/run_test.go`: uses `os.Pipe` to redirect `os.Stderr`, runs with `APITEST_VAULT_STUB=1` and `--env production`, then asserts the message contains `"Resolved 2 secrets from shared template (production)"` and appears exactly once | ✓ tests pass |
| 6 | Low | Behavior #2 (`staging_uses_azure_provider`) only verified no-error; did not assert staging-namespaced resolved values | Replaced `successExecutor` with a `trackingExec` that captures `req.URL`; asserts the URL contains `"stub::staging"` (proving the staging provider path was taken), and asserts `summary.SharedSecretsResolved == 2` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/vault/teamtemplate`) | 91.4% |
| Coverage (`internal/runner`) | 85.9% |
| Coverage (`cmd/apitest`) | 83.0% |
| Overall project coverage | 89.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c26c84d | test(runner): add visitBody branch coverage and collectSecretReferences body-type tests | #1 |
| 67f22df | test(cmd): verify help text content for APITEST_TEAM_CONFIG and shared vault | #2 |
| e63a9cb | test(vault/teamtemplate): cover BulkFetch error, missing-path, field extraction error, and factory error paths | #3, #4 |
| 3b076db | test(runner,cmd): assert log line and staging-namespaced stub values | #5, #6 |

## Summary

6/6 findings resolved. 0 deferred.

All review findings were test quality gaps in newly added code. Fixes were purely additive (new test helpers and test functions); no production code was modified. Coverage improvements across two iterations:
- `visitBody`: 12.5% → 100%
- `resolver.Resolve`: 70.4% → 96.3%
- `NewSecretsResolver`: now 100%
- `internal/vault/teamtemplate` package: 86.2% → 91.4%
- Behavior #5 log line coverage: 0% → 100% (stderr capture test)
- Behavior #2 staging value assertion: no-error only → stub value + count verified
