# Improvement Report: M17-003

**Task:** OAuth 1.0a signer (`type: oauth1`) with HMAC-SHA1 default and HMAC-SHA256 opt-in
**Date:** 2026-04-29
**Review:** management/reviews/M17-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestOAuth1_RealmFirst` swallowed `NewSigner` error (`s, _ := NewSigner(p)`) and `Sign` error (`_ = s.Sign(...)`), risking nil-pointer panic and misleading diagnostics | Replaced `_` assignments with proper `if err != nil { t.Fatal(err) }` checks for both `NewSigner` and `s.Sign` | tests pass |
| 2 | Medium | `TestOAuth1_QueryAndFormParams_Merged` swallowed `NewSigner` error (line 319) and `buildBaseStringForTest` error (line 332) | Replaced `s, _ := NewSigner(p)` with error check and `base, _ :=` with error check | tests pass |
| 3 | Low | No test asserted that `realm` is excluded from the signature base string (RFC 5849 §3.4.1.3 security requirement) | Added `buildBaseStringForTest` call in `TestOAuth1_RealmFirst` asserting the base string does NOT contain `"realm"` | tests pass |
| 4 | Low | `parseFormBody` non-string/non-`[]byte` default branch was untested — struct body with `Content-Type: application/x-www-form-urlencoded` contributed no form params silently | Added `struct_body_ignored` sub-test in `TestOAuth1_QueryAndFormParams_Merged` verifying a struct body produces no form params in the base string | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/signer/oauth1`) | 92.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 251a1cd | test(oauth1): fix swallowed errors and add missing coverage | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred. All fixes are confined to `internal/signer/oauth1/oauth1_test.go` (test quality improvements only — no production code changes). Coverage improved from 92.1% to 92.7%.
