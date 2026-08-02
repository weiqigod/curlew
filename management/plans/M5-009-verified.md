# Verification Report: M5-009

**Task:** go-cli: worker agent protocol
**Verified by:** AI
**Date:** 2026-04-19
**Branch:** feature/M5-009-worker-protocol
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 fail |
| `go test -race ./...` | PASS | No races detected (review confirmed) |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test complete (SIGPIPE bug in worker --help check fixed) |
| Coverage (`internal/worker`) | 91.6% | Meets >= 80% threshold |
| Coverage (total) | 86.7% | Meets >= 80% threshold |

## Smoke Fix

A SIGPIPE issue was identified and fixed in `smoke/run.sh`: the `worker --help` check at line 1688 piped directly (`./curlew worker --help | grep -q "..."`) which causes exit code 141 under `set -euo pipefail` because `grep -q` closes stdin before the binary finishes writing. Fixed by capturing output first:

```bash
WORKER_HELP=$(./curlew worker --help 2>&1)
echo "$WORKER_HELP" | grep -q "Usage: curlew worker"
```

This matches the pattern used by all other smoke test checks.

## Observable Output

The observable is exercised by `TestWorker_E2E_Observable` (integration test with a fake coordinator):

```
Claimed shard shd_1 (3 requests)
Completed shd_1: pass=3 fail=0 duration=...ms
No more shards; exiting
```

The test asserts all three lines appear in stdout, `ShardsCompleted == 1`, `TotalPass == 3`, `TotalFail == 0`, and the coordinator recorded a submit with `PassCount == 3`.

Expected: stdout contains "Claimed shard shd_1 (3 requests)", "Completed shd_1: pass=3 fail=0", "No more shards; exiting"
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Worker polls POST /coordinator/jobs/{job_id}/claim and prints 'Claimed shard <id>' | `TestRun/claim_execute_submit_one_shard_then_204`, `TestWorker_E2E_Observable` | PASS |
| 2 | Executes shard's HTTP requests, collects per-request outcomes | `TestRun/submit_failures_recorded_as_fail`, `TestRun/executor_error_recorded_as_error_status` | PASS |
| 3 | POSTs results on 202, prints 'Completed <id>: pass=N fail=M' | `TestRun/claim_execute_submit_one_shard_then_204`, `TestWorker_E2E_Observable` | PASS |
| 4 | 204 no_shards_available → prints 'No more shards; exiting', exits 0 | `TestRun/no_shards_available_exits_zero` | PASS |
| 5 | Retries 3 times with exponential backoff on connectivity loss | `TestClient_Retry_BackoffGrows`, `TestRun/submit_failure_after_retries_logs_warning_and_continues` | PASS |
| 6 | Sends heartbeat every 15 seconds (interval injectable for tests) | `TestRun/heartbeats_sent_during_long_shard` | PASS |
| 7 | Invalid token → 'error: unauthorized', exits 10 | `TestWorker_E2E_Unauthorized` | PASS |
| 8 | --help documents --job, --org, --coordinator-url, --token, --concurrency | `TestParseWorkerArgs/help_flag`, smoke test | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all 23 worker tests pass | PASS |
| 2 | Observable output works as specified | `TestWorker_E2E_Observable` + smoke `worker --help` | PASS |
| 3 | Test coverage >= 80% | `internal/worker` coverage: 91.6% | PASS |
| 4 | No build warnings or lint errors | `go build ./cmd/curlew` clean; `golangci-lint run` 0 issues | PASS |
| 5 | Help text for curlew worker updated | `cmd/curlew/worker.go` `printWorkerHelp()` documents all flags | PASS |
| 6 | Smoke test or equivalent integration check updated | `smoke/run.sh` — `=== Worker --help (M5-009) ===` block added and now passing | PASS |

## Code Review

Branch A: Review PASS trusted (verdict PASS in `management/reviews/M5-009-review.md`).

Spot-check results:

