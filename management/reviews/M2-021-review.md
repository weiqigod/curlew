# Code Review: M2-021

**Task:** Data-driven output formatting (terminal compact/verbose, JSON, TAP)
**Reviewer:** AI
**Date:** 2026-04-08
**Branch:** feature/M2-021-datadriven-output-formatting
**Iteration:** 2 (post-improvement)

## Verdict: PASS

## Findings

No findings. All 6 findings from the iteration 1 review have been addressed:

| # | Original Finding | Resolution |
|---|-----------------|------------|
| 1 | No compact-mode integration test for failed iterations | Added `TestRunCmdDirect_DataDriven_FailedIterationDetails_Compact` with 12 rows |
| 2 | Weak assertion `"status"` matches unrelated strings | Changed to specific `"expected 404"` assertion text |
| 3 | Stat computation duplicated in `renderDataDrivenGroup` and `buildDataDrivenJSON` | Extracted shared `computeDataDrivenStats` helper with `dataDrivenStats` struct |
| 4 | Verbose-mode summary missing failed iteration numbers | Added `failedIndices` parameter to `DataDrivenSummary`; verbose mode now shows "Failed iterations:" line |
| 5 | Missing quiet-mode tests for `DataDrivenCompactSummary` and `DataDrivenVerboseResult` | Added `VerbosityQuiet` test cases with `wantEmpty` checks |
| 6 | Missing unit tests for `groupDataDrivenResults` and `buildDataDrivenJSON` | Added `TestGroupDataDrivenResults` (5 cases) and `TestBuildDataDrivenJSON` (4 cases) |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error paths introduced; new code is pure transformation/rendering. Existing error paths in `executeDataDriven` properly wrap errors with `%w`. |
| Input Validation | PASS | Edge cases handled: empty group (early return), nil Result (zero duration), skipped iterations excluded from stats. Zero-value defaults for new fields maintain backward compat. |
| Naming | PASS | No stuttering; doc comments on all exported types and methods (`DataDrivenJSON`, `DataDrivenHeader`, `DataDrivenCompactSummary`, `DataDrivenVerboseResult`, `DataDrivenSummary`). Package names clean. |
| Code Organization | PASS | Package boundaries respected; terminal methods in `output` package; data-driven metadata on `RequestResult` in `runner` package; rendering logic in `cmd/curlew/main.go` alongside other rendering code. Shared `computeDataDrivenStats` eliminates duplication. |
| Correctness | PASS | Index manipulation in rendering loop (`i = nextIdx - 1` with `continue`) is correct. Compact/verbose threshold works. JSON `omitempty` ensures backward compat. TAP annotation only on first iteration. `failFast` edge case handled: `len(group)` for actual counts, `IterationTotal` for header. |
| Test Quality | PASS | All 6 behaviors covered by tests. Unit tests for helper functions. Table-driven tests with descriptive names. Quiet-mode suppression tested. Integration tests cover terminal compact, terminal verbose, JSON, TAP, and failed iteration details in both modes. |

## Test Coverage
- `internal/output`: 93.1%
- `internal/runner`: 88.0%
- `cmd/curlew`: 84.4%
- Overall: 86.6%
- All above 80% threshold.

## Summary
The implementation cleanly extends all three output formats (terminal, JSON, TAP) to support data-driven results. The compact/verbose threshold logic, data-driven metadata on `RequestResult`, and aggregation helpers are well-structured. All previous review findings have been resolved: duplication eliminated via `computeDataDrivenStats`, verbose summary now includes failed iteration numbers, and comprehensive unit and integration tests cover all specified behaviors. The code is clean, well-documented, and passes all quality gates.
