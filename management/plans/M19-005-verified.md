# Verification Report: M19-005

**Task:** apitest validate CEL parse/type-check + MANUAL.md section
**Verified by:** AI
**Date:** 2026-05-16
**Branch:** feature/M19-005-validate-cel
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages green |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | M19-005 block: cel_validate_good exits 0, cel_validate_bad exits 3 with both error codes |
| Coverage (`internal/validator`) | 92.5% | Exceeds >= 80% threshold |

## Observable Output

```
FAIL smoke/fixtures/cel_validate_bad.yaml is invalid
  [ERROR]   line 3: requests[0].if: ERR_CEL_TYPE: got int, expected bool (source: 1 + 2)
           Hint: The expression must evaluate to bool. Example: response.body.status == 200
  [ERROR]   line 3: requests[0].assertions[0].cel: ERR_CEL_PARSE: ERROR: <input>:1:19: Syntax error: ...
           Hint: Check the CEL expression syntax. See docs/MANUAL.md §3.10 Expression Language (CEL).
exit=3
OK smoke/fixtures/cel_validate_good.yaml is valid
exit=0
--- PASS: TestValidateCel_ParseErrorReported (0.00s)
--- PASS: TestValidateCel_TypeErrorReported (0.00s)
--- PASS: TestValidateCel_WalksAllCelSites (0.00s)
--- PASS: TestValidateCel_TruncatesSourceTo200Chars (0.00s)
--- PASS: TestValidateCel_NoHttpRequestFiredDuringValidate (0.00s)
PASS
```

Expected:
- bad fixture exits non-zero (exit=3) with ERR_CEL_PARSE on `assertions[0].cel` and ERR_CEL_TYPE on `if` naming `int`
- good fixture exits 0 with no error output
- all TestValidateCel_* tests PASS
- `grep -c "Expression Language (CEL)" docs/MANUAL.md` returns >= 1 (actual: 2)

Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `if:` and `assertions: - cel:` sites walked; precise field paths in issues | `TestValidateCel_WalksAllCelSites`, `TestValidateCel_ParseErrorReported`, `TestValidateCel_TypeErrorReported` | PASS |
| 2 | `ERR_CEL_TYPE` for non-bool `if:`; actual type `int` and expected `bool` in message | `TestValidateCel_TypeErrorReported` | PASS |
| 3 | `ERR_CEL_PARSE` for syntactically invalid `assertions: - cel:` | `TestValidateCel_ParseErrorReported` | PASS |
| 4 | 200-rune source truncation + ellipsis marker | `TestValidateCel_TruncatesSourceTo200Chars` | PASS |
| 5 | No HTTP requests sent during validate | `TestValidateCel_NoHttpRequestFiredDuringValidate` | PASS |
| 6 | `docs/MANUAL.md` §3.10 with activation, sites, decision table, disabled functions, error codes | `grep -c "Expression Language (CEL)" docs/MANUAL.md` → 2 | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 5/5 TestValidateCel_* PASS + 4 pre-existing PASS | PASS |
| 2 | Test coverage >= 80% for validate-CEL pass | `internal/validator`: 92.5% | PASS |
| 3 | No build warnings or lint errors | `ci-local.sh` lint gate clean | PASS |
| 4 | `./scripts/ci-local.sh` passes | ci-local PASS, all gates green | PASS |
| 5 | ERR_CEL_PARSE/ERR_CEL_TYPE documented with field-path semantics and 200-char truncation | §3.10 of MANUAL.md contains both codes with message format examples | PASS |
| 6 | MANUAL.md contains Expression Language (CEL) section | §3.10 added: activation, sites, decision table, disabled functions, error codes; grep → 2 | PASS |
| 7 | Smoke fixtures exercised by `./smoke/run.sh` | M19-005 smoke block PASS (good exits 0, bad exits 3, error codes verified) | PASS |
| 8 | `templates/skills/claude/apitest/` untouched | Not in `git diff --name-only main...HEAD` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: review verdict PASS (management/reviews/M19-005-review.md) trusted. Spot-checks:
- `compileCelSite`: uses `errors.As(compileErr, &cerr)` with `%w`-compatible unwrapping; non-`CelError` fallback present
- All new unexported helpers (`validateCelSites`, `compileCelSite`, `celHint`) have doc comments
- `TestValidateCel_TypeErrorReported` asserts both actual type (`int`) and expected type (`bool`) in the message — correctly exercises the behavior it claims

## Commits

| Hash | Message |
|------|---------|
| `d36ee35b` | docs(review): add passing review for M19-005 |
| `d690425d` | chore(task): mark M19-005 as review |
| `eb7d7355` | feat(validator): add CEL smoke fixtures, run.sh block, and MANUAL.md §3.10 |
| `a480e582` | feat(validator): extend CEL walker to cover assertions.cel sites (M19-005) |
| `7def08a8` | test(validator): add failing tests for CEL walker and type-error format |
| `3a0eb63b` | chore(task): mark M19-005 as in_progress |
| `ebc5c37d` | chore(task): mark M19-005 as planned |
| `b25d91e5` | docs(plan): add implementation plan for M19-005 |

TDD pattern visible: `test(validator)` before `feat(validator)`. All commits carry `Refs: M19-005`.

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/validator/validator.go` | modified | Replaced `validateIfExpressions` with `validateCelSites` + `compileCelSite` + `celHint` |
| `internal/validator/validator_test.go` | modified | Added 5 new `TestValidateCel_*` tests |
| `smoke/fixtures/cel_validate_good.yaml` | created | Happy-path fixture (valid if: and assertions.cel) |
| `smoke/fixtures/cel_validate_bad.yaml` | created | Error fixture (ERR_CEL_TYPE on if:, ERR_CEL_PARSE on assertions.cel) |
| `smoke/run.sh` | modified | Added M19-005 block (lines 2857-2883) |
| `docs/MANUAL.md` | modified | Added §3.10 Expression Language (CEL) with TOC entry |
| `management/backlog.yaml` | modified | Status tracking |
| `management/plans/M19-005-plan.md` | created | Implementation plan |
| `management/reviews/M19-005-review.md` | created | Code review report (PASS) |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
