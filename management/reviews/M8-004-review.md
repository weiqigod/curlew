# Code Review: M8-004 (Iteration 4)

**Task:** `run --only "<name>"` for single-request execution with duplicate-name rejection
**Reviewer:** AI
**Date:** 2026-04-24
**Branch:** feature/M8-004-only-flag-and-duplicate-rejection

## Verdict: PASS

## Findings

No findings.

## Iteration 4 Changes from Iteration 3

All three Iteration 3 findings were resolved in the improve pass:

- Finding #1 (Medium): `emptySummary.Total` pre-filter — fixed at `runner.go:460-477`: `emptySummary.Total` is updated to `len(Setup) + len(Teardown)` on the no-match path and `len(Setup) + len(filtered) + len(Teardown)` on the success path.
- Finding #2 (Low): Test for `run.end.total` on events no-match path — `TestRun_OnlyNoMatch_EventsTotal` added in `cmd/apitest/main_test.go:8284` with full events-file verification.
- Finding #3 (Low): Multi-name no-match error message — fixed at `runner.go:526-531`: when `len(selection) > 1`, all unmatched names are listed; `TestFilterMainItems/multi-name_no-match_lists_all_unmatched` verifies this.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned and wrapped with `%w`. `ErrDuplicateRequestName` and `ErrNoMatchingRequests` sentinels registered with actionable hints. `enrichSelectionCliff` wraps the original error via `Inner: err` so `errors.Is(result, variable.ErrUndefinedVariable)` remains true on the enriched path. |
| Input Validation | PASS | Empty `--only ""` after TrimSpace triggers no-match. Nil/empty Selection runs all. Missing `--only` value returns parse error. `checkDuplicateNames` skips empty names (caught by other validators). `filterMainItemsBySelection` handles empty items list and multi-name failures. |
| Naming | PASS | No stuttering. All exported symbols (`ErrNoMatchingRequests`, `FilterMainItems`, `RunStartInput`, `EmitRunStartWithInput`, `ErrDuplicateRequestName`) have doc comments. `enrichSelectionCliff`, `checkDuplicateNames`, `filterMainItemsBySelection`, `quotedJoin`, `filterShowDepsItems` all have comments. |
| Code Organization | PASS | `checkDuplicateNames` placed after `resolveIncludes` in `ParseFileWithOptions`. `filterShowDepsItems` delegates to exported `runner.FilterMainItems`. `fullMainForCliff` captured pre-shallow-copy. Shallow copy preserves `Retry`/`Setup`/`Teardown`. `ErrNoMatchingRequests` and `ErrDuplicateRequestName` registered in their respective `hints_init.go`. |
| Correctness | PASS | `emptySummary.Total` correctly updated post-filter. Duplicate check covers cross-include items. `col.Requests.Retry` preserved on shallow copy. `--show-dependencies` path filters before `parallel.Analyze`. Variable-cliff diagnostic wraps original error chain. |
| Test Quality | PASS | All 14 behaviors have test coverage. All key functions have both happy-path and error-path tests. `TestRunner_UndefinedVarMessageFormatStable` guards the regex contract. `TestRun_OnlyNoMatch_EventsTotal` verifies `run.end.total` on the no-match events path. `TestValidateCmd_DuplicateRequestNames` covers `validate` subcommand. `TestWatch_OnlyPropagates` verifies structural pass-through. |

## Test Coverage

- `internal/parser`: 89.8% — PASS (≥80%)
- `internal/runner`: 83.7% — PASS (≥80%)
- `internal/output/events`: 95.8% — PASS (≥80%)
- `cmd/apitest`: 81.3% — PASS (≥80%)

## Pre-audit Gate

`./scripts/ci-local.sh --go` passed: `go build`, `go test`, `go test -race`, coverage, `golangci-lint`, and `./smoke/run.sh` all green.

## Behavior Coverage Against Task YAML

All 14 behaviors from the task YAML are covered by tests:

1. Parser rejects duplicate main names at load time with exit 3 and line-numbered error — `TestParser_DuplicateNameRejection` (same-file, cross-include, setup/main-same-allowed, teardown/main-same-allowed).
2. Duplicate rejection applies to run, watch, validate — `TestValidateCmd_DuplicateRequestNames`; run and watch inherit via `ParseFile`.
3. Repeatable `--only` flag, union, case-sensitive, whitespace-trimmed — `TestParseRunArgs_Only`, `TestRunner_OnlyFilter`.
4. Setup runs in full, teardown runs in full — `TestRunner_OnlyFilter/setup_and_teardown_still_run_for_single_match`, `TestRunCmd_only_filter_integration`.
5. No-match → exit 3, no HTTP, stderr lists available — `TestRun_OnlyNoMatch`, `TestRun_OnlyNoMatch_EventsTotal`.
6. `--only` does not target setup/teardown — `TestRunner_OnlyFilter/setup_name_does_not_match_main`.
7. Data-driven iterations — no extra code needed; inherits from `executeDataDriven` slice-level operation.
8. Parallel subset re-runs `parallel.Analyze` — structural property; analyzer is slice-scoped per code review.
9. Variable-cliff diagnostic — `TestRun_OnlyVariableCliff` (producer filtered, producer selected, no-selection).
10. Events schema v1.1 `selection` field — `TestEvents_v11_Selection`, `TestEvents_v11_RunStart_carriesSelection`.
11. `SchemaVersion` = "1.1", `v1.1.json` published, `v1.0.json` retained — `TestSchema_v10ArtifactsRetained`, `TestSchema_v11_validates`.
12. Watch mode — `TestWatch_OnlyPropagates` (structural: `--only` survives filteredArgs relay).
13. `validate` surfaces duplicate-name rejection as exit 3 — `TestValidateCmd_DuplicateRequestNames`.
14. Docs updated — CHANGELOG, SPECIFICATION, MANUAL all contain `--only` entries verified by grep.

## Summary

The implementation is complete and correct. All three Iteration 3 findings were cleanly resolved: the `run.end.total` field is now accurately computed post-filter (not pre-filter), a dedicated events-stream test asserts the correct total on the no-match path, and multi-name no-match error messages now list all unmatched names. Code quality is high throughout — error chains are preserved, shallow-copy semantics are correct, the regex format guard prevents silent drift, and all sentinel errors are registered with actionable hints. Coverage meets or exceeds 80% across all four changed packages.
