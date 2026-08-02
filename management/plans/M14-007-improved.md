# Improvement Report: M14-007

**Task:** CLI: JWKS fetch + cache for offline License JWT verification
**Date:** 2026-05-05
**Review:** management/reviews/M14-007-review.md
**Iteration:** 2 (supersedes iteration 1 report)

## Iteration 1 Resolved Findings (previously fixed)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `ErrInvalidAlgorithm` and `ErrInvalidType` fell through to the `default` branch in `licenseValidateOut`, exiting with code 1 (internal error) instead of 6 (verification failure) | Added `errors.Is(err, license.ErrInvalidAlgorithm)` and `errors.Is(err, license.ErrInvalidType)` cases to the switch in `cmd/apitest/license.go`, both mapping to exit 6 with messages `"invalid_algorithm"` / `"invalid_type"`. Added `TestLicenseValidate_InvalidAlgorithm` and `TestLicenseValidate_InvalidType` CLI tests confirming exit 6 and correct stderr messages. | ✓ tests pass |
| 2 | Medium | Two `fmt.Errorf` calls in `jwks_client.go` used `%v` instead of `%w`, discarding the inner error's type and preventing `errors.Is/As` inspection | Changed `fmt.Errorf("%w: %v", ErrNetworkFailure, err)` and `fmt.Errorf("%w: read jwks body: %v", ErrNetworkFailure, err)` to use `%w` as the second verb (Go 1.20+ multi-`%w` support) | ✓ tests pass |
| 3 | Medium | `TestJWKSClient_NetworkFailure` and `TestJWKSClient_ServerError` only asserted `err != nil`, leaving the sentinel-wrapping contract untested | Added `errors.Is(err, ErrNetworkFailure)` assertion to `TestJWKSClient_NetworkFailure` and `errors.Is(err, ErrServerError)` assertion to `TestJWKSClient_ServerError`; also added `errors` import | ✓ tests pass |
| 4 | Low | `TestJWKSClient_NonJSONBody` (non-JSON response body → parse error) was listed in the plan but not implemented; the code path in `fetch` was untested | Added `TestJWKSClient_NonJSONBody` that serves `text/plain` body and verifies an error is returned; also asserts that the error does NOT wrap `ErrNetworkFailure` (it is a parse error, not a network error) | ✓ tests pass |

## Iteration 2 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `ErrOffline` (backend unreachable during JWKS fetch) exits 1 instead of 6. When a JWT's kid is absent from embedded JWKS and disk cache, and the online fetcher fails, `resolver.Resolve` returns `ErrOffline`. The CLI switch had no case for it, falling through to `default` → exit 1. Task behavior 3 requires exit 6. | Added `case errors.Is(err, license.ErrOffline):` to the switch in `licenseValidateOut` (`cmd/apitest/license.go`), mapping to exit 6 with message `"key_not_found: cannot reach backend to fetch JWKS"`. | ✓ tests pass |
| 2 | Medium | No test covered behavior 3's backend-unreachable path. `TestLicenseValidate_UnknownKid` used `APITEST_OFFLINE=1` (nil-fetcher path, exit 6 via `ErrKeyNotFound`), not the live-but-failing-fetcher path (exit 1 regression invisible to test suite). | Added `TestLicenseValidate_BackendUnreachable_UnknownKid` to `cmd/apitest/license_test.go`. The test sets `APITEST_BACKEND_URL=http://127.0.0.1:1` without `APITEST_OFFLINE`, triggering `ErrOffline`, and asserts exit 6 with `"key_not_found"` in stderr. Test was RED before the fix and GREEN after. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`cmd/apitest`) | 81.5% |
| Coverage (`internal/license/...`) | 86.8% (aggregate) |
| Coverage (`internal/backend/...`) | 84.6% (aggregate) |
| Coverage (overall) | 87.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| aed2dda3 | fix(license): resolve all four M14-007 review findings | Iter 1: #1, #2, #3, #4 |
| 5025c6c8 | fix(license): map ErrOffline → exit 6 for backend-unreachable unknown-kid path | Iter 2: #1, #2 |

## Summary

Iteration 2: 2/2 findings resolved. 0 deferred.
All 6 total findings across both review iterations resolved.
