# Improvement Report: M2-021

**Task:** Data-driven output formatting (terminal compact/verbose, JSON, TAP)
**Date:** 2026-04-08
**Review:** management/reviews/M2-021-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | FailedIterationDetails test uses only 3 rows (verbose, not compact mode) | Renamed existing test to `_Verbose`; added new `_Compact` test with 12 rows verifying compact summary, failed iterations line, no per-iteration lines, and assertion failure details | tests pass |
| 2 | Medium | Assertion `strings.Contains(stdout, "status")` too weak | Replaced with `strings.Contains(stdout, "expected 404")` matching specific assertion failure text | tests pass |
| 3 | Medium | Stat computation duplicated between `renderDataDrivenGroup` and `buildDataDrivenJSON` | Extracted shared `computeDataDrivenStats` helper with `dataDrivenStats` struct; both functions now call it | tests pass |
| 4 | Medium | Verbose-mode `DataDrivenSummary` does not show failed iteration numbers | Added `failedIndices` parameter to `DataDrivenSummary`; verbose mode now shows "Failed iterations: N, M" line when failures exist | tests pass |
| 5 | Low | Missing quiet-mode suppression tests for `DataDrivenCompactSummary` and `DataDrivenVerboseResult` | Added `VerbosityQuiet` test cases with `wantEmpty` checks to both test functions | tests pass |
| 6 | Low | Missing unit tests for `groupDataDrivenResults` and `buildDataDrivenJSON` | Added `TestGroupDataDrivenResults` (5 cases: contiguous, stop at non-DD, stop at different name, single, middle start) and `TestBuildDataDrivenJSON` (4 cases: aggregate, nil, mixed, multiple groups) | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (overall) | 90.0% |
| Coverage (`cmd/apitest`) | 84.4% |
| Coverage (`internal/output`) | 93.1% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `5abdfa2` | fix(output): extract shared stats helper and add failed indices to verbose summary | #3, #4 |
| `42d6894` | test(output): add quiet-mode suppression tests for data-driven methods | #5 |
| `e84a048` | test(main): add unit tests for helpers and strengthen assertion | #2, #6 |
| `4c0016d` | test(main): add compact-mode failure integration test | #1 |

## Summary
6/6 findings resolved. 0 deferred.
