# Code Review: M4-012

**Task:** E2E: CLI run -> backend ingest -> web dashboard
**Reviewer:** AI
**Date:** 2026-04-17
**Branch:** feature/M4-012-e2e-cli-backend-web
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All 7 issues raised in iteration 1 have been resolved.

## Previous Findings — Resolved

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 | High | `wantState` field in `payload_test.go` never asserted — item-level status regressions silently missed | FIXED: assertion added at line 228 |
| 2 | High | `data_driven_iteration_folded` did not assert item names contain `[1/2]`/`[2/2]` markers | FIXED: `wantItemNames` field + index-matched assertion loop added |
| 3 | High | `smoke/run.sh` missing stack lifecycle idempotency assertion (DoD item not met) | FIXED: `=== Stack idempotency (M4-012) ===` section added at line 1720, gated by `APITEST_MANAGE_STACK=1` |
| 4 | Medium | `scripts/seed-test-data.sh` results fixtures not idempotent — every `up` appends duplicate rows | FIXED: `EXISTING_RESULTS`/`FIXTURE_COUNT` check skips fixture posts when rows already present |
| 5 | Medium | `cmd/apitest/main.go` `runFlags` struct gofumpt formatting violation | FIXED: gofumpt applied, golangci-lint now 0 issues |
| 6 | Medium | `internal/prcheck/payload.go` trailing newline gofumpt violation | FIXED |
| 7 | Low | `makeFailResult(name string, _ string)` parameter grouping violation | FIXED: `makeFailResult(name, _ string)` |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`, sentinel errors used (`ErrBackendURLMissing`, `ErrNetworkFailure`, `ErrUnauthorized`). `handleReportUpload` correctly uses exit 2 for upload failures on passing runs without masking run failures. |
| Input Validation | PASS | `--org` required with `--report-upload`; `--pr` and `--repo` must be set together; `APITEST_BACKEND_URL` validated at runtime. Backend validates `repo`, `pr > 0`, `state ∈ {success, failure}`. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. Package names correct. `DetectGitSha`, `BuildPayload`, `UploadRun`, `TriggerInfo`, `UploadConfig` are all well-named. |
| Code Organization | PASS | `internal/prcheck` package boundary respected. `UploadRun` cleanly separated from file-based `Run`. Web/backend layers are independent. `cmd/apitest` correctly delegates to `internal/prcheck`. |
| Correctness | PASS | Exit-code rules correctly implemented. Retry logic with context propagation is correct. Teardown failures excluded from fail count matches exit-code semantics. `UploadRun` has 91.7% cross-package coverage via `cmd/apitest` tests. |
| Test Quality | PASS | All previously dead assertions are now active. `wantState` asserted, `wantItemNames` loop added, smoke idempotency test in place, seed script idempotent. |

## Test Coverage
- `internal/prcheck` (package-local): 85.4%
- `cmd/apitest` (package-local): 83.5%
- `internal/prcheck/run.go:UploadRun` (cross-package via `-coverpkg`): 91.7%
- All packages meet the ≥80% threshold.

## Summary

All 7 findings from iteration 1 have been correctly fixed. The implementation is architecturally sound: the `--report-upload` flag, `BuildPayload`, `DetectGitSha`, `UploadRun`, backend PR-checks endpoint (POST + GET with RBAC), and the SvelteKit pr-checks dashboard page all work together correctly. All Go tests pass, golangci-lint reports 0 issues, and test coverage exceeds 80% across all changed packages. The Playwright spec covers ≥4 assertions including a no-live-backend path (assertion 5). The task is ready for verification.
