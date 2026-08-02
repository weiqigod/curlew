# Code Review: M7-002

**Task:** Progress text relocated from stdout to stderr in perf, license, worker, exec --dry-run
**Reviewer:** AI
**Date:** 2026-04-22
**Branch:** feature/M7-002-progress-to-stderr

## Verdict: PASS

## Findings

No findings. All previous iteration findings have been resolved.

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| — | — | — | — | — | No findings | — |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel errors used correctly; no swallowed errors across all changed files |
| Input Validation | PASS | No changes to public input-validation paths; existing validation unchanged |
| Naming | PASS | No stutter; exported symbols have doc comments; `RunOptions.Stdout` doc comment correctly reflects reserved-but-unused semantics; `syncBuffer` unexported; package names correct |
| Code Organization | PASS | Package boundaries respected; `Stderr io.Writer` added non-breakingly to `RunOptions`; `_ = opts.Stdout` retains the seam for API stability; `heartbeatLoop` parametrized via `stderr io.Writer` |
| Correctness | PASS | All progress strings confirmed on `stderr`-targeted writers; `exec --dry-run` uses `output.Printer.RequestDetail`; no data races (`go test -race ./...` passes); DoD grep checks pass: `Running...` → stderr in perf.go; `Validating license...` → stderr in license.go; `Claimed shard` → stderr in run.go; both `warning:` strings in run.go target stderr |
| Test Quality | PASS | All 11 subtests in `TestRun` now pass `Stderr` buffers; concurrent subtests (`heartbeats_sent_during_long_shard`, `heartbeat_failure_warning_uses_stderr_seam`, `concurrency_3_runs_three_requests_in_parallel`, `context_cancel_aborts_loop_cleanly`) correctly use `syncBuffer`; stdout-empty invariant guarded in all subtests; `TestStreamProgress` binary-level regression tests all four subcommands; `TestExecCmd_DryRun_UsesPrinterRequestDetail` asserts the `  > GET url` shape |

## Test Coverage
- Coverage: `cmd/curlew` = 80.3%, `internal/worker` = 92.7%, total = 86.2%
- All packages meet ≥80% threshold
- `go test -race ./...` passes

## Spec Compliance (Behaviors from task YAML)

| # | Behavior | Tested? | Notes |
|---|----------|---------|-------|
| 1 | `perf`: stdout = summary only; progress on stderr | Yes | `TestStreamProgress/perf`, `TestPerfCmd_SummaryLinePrintedToStdout`, `TestPerfCmd_Run_HTTPTestServer_Success`, `TestPerfCmd_RPSHeaderInStderr` |
| 2 | `license --validate`: stdout = Key/Tier/State; progress on stderr | Yes | `TestStreamProgress/license_validate`, `TestLicenseValidate_Offline_Valid` |
| 3 | `license export`: stdout empty; progress on stderr | Yes | `TestStreamProgress/license_export`, `TestLicenseExport_HappyPath` |
| 4 | `worker`: stdout empty; progress on stderr | Yes (library-level) | `TestRun/claim_execute_submit_one_shard_then_204`, `TestRun/two_shards_then_204`, `TestWorker_E2E_Observable`; binary-level spawn omitted (documented trade-off in plan) |
| 5 | `worker/run.go:133` submit-failure warning → stderr | Yes | `TestRun/submit_failure_after_retries_logs_warning_and_continues` |
| 6 | `exec --dry-run`: uses `output.Printer.RequestDetail`; no raw `fmt.Fprintln(os.Stdout, ...)` | Yes | `TestStreamProgress/exec_dry_run`, `TestExecCmd_DryRun_UsesPrinterRequestDetail` |

## Summary

All previous iteration findings (Low × 2) were resolved in the improvement pass. The `RunOptions.Stdout` doc comment now accurately reflects the reserved-but-unused semantics, and all six previously uncaptured subtests now pass `Stderr` buffers with appropriate concurrent-safe types and stdout-empty guards. The core stream-routing work is correct and complete across all four subcommands. No new findings were identified.
