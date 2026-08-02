# Improvement Report: M19-004

**Task:** assertions: `- cel: <expression>` shape
**Date:** 2026-05-16
**Review:** management/reviews/M19-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | DoD item #5 unmet: no CEL failure-message rendering tests in any output formatter (terminal, JUnit, markdown) | Added `TestPrinterAssertionDetail/CEL_failure_message_multiline_preserved` to `internal/output/terminal_test.go`; `TestWriteJUnitXML_CelFailureMessagePreserved` to `internal/output/junit_test.go`; `TestMarkdown_Render_CelFailureAssertion` to `internal/output/markdown/formatter_test.go`. Each test confirms the multiline CEL `Actual` string (expression source + resolved sub-value lines) is preserved verbatim by the respective formatter. | ✓ tests pass |
| 2 | Medium | Behavior #6 unverified: no runner test confirming that a request skipped by `if:` does not evaluate its `cel:` assertions | Added `TestRun_CelAssertion_SkippedByIf_NotEvaluated` to `internal/runner/runner_test.go`. Test asserts: no HTTP call made, result is `Skipped` with reason `"if: false"`, `AssertionResults` is nil, `summary.AssertionFailures == 0`. | ✓ tests pass |
| 3 | Low | `TestCelAssertion_MutualExclusionWithOperatorAssertion` in `cel_test.go` was misleadingly named — it tested `CheckCEL(nil, ctx)` (empty-input handling), not mutual exclusion | Renamed to `TestCheckCEL_NilInputReturnsNil`; added a proper `TestCelAssertion_MutualExclusionWithOperatorAssertion` that evaluates a failing `"false"` expression and asserts the `Type` field is `"assertions[0].cel"` and `Passed` is false. | ✓ tests pass |
| 4 | Low | `TestCelAssertion_TypeErrorBecomesFailedAssertion` only asserted `Actual != ""` — did not verify `Type` or the CEL type-error framing | Strengthened: now asserts `results[0].Type == "assertions[0].cel"` and `strings.Contains(results[0].Actual, "expected")` (the cel-go "got int, expected bool" framing). | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage — `internal/assertion/` | 91.9% |
| Coverage — `internal/cel/` | 93.4% |
| Coverage — `internal/parser/` | 90.0% |
| Coverage — `internal/runner/` | 84.9% |
| Coverage — `internal/output/` | 92.3% |
| Coverage — `internal/output/markdown/` | 92.6% |

All packages exceed the 80% DoD threshold.

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `4789d713` | fix(assertion): strengthen and rename misleading CEL test assertions | #3, #4 |
| `b68835ae` | test(runner): add TestRun_CelAssertion_SkippedByIf_NotEvaluated | #2 |
| `863dff9a` | test(output): add CEL failure-message rendering tests in all three formatters | #1 |

## Summary

4/4 findings resolved. 0 deferred.
