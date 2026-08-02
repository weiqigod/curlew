# Verification Report: M13-001

**Task:** Dotted-namespace syntax foundation for $faker.* dynamic functions
**Verified by:** AI
**Date:** 2026-04-28
**Branch:** feature/M13-001-dotted-namespace-syntax
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, no failures |
| `go test -race ./...` | PASS | No races detected (ci-local.sh race gate) |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage | 96.7% | Meets >= 80% threshold (exceeds by 16.7 pp) |

## Observable Output

```
# go test -run TestDynPattern_DottedNamespace -v ./internal/variable/...
--- PASS: TestDynPattern_DottedNamespace (0.00s)
    --- PASS: .../single_dot,_no_parens
    --- PASS: .../single_dot,_empty_parens
    --- PASS: .../two_dots
    --- PASS: .../consecutive_dots_rejected
    --- PASS: .../leading_dot_rejected
    --- PASS: .../trailing_dot_rejected
    --- PASS: .../dot_then_underscore-leading_rejected
    --- PASS: .../dot_then_digit-leading_rejected

# go test -run TestDynPattern_DottedNamespace_WithArgs -v
--- PASS: TestDynPattern_DottedNamespace_WithArgs (0.00s)

# go test -run TestRegistry_DottedName_NotYetRegistered -v
--- PASS: TestRegistry_DottedName_NotYetRegistered (0.00s)

# go test -run TestDynPattern_BackwardCompatNoDot -v
--- PASS: TestDynPattern_BackwardCompatNoDot (0.00s)

# grep -n '52 functions' docs/SPECIFICATION.md || echo "typo fixed"
typo fixed

# apitest run faker.yaml 2>&1
[ERROR] request "dotted-name probe": URL: unknown dynamic function "faker.firstName"; available: base64, base64Decode, ...
```

Expected: observable tests PASS, spec typo fixed, CLI names dotted function in error
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `{{$faker.firstName}}` → funcName=`faker.firstName`, rawArgs=`""` | `TestDynPattern_DottedNamespace/"single dot, no parens"` | PASS |
| 2 | `{{$faker.imageUrl(640, 480)}}` → funcName=`faker.imageUrl`, rawArgs=`640, 480` | `TestDynPattern_DottedNamespace_WithArgs/"two int-string args"` | PASS |
| 3 | `{{$timestamp}}` (no dot) → funcName=`timestamp` | `TestDynPattern_BackwardCompatNoDot/"timestamp no parens"` | PASS |
| 4 | `{{$base64('hello')}}` → funcName=`base64`, rawArgs=`'hello'` | `TestDynPattern_BackwardCompatNoDot/"base64 with single arg"` | PASS |
| 5 | Consecutive/leading/trailing dots rejected | `TestDynPattern_DottedNamespace` negative cases | PASS |
| 6 | Unknown-function error names dotted function | `TestRegistry_DottedName_NotYetRegistered`, `TestInterpolate_DottedName_UnknownFunctionError` | PASS |
| 7 | SPECIFICATION.md:748 reads `53 functions` | `grep -n '52 functions' docs/SPECIFICATION.md || echo "typo fixed"` → "typo fixed" | PASS |
| 8 | Cache key shape unchanged with dotted funcName | Indirectly via `TestRegistry_DottedName_NotYetRegistered` (exercises `cacheKey("faker.firstName", nil)` before not-found branch) | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 5 test functions, all PASS | PASS |
| 2 | `go test ./...` passes | ci-local.sh PASS | PASS |
| 3 | `go test -cover ./internal/variable/...` >= 80% | 96.7% coverage | PASS |
| 4 | `golangci-lint run` passes with 0 issues | ci-local.sh lint gate PASS | PASS |
| 5 | `./smoke/run.sh` passes | ci-local.sh smoke gate PASS | PASS |
| 6 | `./scripts/ci-local.sh` passes | ci-local PASS | PASS |
| 7 | Backward-compatible: all existing `{{$fn}}` and `{{$fn(args)}}` forms match | `TestDynPattern_BackwardCompatNoDot` covers timestamp, uuid, base64, hmacSha256, randomInt | PASS |
| 8 | `docs/SPECIFICATION.md:748` reads `53 functions` | `grep -n '53 functions' docs/SPECIFICATION.md` → line 748 | PASS |
| 9 | `docs/MANUAL.md` §3.7 mentions dotted-namespace names | `grep -n 'Dotted-namespace' docs/MANUAL.md` → line 1222 | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (verdict PASS in management/reviews/M13-001-review.md), spot-check clean — dynPattern doc comment updated with dotted-namespace semantics, no new exported symbols, error handling unchanged.

## Commits

| Hash | Message |
|------|---------|
| 1b4597c | docs(review): add passing review for M13-001 |
| c50f860 | chore(task): sync M13-001.yaml status to review |
| a6f69e9 | chore(task): mark M13-001 as review |
| 7dd9bfa | docs(plan): add dotted-namespace note to MANUAL.md §3.7 |
| 672e04c | docs(plan): fix faker function count 52 → 53 in SPECIFICATION.md |
| a265ad2 | test(variable): add registry and interpolate tests for dotted-name dispatch |
| 4ab0b60 | feat(variable): extend dynPattern to accept dotted-namespace funcNames |
| 50e4be2 | test(variable): add failing tests for dotted-namespace dynPattern |
| b89bbb9 | chore(task): mark M13-001 as in_progress |
| f0a12d8 | chore(task): mark M13-001 as planned |
| 8b355cd | docs(plan): add implementation plan for M13-001 |

TDD pattern visible: `test(variable)` commit (50e4be2) precedes `feat(variable)` commit (4ab0b60).

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/variable/variable.go` | modified | `dynPattern` regex extended with `(?:\.[a-zA-Z][a-zA-Z0-9_]*)*`; doc comment updated |
| `internal/variable/variable_test.go` | modified | Added `TestDynPattern_DottedNamespace`, `TestDynPattern_DottedNamespace_WithArgs`, `TestDynPattern_BackwardCompatNoDot`, `TestInterpolate_DottedName_UnknownFunctionError` |
| `internal/variable/dynamic_test.go` | modified | Added `TestRegistry_DottedName_NotYetRegistered` |
| `docs/SPECIFICATION.md` | modified | Line 748: `52 functions` → `53 functions` |
| `docs/MANUAL.md` | modified | §3.7: added dotted-namespace paragraph at line 1222 |

## Issues Found
None

## Recommendation
PASS — ready for PR and merge
