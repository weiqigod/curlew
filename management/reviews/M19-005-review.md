# Code Review: M19-005

**Task:** apitest validate CEL parse/type-check + MANUAL.md section
**Reviewer:** AI
**Date:** 2026-05-16
**Branch:** feature/M19-005-validate-cel

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors propagated correctly. The `sniffTeamTemplate` error swallowing (`isTeam, _ :=`) is pre-existing (introduced before M19-005) and intentionally falls through to the collection validator on sniff failure — not introduced by this task. New CEL paths use structured `CelError` unwrapping correctly, with defensive fallback for non-`CelError` compile errors. |
| Input Validation | PASS | `validateCelSites` handles empty `If` strings (skips), empty `CEL.Items` slices (loop doesn't execute), and `NewEvaluator()` failure (returns an internal error Issue). `compileCelSite` returns `(Issue{}, false)` for nil compile error. |
| Naming | PASS | No stuttering. All exported symbols (`Severity`, `Issue`, `ResultKind`, `Result`, `ValidateAuto`, `Validate`, `SeverityError`, `SeverityWarning`, `KindCollection`, `KindTeamTemplate`) have doc comments. New unexported helpers (`validateCelSites`, `compileCelSite`, `celHint`, `sectionDesc`) are well-named and doc-commented appropriately. |
| Code Organization | PASS | `sectionDesc` promoted from function-local to package-level unexported type — correct, since it is now shared between `validateCelSites` and nothing else leaks across package boundaries. `internal/cel` boundary respected (accesses only exported `Evaluator`, `CelError`, `ErrCelType`, `NewEvaluator`). No circular dependencies. |
| Correctness | PASS | Walker visits all three sections (setup, requests, teardown) for both `if:` and `assertions.cel` sites. Field paths are 0-indexed bracket notation matching the spec. Source truncation is delegated to `internal/cel.truncateSource` (already rune-correct). The `celHint` function returns distinct, actionable strings for parse vs type errors. No goroutine leaks; the no-HTTP test uses a goroutine + channel correctly for wall-clock bounding — the goroutine terminates when `Validate` returns and the channel is closed. Race detector passes. |
| Test Quality | PASS | Five new tests cover all five new behaviors. Table-driven style used where appropriate. `requireIssue` helper avoids duplication. Subtests use descriptive names. All behaviors from the task YAML have at least one covering test. The no-HTTP test uses TEST-NET-1 (`192.0.2.1`) per RFC 5737, an appropriate blackhole address. Edge cases covered: long sources (250-rune input → 200-rune truncation), multi-section walker, parse error vs type error distinction. |

## Test Coverage

- Coverage: **92.5%** of statements in `internal/validator/` (exceeds the 80% minimum)
- Uncovered paths:
  - `validateCelSites`: the `NewEvaluator()` error branch (line 248) — requires injecting a broken CEL environment; defensive path only, acceptable.
  - `compileCelSite`: the non-`CelError` fallback branch (line 305) — requires `Compile` to return a non-`*CelError` error, which does not happen in the current `internal/cel` implementation; defensive path, acceptable.
  - Pre-existing: `sniffTeamTemplate` file-read and YAML-unmarshal error paths, `validateTeamTemplate` read/parse error paths.

## Spec Compliance

All six behaviors from the task YAML are satisfied:

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | `if:` and `assertions: - cel:` sites walked; precise field paths in issues | `TestValidateCel_WalksAllCelSites`, `TestValidateCel_ParseErrorReported`, `TestValidateCel_TypeErrorReported`, smoke M19-005 block |
| 2 | `ERR_CEL_TYPE` for non-bool `if:`; actual type `int` and expected `bool` in message | `TestValidateCel_TypeErrorReported` |
| 3 | `ERR_CEL_PARSE` for syntactically invalid `assertions: - cel:` | `TestValidateCel_ParseErrorReported` |
| 4 | 200-rune source truncation + ellipsis | `TestValidateCel_TruncatesSourceTo200Chars` |
| 5 | No HTTP requests fired during validate | `TestValidateCel_NoHttpRequestFiredDuringValidate` |
| 6 | `docs/MANUAL.md` §3.10 contains activation, sites, decision table, disabled functions, error codes | Verified by `grep -c "Expression Language (CEL)" docs/MANUAL.md` → 2 occurrences (TOC entry + section heading) |

## Smoke Test

The M19-005 block in `smoke/run.sh` (lines 2857–2883) correctly:
- Asserts `cel_validate_good.yaml` exits 0
- Asserts `cel_validate_bad.yaml` exits non-zero (exit 3 observed)
- Greps for `assertions[0].cel`, `ERR_CEL_PARSE`, `ERR_CEL_TYPE`, and `int` in the bad output
- Uses `BAD_RC=0` initialised before `||` assignment — correct and more portable than the plan's inline `|| BAD_RC=$?` pattern (the plan version would trigger errexit issues in strict shells; the actual implementation is safer)

## Definition of Done Verification

| Item | Status |
|------|--------|
| All behavior tests pass | PASS — 5 new tests + 4 pre-existing, all green |
| Test coverage >= 80% for validate-CEL pass | PASS — 92.5% |
| No build warnings or lint errors | PASS — `golangci-lint` reports 0 issues |
| `./scripts/ci-local.sh` passes | PASS — full Go gate green including smoke |
| ERR_CEL_PARSE / ERR_CEL_TYPE documented with field-path semantics and 200-char truncation rule | PASS — §3.10 of MANUAL.md |
| MANUAL.md contains Expression Language (CEL) section | PASS — §3.10, TOC entry, activation table, decision table, disabled functions, error codes |
| Smoke fixtures exercised by `./smoke/run.sh` | PASS |
| `templates/skills/claude/apitest/` untouched | PASS — not in `git diff --name-only main...HEAD` |

## Summary

M19-005 is a clean, well-scoped implementation. The `validateIfExpressions` function was correctly refactored into the broader `validateCelSites` walker with an extracted `compileCelSite` helper, reducing duplication and improving testability. The message format correctly embeds actual/expected types for `ERR_CEL_TYPE` and the inner parse message for `ERR_CEL_PARSE`. The MANUAL.md §3.10 section is complete and correctly placed at the end of Part 3. All six task behaviors are covered by tests and smoke assertions.
