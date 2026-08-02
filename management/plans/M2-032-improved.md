# Improvement Report: M2-032

**Task:** WebSocket protocol adapter (connect, send, expect, close)
**Date:** 2026-04-10
**Review:** management/reviews/M2-032-review.md

## Resolved Findings

| #  | Severity | Finding | Fix Applied | Verified |
|----|----------|---------|------------|----------|
| 1  | High     | `InterpolateRequest` did not walk `WebSocket.Steps`; `{{var}}` placeholders in message payloads leaked to the wire. | Extended `requtil.InterpolateRequest` to deep-copy `WebSocket`, walk each step, and interpolate `MessageRaw`, `Message` (recursively, via `InterpolateBody`), `Reason`, and `Extract` values. Added four unit tests in `requtil_test.go` (raw, nested map + slice, reason/extract, undefined-var error). | PASS |
| 2  | High     | `runExpect` was single-shot: any stray frame before the expected one produced a false negative, contradicting the plan's buffered-match design. | Rewrote `runExpect` as a read loop that retries on every incoming frame, dropping unmatched frames and only failing once the deadline passes. The last unmatched assertion diff is preserved on timeout so formatters surface useful context. Added `TestExecute_ExpectSkipsStrayMessagesBeforeMatch` and `TestExecute_ExpectTimesOutAfterOnlyStrayMessages`. | PASS |
| 3  | High     | WebSocket failures did not populate `rr.Err`, so verbose/JSON/TAP output diverged from the HTTP path. | In `runner.executePhase` WebSocket branch: set `rr.Err = wsRes.Err` on failure; `StatusCode` now stays at `101` on both success and failure (failure signalled via `rr.Err` / `AssertionResults.Passed`), resolving Finding #13's collision with HTTP DNS failures. Added `TestRun_websocket_dial_failure_populates_rr_err`. | PASS |
| 4  | Medium   | Executor wrapped errors with `%w: %v`, dropping the inner error chain. | Switched every call site in `executor.go` to Go 1.20+ double-`%w` wrapping (`%w: %w`) so callers can `errors.Is` against both the sentinel and the underlying cause. | PASS |
| 5  | Medium   | No test covered runner-level interpolation of `Message`/`MessageRaw`. | Added `TestRun_websocket_variable_interpolation_message_raw` and `TestRun_websocket_variable_interpolation_message_map` in `runner_test.go`; both assert the dialled frame carries the resolved value. | PASS |
| 6  | Medium   | No test proved an extracted variable could be used in a later step within the same WebSocket request. | Added `TestExecute_ExtractedVariableAvailableInLaterStep` in `executor_test.go`. To make the test pass, `runSend` and `runClose` now interpolate per-step against the live scope so variables extracted at step *N* resolve at step *N+1*. | PASS |
| 7  | Medium   | Unknown-action defensive branch was unreachable via parser and untested. | Added `TestExecute_UnknownActionErrors` that bypasses the parser by constructing `parser.WebSocketStep{Action: "bogus"}` directly. | PASS |
| 8  | Medium   | `Execute` did not guard against `len(Steps) == 0`. | Added new `ErrNoSteps` sentinel in `errors.go`; `Execute` checks `len(req.WebSocket.Steps) == 0` and returns `ErrNoSteps` before dialling. Added `TestExecute_EmptyStepsErrors`. | PASS |
| 9  | Low      | Fakes slept 1–10 seconds when no deadline was set, making misconfigured tests hang. | `wsFakeConn.ReadMessage` (runner_test.go) and `fakeConn.ReadMessage` (executor_test.go) now return an immediate timeout when no deadline is set. | PASS |
| 10 | Low      | Doc comment on `wsRunnerDialer` said "Implements wsocket.Dialer". | Fixed typo to `websocket.Dialer`. | PASS |
| 11 | Low      | `bodyInputsFromAssertions` duplicated `requtil.ToBodyInputs`. | Removed the duplicate helper; `runExpect` now calls `requtil.ToBodyInputs(step.ExpectAssertions.Items)` directly. | PASS |
| 12 | Low      | `Result.Err` doc comment was incomplete. | Updated comment to describe both paths: "the first error from a failed step (connection, timeout, send, close, or a synthesised 'assertions failed' error)". | PASS |
| 13 | Low      | Synthesised `StatusCode = 0` on WS failure collided with HTTP DNS failures. | Fixed alongside Finding #3: `StatusCode` stays at `101` for WS requests in both success and failure, and the test asserts it. | PASS |

## Out of Scope (Deferred)

No findings deferred. All 13 findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/websocket`) | 80.1% |
| Coverage (total project) | 89.1% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `992050b` | fix(requtil): interpolate variables in WebSocket step fields | #1 |
| `9b8caa0` | fix(websocket): drain stray frames in runExpect and preserve error chains | #2, #4, #6, #7, #8, #11, #12 |
| `e8dbe1c` | fix(runner): populate rr.Err on WebSocket failure | #3, #13 |
| `e37215d` | test(websocket): tighten fake sleeps and add runner interpolation coverage | #5, #9, #10 |

## Summary
13/13 findings resolved. 0 deferred. Quality gate green; websocket package coverage 80.1% (above the 80% threshold).