| Check | Item Checked | Status |
|-------|-------------|--------|
| Error wrapping | `client.go` line 43: `fmt.Errorf("marshalling claim request: %w", err)` — `%w` used | PASS |
| Doc comments | `run.go` lines 42-44: `Run` function has full doc comment | PASS |
| Test quality | `run_test.go` `TestRun` is table-driven with 11 named sub-cases including edge cases | PASS |

## Commits

| Hash | Message |
|------|---------|
| 97bc06a | docs(review): add passing review for M5-009 |
| 92fa309 | docs(review): add improvement report for M5-009 |
| 4fe99a2 | fix(worker): resolve all review findings for M5-009 |
| 332dce7 | docs(review): add review with findings for M5-009 |
| e0d7273 | chore(task): mark M5-009 as review |
| 14934f0 | refactor(worker): fix gofumpt formatting in test files |
| 5b7e1b6 | feat(worker): add worker --help smoke check and CHANGELOG entry (M5-009) |
| c0a9fef | test(worker): add E2E integration tests with fake coordinator and sample-shard.json testdata |
| b2e3130 | feat(cli): add worker subcommand with parseWorkerArgs and printWorkerHelp |
| a605110 | test(cli): add failing tests for parseWorkerArgs |
| e5a38f8 | feat(worker): implement Run orchestrator with claim-execute-submit loop, heartbeat, concurrency |
| 709ef4e | test(worker): add failing tests for Run orchestrator |
| 014d0e8 | feat(worker): implement Client with Claim/SubmitResult/Heartbeat and exponential backoff retry |
| 182970b | test(worker): add failing tests for Client Claim/SubmitResult/Heartbeat/Retry |
| 076d284 | feat(worker): add package skeleton with types, sentinel errors, Config.Validate |
| 20b9479 | test(worker): add failing tests for Config.Validate |
| 9a708aa | chore(task): mark M5-009 as in_progress |
| 957308a | chore(task): mark M5-009 as planned |
| b01400b | docs(plan): add implementation plan for M5-009 |

TDD pattern is visible: every `test(worker)` commit precedes the corresponding `feat(worker)` commit.

## Files Changed

| File | Action |
|------|--------|
| `internal/worker/worker.go` | created — package types, sentinel errors, Config.Validate |
| `internal/worker/worker_test.go` | created — TestConfig_Validate (6 cases) |
| `internal/worker/client.go` | created — HTTP client with claim/submit/heartbeat + retry |
| `internal/worker/client_test.go` | created — TestClient_Claim, TestClient_SubmitResult, TestClient_Heartbeat, TestClient_Retry_BackoffGrows |
| `internal/worker/run.go` | created — Run orchestrator, executeShard, heartbeatLoop |
| `internal/worker/run_test.go` | created — TestRun (11 cases) |
| `internal/worker/coordinator_fake_test.go` | created — httptest-based fake coordinator |
| `internal/worker/integration_test.go` | created — TestWorker_E2E_Observable, TestWorker_E2E_Unauthorized |
| `cmd/curlew/worker.go` | created — workerCmd, parseWorkerArgs, printWorkerHelp |
| `cmd/curlew/worker_test.go` | created — TestParseWorkerArgs (7 cases) |
| `cmd/curlew/main.go` | modified — added `case "worker":` and `worker` to printHelp |
| `smoke/run.sh` | modified — added worker --help check; fixed SIGPIPE bug in check |
| `testdata/worker/sample-shard.json` | created — 3-request fixture |
| `CHANGELOG.md` | modified — Unreleased entry added |

## Issues Found

One issue found during verification: the smoke test's `worker --help` check used a direct pipe (`./curlew worker --help | grep -q "..."`) which fails with exit code 141 (SIGPIPE) under `set -euo pipefail`. Fixed by capturing output first (`WORKER_HELP=$(./curlew worker --help 2>&1)`) to match the pattern used by all other smoke checks.

## Recommendation

PASS — ready for PR and merge.
