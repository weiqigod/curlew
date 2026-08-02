# Verification Report: M19-002

**Task:** internal/cel/ foundation: Evaluator + standard activation
**Verified by:** AI
**Date:** 2026-05-16
**Branch:** feature/M19-002-cel-foundation
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected (via ci-local.sh) |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean (via ci-local.sh) |
| Coverage (`internal/cel/`) | 92.6% | Exceeds >= 80% threshold |

## Observable Output

```
=== RUN   Test_truncateSource
--- PASS: Test_truncateSource (0.00s)
=== RUN   TestEvaluator_ParseValidExpression
--- PASS: TestEvaluator_ParseValidExpression (0.00s)
=== RUN   TestEvaluator_StdLibStringOpsAvailable
--- PASS: TestEvaluator_StdLibStringOpsAvailable (0.00s)
=== RUN   TestEvaluator_StdLibListOpsAvailable
--- PASS: TestEvaluator_StdLibListOpsAvailable (0.00s)
=== RUN   TestEvaluator_StandardActivationBindsResponsePreviousVarsEnv
--- PASS: TestEvaluator_StandardActivationBindsResponsePreviousVarsEnv (0.00s)
=== RUN   TestEvaluator_ResponseBodyIsDyn
--- PASS: TestEvaluator_ResponseBodyIsDyn (0.00s)
=== RUN   TestEvaluator_RejectsTimeOfDayNow
--- PASS: TestEvaluator_RejectsTimeOfDayNow (0.00s)
=== RUN   TestEvaluator_RejectsZeroArgTimestamp
--- PASS: TestEvaluator_RejectsZeroArgTimestamp (0.00s)
=== RUN   TestEvaluator_TypeCheckRejectsNonBool
--- PASS: TestEvaluator_TypeCheckRejectsNonBool (0.00s)
=== RUN   TestEvaluator_ParseErrorReturnsErrCelParse
--- PASS: TestEvaluator_ParseErrorReturnsErrCelParse (0.00s)
=== RUN   TestEvaluator_TypeErrorReturnsErrCelType
--- PASS: TestEvaluator_TypeErrorReturnsErrCelType (0.00s)
=== RUN   TestEvaluator_TruncatesSourceTo200CharsInErrorMessage
--- PASS: TestEvaluator_TruncatesSourceTo200CharsInErrorMessage (0.00s)
=== RUN   TestEvaluator_SensitiveObserverFiresOnVarReference
--- PASS: TestEvaluator_SensitiveObserverFiresOnVarReference (0.00s)
PASS
ok      github.com/weiqigod/curlew/internal/cel  0.324s
```

