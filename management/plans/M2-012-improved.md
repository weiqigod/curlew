# Improvement Report: M2-012

**Task:** Watch mode terminal UX and incremental feedback
**Date:** 2026-04-01
**Review:** management/reviews/M2-012-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `printRunningTotals` produces "Totals (1 runs)" — grammatically incorrect | Added conditional plural: "1 run" vs "N runs". Updated tests to expect correct singular form and added explicit singular test case in `TestPrintRunningTotals`. | ✓ tests pass |
| 2 | Low | Rerun separator test only checks for "Re-running" but not the changed file name | Added assertion for `"changed: col.yaml"` in the separator output test. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 89.7% (watch), 90.9% (total) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| f65c3b0 | fix(watch): use correct singular/plural for running totals | #1 |
| f8136a5 | test(watch): assert changed file name in rerun separator test | #2 |

## Summary
2/2 findings resolved. 0 deferred.
