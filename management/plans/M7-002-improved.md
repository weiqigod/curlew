# Improvement Report: M7-002

**Task:** Progress text relocated from stdout to stderr in perf, license, worker, exec --dry-run
**Date:** 2026-04-22
**Review:** management/reviews/M7-002-review.md

## Resolved Findings (Iteration 1 — previous review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `heartbeatLoop` bypassed `RunOptions.Stderr` seam by writing directly to `os.Stderr`, making heartbeat warnings uncapturable in tests and inconsistent when stderr is redirected | Added `stderr io.Writer` parameter to `heartbeatLoop`; `Run()` now passes its resolved `stderr` writer; added `TestRun/heartbeat_failure_warning_uses_stderr_seam` to verify capture via seam | ✓ tests pass; `heartbeatLoop` now at 100% coverage |
| 2 | Low | `perfCmdOut` doc comment falsely stated "stdout receives progress and summary lines" — the old routing before M7-002 | Updated doc comment to: "stdout carries only the declared result payload (the final `Results: requests=...` summary line); stderr carries progress lines, warnings, and error messages." | ✓ build and lint pass |

## Resolved Findings (Iteration 2 — current review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `heartbeat_failure_warning_uses_stderr_seam` used a plain `bytes.Buffer` as the `Stderr` seam. `heartbeatLoop` runs in a goroutine concurrently with `Run()`, so unprotected writes to the buffer caused a DATA RACE under `go test -race`. | Added `syncBuffer` type (mutex-protected `bytes.Buffer`) to `internal/worker/run_test.go`. Changed `var stderr bytes.Buffer` to `var stderr syncBuffer` in the affected subtest. `go test -race ./internal/worker/` now passes with no races. | ✓ tests pass with `-race` |

## Resolved Findings (Iteration 3 — current review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `RunOptions.Stdout` field-level doc comment read `nil = os.Stdout — carries only declared result payload`, implying a nil-coalesce-to-os.Stdout behaviour that does not exist (`_ = opts.Stdout` blanks the field). | Updated doc comment to: `nil is accepted; reserved for a future stdout payload (e.g. --report JSON) and is not currently read by Run.` | ✓ build and lint pass |
| 2 | Low | Six `TestRun` subtests (`submit_failures_recorded_as_fail`, `executor_error_recorded_as_error_status`, `shard_with_invalid_requests_json_continues`, `heartbeats_sent_during_long_shard`, `unauthorized_propagates_err`, `context_cancel_aborts_loop_cleanly`, `concurrency_3_runs_three_requests_in_parallel`) did not pass a `Stderr` buffer, causing progress text to leak to real `os.Stderr` during test runs. The stdout-empty invariant was also unguarded in these subtests. | Added `Stderr: &bytes.Buffer{}` (or `syncBuffer` for concurrent subtests: `heartbeats_sent_during_long_shard`, `context_cancel_aborts_loop_cleanly`, `concurrency_3_runs_three_requests_in_parallel`) to all six subtests. Added `stdout.Len() != 0` guards to assert the stdout-empty invariant. | ✓ `go test -race ./internal/worker/...` passes |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `go test -race ./internal/worker/...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `cmd/curlew` | 80.3% |
| Coverage `internal/worker` | 92.7% |
| Coverage (total) | 86.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 537ee6a | fix(worker): pass injected stderr writer to heartbeatLoop | iter 1 #1 |
| 05a4cc6 | docs(perf): update stale perfCmdOut doc comment for M7-002 stream routing | iter 1 #2 |
| 411258b | fix(worker): use syncBuffer for concurrent stderr seam in tests | iter 2 #1 |
| 691da18 | fix(worker): fix RunOptions.Stdout doc comment and capture stderr in tests | iter 3 #1, #2 |

## Summary

5/5 findings resolved across 3 review iterations. 0 deferred.
