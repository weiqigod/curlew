# Verification Report: M17-003

**Task:** OAuth 1.0a signer (`type: oauth1`) with HMAC-SHA1 default and HMAC-SHA256 opt-in
**Verified by:** AI
**Date:** 2026-04-29
**Branch:** feature/M17-003-oauth1-signer
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected (run via ci-local.sh) |
| `golangci-lint run` | PASS | No findings (run via ci-local.sh) |
| `./smoke/run.sh` | PASS | OAuth1 smoke: "PASS: OAuth1 Authorization header present" |
| Coverage `internal/signer/oauth1` | 92.7% | Meets >= 80% threshold |

## Observable Output

All 6 observable test commands from the task YAML pass:

```
=== RUN   TestOAuth1_HMACSHA1_RFC5849Vector
--- PASS: TestOAuth1_HMACSHA1_RFC5849Vector (0.00s)
=== RUN   TestOAuth1_DefaultMethodIsHMACSHA1
--- PASS: TestOAuth1_DefaultMethodIsHMACSHA1 (0.00s)
=== RUN   TestOAuth1_HMACSHA256_OptIn
--- PASS: TestOAuth1_HMACSHA256_OptIn (0.00s)
=== RUN   TestOAuth1_RejectsUnsupportedMethods/RSA-SHA1
--- PASS: TestOAuth1_RejectsUnsupportedMethods/RSA-SHA1 (0.00s)
=== RUN   TestOAuth1_RejectsUnsupportedMethods/PLAINTEXT
--- PASS: TestOAuth1_RejectsUnsupportedMethods/PLAINTEXT (0.00s)
=== RUN   TestOAuth1_AuthorizationHeaderShape
--- PASS: TestOAuth1_AuthorizationHeaderShape (0.00s)
=== RUN   TestOAuth1_SecretsSensitivePropagation
--- PASS: TestOAuth1_SecretsSensitivePropagation (0.00s)
```

Smoke test Authorization header begins with `OAuth `: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | RFC 5849 §3.4.1.1 base string + HMAC-SHA1 signature byte-exact | `TestOAuth1_HMACSHA1_RFC5849Vector` | PASS |
| 2 | Default method is HMAC-SHA1 | `TestOAuth1_DefaultMethodIsHMACSHA1` | PASS |
| 3 | HMAC-SHA256 opt-in (algorithm swap, same base string structure) | `TestOAuth1_HMACSHA256_OptIn`, `TestOAuth1_HMACSHA256_OptIn_Vector` | PASS |
| 4 | Unsupported methods (RSA-SHA1, PLAINTEXT, etc.) rejected with SIGNER_OAUTH1_UNSUPPORTED_METHOD | `TestOAuth1_RejectsUnsupportedMethods` | PASS |
| 5 | Query + form-body params merged and lex-sorted in base string | `TestOAuth1_QueryAndFormParams_Merged` | PASS |
| 6 | Nonce (≥16 base32 chars) and timestamp from seams | `TestOAuth1_GeneratedNonceAndTimestamp` | PASS |
| 7 | consumer_secret + token_secret sensitive propagation | `TestOAuth1_SecretsSensitivePropagation`, `TestOAuth1_LiteralSecret_NotMarked` | PASS |
| 8 | Authorization header starts with `OAuth `, alphabetical key order, double-quoted percent-encoded values | `TestOAuth1_AuthorizationHeaderShape` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | All 8 behaviors verified above | PASS |
| 2 | `go test ./...` passes | 0 failures across all packages | PASS |
| 3 | `go test -cover ./internal/signer/oauth1/... >= 80%` | 92.7% coverage | PASS |
| 4 | `golangci-lint run` passes with 0 issues | ci-local.sh lint gate clean | PASS |
| 5 | `./smoke/run.sh` passes | "PASS: OAuth1 Authorization header present" | PASS |
| 6 | `./scripts/ci-local.sh` passes | "=== ci-local PASS ===" | PASS |
| 7 | RFC 5849 §3.4.1.1 test vector passes byte-exactly | `TestOAuth1_HMACSHA1_RFC5849Vector` + fixture files | PASS |
| 8 | Smoke fixture exercises OAuth1-signed request end-to-end | smoke/run.sh oauth1 block outputs Authorization with `OAuth ` prefix | PASS |
| 9 | docs/MANUAL.md signing-types table includes oauth1 row | MANUAL.md:2808 updated with HMAC-SHA1 default, HMAC-SHA256 opt-in, RSA-SHA1/PLAINTEXT not supported | PASS |
| 10 | `internal/auth/profile.go` remains unchanged | `git diff main..HEAD -- internal/auth/profile.go` is empty | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling (return, not panic) | PASS |
| Errors wrapped with `fmt.Errorf("context: %w", err)` | PASS — 4 wrapping sites confirmed |
| Sentinel/structured errors for well-known failures | PASS — `SIGNER_OAUTH1_UNSUPPORTED_METHOD`, `SIGNER_OAUTH1_MISSING_FIELD` |
| Naming conventions (no stuttering) | PASS |
| Doc comments on all exports | PASS — Factory, NewSigner, WithClock, WithNonceSource, Option, MethodHMACSHA1, MethodHMACSHA256 |
| Code organization | PASS — new package, init.go mirrors M17-002, single blank import in main.go |
| Test quality | PASS — table-driven tests, error checks not swallowed, RFC vector byte-exact fixtures |

Branch A: Review PASS trusted (management/reviews/M17-003-review.md, iteration 2), spot-check confirmed.

## Commits

| Hash | Message |
|------|---------|
| 4dca35c | docs(plan): add implementation plan for M17-003 |
| 2f89872 | chore(task): mark M17-003 as planned |
| d515c1c | chore(task): mark M17-003 as in_progress |
| 7a4d2d5 | test(oauth1): add failing tests for OAuth1 signer |
| 8b1afe8 | feat(oauth1): implement OAuth 1.0a HMAC-SHA1/SHA256 signer |
| b99faca | refactor(oauth1): fix De Morgan's law lint warning in nonce alphabet check |
| c8caba5 | feat(oauth1): add smoke fixture and update MANUAL.md signing table |
| c0f0921 | docs(plan): update plan with RFC fixture deviation for M17-003 |
| f7cdf0d | chore(task): mark M17-003 as review |
| c21f570 | docs(review): add review with findings for M17-003 |
| 251a1cd | test(oauth1): fix swallowed errors and add missing coverage |
| a9e828a | docs(review): add improvement report for M17-003 |
| a02defd | docs(review): add passing review for M17-003 |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/signer/oauth1/oauth1.go` | created | Full OAuth1 implementation |
| `internal/signer/oauth1/oauth1_test.go` | created | 14 test functions, 92.7% coverage |
| `internal/signer/oauth1/init.go` | created | init-time registration |
| `internal/signer/oauth1/testdata/rfc5849-example/base-string.txt` | created | RFC 5849 base string fixture |
| `internal/signer/oauth1/testdata/rfc5849-example/signature.txt` | created | HMAC-SHA1 fixture (msrTmwtDEKqeVXeJaufuiXOpbJI=) |
| `internal/signer/oauth1/testdata/rfc5849-example-sha256/signature.txt` | created | HMAC-SHA256 fixture (WRDBO0foVD0tBkZ2wz6TzQJ5c0/KFGz6dfY2eXCcJoA=) |
| `cmd/curlew/main.go` | modified | blank import for oauth1 |
| `smoke/run.sh` | modified | oauth1 smoke fixture |
| `docs/MANUAL.md` | modified | oauth1 row in signing-types table + example block |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
