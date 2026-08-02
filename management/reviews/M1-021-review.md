# Code Review: M1-021

**Task:** TAP output format (--format tap)
**Reviewer:** AI
**Date:** 2026-03-17
**Branch:** feature/M1-021-tap-output-format

## Verdict: PASS

*(Second review after `/improve`. All findings from first review resolved.)*

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All I/O errors in `WriteTAP`/`writeTAPDiagnostics` returned. Pre-exec bail-outs consistent with JSON pattern. Empty-collection write error now logged to stderr. |
| Input Validation | PASS | `sanitizeTAPName` strips `#`, collapses whitespace runs via `strings.Fields`. `WriteTAP` handles nil/empty slice correctly. |
| Naming | PASS | No stuttering; all exported types have doc comments; unexported helpers clearly named. |
| Code Organization | PASS | TAP formatter isolated in `internal/output/tap.go`; wiring in `cmd/apitest/main.go`; no circular deps; minimal exported surface. |
| Correctness | PASS | TAP 13 format correct (version, plan, ok/not ok, YAML diagnostics, bail-out, SKIP). Exit codes correct. Empty collection emits `1..0`. Summary nil-check consistent with JSON block. |
| Test Quality | PASS | All 5 behaviors covered; table-driven; integration tests cover pass/fail/parse-error/empty collection; `buildTAPOutput` has dedicated unit test; smoke test updated. |

## Test Coverage
- `internal/output`: **87.6%** (above 80% threshold)
- `cmd/apitest`: **85.1%** (above 80% threshold)
- Overall: **85.7%**
- Uncovered lines: I/O error return paths in `WriteTAP`/`writeTAPDiagnostics`/`writeTAPBailout` — acceptable (require error-injecting mock writer)

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| `--format tap` produces TAP output | `TestRunCmdDirect_TAPSuccess`, smoke test |
| Output starts with `1..N` plan line | `TestWriteTAP/version_line_and_plan_line_emitted`, `TestRunCmdDirect_TAPSuccess` |
| Passing assertion → `ok` line | `TestWriteTAP/passing_request_emits_ok_line_with_duration`, `TestRunCmdDirect_TAPSuccess` |
| Failing assertion → `not ok` + diagnostic | `TestWriteTAP/failing_request_emits_not_ok_line_with_YAML_diagnostic_block`, `TestRunCmdDirect_TAPAssertionFailure` |
| No assertions → `1..0` | `TestWriteTAP/no_results_-_plan_is_1..0`, `TestRunCmdDirect_TAPEmptyCollection` |

## Prior Findings (resolved by /improve)

| # | Severity | Finding | Fix |
|---|----------|---------|-----|
| 1 | Low | `summary.Passed`/`summary.Failed` accessed before nil-check in TAP block | Added `var passed, failed int` with `if summary != nil` guard |
| 2 | Low | Empty-collection TAP write error silently discarded | Now logs error to stderr before returning |
| 3 | Low | Missing test for `TAPResult{Passed:false}` with no error and no failures | Added `failing_request_with_no_error_and_no_failures_emits_empty_diagnostic_block` subcase |
| 4 | Low | `sanitizeTAPName` left double-space after stripping `#` | Replaced `strings.TrimSpace` with `strings.Join(strings.Fields(name), " ")` |

## Summary

Clean, spec-compliant implementation. All four prior findings resolved. TAP 13 format correct throughout. Coverage above threshold in all changed packages. All task behaviors have test coverage.
