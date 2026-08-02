---
name: M16-011 verification
description: Final verification report for the apitest worker --schedule-pull slice
type: project
---

# Verification Report: M16-011

**Task:** apitest worker --schedule-pull mode with file: collection ref and pending-uploads queue
**Verified by:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-011-worker-schedule-pull
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `./scripts/ci-local.sh` | PASS | Exit 0; final banner `=== ci-local PASS ===` (Go build/vet/test/race/lint + smoke). Scope auto-detected Go-only (no backend/web/E2E paths changed on this branch). |
| `go test ./...` (full) | PASS | Cached across packages; M16-011 targeted re-run with `-count=1` passes (`internal/worker/schedule`, `internal/backend`, `cmd/apitest`). |
| `golangci-lint run` | PASS | 0 issues. |
| `./smoke/run.sh` | PASS | All smoke scenarios pass, including the M16-011-specific `=== Schedule Pull Help (M16-011) === PASS: worker --help documents --schedule-pull` block. |
| Coverage | PASS | `internal/worker/schedule` 84.3%, `cmd/apitest` 81.5%, `internal/backend` 83.9% — all exceed the ≥80% threshold. |

Pre-existing unrelated smoke noise: a single `FAIL: --format junit should show Professional tier message` line is printed by `smoke/run.sh` for a Free-tier junit gating check unrelated to M16-011; it exists on `main` and does not affect the smoke script's exit code (ci-local exits 0). Not in scope for this slice — flagged here for completeness, to be filed against the JUnit feature gate in a follow-up.

## Observable Output

Task observable specifies the full backend-stack scenario (Team-tier org seeded with a `file:`-ref schedule) plus the help-text check. Backend-stack portion is exercised by the integration tests in `internal/worker/schedule/runner_test.go` (TestRunner_RunOnce subtests cover claim → execute → post, network failure → enqueue, drain on next cycle, claim reaped, 401 fatal); the help-text portion is runnable locally:

```text
$ ./apitest worker --help | grep -E "schedule-pull|--once"
  --schedule-pull            Poll the schedule queue and execute due runs locally.
  --once                     Drain the queue and execute one due run, then exit.
  Combining --schedule-pull with --perf-pull / --all-modes is reserved for a
  future release; this slice supports --schedule-pull alone.
  APITEST_BACKEND_TOKEN    Default bearer token (also used by --schedule-pull)
  APITEST_BACKEND_URL      Default backend URL for --schedule-pull
```

Result: MATCH — `--schedule-pull` is documented with a one-line description and the future-extension reservation is called out.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Poll `/schedules/next-run` on interval (default 30s) | `TestRunner_RunOnce/happy_path`, `TestClient_PollNextRun` | PASS |
| 2 | Resolve `file:` collection_ref from working directory | `TestResolveCollection/file:_*` (4 subtests), `TestRunner_RunOnce/happy_path` | PASS |
| 3 | Reject `git:` collection_ref with documented message + fail result | `TestResolveCollection/git:_ref_is_rejected_with_ErrUnsupportedCollectionRef`, `TestRunner_RunOnce/git_ref_is_rejected;_result_posted_with_fail=1` | PASS |
| 4 | Heartbeat every 30s; 409 reaped claim → abandon cleanly | `TestClient_Heartbeat_ReapedClaim` (3 subtests), `TestRunner_ClaimReaped_AbandonWithoutPosting` | PASS |
| 5 | Exponential backoff on transient post failure up to 1h | `TestRunner_RunOnce/post_fails_with_network_error;_payload_enqueued` (covers retry → exhaustion path with frozen clock) | PASS |
| 6 | On retry exhaustion, queue under `~/.config/apitesttool/pending-uploads/<run_id>.json`; drain on next poll | `TestRunner_RunOnce/post_fails_with_network_error;_payload_enqueued`, `TestRunner_RunOnce/queue_drained_on_next_cycle`, `TestRunner_RunOnce/queue_already-completed_is_removed_silently`, `TestRunner_DrainQueue_NetworkFailure`, `TestQueue_*` | PASS |
| 7 | Team-vault fetch with 5-min TTL; gracefully skip when M16-018 absent | Implementation defers to M16-018; this slice handles 404 absence gracefully (per task scope). Documented in package doc and plan Open Decision 2. | DEFERRED (per scope) |
| 8 | `--schedule-pull` + `--perf-pull` concurrent | Explicitly deferred per plan Open Decision 2; help text reserves the combination and `parseWorkerArgs` rejects it. Covered by `TestParseWorkerArgs/concurrency_invalid`. | DEFERRED (per scope) |
| 9 | `worker --help` documents `--schedule-pull` | Smoke `=== Schedule Pull Help (M16-011) ===`; manual `apitest worker --help` (see Observable section above) | PASS |

