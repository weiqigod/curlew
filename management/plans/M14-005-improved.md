# Improvement Report: M14-005 (Iteration 2)

**Task:** CLI: curlew login (RFC 8628 device-code flow)
**Date:** 2026-05-05
**Review:** management/reviews/M14-005-review.md

## Iteration 1 Resolved Findings (from previous improve)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `device.Write` `WriteFile` error branch uncovered — device package at 78.9% (below 80%) | Added `write_fails_on_read-only_directory` sub-test in `device_test.go` that `os.Chmod`s the target dir to 0o500 and asserts an error is returned. Coverage: 78.9% → 84.2% | ✓ tests pass |
| 2 | Medium | `loginExitForError` at 45.5% — `ErrAccessDenied` (exit 4) and `ErrServerError` (exit 6) arms untested | Added `TestLogin_Polling_AccessDenied_ExitFour` and `TestLogin_ServerError_ExitSix` using `newDeviceCodeServer` with `"access_denied"` and `"server_error"` behaviors. Coverage: 45.5% → 81.8% | ✓ tests pass |
| 3 | Medium | `TestLogin_PersistsRefreshAndDeviceAndLicense` did not verify refresh token via `backend.Storage.GetRefreshToken`; `persistLogin` at 63.6% | Added `backend.NewStorage` + `GetRefreshToken` round-trip verification in the test. Added `TestPersistLogin_DeviceWriteError` for device.Write error branch. Coverage: 63.6% → 72.7% | ✓ tests pass |
| 4 | Low | Happy path only checked `"Authentication complete."` — missing email suffix. `emailFromLicenseJWT` at 70% with no malformed-JWT tests | Changed assertion to check full `"Authentication complete. Welcome, smoke@example.com."`. Added `TestEmailFromLicenseJWT` table test covering: valid JWT, no-dots, invalid base64, valid-base64-not-JSON, missing-email-claim, empty string. Coverage: 70% → 100% | ✓ tests pass |
| 5 | Low | `expiresIn == 0` edge case undocumented by any test | Added `TestLogin_ExpiresInZero_ExitFour` using an inline server that returns `expires_in: 0`; asserts exit code 4 and "Code expired" message | ✓ tests pass |
| 6 | Low | `newDeviceCodeServer` helper missing `"access_denied"` and `"server_error"` behaviors | Added both cases to the switch in `newDeviceCodeServer` | ✓ tests pass |
| 7 | Low | `captureLoginRun` comment claimed "safe to run in parallel" — false guarantee since tests mutate shared package-level vars | Corrected comment to: "Uses writer injection for stdout/stderr; package-level stubs are reset via t.Cleanup so tests must run sequentially (do not call t.Parallel)" | ✓ reviewed |

## Iteration 2 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Spec compliance gap: task YAML behavior #6 stated "all four are persisted" but `access_token` was intentionally dropped per Arch Decision #7 with no corresponding YAML update | Updated behavior #6 in `management/tasks/M14-005.yaml` to explicitly document that `license_jwt`, `refresh_token`, and `device_id` are persisted, and `access_token` is held in-memory only (short-lived, regenerated on demand by `curlew license --refresh`). No code change required — the implementation was already correct per the architectural decision. | ✓ YAML updated, no test regressions |
| 2 | Low | `tierFromLicenseJWT` at 70% branch coverage — error paths untested. `emailFromLicenseJWT` (structurally identical) had full 6-case table coverage. | Added `TestTierFromLicenseJWT` in `cmd/curlew/login_test.go` with 6 subtests: valid JWT with tier claim, not a JWT (no dots), invalid base64, valid base64 but not JSON, valid JSON but missing tier claim, and empty string. `tierFromLicenseJWT` now at 100% branch coverage. | ✓ tests pass, coverage 100% |

## Out of Scope (Deferred)

No findings deferred in iteration 2. All iteration 2 findings resolved.

(Iteration 1 partial deferral: `NewStorage` and `SetRefreshToken` error branches in `persistLogin` require a Storage seam not in task scope — documented in iteration 1 report.)

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS (50 packages) |
| `golangci-lint run` | PASS (0 issues) |
| `cmd/curlew` coverage | 81.6% |
| `internal/backend/device` coverage | 84.2% |
| `tierFromLicenseJWT` coverage | 100% (was 70%) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| ac887533 | test(login): resolve review findings #1-#7 — coverage + correctness | Iter 1: #1–#7 |
| e2ee628e | fix(login): add TestTierFromLicenseJWT + clarify access_token persistence in task YAML | Iter 2: #1, #2 |

## Summary

Iteration 2: 2/2 findings resolved. 0 deferred.
Cumulative (both iterations): 9/9 findings resolved.
