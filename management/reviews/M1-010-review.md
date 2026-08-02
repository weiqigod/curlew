# Code Review: M1-010

**Task:** Variable extraction from responses
**Reviewer:** AI
**Date:** 2026-03-11
**Branch:** feature/M1-010-variable-extraction

## Verdict: PASS

## Findings

No findings. All code meets project standards.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped in `apierrors.Structured` with sentinel `Inner` values (`ErrExtractionFailed`, `ErrNotJSON`). Callers can match via `errors.Is()`. No swallowed errors, no panics for expected failures. |
| Input Validation | PASS | `Extract` handles nil/empty extractions (returns empty result), empty body (returns `ErrNotJSON`), non-JSON body (returns `ErrNotJSON`). `Set` handles nil maps defensively. |
| Naming | PASS | No stuttering (`variable.Extract` not `variable.VariableExtract`). Doc comments on all exported symbols. Package-level names descriptive, helper names (`stringify`) short and scoped. |
| Code Organization | PASS | `extract.go` in `variable` package — correct domain. Narrow exported surface (`Extract`, `ExtractionInput`, `ExtractionResult`, `Set`, sentinels). No circular dependencies. |
| Correctness | PASS | Sorted extraction order for deterministic errors. `stringify` handles all JSON types (string, nil, float64, bool, objects, arrays). Integer floats formatted without decimal. Runner extraction only runs after assertions pass — correct sequencing. `Passed++` only after extraction succeeds. |
| Test Quality | PASS | 14 extraction unit tests (happy + error paths + edge cases). 9 runner integration tests covering all behaviors. Table-driven with `t.Run`. Testdata fixtures for parser. 100% runner coverage, 94.1% variable coverage, 93.6% overall. |

## Test Coverage

- Overall: 93.6%
- `internal/runner`: 100.0%
- `internal/variable`: 94.1%
- `internal/parser`: 89.9%
- Missing coverage: none critical — uncovered lines are in other packages' pre-existing code

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| Extract with matching JSONPath sets variable | `TestExtract/single_string_value`, `TestRun_extract_sets_variable_for_next_request` |
| Extracted variable interpolated in subsequent request | `TestRun_extract_sets_variable_for_next_request`, `TestRun_extract_sets_variable_used_in_headers` |
| JSONPath no-match returns extraction error | `TestExtract/path_not_found`, `TestRun_extract_path_not_found_fails_request` |
| Non-JSON response returns not-JSON error | `TestExtract/non-JSON_body`, `TestExtract/empty_body`, `TestRun_extract_non_json_body_fails_request` |
| Multiple extractions all set | `TestExtract/multiple_extractions`, `TestRun_extract_multiple_variables` |
| Extraction overrides collection variable | `TestRun_extract_overrides_collection_variable` |
| Extraction in last request, no error | `TestRun_extract_in_last_request_no_error` |

## Summary

Clean implementation following the plan precisely. The extraction logic is well-isolated in `variable.Extract`, the runner wiring is minimal and correctly sequenced (extraction only after assertions pass, pass count only after extraction succeeds), and all seven specified behaviors have test coverage. The `Set` method on `Scope` updates both `resolved` and `vars` maps, ensuring extracted variables appear in error hints for undefined variable messages.
