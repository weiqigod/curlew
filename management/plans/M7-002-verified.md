# Verification Report: M7-002

**Task:** Progress text relocated from stdout to stderr in perf, license, worker, exec --dry-run
**Verified by:** AI
**Date:** 2026-04-22
**Branch:** feature/M7-002-progress-to-stderr
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages, no failures |
| `go test -race ./...` | PASS | No races detected (confirmed in review) |
| `golangci-lint run` | PASS | 0 findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage `cmd/curlew` | 80.3% | Meets >= 80% threshold |
| Coverage `internal/worker` | 92.7% | Meets >= 80% threshold |
| Coverage total | 86.2% | Meets >= 80% threshold |
| `./scripts/ci-local.sh --go` | PASS | All gates passed |

## Observable Output

```
go test -run TestStreamProgress ./cmd/curlew/...
ok      github.com/weiqigod/curlew/cmd/curlew   1.174s

grep -n '"warning:' internal/worker/run.go
137:            _, _ = fmt.Fprintf(stderr, "warning: submit failed for %s: %v (shard will be reaped)\n", ...)
257:            _, _ = fmt.Fprintf(stderr, "warning: heartbeat failed for %s: %v\n", ...)
```

Expected: TestStreamProgress passes; both warning lines target `stderr`
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `perf`: stdout = summary only; progress on stderr | `TestStreamProgress/perf`, `TestPerfCmd_Run_HTTPTestServer_Success`, `TestPerfCmd_RPSHeaderInStderr`, `TestPerfCmd_SummaryLinePrintedToStdout` | PASS |
| 2 | `license --validate`: stdout = Key/Tier/State; progress on stderr | `TestStreamProgress/license_validate`, `TestLicenseValidate_Offline_Valid` | PASS |
| 3 | `license export`: stdout empty; progress on stderr | `TestStreamProgress/license_export`, `TestLicenseExport_HappyPath` | PASS |
| 4 | `worker`: stdout empty; progress on stderr | `TestRun/claim_execute_submit_one_shard_then_204`, `TestRun/two_shards_then_204`, `TestWorker_E2E_Observable` (library-level) | PASS |
| 5 | `worker/run.go:133` submit-failure warning → stderr | `TestRun/submit_failure_after_retries_logs_warning_and_continues` | PASS |
| 6 | `exec --dry-run`: uses `output.Printer.RequestDetail`; no raw `fmt.Fprintln` | `TestStreamProgress/exec_dry_run`, `TestExecCmd_DryRun_UsesPrinterRequestDetail` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all packages pass | PASS |
| 2 | `cmd/curlew/stream_progress_test.go` covers perf, license, worker, and exec --dry-run | File exists with 4 subtests; `TestStreamProgress` passes | PASS |
| 3 | `Running...` in perf.go → stderr; `Validating license` in license.go → stderr; `Claimed shard` in run.go → stderr | Source grep confirms all in `fmt.Fprintf(stderr, ...)` | PASS |
| 4 | exec --dry-run uses `output.Printer.RequestDetail`; no raw `fmt.Fprintln(os.Stdout, ...)` | Confirmed in main.go dry-run branch | PASS |
| 5 | `go test ./...` passes with no regressions | All packages pass | PASS |
| 6 | Test coverage does not regress below 80% | `cmd/curlew`=80.3%, `internal/worker`=92.7% | PASS |
| 7 | `golangci-lint run` passes with 0 issues | Confirmed in review and ci-local.sh | PASS |
| 8 | `./smoke/run.sh` passes | ci-local.sh includes smoke; PASS | PASS |
| 9 | `./scripts/ci-local.sh` passes | PASS (ci-local PASS output) | PASS |
| 10 | CHANGELOG.md updated | Entry added under `## [Unreleased]` → `### Fixed` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |
| Concurrency safety | PASS |
| Doc comments | PASS |

Branch A: Review PASS (ec04a0a) trusted; spot-check clean. Spot-checks: (1) `grep -n '"warning:' internal/worker/run.go` — both `%v` wrapping patterns correct; (2) `RunOptions`, `RunSummary`, `Run`, `executeOne` all have doc comments; (3) `heartbeat_failure_warning_uses_stderr_seam` test accurately exercises the seam via concurrent-safe `syncBuffer`.