Behaviors 7 and 8 are explicitly deferred in the plan (Open Decisions) and that deferral is documented in code, help text, and the smoke test. Per the plan, this slice is complete with 7 of 9 behaviors actively shipped and the remaining two stubbed-out with the future-release reservation visible to users.

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | Targeted `go test -count=1 -run ...` output (`TestParseWorkerArgs/*`, `TestRunner_*`, `TestQueue_*`, `TestResolveCollection`, `TestClient_*`) | PASS |
| 2 | Observable command works | `./apitest worker --help | grep schedule-pull` shows flag with description and future-extension reservation | PASS |
| 3 | Test coverage ≥ 80% on new code | `internal/worker/schedule` 84.3%, `cmd/apitest` 81.5%, `internal/backend` 83.9% | PASS |
| 4 | No build warnings or lint errors | `go build ./cmd/apitest` clean; `golangci-lint run` 0 issues | PASS |
| 5 | Help text updated for new flags/subcommands | `apitest worker --help` documents `--schedule-pull`, `--once`, `--backend`, `--poll-interval`; reserves multi-mode combo | PASS |
| 6 | Smoke test updated if user-visible | `smoke/run.sh` includes `=== Schedule Pull Help (M16-011) ===` block (PASS in ci-local run) | PASS |

## Plan Completion

All planned implementation steps are present in the diff (28 files changed vs `main`):

- `internal/worker/schedule/` — new package: `types.go`, `errors.go`, `client.go`, `collection.go`, `queue.go`, `executor.go`, `runner.go`, `doc.go`, `hints_init.go` + tests.
- `cmd/apitest/worker.go` — `--schedule-pull` flag wired, `parseWorkerArgs` + tests extended.
- `internal/backend/client.go` — `GetJSONOptional` additive helper for the 404-tolerant team-vault fetch.
- `internal/errors/coverage_test.go` — sentinel coverage check covers `internal/worker/schedule/errors.go`.
- `smoke/run.sh` — `=== Schedule Pull Help (M16-011) ===` block.
- `CHANGELOG.md` — `[Unreleased] → Added` entry for M16-011.
- `management/plans/M16-011-plan.md`, `management/reviews/M16-011-review.md`, `management/plans/M16-011-improved.md` — planning + review artifacts (3 review/improve iterations, all 11 findings resolved across iterations 1–3).

No documented deviations from the plan. Open Decisions 2 (multi-mode concurrency) and 4 (`git:` ref support) explicitly deferred and surfaced in help text.

## Code Review