Expected: PASS for all 12 named tests listed in task YAML observable field.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | StandardActivation compiles and evaluates `response.body.x == 1`, returns bool with no error | `TestEvaluator_ParseValidExpression`, `TestEvaluator_StandardActivationBindsResponsePreviousVarsEnv` | PASS |
| 2 | `now()` rejected at compile time with ErrCelParse naming "now" | `TestEvaluator_RejectsTimeOfDayNow` | PASS |
| 3 | `timestamp()` with zero args rejected; single-arg form remains available | `TestEvaluator_RejectsZeroArgTimestamp` | PASS |
| 4 | `"hello".upperAscii()` and `[1,2,3].size()` compile and evaluate correctly | `TestEvaluator_StdLibStringOpsAvailable`, `TestEvaluator_StdLibListOpsAvailable` | PASS |
| 5 | `1 + 1` with expectType=bool returns ErrCelType naming int/bool | `TestEvaluator_TypeCheckRejectsNonBool`, `TestEvaluator_TypeErrorReturnsErrCelType` | PASS |
| 6 | Observer invoked exactly once for `vars.api_key` when in sensitive set | `TestEvaluator_SensitiveObserverFiresOnVarReference` | PASS |
| 7 | Expression > 200 chars truncated with ellipsis in error message | `TestEvaluator_TruncatesSourceTo200CharsInErrorMessage` | PASS |
| 8 | `response.body.items[0].price` evaluates on JSON-decoded body (dyn) | `TestEvaluator_ResponseBodyIsDyn` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./internal/cel/... -v`: 20 tests PASS | PASS |
| 2 | Test coverage >= 80% for `internal/cel/` | `go tool cover`: 92.6% | PASS |
| 3 | No build warnings or lint errors | `golangci-lint run` 0 findings; `go build ./cmd/curlew` clean | PASS |
| 4 | `./scripts/ci-local.sh --go` passes | ci-local.sh exits 0 | PASS |
| 5 | `go.mod` and `go.sum` updated with cel-go pinned to tagged release | `github.com/google/cel-go v0.28.1` in go.mod; `go mod tidy` leaves no diff | PASS |
| 6 | Public package documentation (doc.go) | `internal/cel/doc.go` explains standard activation, disabled time-of-day, and sensitive-observer contract | PASS |
| 7 | `ErrCelParse` and `ErrCelType` exported and reachable via `errors.Is` | Both registered in `internal/errors`; `Unwrap()` chains correctly; verified by `TestCelError_Unwrap` | PASS |

## Code Review

Branch A: Review PASS exists (`management/reviews/M19-002-review.md`). Spot-check performed:

| Check | Item Checked | Status |
|-------|-------------|--------|
| Error wrapping | `NewEvaluator` wraps with `fmt.Errorf("cel: build environment: %w", err)` | PASS |
| Doc comments | All exported types (`Evaluator`, `Program`, `CelError`, `Response`, `StandardActivation`, `SensitiveObserver`, `EvalOptions`, `ErrCelParse`, `ErrCelType`, `NewEvaluator`) have godoc | PASS |
| Test quality | `TestEvaluator_SensitiveObserverFiresOnVarReference` tests: fires once, not for non-sensitive, deduplication, nil-observer no-panic — correctly exercises the behavior | PASS |

## Commits

| Hash | Message |
|------|---------|
| 55ad7a30 | docs(review): add passing review for M19-002 |
| 83f0df1b | docs(review): add improvement report for M19-002 |
| 7810c747 | docs(changelog): add M19-002 entry for internal/cel/ foundation |
| 6f5a5c9a | test(cel): strengthen timestamp rejection and add headers access test |
| 2809515c | fix(cel): remove redundant CelError.Is method |
| e75287f2 | docs(review): add review with findings for M19-002 |
| 743c6544 | chore(task): mark M19-002 as review |
| 64a0a55f | docs(plan): update plan with native-type deviation for M19-002 |
| 46c812c6 | refactor(cel): remove redundant min helper (Go 1.24 builtin) |
| be8f59cf | feat(cel): implement CEL evaluator foundation M19-002 |
| 1142c073 | test(cel): add failing tests for CEL evaluator foundation M19-002 |
| fd6a2e7f | chore(task): mark M19-002 as in_progress |
| 19e5baf9 | chore(task): mark M19-002 as planned |
| 84191f50 | docs(plan): add implementation plan for M19-002 |

TDD pattern visible: `test(cel)` → `feat(cel)` → `refactor(cel)`. All commits use conventional format with scope.

## Files Changed

| File | Action |
|------|--------|
| `CHANGELOG.md` | modified |
| `go.mod` | modified |
| `go.sum` | modified |
| `internal/cel/cel.go` | created |
| `internal/cel/cel_test.go` | created |
| `internal/cel/doc.go` | created |
| `internal/cel/errors.go` | created |
| `internal/cel/errors_test.go` | created |
| `internal/cel/forbid.go` | created |
| `internal/cel/hints_init.go` | created |
| `internal/cel/observe.go` | created |
| `internal/errors/coverage_test.go` | modified |
| `management/backlog.yaml` | modified |
| `management/plans/M19-002-improved.md` | created |
| `management/plans/M19-002-plan.md` | created |
| `management/reviews/M19-002-review.md` | created |
| `management/tasks/M19-002.yaml` | modified |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
