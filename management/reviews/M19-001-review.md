# Code Review: M19-001

**Task:** `if:` field on request items (CEL boolean gate)
**Reviewer:** AI
**Date:** 2026-05-16
**Branch:** feature/M19-001-if-conditional

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`. `ErrUnknownDependsOn` is a named sentinel with hint registration. CEL errors propagated with field-path enrichment in both the runner (`cerr.FieldPath = fmt.Sprintf("%s[%s].if", phase, item.Name)`) and the validator. No swallowed errors anywhere in the changed paths. `ensureCelEvaluator` failure is surfaced as a fatal error with wrapped context. |
| Input Validation | PASS | `validateDependsOn` catches unknown `depends_on:` names at parse time. `validateIfExpressions` catches CEL parse errors and type errors before execution. Empty `If` string is the fast-path no-op. CEL type errors produce `ERR_CEL_TYPE`, parse errors produce `ERR_CEL_PARSE`, both with field path and 200-char-truncated source. |
| Naming | PASS | No stuttering. All exported types, functions, and fields have doc comments. `collectionHasIf`, `compileIfProgram`, `buildCelResponse`, `ensureCelEvaluator`, `ensureIfProgCache` are clear and non-stutter. `ErrUnknownDependsOn` follows sentinel naming convention. |
| Code Organization | PASS | `internal/` boundaries respected throughout. Parser stores raw CEL source; runner handles compilation. CEL evaluator wired through `VarSources.CelEvaluator` field with lazy init via `ensureCelEvaluator`. `validateDependsOn` added to `parser.go` after `populateSlugs` in correct order. `ifProgCache` lives on `VarSources` (per-run) not globally. |
| Correctness | PASS | `previous` is updated BEFORE assertion evaluation (line 2136 in `runner.go`), so subsequent `if:` expressions see the actual HTTP response even from assertion-failing requests. `TestIfConditional_PreviousUpdatesOnAssertionFail` verifies this. Skipped items do not update `previous`. Data-driven gate fires before row expansion. Parallel-with-if falls back to sequential with diagnostics line. `previous` is per-phase; first item in each phase sees nil. |
| Test Quality | PASS | All eight task behaviors are covered by named tests. Five observable subtests exist: `TestIfConditional_FalseSkips`, `TestIfConditional_TrueRuns`, `TestIfConditional_SkipsBeforeTemplating`, `TestIfConditional_SkipPropagatesToDependents`, `TestIfConditional_NonBoolRejectedByValidate`. Additional tests cover: sensitive observer routing, parallel fallback, data-driven skipping all rows, previous semantics, env activation, assertion-fail update, cascade skip propagation. All six formatters (terminal, JSON, TAP, JUnit, HTML via main.go, Markdown) have SkipReason tests. Validator has `TestValidate_If_ParseError`, `TestValidate_If_TypeError`, `TestValidate_If_BoolPasses`, `TestValidate_If_FieldPath`. Parser has `TestParseFile_if_field_parsed`, `TestParseFile_depends_on_parsed`, `TestParseFile_depends_on_unknown_name_returns_error`. |

## Test Coverage

- `internal/parser`: 90.3% — above 80% threshold
- `internal/runner`: 85.3% — above 80% threshold
- `internal/validator`: 92.1% — above 80% threshold
- `internal/output`: 92.3% — above 80% threshold
- `internal/output/markdown`: 92.6% — above 80% threshold
- `internal/variable`: 97.3% — above 80% threshold
- `cmd/apitest`: 81.5% — above 80% threshold

All packages meet the 80% threshold.

## Resolved Findings

**Iteration 3 finding (correctness):** `previous` update ordering was fixed. It now fires after a confirmed HTTP response (`execErr == nil && result != nil`) and before assertion evaluation, matching the spec requirement that `previous` reflects the actual HTTP response regardless of assertion outcome.

**Iteration 4 finding (markdown golden test):** `TestMarkdown_RunMD_SkippedEntry` was added to `internal/output/markdown/run_md_test.go`. The test builds a report with two skipped entries (`if: false` and `parent skipped: confirm-pending-order`) and asserts the `- skip:` bullet lines appear, the summary table shows `Skipped == 2`, and the passing entry appears as `- pass:`. The Markdown formatter now has the same level of SkipReason coverage as the other five formatters.

## Summary

The implementation is complete, correct, and well-tested. All eight task behaviors have test coverage, all six formatters render `skipped` with reason, the validate command surfaces `ERR_CEL_PARSE` and `ERR_CEL_TYPE` with field path and truncated source, the smoke fixture exercises the observable, and help text for `apitest run` documents `if:` and `depends_on:`. The gate passes: build clean, all tests green (including race detector), golangci-lint reports 0 issues, smoke passes, all coverages above 80%.
