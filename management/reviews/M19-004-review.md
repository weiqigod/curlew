# Code Review: M19-004

**Task:** assertions: `- cel: <expression>` shape
**Reviewer:** AI
**Date:** 2026-05-16
**Branch:** feature/M19-004-cel-assertions
**Iteration:** 2 (re-review after /improve)

## Verdict: PASS

## Findings

No findings. All four findings from iteration 1 have been resolved, and no new issues were identified.

## Iteration 1 Finding Resolution

| # | Prior Finding | Status |
|---|---------------|--------|
| 1 | DoD #5 unmet — no CEL golden tests in output formatters | Fixed: `TestPrinterAssertionDetail/CEL_failure_message_multiline_preserved` added to terminal_test.go; `TestWriteJUnitXML_CelFailureMessagePreserved` added to junit_test.go; `TestMarkdown_Render_CelFailureAssertion` added to markdown/formatter_test.go |
| 2 | Missing `TestRun_CelAssertion_SkippedByIf_NotEvaluated` | Fixed: test added to runner_test.go, verifies Skipped=true, AssertionResults=nil, summary.Skipped=1, summary.AssertionFailures=0 |
| 3 | `TestCelAssertion_MutualExclusionWithOperatorAssertion` was testing nil-input, not mutual exclusion | Fixed: renamed to `TestCheckCEL_NilInputReturnsNil`; proper `TestCelAssertion_MutualExclusionWithOperatorAssertion` added that parses a YAML doc with cel:+eq: on same entry and asserts `ErrCelAndOperatorMutuallyExclusive` |
| 4 | `TestCelAssertion_TypeErrorBecomesFailedAssertion` had weak assertions | Fixed: test now checks `results[0].Type == "assertions[0].cel"` and `strings.Contains(results[0].Actual, "expected")` |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`. Sentinel `ErrCelAndOperatorMutuallyExclusive` properly defined, used with `%w`, and registered in hints_init.go. `compileCELAssertion` and `evaluateSubexpression` return errors without swallowing. `buildCELCtxForItem` is only called after `vars.CelEvaluator != nil` check. |
| Input Validation | PASS | Empty CEL expression strings rejected in both scalar and mapping YAML form. `CheckCEL` returns nil on empty input. `nil` SensitiveSet fields are handled via nil-safe `Names()`/`Values()` methods on `SensitiveSet`. CEL context only built when `result != nil`. |
| Naming | PASS | No stuttering. `CELInput`, `CELContext`, `CELAssertions`, `CELAssertion`, `CheckCEL`, `CollectTopLevelRefs`, `buildCELCtxForItem`, `toCELInputs`, `snapshotSensitiveValues` all follow Effective Go. All exported symbols have doc comments. Package names are lowercase single-word. |
| Code Organization | PASS | `internal/cel/refs.go` keeps source-splitting logic in the cel package. `internal/assertion/cel.go` encapsulates evaluation. Parser YAML shape confined to `internal/parser/`. No circular imports. All three `assertion.Evaluate` call sites in runner.go are wired with CEL context (lines 2182, 2650, 2884). |
| Correctness | PASS | Compile cache (`assertProgCache`) shared across all `cel:` entries per run. `previous` snapshot taken before update so assertions see the prior-request response. Parallel fallback fires at `runPhases` before any phase executes. `evaluateSubexpression` uses `nil` expectType (DynType) to accept any expression type for sub-expression resolution. Sensitive values sorted longest-first. `SensitiveSet.Names()` and `Values()` are nil-safe. `collectionHasCelAssertions` checks all three phases (setup/requests/teardown). |
| Test Quality | PASS | All 7 task behaviors are covered. Five named test functions from the task observable all pass. Golden tests in all three output formatters. Runner-level integration tests. Parser-level YAML shape tests. Refs-collector unit tests. Smoke fixture exercises the full stack. |

## Test Coverage
- `internal/assertion/`: 91.9%
- `internal/cel/`: 93.4%
- `internal/parser/`: 90.0%
- `internal/runner/`: 84.9%
- All packages exceed the 80% DoD threshold.

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| #1 — true assertion recorded as pass | `TestCelAssertion_TruePasses` (assertion + runner) |
| #2 — false assertion: failure message includes expression source + resolved ref values | `TestCelAssertion_FalseFails`, `TestCelAssertion_FailureMessageIncludesResolvedSubvalues`, `TestCelAssertion_FalseFails` (runner) |
| #3 — cel:+operator key mutual exclusion rejected at parse time | `TestParse_CelAssertion_MutualExclusionWithOperator`, `TestCelAssertion_MutualExclusionWithOperatorAssertion` (runner) |
| #4 — sensitive value replaced with [REDACTED] in failure message | `TestCelAssertion_SensitiveValueRedactedInFailureMessage` |
| #5 — non-bool expression produces failed assertion with ERR_CEL_TYPE framing | `TestCelAssertion_TypeErrorBecomesFailedAssertion` |
| #6 — if:-skipped request: no cel: assertion evaluated, not counted | `TestRun_CelAssertion_SkippedByIf_NotEvaluated` |
| #7 — `request` identifier: compilation fails with undeclared reference | `TestCelAssertion_RequestIdentifierRejected` |

## Summary

All four findings from iteration 1 have been addressed. The implementation is correct, well-tested, and fully compliant with the project's Go Development Standards. The parser, assertion evaluator, CEL reference collector, runner wiring, output formatter golden tests, and smoke fixture all work correctly with the CI gate passing cleanly.
