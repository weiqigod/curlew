# Code Review: M1-020

**Task:** JSON output format (--format json)
**Reviewer:** AI
**Date:** 2026-03-15
**Branch:** feature/M1-020-json-output-format

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error paths return wrapped errors; all pre-exec error paths correctly branch to JSON in json mode; no swallowed errors |
| Input Validation | PASS | Unknown `--format` values rejected with exit 1 and structured error; `--format` missing value returns error |
| Naming | PASS | No stuttering; doc comments on all exported types in `json.go`; `buildJSONOutput` and `extractJSONOperator` are clear and well-scoped |
| Code Organization | PASS | `internal/output/json.go` is a clean minimal new file; helpers in `main.go` are properly scoped; `runner.RequestResult` fields added without breaking existing callers |
| Correctness | PASS | `status_code` always emitted (no `omitempty`); assertions array always `[]` not `null`; Method/URL populated at all three population sites in executePhase; exit codes identical between JSON and terminal modes |
| Test Quality | PASS | All 7 behaviors covered; in-process tests for all key runCmd branches added; `TestRunPopulates*` tests verify Method/URL fields; `TestBuildJSONOutput*` suite covers all result shapes |

## Test Coverage
- Package `cmd/curlew`: **87.9%** (DoD threshold: 80% ✓)
- Package `internal/output`: **100.0%** ✓
- Package `internal/runner`: **91.2%** ✓
- Total: **93.0%** ✓
- `runCmd` function: **77.4%** (below 80% at function level, but DoD threshold is package-level — met at 87.9%)
- `buildJSONOutput`: **100.0%** ✓
- `extractJSONOperator`: **100.0%** ✓

## Behavior Coverage

| Behavior | Covered | Tests |
|----------|---------|-------|
| `--format json` produces valid JSON | ✓ | `TestWriteJSON`, `TestRunCmdDirect_JSONSuccess`, `TestCLIIntegration_JSONMode_AllFields` |
| JSON contains name, status, duration_ms, requests | ✓ | `TestBuildJSONOutput`, `TestWriteJSON` "all top-level fields present", `TestCLIIntegration_JSONMode_AllFields` |
| Each request contains name, method, url, status_code, duration_ms, assertions | ✓ | `TestCLIIntegration_JSONMode_AllFields`; `TestRunCmdDirect_JSONNetworkError` verifies `status_code: 0` for error requests |
| Failing assertion contains expected, actual, operator, passed:false | ✓ | `TestBuildJSONOutput_assertionFields`, `TestExtractJSONOperator`, `TestWriteJSON` "assertion failure fields present" |
| Errors included in JSON not stderr | ✓ | `TestRunCmdDirect_JSONParseError`, `TestRunCmdDirect_JSONVarError`, `TestRunCmdDirect_JSONNetworkError`, `TestRunCmdDirect_JSONMissingEnv`, `TestCLIIntegration_JSONMode_ParseError` |
| No extraneous text in JSON output | ✓ | `TestWriteJSON` "no extraneous text", JSON parsing in all integration tests |
| assertions array is `[]` not omitted when empty | ✓ | `TestBuildJSONOutput_emptyAssertionsNotNull`, `TestWriteJSON` "empty assertions array not null" |

## Previous Findings Resolution

All three findings from the first review are confirmed resolved:

| # | Finding | Status |
|---|---------|--------|
| 1 | `--format <unknown>` silently fell through to terminal | Resolved — validation block at `runCmd:131` rejects unknown formats with exit 1 |
| 2 | `status_code omitempty` caused field absent for error requests | Resolved — `omitempty` removed; `status_code: 0` always emitted; `TestRunCmdDirect_JSONNetworkError` verifies presence |
| 3 | `runCmd` coverage 68.6%; terminal exit 4/5 and --env error paths untested in-process | Resolved — `TestRunCmdDirect_TerminalNetworkError`, `TestRunCmdDirect_TerminalVarError`, `TestRunCmdDirect_JSONMissingEnv` added; coverage now 77.4% |

## Summary

Implementation is solid end-to-end. The JSON output schema is well-defined, `buildJSONOutput` and `extractJSONOperator` are cleanly implemented and fully covered, all seven behaviors are verified by tests, and the smoke test covers the new `--format json` capability. All three previous findings are confirmed resolved. Lint passes clean; all packages meet the 80% coverage threshold. The `runCmd` function sits at 77.4% (below 80% at function level), but the DoD threshold is package-level coverage which is met at 87.9%.
