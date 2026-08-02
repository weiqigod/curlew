# Code Review: M17-005

**Task:** JWT decode dynamic functions: $jwtDecodeHeader / $jwtDecodeClaims
**Reviewer:** AI
**Date:** 2026-04-29
**Branch:** feature/M17-005-jwt-decode-dynamic-functions
**Iteration:** 3

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All three error codes use `*apierrors.Structured` with `CategoryInput`; no `fmt.Errorf("%v")` — `Structured.Inner` carries the wrapped error; no swallowed errors; no panics for expected failures; the defensive `json.Marshal` branch returns an error rather than panicking. |
| Input Validation | PASS | Empty token (1 segment → BAD_FORMAT), nil args (arity error via `oneArg`), invalid base64, invalid JSON, and 0/2/3-arg calls all handled with structured errors and truncated input in messages. |
| Naming | PASS | All new symbols are unexported (`decodeJWTSegment`, `hmacHexLower` etc.). No stuttering. Doc comment present on `decodeJWTSegment`. |
| Code Organization | PASS | Helper lives in `dynamic_helpers.go`; registrations in `dynamic.go`; no circular dependencies; no reaching into other package internals. |
| Correctness | PASS | `base64.RawURLEncoding` (no padding, URL-safe alphabet) used — correct for JWT per RFC 7515 §2. JSON roundtrip via `interface{}` → `json.Marshal` produces deterministic alphabetically-sorted output. Signature segment intentionally not examined (no-verification contract enforced by `TestRegistry_JwtDecode_NoSignatureVerification`). |
| Test Quality | PASS | 11 test functions covering all 8 task behaviors: vector tests, all three error codes, truncation on each error code, arity errors, UTF-8 roundtrip, no-signature-verification. Table-driven tests used where multiple similar cases exist. All tests use `t.Run` with descriptive names. |

## Spec Compliance

All 8 behaviors from the task YAML are covered:

| Behavior | Test(s) | Status |
|----------|---------|--------|
| B1: $jwtDecodeHeader returns correct JSON for JWT.io canonical token | `TestRegistry_JwtDecodeHeader_Vector` | PASS |
| B2: $jwtDecodeClaims returns correct JSON for same token | `TestRegistry_JwtDecodeClaims_Vector` | PASS |
| B3: BAD_FORMAT error for non-3-segment input, truncated to 32 chars | `TestRegistry_JwtDecode_BadFormat`, `TestRegistry_JwtDecode_BadFormat_Truncates` | PASS |
| B4: BAD_BASE64 error for invalid base64-url segment, truncated | `TestRegistry_JwtDecode_BadBase64`, `TestRegistry_JwtDecode_BadBase64_Truncates` | PASS |
| B5: BAD_JSON error for valid base64 but non-JSON bytes, truncated | `TestRegistry_JwtDecode_BadJSON`, `TestRegistry_JwtDecode_BadJSON_Truncates` | PASS |
| B6: Mangled signature still decodes header and claims successfully | `TestRegistry_JwtDecode_NoSignatureVerification` | PASS |
| B7: Arity != 1 returns DYNFN_ARITY error | `TestRegistry_JwtDecode_ArityErrors` | PASS |
| B8: RawURLEncoding + UTF-8 multi-byte roundtrip | `TestRegistry_JwtDecode_UTF8Roundtrip` + vector tests (JWT.io token uses `_` and no padding) | PASS |

## Issues Fixed Since Previous Reviews

- **Iteration 1 (FAIL):** Smoke fixture used `$jsonpath(...)` as a dynamic function with a nested call — not supported; gate failed. Fixed in iteration 2.
- **Iteration 2 (FAIL):** The "fix" applied in improve phase introduced a new broken smoke variant still using `$jsonpath(...)` as a dynamic function. Fixed in the second improve phase: smoke now passes the JWT token as a literal in request headers and extracts `alg`/`sub` by parsing the returned JSON header values with Python — a pattern that correctly exercises `$jwtDecodeHeader` and `$jwtDecodeClaims` end-to-end.

## Test Coverage
- Coverage: 97.4% of statements (`internal/variable` package — well above the 80% floor)
- Missing coverage: None identified

## Pre-audit Gate

`./scripts/ci-local.sh --go` passes all steps: go build, go test, go test -race, coverage (97.4%), golangci-lint (0 issues), smoke. Gate output confirms:

```
PASS: jwtDecodeHeader returned valid JSON with alg=HS256
PASS: jwtDecodeClaims returned valid JSON with sub=1234567890
=== Smoke Test Complete ===
=== ci-local PASS ===
```

## Summary

The implementation is correct and complete. The `decodeJWTSegment` helper uses `base64.RawURLEncoding` (the only correct choice for unpadded JWT segments), JSON-roundtrips via `encoding/json` for deterministic alphabetical key ordering, and returns well-typed structured errors for all three failure modes. The smoke fixture is now correct — it passes the JWT token inline and extracts individual fields from the decoded JSON header values without attempting unsupported nested dynamic-function calls. Documentation in MANUAL.md §3.7 includes the required table rows, usage examples, error code reference, and security caveat. All 8 behaviors are tested; coverage is 97.4%.
