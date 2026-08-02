# Code Review: M17-003 (Iteration 2)

**Task:** OAuth 1.0a signer (`type: oauth1`) with HMAC-SHA1 default and HMAC-SHA256 opt-in
**Reviewer:** AI
**Date:** 2026-04-29
**Branch:** feature/M17-003-oauth1-signer

## Verdict: PASS

## Findings

No findings. All 4 findings from iteration 1 are confirmed resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors in production code returned and wrapped with `%w`. Structured errors (`SIGNER_OAUTH1_UNSUPPORTED_METHOD`, `SIGNER_OAUTH1_MISSING_FIELD`) carry category + code + message + hint. No swallowed errors anywhere. |
| Input Validation | PASS | Required params validated at factory time and re-validated post-interpolation. Method validated before HTTP work. `nil` params map, empty strings, nil sensitives set all handled correctly. URL parse error returned. |
| Naming | PASS | No stuttering. All exported symbols have doc comments (`Factory`, `NewSigner`, `WithClock`, `WithNonceSource`, `Option`, `MethodHMACSHA1`, `MethodHMACSHA256`). Package doc thorough. Internal types appropriately unexported. |
| Code Organization | PASS | New package `internal/signer/oauth1/` with narrow interface. `init.go` mirrors M17-002 pattern. Single blank import in `cmd/apitest/main.go`. `internal/auth/profile.go` and `internal/signer/signer.go` unchanged (verified via `git diff`). No circular dependencies. |
| Correctness | PASS | RFC 5849 §3.4.1.1 base string confirmed byte-exact via openssl. HMAC-SHA1 signature fixture (`msrTmwtDEKqeVXeJaufuiXOpbJI=`) independently verified. HMAC-SHA256 fixture (`WRDBO0foVD0tBkZ2wz6TzQJ5c0/KFGz6dfY2eXCcJoA=`) independently verified against the HMAC-SHA256 base string (which carries `HMAC-SHA256` in `oauth_signature_method`, producing a different base string from the SHA-1 case). `realm` excluded from base string per RFC 5849 §3.4.1.3 and asserted by test. Signing key uses `uriEncode(consumer_secret) + "&" + uriEncode(token_secret)` with correct trailing `&`. |
| Test Quality | PASS | All 4 iteration-1 findings resolved: error checks in `TestOAuth1_RealmFirst` and `TestOAuth1_QueryAndFormParams_Merged` added; realm-from-base-string exclusion asserted; `struct_body_ignored` subtest added for `parseFormBody` default branch. All 8 task behaviors covered by named tests. |

## Test Coverage

- Coverage: 92.7% (`internal/signer/oauth1`)
- Uncovered paths (all acceptable — below 80% threshold on no individual function):
  - `resolveNonce` 83.3%: `io.ReadFull` error path (requires injecting a broken reader; low value to test)
  - `parseFormBody` 83.3%: `[]byte` body case not exercised (string + default + return paths covered)
  - `extractVarName` 85.7%: some complex-expression rejection branches not reached
  - `buildBaseString` 88.6%: form-body parse-error return path
  - `Sign` 89.5%: nonce-generation error path (same as resolveNonce)
  - `buildBaseStringForTest` 87.5%: `oauth_token` absence branch

## Iteration 1 Findings — Resolution Confirmed

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 | Medium | `TestOAuth1_RealmFirst` swallowed `NewSigner` and `Sign` errors | FIXED — proper `if err != nil { t.Fatal(err) }` guards at lines 310-314, 316-320 |
| 2 | Medium | `TestOAuth1_QueryAndFormParams_Merged` swallowed `NewSigner` and `buildBaseStringForTest` errors | FIXED — proper error checks at lines 337-341, 351-355 |
| 3 | Low | No assertion that `realm` is absent from the signature base string | FIXED — `buildBaseStringForTest` called in `TestOAuth1_RealmFirst` at lines 322-329 |
| 4 | Low | `parseFormBody` default branch (struct body) untested | FIXED — `struct_body_ignored` subtest added at lines 366-389 |

## Summary

The implementation is correct, complete, and well-tested. RFC 5849 vector correctness is independently confirmed via openssl for both HMAC-SHA1 and HMAC-SHA256. All 4 iteration-1 findings have been properly addressed — the two medium-severity error-swallowing issues in test code are resolved, and the two low-severity test coverage gaps are filled. No production-code defects found at any iteration. Coverage at 92.7% is well above the 80% threshold.
