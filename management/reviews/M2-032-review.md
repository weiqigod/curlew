# Code Review: M2-032 (Iteration 2)

**Task:** WebSocket protocol adapter (connect, send, expect, close)
**Reviewer:** AI
**Date:** 2026-04-10
**Branch:** feature/M2-032-websocket-adapter

## Verdict: PASS

## Findings

No findings. All 13 findings from the iteration-1 review (`management/plans/M2-032-improved.md`) have been verified as resolved in the current tree.

| Category checked | Result |
|---|---|
| Error Handling (double-`%w`, sentinels, no swallowed errors) | Clean |
| Input Validation (`nil req`, `nil WebSocket`, empty `Steps`, unknown action, `ctx.Err()` at every step boundary) | Clean |
| Naming (no stuttering, doc comments on all exported symbols, `Dialer`/`Conn` interface names) | Clean |
| Code Organization (`internal/websocket/` owns its domain; `requtil.ToBodyInputs` reused; no duplicated helpers) | Clean |
| Correctness (per-step interpolation walks `WebSocket.Steps`; `runExpect` drains stray frames until deadline; `rr.Err` populated on WebSocket failure; `StatusCode=101` on both success and failure for stable formatter semantics; `defer conn.Close()` covers all exit paths) | Clean |
| Test Quality (unit + runner + real `httptest.Server` integration; interpolation tests at both `requtil` and runner level; stray-frame skip, timeout, extracted-var-in-later-step, empty-steps, unknown-action all covered) | Clean |

### Verification notes

1. **Finding #1 (variable interpolation in WebSocket steps)** — `requtil.InterpolateRequest` now deep-copies `WebSocket` and walks every step via `interpolateWebSocketStep`, handling `MessageRaw`, `Message` (recursively via `InterpolateBody`), `Reason`, and `Extract`. Input is never mutated — verified by `TestInterpolateRequest_websocket_steps_interpolate_message_map` which asserts `req.WebSocket.Steps[0].Message["channel"] == "{{channel}}"` after the call. Runner-level coverage via `TestRun_websocket_variable_interpolation_message_raw` and `TestRun_websocket_variable_interpolation_message_map`.
2. **Finding #2 (single-shot `runExpect`)** — `runExpect` is now a read loop: on every frame it evaluates assertions, remembers the last-diff in `lastAssertions`, and either returns on match or loops until deadline/ctx/read-error. `TestExecute_ExpectSkipsStrayMessagesBeforeMatch` proves it survives two heartbeats before the match; `TestExecute_ExpectTimesOutAfterOnlyStrayMessages` proves it still times out when no frame matches. The last-unmatched diff is preserved on timeout so formatters surface useful context.
3. **Finding #3 (`rr.Err` parity)** — `executePhase` WebSocket branch now sets `rr.Err = wsRes.Err` on failure and keeps `StatusCode = 101` on both paths. `TestRun_websocket_dial_failure_populates_rr_err` locks in the contract.
4. **Finding #4 (error wrapping)** — All `fmt.Errorf` call sites in `executor.go` use Go 1.20+ double-`%w` (`fmt.Errorf("%w: %w", sentinel, inner)`) so callers can `errors.Is` against both the sentinel and the underlying cause. Grep of `internal/websocket/executor.go` finds no `%w: %v` pattern.
5. **Finding #5 (interpolation test coverage)** — Added at both `requtil_test.go` (4 new tests) and `runner_test.go` (`TestRun_websocket_variable_interpolation_message_raw` / `_map`).
6. **Finding #6 (extracted var in later step)** — `TestExecute_ExtractedVariableAvailableInLaterStep` passes: the `send` step's `MessageRaw: "subscribe {{sid}}"` is resolved per-step at send time against the live scope, after the preceding `expect` step wrote `sid=S42`. `runSend` / `runClose` now call `scope.Interpolate` per step.
7. **Finding #7 (unreachable unknown-action branch)** — `TestExecute_UnknownActionErrors` constructs `parser.WebSocketStep{Action: "bogus"}` directly (bypassing parser validation) and asserts `errors.Is(result.Err, ErrUnknownAction)`.
8. **Finding #8 (empty-steps guard)** — New `ErrNoSteps` sentinel; `Execute` returns it when `len(req.WebSocket.Steps) == 0`. `TestExecute_EmptyStepsErrors` locks the contract.
9. **Finding #9 (long fake sleeps)** — Both `wsFakeConn.ReadMessage` (runner_test.go) and `fakeConn.ReadMessage` (executor_test.go) now return an immediate `&timeoutError{}` / `&wsRunnerTimeoutErr{}` when no deadline is set, so misconfigured tests fail fast instead of hanging.
10. **Finding #10 (typo in doc comment)** — `wsRunnerDialer` comment now reads "Implements websocket.Dialer".
11. **Finding #11 (duplicated helper)** — `bodyInputsFromAssertions` is gone; `runExpect` calls `requtil.ToBodyInputs(step.ExpectAssertions.Items)` directly.
12. **Finding #12 (Result.Err doc)** — Comment now explicitly describes both paths: "first error from a failed step (connection, timeout, send, close, or a synthesised 'step N (action): assertions failed' error)".
13. **Finding #13 (StatusCode collision)** — Resolved alongside #3: `StatusCode = 101` in both success and failure for WebSocket, with `rr.Err` as the failure signal. Test asserts `results[0].Result.StatusCode == 101` on dial failure.

