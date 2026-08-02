# Code Review: M12-006

**Task:** $dateAdd and $dateSubtract relative-time arithmetic
**Reviewer:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-006-dateadd-datesubtract

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `applyDateOffset` returns `apierrors.Structured` with `Code`, `Message`, `Hint`, and `Inner` correctly populated. `Evaluate` wraps with `fmt.Errorf("$%s: %w", name, err)` — uses `%w`, not `%v`. No swallowed errors. |
| Input Validation | PASS | Bad amount (`strconv.Atoi` failure) → `DYNFN_DATE_BAD_AMOUNT`. Bad unit (switch default) → `DYNFN_DATE_BAD_UNIT`. Empty string amount and empty unit both produce structured errors. Wrong arity → `DYNFN_ARITY` from `twoArgs` wrapper. |
| Naming | PASS | `applyDateOffset` has a doc comment naming its sign parameter. All exported symbols have doc comments. No stuttering. Clock seam `r.now` is unexported. |
| Code Organization | PASS | Clock seam is instance-level on `Registry`, preventing cross-test interference. `twoArgs` wrapper reused consistently. `applyDateOffset` is an unexported method, not a package-level function. |
| Correctness | PASS | `result.UTC().Format(...)` applies UTC before formatting. `r.now()` called at evaluation time inside closure, not at registration time. `n *= sign` applied before dispatch. AddDate month-rollover behaviour documented in plan and MANUAL.md. |
| Test Quality | PASS | All 8 task YAML behaviors have a direct test. Both previous findings resolved: `TestRegistry_DateSubtract` now uses `('7', 'day')` per behavior 2; `TestRegistry_DateAdd_AddDateMonthRollover` is now a table test covering both `amount='12'` (no rollover, per behavior 6) and `amount='13'` (rollover). `TestRegistry_DateSubtract_AllUnits` preserves 30-minute coverage. `applyDateOffset` coverage is 100%. |

## Test Coverage
- Coverage: 96.5% (internal/variable package)
- `applyDateOffset`: 100.0%
- Pre-existing gap (not introduced by this task): `SensitiveArgIndex` nil-guard branch (75%), `register` one branch (98.5%)

## Summary

The implementation is clean, correct, and fully aligned with the task specification. The clock seam is well-scoped at the instance level, error types are structured and consistent with the project pattern, and all eight specified behaviors have direct test coverage. Both medium-severity findings from the first review (behavior 2 day-unit mismatch and behavior 6 amount-12 rollover mismatch) are resolved cleanly.
