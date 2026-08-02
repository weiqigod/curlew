# Improvement Report: M2-034

**Task:** WebSocket reconnection, heartbeat, and parallel connection support
**Date:** 2026-04-10
**Review:** management/reviews/M2-034-review.md

## Iteration 1 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `shouldReconnect` did not exclude context-cancellation errors. When the parent context was cancelled mid-expect, `ErrContextCanceled` does not satisfy `Timeout() bool`, so `isTimeoutErr` returned false and `shouldReconnect` returned true — causing reconnect attempts on a cancelled context. | Added `errors.Is` guards for `ErrContextCanceled`, `context.Canceled`, and `context.DeadlineExceeded` before the `isTimeoutErr` check in `reconnect.go`. Added two new test sub-cases in `TestReconnectState_shouldReconnect`. | ✓ tests pass |
| 2 | Medium | Heartbeat goroutine not restarted after reconnect. After `conn.Close()` on the old connection, the heartbeat goroutine continued writing to it; `WriteControl` failed, sending a spurious `ErrHeartbeatFailed` that the next step's pre-check observed as a false failure. | Added `stopHeartbeat()` + `startHeartbeat(ctx, conn, ...)` after each successful `redial` in the reconnect loop in `executor.go`. Also added `TestExecute_heartbeatRestartedAfterReconnect` to verify no spurious error after reconnect. | ✓ tests pass |
| 3 | Medium | `TestExecute_heartbeatFailureFailsTest` accepted both `result.Passed == true` and `result.Passed == false` with only a `t.Logf`, leaving the core "heartbeat timeout fails test" behavior untested. | Fixed the executor to add a post-step (non-blocking) `heartbeatErr` check so errors that fire while `runExpect` is blocked on `ReadMessage` are observed before the step-loop exits. Rewrote the test to unconditionally assert `result.Passed == false` and `errors.Is(result.Err, ErrHeartbeatTimeout)`. | ✓ tests pass |
| 4 | Low | Dead code: `callCount := 0` declared and assigned in `TestExecute_reconnect_succeedsAfterDrop` but only discarded via `_ = callCount`. | Removed both the declaration and the blank-identifier discard. | ✓ tests pass |

## Iteration 2 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `buildWSOutcome` failure branch (lines 692–706 in `internal/runner/runner.go`) had 0% effective coverage. No runner test exercised a WebSocket request that fails through the parallel `buildWebSocketFunc` → `buildWSOutcome` path. The error message shape, `AssertionResults` content, and the `"unknown failure"` fallback string (line 694) were completely untested. | Added `TestRun_websocket_parallel_failure_propagates` in `internal/runner/runner_test.go`. Uses `wsRunnerDialer{dialErr: errors.New("connection refused")}` with `Parallel: true`. Asserts `rr.Err != nil`, `AssertionResults.Passed == false`, `Items[0].Type == "websocket"`, `Items[0].Expected == "all steps pass"`, `Items[0].Actual` contains the real error string (not the `"unknown failure"` fallback), `Items[0].Passed == false`, and summary failure counts. `buildWSOutcome` is now 100% covered. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage — `internal/runner` | 86.7% |
| Coverage — `internal/websocket` | 85.6% |
| Coverage — `internal/parallel` | 90.5% |
| Coverage — `internal/parser` | 89.5% |
| Coverage — overall | 89.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `5acf43a` | fix(websocket): remove dead callCount variable from reconnect_test | iter-1 #4 |
| `d4fad0d` | fix(websocket): exclude context cancellation from reconnect triggers | iter-1 #1 |
| `c9ff07a` | fix(websocket): restart heartbeat after reconnect; assert heartbeat timeout in test | iter-1 #2, #3 |
| `fc47aac` | test(runner): add failing parallel WS test to cover buildWSOutcome failure branch | iter-2 #1 |

## Summary

5/5 total findings resolved across two iterations. 0 deferred.
