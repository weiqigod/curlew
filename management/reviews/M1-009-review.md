# Code Review: M1-009

**Task:** Collection-level variables with interpolation
**Reviewer:** AI
**Date:** 2026-03-11
**Branch:** feature/M1-009-collection-variables
**Review round:** 2 (re-review after `/improve`)

## Verdict: PASS

## Findings

No findings. All issues from the previous review have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`, context messages describe WHERE, sentinel errors used correctly, structured errors with CategoryConfig and hints |
| Input Validation | PASS | nil/empty inputs handled gracefully, undefined variables return clear errors listing available variables |
| Naming | PASS | No stuttering, doc comments on all exported symbols, short names in tight scopes |
| Code Organization | PASS | Clean package boundaries (`internal/variable` depends only on `internal/errors`), minimal exported surface, no circular deps |
| Correctness | PASS | Shallow copy before interpolation prevents collection mutation, cycle detection correct (DFS with stack), depth limiting correct (MaxDepth=10), race detector passes |
| Test Quality | PASS | All 7 behaviors covered at unit, integration, and CLI levels; error paths tested for all interpolation targets (URL, headers, query params, body); table-driven tests used |

## Test Coverage
- `internal/variable/`: 94.0%
- `internal/runner/`: 100.0%
- `internal/parser/`: 89.9%
- `cmd/curlew/`: 87.9%
- All packages above 80% threshold

## Behavior Coverage

| # | Behavior | Tests |
|---|----------|-------|
| 1 | Variable in URL interpolated | `TestScope_Interpolate`, `TestRun_with_variables_interpolates_url`, `TestCLIIntegration_variable_interpolation` |
| 2 | Variables in headers, body, query params interpolated | `TestScope_InterpolateMap`, `TestScope_InterpolateBody`, `TestRun_with_variables_interpolates_headers`, `TestRun_with_variables_interpolates_query_params`, `TestRun_with_variables_interpolates_body` |
| 3 | Variable chain followed | `TestScope_Resolve` (simple/two-level chain), `TestScope_Resolve_chain_value` |
| 4 | Circular reference → exit 5 | `TestScope_Resolve` (circular/self/three-way), `TestRun_with_circular_variables_returns_error`, `TestCLIIntegration_variable_circular_error` |
| 5 | Depth > 10 → exit 5 | `TestScope_Resolve` (depth 11), `TestCLIIntegration_variable_depth_limit_error` |
| 6 | Undefined var → exit 5 with available list | `TestScope_Interpolate` (undefined), `TestScope_Resolve_error_messages`, `TestRun_with_undefined_variable_returns_error`, `TestCLIIntegration_variable_undefined_error` |
| 7 | Special characters verbatim | `TestScope_Interpolate` (special characters) |

## Summary
Clean, well-tested implementation. The variable package provides a solid interpolation engine with proper cycle detection and depth limiting. All errors are structured with hints. Runner integration uses shallow copies to prevent mutation. All 7 spec behaviors have multi-level test coverage. Previous review findings (runner body interpolation coverage, CLI depth-limit test) fully resolved.
