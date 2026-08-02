# Improvement Report: M1-009

**Task:** Collection-level variables with interpolation
**Date:** 2026-03-11
**Review:** management/reviews/M1-009-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `interpolateRequest` has 72.7% coverage — no runner-level test for body interpolation, headers/query/body error paths untested | Added 4 tests: `TestRun_with_variables_interpolates_body`, `TestRun_with_undefined_variable_in_headers_returns_error`, `TestRun_with_undefined_variable_in_query_params_returns_error`, `TestRun_with_undefined_variable_in_body_returns_error` | ✓ `interpolateRequest` 100%, runner 100% |
| 2 | Low | No CLI integration test for depth-limit error (behavior 5) | Added `TestCLIIntegration_variable_depth_limit_error` with 11-level chain asserting exit code 5 and "depth" in stderr | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (runner) | 100.0% |
| Coverage (overall) | 93.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c2b014e | test(runner,cli): add missing coverage for variable interpolation | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