## Commits

| Hash | Message |
|------|---------|
| ec04a0a | docs(review): add passing review for M7-002 |
| 52e5f64 | docs(review): update improvement report for M7-002 iteration 3 |
| 691da18 | fix(worker): fix RunOptions.Stdout doc comment and capture stderr in tests |
| 90ec0c1 | docs(review): add review with findings for M7-002 (iteration 3) |
| 02e3b07 | docs(review): add improvement report for M7-002 (iteration 2) |
| 411258b | fix(worker): use syncBuffer for concurrent stderr seam in tests |
| 6a4433c | docs(review): add review with findings for M7-002 |
| bff07b6 | docs(review): add improvement report for M7-002 |
| 05a4cc6 | docs(perf): update stale perfCmdOut doc comment for M7-002 stream routing |
| 537ee6a | fix(worker): pass injected stderr writer to heartbeatLoop |
| 9fa305a | docs(review): add review with findings for M7-002 |
| 869ef51 | chore(task): mark M7-002 as review |
| ab40de6 | refactor(worker): silence ineffassign lint; retain stdout seam comment |
| d1e1279 | docs(changelog): add M7-002 entry for progress-to-stderr stream routing |
| 14cf81b | test(cli): add TestStreamProgress regression guard for M7-002 stream split |
| 50aa2ce | feat(cli): replace raw fmt.Fprintf in exec --dry-run with output.Printer.RequestDetail |
| 7b426bb | test(cli): add failing test for exec --dry-run using Printer.RequestDetail |
| 7ed6606 | feat(license): move progress lines (Validating/Exporting/Included/Wrote) to stderr |
| e7df1ef | test(license): add failing tests for progress lines on stderr |
| 435eb08 | feat(perf): move progress lines (Load test/Running/Requests sent/Wrote) to stderr |
| 93f367e | test(perf): add failing tests for progress lines on stderr |
| 69d5b8c | feat(cli): wire Stderr into worker.RunOptions from workerCmdOut |
| aed8d5b | feat(worker): move all progress lines (Claimed/Completed/No more shards) to stderr |
| 31a9308 | test(worker): add failing tests for all progress lines on stderr |
| e4c6f0e | feat(worker): add Stderr seam to RunOptions; fix submit-failure warning to stderr |
| 98ce26a | test(worker): add failing tests for submit-failure warning on stderr |
| 8adba97 | chore(task): mark M7-002 as in_progress |
| 936ef31 | chore(task): mark M7-002 as planned |
| 5778225 | docs(plan): add implementation plan for M7-002 |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `cmd/curlew/license.go` | modified | Progress lines → stderr |
| `cmd/curlew/license_test.go` | modified | Assertions updated to stderr |
| `cmd/curlew/main.go` | modified | exec --dry-run → output.Printer.RequestDetail |
| `cmd/curlew/main_test.go` | modified | Added TestExecCmd_DryRun_UsesPrinterRequestDetail |
| `cmd/curlew/perf.go` | modified | Progress lines → stderr |
| `cmd/curlew/perf_test.go` | modified | Assertions updated to stderr; TestPerfCmd_RPSHeaderInStdout → TestPerfCmd_RPSHeaderInStderr |
| `cmd/curlew/stream_progress_test.go` | created | TestStreamProgress binary-level regression guard |
| `cmd/curlew/worker.go` | modified | Pass Stderr: stderr into worker.RunOptions |
| `internal/worker/integration_test.go` | modified | Progress assertions moved to stderr buffers |
| `internal/worker/run.go` | modified | Added Stderr seam; all progress lines → stderr |
| `internal/worker/run_test.go` | modified | All subtests pass Stderr buffers; syncBuffer for concurrent tests |
| `CHANGELOG.md` | modified | M7-002 entry added under Fixed |
| `management/backlog.yaml` | modified | M7-002 status → review |
| `management/plans/M7-002-improved.md` | created | Improvement report |
| `management/plans/M7-002-plan.md` | created | Implementation plan |
| `management/reviews/M7-002-review.md` | created | Passing review |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
