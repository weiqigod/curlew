# Improvement Report: M1-008

**Task:** Structured error messages (parse, network, config)
**Date:** 2026-03-11
**Review:** management/reviews/M1-008-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Timeout error message says "request timed out" but behavior #6 requires including the duration | Updated executor.go to patch timeout message/hint after classification with actual elapsed duration (e.g., "request timed out after 10ms") | tests pass |
| 2 | Medium | Custom `contains`/`containsStr` test helpers duplicate `strings.Contains` | Replaced with `strings.Contains`, deleted custom functions, added `"strings"` import | tests pass |
| 3 | Low | `NetworkError.Error()` has 0% test coverage | Added `TestNetworkErrorError` test case | tests pass |
| 4 | Low | `normalizeAndValidateMethod` returns unwrapped `fmt.Errorf` — only parse error not using `Structured` | Wrapped at call site in `ParseFile` with `Structured` error including file path and hint. `errors.Is` chain preserved via `Inner`. | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 93.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 1b4be1c | fix(httpexec): include timeout duration in error message and hint | #1 |
| 9334f0e | refactor(parser): replace custom contains with strings.Contains | #2 |
| d5d6efc | test(errors): add coverage for NetworkError.Error() | #3 |
| 73ba3ae | fix(parser): wrap unsupported method error in Structured | #4 |

## Summary
4/4 findings resolved. 0 deferred.
