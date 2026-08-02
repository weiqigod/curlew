# Code Review: M1-016

**Task:** Setup and teardown sections
**Reviewer:** AI
**Date:** 2026-03-14
**Branch:** feature/M1-016-setup-teardown

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors use `%w`. Structured errors carry `FilePath`. `executePhase` propagates fatal errors correctly. No swallowed errors |
| Input Validation | PASS | URL validation covers `col.Setup` and `col.Teardown` via three-section loop at `parser.go:93–97`. Empty-collection guard checks all three sections at `main.go:102` |
| Naming | PASS | `Phase`, `PhaseSetup/Main/Teardown`, `TeardownErrors`, `TeardownAssertionErrors`, `IsRequired()`, `PrintSectionHeader` — all idiomatic, doc comments present, no stuttering |
| Code Organization | PASS | Package boundaries respected. `executePhase` cleanly extracted. `visited` map sharing across sections is safe (cycle detection only). No circular deps |
| Correctness | PASS | All 7 task behaviours correctly implemented. Teardown always executes. Required setup failures gate main. Extract variables flow across all three phases. Exit code correctly excludes teardown errors |
| Test Quality | PASS | All 7 behaviours covered. `TestRun_phases` covers all runner behaviours (8 sub-tests). Parser tests cover setup/teardown parsing, required field, extract, external refs, bad method, missing URL. `TestPrintSectionHeader` uses exact-match. Three `TestCLIIntegration_setup_teardown_*` tests exercise the real binary for execution order, teardown-after-main-failure, and exit code isolation |

## Test Coverage

- `internal/runner`: 90.8%
- `internal/parser`: 91.5%
- `internal/output`: 100%
- `cmd/curlew`: 85.2%

All packages above 80% threshold.

## Resolved Findings History

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 | Medium | `validateRequests` only called for `col.Requests` — setup/teardown inline items with missing `url` not validated | **FIXED** — three-section loop at `parser.go:93–97` |
| 2 | Medium | `if len(col.Requests) == 0` guard prevented setup/teardown from executing | **FIXED** — guard now checks all three sections at `main.go:102` |
| 3 | Medium | No `TestCLIIntegration_*` test for setup/teardown behaviour | **FIXED** — three CLI integration tests added: execution order, teardown-after-main-failure, exit code isolation |

## Summary

Implementation is sound, lint-clean, and all prior findings are resolved. All 7 task behaviours are covered by unit tests and three CLI integration tests exercising the real binary. Coverage is above threshold in all packages.
