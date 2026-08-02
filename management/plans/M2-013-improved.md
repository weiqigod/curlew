# Improvement Report: M2-013

**Task:** Basic retry logic with configurable max attempts
**Date:** 2026-04-02
**Review:** management/reviews/M2-013-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Missing tests for `RetryCount` in JSON output (`omitempty` behavior untested) | Added `TestWriteJSON/retry_count_omitted_when_zero` and `TestWriteJSON/retry_count_present_when_greater_than_zero` to `json_test.go` | ✓ tests pass |
| 2 | Medium | Missing tests for `RetryCount` in TAP output (`(retry: N)` suffix untested) | Added 4 TAP test cases covering retry suffix on passing/failing requests with RetryCount=0 and RetryCount>0 | ✓ tests pass |
| 3 | Low | `ParseRetryAfter` returns 0 for `Retry-After: 0`, treated as "no header" instead of RFC 9110 immediate retry | Changed return type to `(time.Duration, bool)` — `Retry-After: 0` now returns `(0, true)` signaling immediate retry. Updated caller to use `ok` flag instead of `ra > 0` | ✓ tests pass |
| 4 | Low | `ParseRetryAfter` only handles integer-seconds, not HTTP-date format per RFC 7231/9110 | Added `time.Parse(http.TimeFormat, val)` fallback. Dates in the past return `(0, true)` for immediate retry. Future dates capped at 30s | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/retry/`) | 89.8% |
| Coverage (`internal/output/`) | 92.5% |
| Coverage (total) | 90.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 795458a | fix(retry): support Retry-After: 0 and HTTP-date format in ParseRetryAfter | #3, #4 |
| 3057425 | test(output): add JSON retry_count serialization tests | #1 |
| 460c5db | test(output): add TAP retry_count output tests | #2 |

## Summary
4/4 findings resolved. 0 deferred.
