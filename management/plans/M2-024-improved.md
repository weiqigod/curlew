# Improvement Report: M2-024

**Task:** Advanced retry trigger conditions and method restrictions
**Date:** 2026-04-09
**Review:** management/reviews/M2-024-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | CHANGELOG.md not updated for M2-024 | Added entry under `[Unreleased] > Added` describing retry trigger conditions, status range matching, do_not_retry_on exclusions, method restrictions with warnings, network error / timeout conditions | ✓ build passes |
| 2 | Low | `containsStr` and `searchStr` re-implement `strings.Contains` | Replaced with `strings.Contains` in both `condition_test.go` and `retry_test.go`; removed custom helper functions | ✓ tests pass |
| 3 | Low | Dead code: `if len(parts) != 2` unreachable after `strings.Count(s, "-") != 1` guard | Removed the dead check (lines 99-101) since `SplitN(s, "-", 2)` always yields 2 parts when exactly one dash exists | ✓ tests pass |
| 4 | Low | `Resolve()` shallow-copies structs but slice fields share backing arrays | Deep-copied all slice fields (`StatusCodes`, `StatusRanges`, `Methods`) explicitly in both `RetryOnConfig` and `DoNotRetryOnConfig` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 90.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 0c30862 | fix(retry): remove dead code in ParseStatusRange | #3 |
| 791ed78 | fix(retry): deep-copy slices in Resolve() to prevent aliasing | #4 |
| e3d552d | fix(retry): replace custom string helpers with strings.Contains | #2 |
| 12d3847 | docs(changelog): add M2-024 entry for retry trigger conditions | #1 |

## Summary
4/4 findings resolved. 0 deferred.