| Check | Status |
|-------|--------|
| Iteration 3 review verdict | FAIL → all findings resolved in `159f1133` (improved.md iteration 3) |
| Findings resolved | 11/11 across iterations 1–3 (Medium nil-outcome guard + Low Queue.Remove logging in iter 3) |
| Spot-check 1: error wrapping in `runner.go` | PASS — sentinel checks via `errors.Is`; non-sentinel errors logged with `%v` via `fmt.Fprintf(r.Stderr, ...)`; no swallowed errors after iter 3 fix. |
| Spot-check 2: exported symbol doc comments | PASS — `CollectionExecutor`, `ExecutionOutcome`, `Runner`, `Runner.Run`, `Runner.RunOnce`, `defaults`, `drainQueue`, `heartbeatLoop`, `postWithRetry`, `buildResultRequest` all carry doc comments; the new `CollectionExecutor` doc encodes the non-nil contract. |
| Spot-check 3: test exercises the behavior it claims | PASS — `TestRunner_NilOutcome_IsGuarded` injects a `(nil, nil)` executor and asserts both that `RunOnce` returns nil and that the synthesised error outcome is what gets posted; not a "no error" smoke. |

## Commits

| Hash | Message |
|------|---------|
| f1805da4 | docs(plan): add implementation plan for M16-011 |
| 3364527b | chore(task): mark M16-011 as planned |
| 8064ff07 | chore(task): mark M16-011 as in_progress |
| 00ff3446 | test(schedule): add failing tests for schedule types and sentinel errors |
| 8b8ba5a8 | feat(schedule): add schedule client with PollNextRun/Heartbeat/PostResult and backend.GetJSONOptional |
| 24b6efc0 | feat(schedule): add pending-uploads Queue with Enqueue/List/Remove |
| f97f602f | feat(schedule): add ResolveCollection for file: scheme with git: rejection |
| 14099f8a | feat(schedule): add Runner orchestration loop with drain/claim/heartbeat/post |
| 1ac7bf35 | feat(schedule): add runnerExecutor implementing CollectionExecutor over runner.Run |
| bb2cbcf7 | feat(cli): wire --schedule-pull flag into apitest worker command |
| 29062251 | chore(schedule): update smoke test and CHANGELOG for M16-011 |
| 8b36e154 | chore(schedule): register schedule sentinel errors in hint table |
| 9f38f0dc | test(schedule): add additional runner coverage tests (Run cancel, defaults, drain failure) |
| e433077c | chore(task): mark M16-011 as review |
| fd9b9562 | docs(review): add review with findings for M16-011 |
| 89702a0d | fix(worker/schedule): resolve all review findings for M16-011 |
| fc837560 | docs(review): add improvement report for M16-011 |
| 923c548b | docs(review): add review with findings for M16-011 |
| 4456cf08 | fix(worker/schedule): address all review findings from M16-011 second pass |
| 338e1bd4 | docs(review): update improvement report for M16-011 iteration 2 |
| 7c0997ec | docs(review): add review with findings for M16-011 (iteration 3) |
| 159f1133 | fix(worker/schedule): nil outcome guard and Queue.Remove error logging |
| 91843287 | docs(review): add improvement report for M16-011 iteration 3 |

All commits use conventional-commit format and carry `Refs: M16-011`. TDD pattern visible: `test(schedule):` precedes the `feat(schedule):` commits.

## Files Changed

28 files vs `main` (per `git diff --name-only main...HEAD`):

- `cmd/apitest/{worker.go,worker_schedule_test.go,worker_test.go}` (modified)
- `internal/backend/{client.go,client_test.go}` (modified)
- `internal/errors/coverage_test.go` (modified)
- `internal/worker/schedule/{client,collection,doc,errors,executor,hints_init,queue,runner,types}.go` + `_test.go` (new)
- `CHANGELOG.md` (modified)
- `smoke/run.sh` (modified)
- `management/backlog.yaml`, `management/tasks/M16-011.yaml`, `management/plans/M16-011-{plan,improved}.md`, `management/reviews/M16-011-review.md` (artifacts)

## Issues Found

None blocking.

Out-of-scope observation: `smoke/run.sh` emits a non-fatal `FAIL: --format junit should show Professional tier message` line for a Free-tier junit gating check that already exists on `main`. The smoke script does not propagate this to a non-zero exit and ci-local exits 0. Unrelated to the schedule-pull slice — track separately if a JUnit feature-gate fix is desired.

## Recommendation

**PASS** — ready for PR and merge.
