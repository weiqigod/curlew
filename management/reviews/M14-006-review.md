# Code Review: M14-006

**Task:** CLI: curlew license --refresh + --debug + 8-code exit taxonomy
**Reviewer:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-006-license-refresh-and-debug
**Iteration:** 4

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` exits `ci-local PASS`. All Go build, test, race, coverage, lint, and smoke steps pass cleanly.

## Findings

No findings.

## Previous Findings (Iterations 1–3) — All Resolved

| Finding | Resolution |
|---------|-----------|
| Smoke test non-idempotency (stale stub on port) | Fixed: pre-start cleanup with `lsof -ti "tcp:${PORT}" \| xargs kill -9` added at line 2532 |
| `licenseRefreshOut` corrupts cache on flock-timeout / empty LicenseJWT | Fixed: guard `if tokens.LicenseJWT == ""` at line 335 with test `TestLicenseRefresh_EmptyLicenseJWT_DoesNotCorruptCache` |
| `testdata/m14/stub.log` not listed in `.gitignore` | Fixed: `testdata/m14/*.log` pattern added to `.gitignore` at line 71 |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinels `ErrRefreshExpired`, `ErrRefreshReused`, `ErrDeviceMismatch` in `internal/backend/refresh.go`; `translateRefreshError` preserves `*ProblemDetails` via multi-unwrap `refreshSentinelError.Unwrap() []error`; `errors.Is` and `errors.As` both work through the chain; no swallowed errors; best-effort operations (`os.WriteFile` for last_error, `os.Remove`, `enc.Encode`) explicitly blanked with `_ =` and documented as best-effort. |
| Input Validation | PASS | nil token handled in `claimStringFn`, `claimInt64Fn`, `headerKidFn`; missing `device.json` exits 7; missing refresh token exits 2; malformed JWT in `--debug` falls back gracefully (best-effort parse, nil-safe helpers). |
| Naming | PASS | No stuttering; doc comments on all exported symbols (`ErrRefreshExpired`, `ErrRefreshReused`, `ErrDeviceMismatch`, `Tokens`, `RefreshTokens`); unexported helpers named correctly (`translateRefreshError`, `refreshSentinelError`, `refreshCodeToSentinel`). |
| Code Organization | PASS | RFC 7807 → sentinel translation in `internal/backend`; exit-code mapping in `cmd/curlew/license.go`; clear separation of concerns mirrors `loginExitForError` pattern; `defer` used for flock unlock and file close; `internal/backend/hints_init.go` registers the three new sentinels in the error registry. |
| Correctness | PASS | Empty-LicenseJWT guard prevents cache corruption on flock-timeout path (covered by `TestLicenseRefresh_EmptyLicenseJWT_DoesNotCorruptCache`); flock serialises concurrent `--refresh` invocations; previous JWT untouched on exit 3 and exit 6 (covered by `TestLicenseRefresh_PreviousJWTRemainsValidOnNetworkFailure` and `…OnServerError`); `daysUntilExpiry` returns 0 for already-expired tokens (non-negative path) and -1 on any parse error; `errors.As(*ProblemDetails)` traverses the multi-unwrap chain correctly on Go 1.25; context propagation uses `context.Background()` consistent with all other CLI commands. |
| Test Quality | PASS | 13 `TestLicenseRefresh_*` tests (DOD requires ≥10); 4 `TestLicenseDebug_*` tests; 2 help-text tests; all 8 behaviors from the task YAML have corresponding test(s); error paths tested (not just happy path); `errors.As(*ProblemDetails)` asserted in `TestRefreshTokens_AuthRefreshExpired_ReturnsSentinel`; sentinel non-match asserted in `TestRefreshTokens_UnknownAuthCode_BubblesProblemDetails`; smoke test covers 6 exit-code paths (0, 3, 5, 6, 7, no-cache) against the real binary. |

## Test Coverage
- `cmd/curlew`: **81.5%** (above 80% gate — PASS)
- `internal/backend`: **84.4%** (PASS)
- `internal/license`: **88.1%** (PASS)

## Definition of Done Verification

| DOD Item | Status |
|----------|--------|
| `go test ./cmd/curlew/... -run TestLicenseRefresh` passes with ≥10 tests | PASS (13 tests) |
| Real binary invocation against stub produces exit 0, 2/7, 3, 5, 6, 7 | PASS (smoke test covers all six paths) |
| Help text documents every exit code for `--refresh` and `--debug` | PASS (verified by `TestLicenseHelp_DocumentsRefreshExitCodes` and `…DebugExitCodes`) |
| `MANUAL.md` updated with 8-code exit-taxonomy table | PASS |
| `SPECIFICATION.md:8269–8284` cited in `cmd/curlew/license.go` header | PASS |
| `smoke/run.sh` extended to verify `--refresh` exits 2/7 when no cache exists | PASS |

## Summary

All three previously reported findings have been resolved: the smoke test is idempotent (stale-stub cleanup), the flock-timeout / sibling-refresh path no longer corrupts `license.json` with an empty LicenseJWT (empty-JWT guard + test), and `testdata/m14/stub.log` is now covered by the `testdata/m14/*.log` gitignore pattern. The implementation is correct, well-tested, and fully compliant with the Go development standards. No findings remain.
