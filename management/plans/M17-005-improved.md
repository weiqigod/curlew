# Improvement Report: M17-005

**Task:** JWT decode dynamic functions: $jwtDecodeHeader / $jwtDecodeClaims
**Date:** 2026-04-29
**Review:** management/reviews/M17-005-review.md

## Resolved Findings (iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `smoke/run.sh` jwt-decode block used `$jsonpath('$.alg', $jwtDecodeHeader(...))` — `$jsonpath` is not a registered dynamic function; `parseDynArgs` rejects nested function-call arguments; `apitest run` exited 5, aborting the script under `set -euo pipefail` | Rewrote smoke YAML to place `$jwtDecodeHeader` / `$jwtDecodeClaims` directly in request header values (they return JSON strings); python3 parses those JSON strings to verify `alg=HS256` and `sub=1234567890` — equivalent verification to what `$jsonpath` would do | ✓ smoke passes |
| 2 | High | `docs/MANUAL.md §3.7` `extract:` example documented the same bogus `{{$jsonpath('$.sub', $jwtDecodeClaims(...))}}` pattern, implying nested function-call arguments and `$jsonpath` as a dynamic function — neither works at runtime | Replaced the broken `extract:` block with two valid examples: (a) using the functions directly in header values with a comment showing the rendered JSON; (b) a two-request extract-then-inspect pattern using standard `extract: access_token: "$.token"` and then the decoded JSON in a header | ✓ manual accurate |

## Previously Resolved Findings (iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 3 | High | Smoke fixture did not exercise `$jsonpath` chaining (pre-iteration-1 version used raw decode into headers) | Superseded by iteration 2 fix — iteration 1 introduced the broken nested-call approach; now corrected | ✓ |
| 4 | Medium | `TestRegistry_JwtDecode_BadBase64` and `TestRegistry_JwtDecode_BadJSON` did not verify truncation property | Added `TestRegistry_JwtDecode_BadBase64_Truncates` and `TestRegistry_JwtDecode_BadJSON_Truncates` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| `./smoke/run.sh` | PASS |
| `./scripts/ci-local.sh --go` | PASS |
| Coverage | 97.4% (internal/variable) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `109f3e41` | fix(smoke): replace broken $jsonpath dynamic-fn chain with correct JWT decode smoke | #1, #2 |

## Summary

2/2 findings resolved. 0 deferred.
