# Improvement Report: M8-004

**Task:** `run --only "<name>"` for single-request execution with duplicate-name rejection
**Date:** 2026-04-24
**Review:** management/reviews/M8-004-review.md

## Context

This is the consolidated improvement report covering all three improve iterations.
- Iteration 1: 5 findings resolved (High x3, Medium x1, Low x1)
- Iteration 2: 4 findings resolved (High x1, Medium x2, Low x1)
- Iteration 3: 3 findings resolved (Medium x1, Low x2)

---

## Iteration 1 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Missing cross-include duplicate test case in `TestParser_DuplicateNameRejection` | Added `"cross-include duplicate rejected"` subtest. | ✓ tests pass |
| 2 | High | `wantMsgParts` missing line-number assertions | Added `dup.yaml:3` and `dup.yaml:5` to `wantMsgParts`. | ✓ tests pass |
| 3 | High | No integration test for `curlew validate` with duplicate-name rejection | Added `TestValidateCmd_DuplicateRequestNames`. | ✓ tests pass |
| 4 | Medium | `--only` not applied in `--show-dependencies` path | Added `filterShowDepsItems`; applied before `parallel.Analyze`. | ✓ tests pass |
| 5 | Low | Dead code `wantPaths` / `_ = wantPaths` | Removed dead code. | ✓ tests pass |

---

## Iteration 2 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `filterShowDepsItems` at 0% test coverage — `--only` + `--show-dependencies` had no test | Added `TestRun_ShowDeps_OnlyFilter` in `cmd/curlew/main_test.go` with three sub-cases: selected request in waves, union of two, no-match exit 3. `filterShowDepsItems` is now at 100%. | ✓ tests pass |
| 2 | Medium | `enrichSelectionCliff` "producer IS selected" early-return branch untested (75% coverage) | Added `"producer in selection falls back to plain error"` sub-case to `TestRun_OnlyVariableCliff`. Also added `TestFilterMainItems` to directly cover the exported wrapper. | ✓ tests pass |
| 3 | Medium | `filterShowDepsItems` near-verbatim duplicate of `filterMainItemsBySelection`; risk of silent divergence | Exported `FilterMainItems` from runner as a thin delegating wrapper. `filterShowDepsItems` now delegates via `runner.FilterMainItems` — one line. | ✓ tests pass |
| 4 | Low | `filterShowDepsItems` used `fmt.Sprintf("%q")` + `strings.Join` instead of `quotedJoin`, yielding inconsistent `(none)` handling | Fixed naturally by delegating to `runner.FilterMainItems` which uses `quotedJoin` internally. | ✓ tests pass |

---

## Iteration 3 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `emptySummary.Total` computed before `--only` filter, causing `run.end.total` to be overstated on the no-match path (`ErrNoMatchingRequests`). Setup+all_main+teardown reported instead of setup+0+teardown. | Updated `runner.Run` to recompute `emptySummary.Total` on the no-match path to `len(col.Setup.Items) + len(col.Teardown.Items)` (zero main items matched), and to the post-filter count `len(setup) + len(filtered) + len(teardown)` on success. | ✓ tests pass |
| 2 | Low | No test asserted `run.end.total` correctness on the no-match events path. | Added `TestRun_OnlyNoMatch_EventsTotal` in `cmd/curlew/main_test.go`: emits events with `--events`, finds `run.end`, asserts `total==0` (fixture has no setup/teardown) and `exit_code==3`. | ✓ tests pass |
| 3 | Low | Multi-name no-match error (`--only A --only B` where neither matches) only mentioned `selection[0]` in the error message, silently omitting other unmatched names. | Changed `filterMainItemsBySelection` to distinguish single vs. multi-name case: single uses `no request named "A"; available: ...`; multi uses `no requests named "A", "B"; available: ...`. Added `TestFilterMainItems/multi-name_no-match_lists_all_unmatched`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage — `internal/parser` | 89.8% |
| Coverage — `internal/runner` | 83.7% |
| Coverage — `internal/output/events` | 95.8% |
| Coverage — `cmd/curlew` | 81.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| f5c26b4 | fix(runner): export FilterMainItems; delegate filterShowDepsItems | iter2 #3, #4 |
| 2081b13 | test(cmd): add TestRun_ShowDeps_OnlyFilter for --only + --show-dependencies | iter2 #1 |
| 503b954 | test(runner): cover enrichSelectionCliff inSelection branch + FilterMainItems | iter2 #2, #3 |
| 3d7e243 | fix(runner): correct run.end.total on --only no-match path; list all unmatched names | iter3 #1, #2, #3 |

## Summary

12/12 total findings resolved across 3 iterations. 0 deferred.
