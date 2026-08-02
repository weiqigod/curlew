# Improvement Report: M5-011 (iteration 2)

**Task:** go-cli: load generation mode (virtual users, ramp profile)
**Date:** 2026-04-20
**Review:** management/reviews/M5-011-review.md

## Resolved Findings (all iterations)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `TestRun_RampUp_LinearActivation` has a latent data race: concurrent `append` to `startTimes` slice unprotected by a mutex | Rewrote the fake executor to use a buffered channel (one slot per VU) and a `sync.Mutex`-protected activation counter. Each VU blocks after sending its timestamp, so only one goroutine per slot is ever active — eliminating the race | ✓ tests pass, race detector clean |
| 2 | High | `TestRun_RampUp_LinearActivation` makes no assertions — always passes regardless of ramp-up behaviour | Added assertion: the max−min spread of the 4 activation timestamps must be ≥ 50% of the 300ms ramp window (i.e. ≥ 150ms). Test now fails when ramp-up is broken | ✓ tests pass |
| 3 | Medium | No CLI-level test for `sum.Aborted → return 130` exit code path | Added `TestPerfCmd_ContextCancelExitCode130`: starts a blocking httptest server, fires `syscall.Kill(syscall.Getpid(), syscall.SIGINT)` once a request arrives, and asserts `perfCmd` returns 130 | ✓ tests pass |
| 4 | Medium | No test for `--rps > 0` → `"Target rate:"` stdout header | Added `TestPerfCmd_RPSHeaderInStdout`: runs `perfCmd` with `--rps 5` against a 200-OK server and asserts stdout contains `"Target rate:"` | ✓ tests pass |
| 5 | Medium | `RunOptions.Stdout` declared and defaulted to `os.Stdout` but never read or written in `run.go` — dead field | Removed `Stdout io.Writer` from `RunOptions` and dropped the `io` and `os` imports from `run.go`. All output is emitted by `perfCmd` via `fmt.Printf` | ✓ tests pass, lint clean |
| 6 | Low | `TestPerfCmd_ContextCancelExitCode130` uses `make(chan int, 1)` — in the timeout path the goroutine sends `-1` to fill the buffer, then `done <- code` in the main goroutine blocks because the buffer is full (latent deadlock) | Increased `done` channel buffer from 1 to 2, and added a comment explaining the two-slot invariant | ✓ tests pass |
| 7 | Low | Three `os.OpenFile` calls in `TestPerfCmd_Run_HTTPTestServer_AllFailures` and `TestPerfCmd_ContextCancelExitCode130` silently ignored error returns (`devNull, _ :=`), inconsistent with the checked pattern at line 177 | Replaced all three with `devNull, err := os.OpenFile(...)` followed by `t.Fatalf("open devnull: %v", err)` checks | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (total) | 86.7% |
| Coverage (`internal/loadgen`) | 95.9% |
| Coverage (`cmd/apitest`) | 81.4% |
| Coverage (`internal/auth`) | 89.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| f6c3ebc | fix(loadgen): remove dead RunOptions.Stdout field | #5 |
| 9f4fb83 | fix(loadgen): fix racy ramp-up test and add real spread assertion | #1, #2 |
| 546aac0 | test(perf): add exit-130 and RPS-header CLI tests | #3, #4 |
| 14c6930 | fix(loadgen): fix lint issues in run.go and run_test.go | lint cleanup |
| 7c56198 | fix(perf): fix latent deadlock and unchecked os.OpenFile errors in perf_test.go | #6, #7 |

## Summary

7/7 findings resolved. 0 deferred.
