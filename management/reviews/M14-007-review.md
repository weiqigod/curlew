# Code Review: M14-007 (Iteration 3)

**Task:** CLI: JWKS fetch + cache for offline License JWT verification
**Reviewer:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-007-jwks-fetch-cache

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Previous Iterations

- **Iteration 1** — FAIL: missing `ErrInvalidAlgorithm`/`ErrInvalidType` CLI handling, `%v` instead of `%w`, no sentinel assertions in `TestJWKSClient_NetworkFailure`/`ServerError`, missing `TestJWKSClient_NonJSONBody`. All resolved in iteration 2.
- **Iteration 2** — FAIL: `ErrOffline` (failing live fetcher) fell through to `default` → exit 1 instead of exit 6 (behavior 3 violation); no test for the live-but-failing fetcher path. Both resolved in iteration 3.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error returns use `%w`; sentinel errors defined for every well-known failure (`ErrInvalidAlgorithm`, `ErrInvalidType`, `ErrKeyNotFound`, `ErrOffline`, `ErrSignatureInvalid`); `ErrOffline` now correctly mapped to exit 6 in CLI; best-effort paths (cache write, `writeCache` in `FetchJWKS`) documented and intentional |
| Input Validation | PASS | nil/empty kid → `(nil, false)` from `LookupES256`; wrong curve/alg → rejected; non-64-byte signature → `ErrSignatureInvalid`; non-JSON body → parse error (not network error); cache-meta corrupt/missing → falls back to network fetch |
| Naming | PASS | No stuttering; all exported symbols have doc comments; `VerifyRS256` fully removed with no alias; no `curlew-2025-0x` kid strings remain in production code |
| Code Organization | PASS | `internal/license` never imports `internal/backend`; dependency direction is `cmd → license, cmd → backend`; `defer resp.Body.Close()` present; `JWKSClient` correctly in `internal/backend`; `OnlineFetcher` interface in `internal/license` keeps the boundary clean |
| Correctness | PASS | ES256 R‖S 64-byte signature format correct; `ecdh.P256().NewPublicKey` validates on-curve; soft-TTL short-circuits redundant network calls; stale-cache fallback during outages handled at resolver layer; `algRegistry` checked before key lookup (behavior 4); `typ == "license+jwt"` checked before alg lookup (behavior 5) |
| Test Quality | PASS | All 7 behaviors covered; `TestLicenseValidate_BackendUnreachable_UnknownKid` covers the live-but-failing fetcher path; 8 `JWKSClient` unit tests; table-driven `TestVerifyJWT` with 8 cases; `TestValidator_OnlineFetcherWiring` exercises online→cache path end-to-end; smoke test proves offline-after-warm-up with real binary |

## Test Coverage

- `internal/license/...`: 88.3% (license), 89.3% (jwks), 80.8% (export) — all above 80% DoD threshold
- `internal/backend/...`: 84.6% — above 80% DoD threshold
- `cmd/curlew/...`: 81.5% — above 80% DoD threshold
- Test count in `internal/license/...`: 104 test runs — well above DoD requirement of ≥12
- Uncovered lines: error branches for internal OS I/O failures (e.g. `MkdirAll`, `WriteFile` permission errors in `writeCache`; corrupt cache file mid-read in `cacheIfFresh`; 4xx HTTP responses from JWKS endpoint) and rare `json.Marshal` failures on internal structs — all best-effort paths or OS-level failures not worth mocking in unit tests given the 80%+ threshold is comfortably met

## Summary

All seven task behaviors are implemented and tested. The iteration-2 regression (`ErrOffline` → exit 1 instead of exit 6) is fixed with a new `case errors.Is(err, license.ErrOffline)` branch and a corresponding test. The ES256-only verifier, JWKS resolver chain, `JWKSClient` HTTP fetcher with Cache-Control soft-TTL, help text update, and smoke test proof of offline-after-warm-up all meet the definition of done. No findings remain.
