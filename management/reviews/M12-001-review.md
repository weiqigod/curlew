# Code Review: M12-001

**Task:** Argument-parsing foundation for dynamic functions
**Reviewer:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-001-arg-parsing-foundation
**Iteration:** 3 (post-improve, iteration 2)

## Verdict: PASS

## Findings

No findings.

## Iteration History

All findings from previous iterations resolved:

- **Iteration 1, Finding #1** (Medium): `arityError("echo", 0, …)` used wrong expected arity — Fixed.
- **Iteration 1, Finding #2** (Medium): Behavior 6 N-arg arity mismatch had no test — Fixed via `TestInterpolate_ArityMismatch_NArgFunction`.
- **Iteration 1, Finding #3** (Low): `TestRegistry_available_sorted` used soft `>= 15` bound — Fixed to exact `!= 15`.
- **Iteration 1, Finding #4** (Low): Task YAML status stale — Fixed to `status: review`.
- **Iteration 2, Finding #1** (Medium): Double-prefixed arity error: `arityError` embedded `"$funcName:"` in `Message`, then `Evaluate` wrapped again — Fixed: `arityError.Message` is now `"expected %d arguments, got %d"` without prefix; `Evaluate`'s `fmt.Errorf("$%s: %w", name, err)` provides the sole context prefix. Test verifies `strings.Count(err.Error(), "$timestamp") == 1`.
- **Iteration 2, Finding #2** (Low): No `Interpolate`-level test for empty-parens equivalence documented in MANUAL.md §3.7 — Fixed via `TestInterpolate_EmptyParensEquivalence`.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. `parseDynArgs` returns `*apierrors.Structured` with `CategoryInput`. `arityError` returns `*apierrors.Structured` with `DYNFN_ARITY` code. `Evaluate` wraps function errors with `fmt.Errorf("$%s: %w", name, err)` — single prefix, no doubling. Sentinel errors defined for all well-known failure modes. No swallowed errors. |
| Input Validation | PASS | `parseDynArgs`: empty/nil raw → `nil, nil`; unterminated quotes, trailing commas, missing commas, unquoted args all produce structured `CategoryInput` errors. `noArgs` rejects non-empty args via `arityError`. Nil cache handled gracefully in `Evaluate`. |
| Naming | PASS | No stuttering. `parseDynArgs`, `arityError`, `noArgs`, `cacheKey` are clear and correctly scoped (unexported). Exported symbols (`DynFunc`, `Registry`, `Evaluate`, `Available`, `NewRegistry`) all carry doc comments. |
| Code Organization | PASS | `parseDynArgs` and `arityError` are unexported. Single responsibility per function. No circular imports. `defer` used correctly in all test subtests. `internal/variable` boundary respected. |
| Correctness | PASS | NUL-separated cache key prevents collisions. Non-greedy `.*?` in `dynPattern` correctly handles nested `{{var}}` references inside arg literals. Backward-compatible: `{{$timestamp}}` resolves identically to before. `{{$timestamp()}}` (empty parens) memoizes to same value as `{{$timestamp}}` within a request. `TestInterpolate_EmptyParensEquivalence` confirms this end-to-end. |
| Test Quality | PASS | All 9 task behaviors covered. Table-driven tests used throughout. Error paths verified with `errors.As` unwrapping to `*apierrors.Structured`. Empty-parens equivalence tested. Arity mismatches for both zero-arg and N-arg functions tested. Malformed arg list tested. Per-request distinct-args caching tested. Coverage at 96.0% — well above 80% threshold. |

## Test Coverage
- Coverage: **96.0%** (`internal/variable` package — both `variable.go` and `dynamic.go`)
- `dynamic.go` key paths: `arityError` 100%, `noArgs` 100%, `cacheKey` 100%, `Evaluate` 100%, `register` 97.5%.
- `variable.go` key paths: `parseDynArgs` 92%, `Interpolate` 90.2%, `ParseVarFlag` 100%, `ParseEnvVarFlag` 100%.
- All observable tests from task YAML pass (`TestDynPattern_ParensSyntax`, `TestRegistry_Evaluate_AcceptsArgs`, `TestInterpolate_DynamicArgs_NestedVarSubstitution`, `TestInterpolate_BackwardCompatNoArgs`, `go test ./...`).

## Summary
The implementation is correct, well-structured, and complete. Both medium and low findings from iteration 2 have been properly resolved: the doubled function-name prefix in arity error messages is gone (confirmed by `strings.Count` assertion in the test), and end-to-end empty-parens equivalence is now tested. All 9 task behaviors are covered, coverage is 96%, and `./scripts/ci-local.sh --go` passes cleanly with zero lint issues.