### Additional spot checks that also passed

- `defer conn.Close()` runs under all exit paths (including the successful `close` step that breaks out of the loop at `executor.go:104-106`).
- `ctx.Err()` is checked at the top of every step iteration (`executor.go:90`) and inside `runExpect`'s read loop (`executor.go:195`), so context cancellation terminates mid-step without goroutine leaks.
- `runWait` uses `time.NewTimer` + `defer timer.Stop()` + `select { ctx.Done(); timer.C }` — no resource leak, responds to cancellation.
- Parser `UnmarshalYAML` action validation is the single source of truth; the executor's `default:` branch in `runStep` is a defensive guard exercised only by direct struct construction, and is now covered.
- Feature gate wiring: `auth.Registry.Register("protocol_websocket", ...)` registers at Professional tier; `runner.executePhase` checks the gate before dialling. `TestRun_websocket_feature_gate_free_tier` asserts `auth.GateError` is returned.
- Step count: `*counter++` is incremented exactly once per WebSocket request regardless of step count, matching the spec ("each WebSocket request counts as 1 request"). `TestRun_websocket_counts_as_one_request` pins the semantics with a 5-step request.
- Parser `method: "WS"` default is applied only when `Method == ""`, so explicit user methods are preserved. Parsed in `parser.go:140`.
- `smoke/run.sh` adds both the Professional-tier gate check and an invalid-action parse error check.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All wraps use `%w`; sentinels defined and matchable via `errors.Is`; no panics on user input. |
| Input Validation | PASS | `nil` req, `nil` WebSocket, empty Steps, unknown action all guarded and tested. |
| Naming | PASS | No stuttering; all exported symbols documented; typo fixed. |
| Code Organization | PASS | `internal/` boundary respected; duplicated helper removed; gorilla import aliased as `gws` to avoid collision with package name. |
| Correctness | PASS | Stray-frame loop, `rr.Err` parity, per-step interpolation, defer cleanup, ctx propagation all in place. |
| Test Quality | PASS | Unit, runner, and real-server integration tests; all 13 prior-review gaps covered; table-driven / subtests used where natural. |

## Test Coverage

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./internal/websocket/... ./internal/parser/... ./internal/runner/... ./internal/requtil/...` | PASS |
| `golangci-lint run ./...` | PASS (0 issues) |
| Coverage `internal/websocket` | 80.1% (>= 80% threshold) |

Behaviors from `management/tasks/M2-032.yaml` all covered:

| # | Behavior | Test(s) |
|---|---|---|
| 1 | ws connection via HTTP upgrade | `TestExecute_realGorillaDialer` |
| 2 | `send` action writes JSON message | `TestExecute_SendJSONMessage`, `TestRun_websocket_basic_lifecycle` |
| 3 | `expect` with assertions + extract | `TestExecute_ExpectMatchesFromBuffer`, `TestExecute_ExpectExtractsVariable`, `TestExecute_realGorillaDialer_ExtractVariable` |
| 4 | `expect` timeout failure | `TestExecute_ExpectTimeout`, `TestRun_websocket_expect_timeout_fails` |
| 5 | `close` with code 1000 | `TestExecute_CloseWritesCloseFrame`, `TestExecute_CloseDefaultsCodeToNormalClosure` |
| 6 | `wait` pauses for duration | `TestExecute_WaitPausesForDuration`, `TestExecute_WaitRespectsCancellation` |
| 7 | Free-tier feature gate exit code 6 | `TestRun_websocket_feature_gate_free_tier` |
| 8 | Sequential execution within single connection | `TestExecute_BufferFIFO`, `TestExecute_StopsOnFirstFailure`, `TestRun_websocket_counts_as_one_request` |

## Summary

Iteration 2 cleanly addresses every finding from iteration 1. The WebSocket adapter is now internally consistent (variable interpolation reaches every step, `runExpect` drains stray frames, `rr.Err` parity with the HTTP path, double-`%w` error chains throughout), defensively validated (nil req, empty steps, unknown action), and well-tested at unit / runner / real-server-integration levels. Coverage sits at 80.1% on `internal/websocket`, lint is clean, and the full test suite passes. No new findings.
