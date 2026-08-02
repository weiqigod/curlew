# Verification Report: M8-005

**Task:** run --only: prune setup to transitive closure of {{variable}} references
**Verified by:** AI
**Date:** 2026-04-24
**Branch:** feature/M8-005-only-prune-setup
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 0 failures, all packages pass |
| `go test -race ./...` | PASS | No races detected (run via ci-local.sh) |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage: internal/parallel | 90.1% | Meets >= 80% threshold |
| Coverage: internal/runner | 84.1% | Meets >= 80% threshold |
| `ci-local.sh` | PASS | All gates passed |

## Observable Output

The observable scenario is verified via integration tests:

- `TestRunner_OnlyPrunesSetup/Get_user_prunes_seed_users_and_seed_posts` — Case 1: only Login + Warm cache + Get user execute
- `TestRunner_OnlyPrunesSetup/List_posts_prunes_Login_only` — Case 2: Seed users + Seed posts + Warm cache + List posts execute
- `TestRunner_OnlyPrunesSetup_AnalyzerFallback` — Case 3: analyzer fallback with stderr diagnostic
- `TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly` — Case 4: no pruning without --only

Expected: Setup pruned to transitive closure of variable references
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Setup pruned to transitive closure on --only | `TestRunner_OnlyPrunesSetup` (2 sub-tests) | PASS |
| 2 | Analyzer seeded with preExecVars from scope.Resolved() | `TestRunner_BuildPreExecVarSet` + integration tests | PASS |
| 3 | No-extract setup items always included | `TestRunner_OnlyPrunesSetup_NoExtractSeeder` | PASS |
| 4 | Single Analyze call + AncestorClosure primitive | `TestParallel_AncestorClosure` (12 cases) | PASS |
| 5 | IsValid==false fallback to full setup + stderr diagnostic | `TestRunner_OnlyPrunesSetup_AnalyzerFallback` + `TestRun_OnlyAnalyzerFallback_StderrDiagnostic` | PASS |
| 6 | Teardown unaffected | `TestRunner_OnlyFilter` (existing) | PASS |
| 7 | No pruning without --only (regression) | `TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly` | PASS |
| 8 | enrichSelectionCliff scans fullSetupItems | `TestRunner_OnlyCliff_SetupProducer` | PASS |
| 9 | Data-driven parent item contributes vars | Deferred per task scope | N/A |
| 10 | watch propagates transparently | `TestWatch_OnlyPropagates` (existing) | PASS |
| 11 | AncestorClosure exported primitive | `TestParallel_AncestorClosure` (12 cases) | PASS |
| 12 | No events schema change | No new test needed (additive) | PASS |
| 13 | CHANGELOG.md updated | Verified in diff | PASS |
| 14 | IMPROVEMENT.md W3 + §8 Q3 updated | Verified in diff | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` 0 failures | PASS |
| 2 | TestParallel_AncestorClosure passes (12 shapes) | All 12 sub-tests PASS | PASS |
| 3 | TestRunner_OnlyPrunesSetup passes | 2 sub-tests PASS | PASS |
| 4 | TestRunner_OnlyPrunesSetup_ChainedProducers passes | PASS | PASS |
| 5 | TestRunner_OnlyPrunesSetup_NoExtractSeeder passes | PASS | PASS |
| 6 | TestRunner_OnlyPrunesSetup_AnalyzerFallback passes | PASS | PASS |
| 7 | TestRunner_OnlyCliff_SetupProducer passes | PASS | PASS |
| 8 | Regression: TestRunner_OnlyFilter from M8-004 | PASS (cached) | PASS |
| 9 | Regression: without --only no pruning | `TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly` PASS | PASS |
| 10 | docs/SPECIFICATION.md --only section updated | 4 matches found | PASS |
| 11 | docs/MANUAL.md --only section has worked example | 2 matches found | PASS |
| 12 | CHANGELOG.md [Unreleased] updated | Entry present | PASS |
| 13 | IMPROVEMENT.md W3 + §8 Q3 updated | Entries present with M8-005 reference | PASS |
| 14 | go test ./... passes | 0 failures | PASS |
| 15 | go test -cover internal/parallel >= 80% | 90.1% | PASS |
| 16 | go test -cover internal/runner >= 80% | 84.1% | PASS |
| 17 | golangci-lint run passes | 0 issues | PASS |
| 18 | ./smoke/run.sh passes | Passed via ci-local.sh | PASS |
| 19 | ./scripts/ci-local.sh passes | PASS | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |
| Doc comments on exports | PASS |
| Error wrapping with %w | PASS |

Branch A: Review PASS trusted (management/reviews/M8-005-review.md verdict PASS), spot-check clean:
- `AncestorClosure` has full doc comment at `waves.go:73`
- `buildPreExecVarSet` has doc comment at `runner.go:1164`
- `Diagnostics` field has doc comment in `VarSources`
- `TestParallel_AncestorClosure` table-driven, tests linear-chain/diamond/disconnected/empty-starts shapes

## Commits

| Hash | Message |
|------|---------|
| 21e0451 | chore(task): add M8-005 task file at review status |
| 6181286 | docs(review): add passing review for M8-005 |
| f687ea8 | chore(task): mark M8-005 as review |
| 637401e | docs(plan): update CHANGELOG, SPECIFICATION, MANUAL, IMPROVEMENT for M8-005 |
| a75638c | refactor(runner): fix gofumpt alignment in test map literals |
| 59b2929 | feat(cli): wire stderr to VarSources.Diagnostics for --only fallback diagnostic |
| 4a6b026 | test(cli): add failing integration test for analyzer fallback stderr diagnostic |
| e11e023 | feat(runner): extend enrichSelectionCliff to scan pruned setup producers |
| b82b03b | test(runner): add failing test for cliff diagnostic setup-producer scan |
| c8e2f34 | feat(runner): prune setup phase to transitive closure of variable references for --only |
| 934a0a0 | test(runner): add failing tests for setup pruning with --only |
| afb9ab7 | feat(runner): add buildPreExecVarSet helper, VarSources.Diagnostics and fullSetupForCliff fields |
| c84f3b0 | test(runner): add failing test for buildPreExecVarSet |
| ca5792b | feat(parallel): implement AncestorClosure reverse-BFS graph primitive |
| 977f255 | test(parallel): add failing tests for AncestorClosure |
| cc4a35f | chore(task): mark M8-005 as in_progress |
| f09c3fe | chore(task): mark M8-005 as planned |
| c1d87ce | docs(plan): add implementation plan for M8-005 |

TDD pattern confirmed: test commits precede feat commits throughout.

## Files Changed

| File | Action |
|------|--------|
| `internal/parallel/waves.go` | modified — added AncestorClosure |
| `internal/parallel/waves_test.go` | modified — added TestParallel_AncestorClosure |
| `internal/runner/runner.go` | modified — setup pruning, buildPreExecVarSet, cliff extension |
| `internal/runner/runner_test.go` | modified — all M8-005 runner tests |
| `cmd/apitest/main.go` | modified — wire Diagnostics to stderr |
| `cmd/apitest/main_test.go` | modified — TestRun_OnlyAnalyzerFallback_StderrDiagnostic |
| `CHANGELOG.md` | modified — [Unreleased] Changed entry |
| `docs/SPECIFICATION.md` | modified — --only minimal-setup paragraph |
| `docs/MANUAL.md` | modified — --only worked example |
| `IMPROVEMENT.md` | modified — W3 status + §8 Q3 updated |
| `management/backlog.yaml` | modified — M8-005 status: review |
| `management/plans/M8-005-plan.md` | added |
| `management/reviews/M8-005-review.md` | added |
| `management/tasks/M8-005.yaml` | added |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
