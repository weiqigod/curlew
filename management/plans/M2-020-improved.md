# Improvement Report: M2-020

**Task:** Data-driven YAML support, filtering, and row control
**Date:** 2026-04-08
**Review:** management/reviews/M2-020-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `applyRange` does not validate negative `start_row` values, causing runtime panic | Added bounds checks: clamp negative `start` to 0, return empty for negative `end`. Added 4 test cases for negative start_row/end_row. | ✓ tests pass |
| 2 | Low | `ErrUnsupportedFilter` uses `fmt.Errorf()` instead of `errors.New()`, inconsistent with other sentinel errors | Changed to `errors.New(...)` and added `errors` import | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 91.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 7b15d1a | fix(datadriven): validate negative start_row/end_row in applyRange | #1 |
| f613d28 | fix(datadriven): use errors.New for ErrUnsupportedFilter sentinel | #2 |

## Summary
2/2 findings resolved. 0 deferred.
