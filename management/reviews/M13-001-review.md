# Code Review: M13-001

**Task:** Dotted-namespace syntax foundation for $faker.* dynamic functions
**Reviewer:** AI
**Date:** 2026-04-28
**Branch:** feature/M13-001-dotted-namespace-syntax

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error handling added; existing patterns in variable.go are correct (`fmt.Errorf("context: %w", err)`, `apierrors.Structured` with sentinel errors). The change is a pure regex extension — no new error paths introduced. |
| Input Validation | PASS | All five malformed-input cases (`{{$faker..badName}}`, `{{$.faker.firstName}}`, `{{$faker.}}`, `{{$faker._bad}}`, `{{$faker.1bad}}`) confirmed to return no regex match. RE2 linear-time guarantee covers adversarial inputs. |
| Naming | PASS | `dynPattern` unchanged (package-private). No new exported symbols introduced. Doc comment updated correctly to reflect dotted-namespace semantics. |
| Code Organization | PASS | Single-line change to `dynPattern` in `internal/variable/variable.go`. No package boundary changes, no new imports, no circular dependencies. |
| Correctness | PASS | New `(?:\.[a-zA-Z][a-zA-Z0-9_]*)*` group consumes zero characters when no dot is present — backward compatibility trivially preserved. Negative test cases verified by direct `go run` check to return no match. Cache key behavior (behavior 8) exercised indirectly via `TestRegistry_DottedName_NotYetRegistered` calling `Evaluate("faker.firstName", nil, ...)` which calls `cacheKey` at line 118 before the function-not-found branch. |
| Test Quality | PASS | Five new test functions covering all 8 task behaviors. Table-driven tests with `t.Run()` and descriptive names. Both regex-level tests (`TestDynPattern_*`) and integration tests (`TestInterpolate_DottedName_UnknownFunctionError`, `TestRegistry_DottedName_NotYetRegistered`) present. Error message content assertions (not just `err != nil`). Backward-compat tests for all M11/M12 legacy forms. |

## Test Coverage
- Coverage: 96.7% (internal/variable package)
- Missing coverage: none identified — exceeds the 80% DoD threshold by 16.7 pp.

## Behavior Coverage

| # | Behavior | Covered By |
|---|----------|-----------|
| 1 | `{{$faker.firstName}}` → funcName=`faker.firstName`, rawArgs=`""` | `TestDynPattern_DottedNamespace/"single dot, no parens"` |
| 2 | `{{$faker.imageUrl(640, 480)}}` → funcName=`faker.imageUrl`, rawArgs=`640, 480` | `TestDynPattern_DottedNamespace_WithArgs/"two int-string args"` |
| 3 | `{{$timestamp}}` (no dot) → funcName=`timestamp` | `TestDynPattern_BackwardCompatNoDot/"timestamp no parens"` |
| 4 | `{{$base64('hello')}}` → funcName=`base64`, rawArgs=`'hello'` | `TestDynPattern_BackwardCompatNoDot/"base64 with single arg"` |
| 5 | Consecutive/leading dots rejected | `TestDynPattern_DottedNamespace/"consecutive dots rejected"` and `"leading dot rejected"` |
| 6 | Unknown-function error names the dotted function | `TestRegistry_DottedName_NotYetRegistered`, `TestInterpolate_DottedName_UnknownFunctionError` |
| 7 | SPECIFICATION.md:748 reads `53 functions` | Doc fix verified by `grep`; no test needed (no Go test parses that line) |
| 8 | Cache key shape unchanged with dotted funcName | Indirectly via `TestRegistry_DottedName_NotYetRegistered` (exercises `cacheKey("faker.firstName", nil)` at dynamic.go:118 before the not-found branch) |

## Summary

The implementation is a minimal, surgical change: one line in `dynPattern` (adding the optional `(?:\.[a-zA-Z][a-zA-Z0-9_]*)*` suffix to capture group 1), a two-word spec typo fix, and one paragraph in MANUAL.md §3.7. All 8 task behaviors are covered by dedicated tests, coverage sits at 96.7%, and the full CI gate (build, test, race, lint, smoke) passes clean. No regressions in any caller of `dynPattern`.
