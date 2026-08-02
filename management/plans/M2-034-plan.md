# Implementation Plan: M2-034

## Overview

Extend the WebSocket protocol adapter with auto-reconnection (configurable max
attempts + backoff), periodic heartbeat (ping/pong), routing of WebSocket
requests through the parallel wave executor so independent WebSocket tests run
concurrently, and URL scheme auto-detection for `ws://` / `wss://`.

## Task Details

- **ID:** M2-034
- **Title:** WebSocket reconnection, heartbeat, and parallel connection support
- **Phase:** M2: WebSocket
- **Priority:** 5
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-033 | WebSocket message buffering, expect patterns, and variable extraction | done |
| M2-016 | Parallel execution (wave-based) | done |

## Architectural Decisions

1. **Reconnect semantics (best-effort, test isolation preserved).**
   Reconnection happens only on connection-level read failures (net.Error other
   than timeouts, unexpected close, EOF) during `expect` steps. It does *not*
   fire on assertion failures, expect timeouts, or context cancellation —
   those remain deterministic test failures. After a successful reconnect,
   execution resumes at the step that observed the drop (the dropped message
   is considered lost). The executor explicitly does **not** replay already
   completed steps; spec-wise this is a pragmatic choice noted inline.
2. **Backoff strategy.** Only `exponential` is supported at implementation
   level; any other value (empty = exponential default) falls back to
   exponential. This matches the existing `retry` package precedent and keeps
   the initial slice small. `initial_delay_ms` defaults to 1000 as documented
   in the spec.
3. **Heartbeat runs on a dedicated goroutine per connection.**
   It writes a control ping (gorilla `PingMessage`) or a user-provided payload
   via `WriteMessage`, then verifies pong by installing a gorilla pong handler
   on a per-connection basis. Heartbeat failure is surfaced by closing the
   connection and posting an error the step loop observes via a shared
   `heartbeatErr` channel; the current/next step then fails with
   `ErrHeartbeatTimeout`. We do **not** send user-payload "ping" frames as
   data messages unless `heartbeat.message` is set; when set, we send a data
   frame and expect a matching data-frame pong evaluated against
   `heartbeat.expect` assertions.
4. **Parallel routing.** Instead of pushing WebSocket knowledge into
   `parallel.ExecuteWaves`, we generalise the existing `DataDrivenFunc`
   pattern by adding a sibling `WebSocketFunc` injection point on
   `parallel.Config`. The runner wires it to a closure that calls
   `websocket.Execute` and synthesises a `RequestOutcome`. This keeps the
   parallel package free of protocol-specific imports and mirrors the
   established pattern for data-driven items.
5. **URL scheme auto-detection runs in the parser protocol-validation pass.**
   When `protocol:` is empty and URL starts with `ws://` or `wss://`, we set
   `Protocol = "websocket"` and run the existing WebSocket validation
   (`websocket.steps` required, step action validation, default
   `Method = "WS"`). This keeps one code path for all WebSocket requests —
   explicit `protocol: websocket` vs. auto-detected URL converge to identical
   downstream behaviour.
6. **Config fields live on `parser.WebSocketConfig`.** Adding `Reconnect` and
   `Heartbeat` there keeps the YAML surface identical to the spec. Both types
   are small structs with pointer fields so "omitted" vs "disabled" is
   distinguishable.
7. **Dialer interface is NOT changed.** Reconnection redials via the same
   `Dialer` that the first attempt used. Tests inject a `fakeDialer` whose
   `Dial` can be programmed to fail N times before succeeding.

## Implementation Steps

Step order is chosen so each step has the smallest possible blast radius and
the failing tests introduced earlier do not interact with unreleased logic.

### Step 1: Add reconnect/heartbeat config to parser

**Rationale:** YAML schema must exist before any downstream code can rely on
it. Nothing in the runner or executor reads the new fields yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `ReconnectConfig`, `HeartbeatConfig` types; embed on `WebSocketConfig` |
| `internal/parser/parser.go` | modify | Validate `reconnect.max_attempts >= 0`, `reconnect.backoff` in `{"", "exponential"}`, `heartbeat.interval_ms > 0` when enabled |
| `internal/parser/parser_test.go` | modify | Add table cases for new validation paths |
| `internal/parser/testdata/websocket_reconnect.yaml` | create | Golden valid file |
| `internal/parser/testdata/websocket_heartbeat.yaml` | create | Golden valid file |
| `internal/parser/testdata/websocket_reconnect_invalid_backoff.yaml` | create | Invalid backoff value |
| `internal/parser/testdata/websocket_heartbeat_bad_interval.yaml` | create | Invalid interval |

#### Current Code
```go
// internal/parser/collection.go:348
type WebSocketConfig struct {
    Steps []WebSocketStep `yaml:"steps"`
}
```

#### New Code
```go
// ReconnectConfig controls auto-reconnection behaviour for a WebSocket request.
// Reconnection fires only on connection-level read failures during an expect
// step; assertion failures, expect timeouts, and context cancellation are not
// retried.
type ReconnectConfig struct {
    Enabled        bool   `yaml:"enabled"`
    MaxAttempts    int    `yaml:"max_attempts,omitempty"`     // default 3
    InitialDelayMs int    `yaml:"initial_delay_ms,omitempty"` // default 1000
    Backoff        string `yaml:"backoff,omitempty"`          // "exponential" (default)
}

// HeartbeatConfig controls periodic ping/pong keepalive for a WebSocket
// request. When Message is nil the adapter sends a gorilla PingMessage
// control frame and relies on the pong handler. When Message is set, the
// adapter sends a data frame and matches the pong against Expect assertions.
type HeartbeatConfig struct {
    Enabled    bool           `yaml:"enabled"`
    IntervalMs int            `yaml:"interval_ms,omitempty"` // default 30000
    Message    map[string]any `yaml:"message,omitempty"`
    Expect     BodyAssertions `yaml:"expect,omitempty"`
}

// WebSocketConfig holds the WebSocket-specific request configuration.
type WebSocketConfig struct {
    Steps     []WebSocketStep  `yaml:"steps"`
    Reconnect *ReconnectConfig `yaml:"reconnect,omitempty"`
    Heartbeat *HeartbeatConfig `yaml:"heartbeat,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParse_websocketReconnectConfig(t *testing.T) {
    tests := []struct {
        name       string
        file       string
        wantErr    bool
        wantMsg    string
        assertFn   func(*testing.T, *Collection)
    }{
        {"reconnect_enabled_defaults_applied", "websocket_reconnect.yaml", false, "", ...},
        {"heartbeat_enabled_with_message",     "websocket_heartbeat.yaml", false, "", ...},
        {"reconnect_bad_backoff_rejected",     "websocket_reconnect_invalid_backoff.yaml", true, "unsupported reconnect.backoff", nil},
        {"heartbeat_bad_interval_rejected",    "websocket_heartbeat_bad_interval.yaml", true, "heartbeat.interval_ms must be > 0", nil},
    }
    // ...
}
```

#### Impact on Existing Tests
No existing parser tests change. The additional fields are `omitempty` and
unused by existing YAML fixtures.

---

### Step 2: URL scheme auto-detection in parser

**Rationale:** Independent from reconnect/heartbeat and narrow in scope. Runs
before the existing WebSocket validation so one downstream code path handles
both explicit and auto-detected requests.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | In the protocol validation loop, when `protocol == ""` and URL starts with `ws://`/`wss://`, set `Protocol = "websocket"` before validation |
| `internal/parser/parser_test.go` | modify | Add table cases for auto-detection |
| `internal/parser/testdata/websocket_autodetect_ws.yaml` | create | `url: ws://...`, no explicit protocol, explicit `websocket.steps` |
| `internal/parser/testdata/websocket_autodetect_wss.yaml` | create | `url: wss://...` |
| `internal/parser/testdata/websocket_autodetect_missing_steps.yaml` | create | `ws://...` without `websocket.steps` → error |

#### Current Code
```go
// internal/parser/parser.go:205
for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
    for i := range *section {
        protocol := (*section)[i].Request.Protocol
        if protocol != "" && protocol != "http" && protocol != "graphql" && protocol != "websocket" {
            return nil, &apierrors.Structured{...}
        }
```

#### New Code
```go
for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
    for i := range *section {
        req := &(*section)[i].Request
        // Auto-detect WebSocket from URL scheme when protocol is unset.
        if req.Protocol == "" {
            lower := strings.ToLower(req.URL)
            if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
                req.Protocol = "websocket"
            }
        }
        protocol := req.Protocol
        // ... (existing validation continues unchanged)
```

#### Tests to Write FIRST (RED phase)

```go
func TestParse_websocketAutoDetectURLScheme(t *testing.T) {
    tests := []struct {
        name     string
        file     string
        wantProto string
        wantErr   bool
    }{
        {"ws_scheme_sets_protocol", "websocket_autodetect_ws.yaml", "websocket", false},
        {"wss_scheme_sets_protocol", "websocket_autodetect_wss.yaml", "websocket", false},
        {"uppercase_WSS_scheme", "websocket_autodetect_uppercase.yaml", "websocket", false},
        {"ws_scheme_still_requires_steps", "websocket_autodetect_missing_steps.yaml", "", true},
        {"explicit_http_with_ws_url_is_honoured", "websocket_autodetect_explicit_http.yaml", "http", false},
    }
    // ...
}
```

#### Impact on Existing Tests
- Requests in existing test fixtures use explicit `protocol: websocket`, so
  adding the auto-detect branch is additive. No assertions change.
- `requtil` already handles WebSocket fields regardless of how Protocol got
  set.

---

### Step 3: Reconnect sentinel + dialer-aware executor

**Rationale:** Pure addition inside `internal/websocket`. Runner still calls
`Execute` with the same signature. We add a helper `reconnect` path triggered
on read failures from `runExpect`. The step loop becomes reconnect-aware but
no other package notices.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/websocket/errors.go` | modify | Add `ErrReconnectExhausted`, `ErrHeartbeatTimeout`, `ErrHeartbeatFailed` |
| `internal/websocket/reconnect.go` | create | `redial(ctx, cfg, url, headers, dialer, attempt) (Conn, error)` + backoff helper |
| `internal/websocket/reconnect_test.go` | create | Table-driven tests for redial attempt counting & delay |
| `internal/websocket/executor.go` | modify | On expect-read connection error, call `redial`; on success, replace `conn` in the step loop and retry the current expect with the remaining timeout budget |
| `internal/websocket/executor_test.go` | modify | Add tests: (a) succeed after 1 reconnect, (b) exhaust attempts, (c) reconnect disabled → behaviour unchanged |

#### New Code (reconnect.go)
```go
package websocket

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/peterlindqvist/apitest/internal/parser"
)

// reconnectState holds per-request reconnection bookkeeping owned by the
// step loop. It is not safe for concurrent use.
type reconnectState struct {
    cfg      *parser.ReconnectConfig
    attempts int
}

// shouldReconnect reports whether the executor should attempt a redial in
// response to err and bumps the attempt counter.
func (r *reconnectState) shouldReconnect(err error) bool {
    if r == nil || r.cfg == nil || !r.cfg.Enabled {
        return false
    }
    max := r.cfg.MaxAttempts
    if max <= 0 {
        max = 3
    }
    if r.attempts >= max {
        return false
    }
    return isReconnectableErr(err)
}

// nextDelay computes the delay before the n-th (0-indexed) reconnect attempt.
// Exponential: delay = initial * 2^n. Initial defaults to 1s.
func (r *reconnectState) nextDelay() time.Duration {
    base := time.Duration(r.cfg.InitialDelayMs) * time.Millisecond
    if base <= 0 {
        base = time.Second
    }
    // exponential
    mult := 1 << r.attempts // 0 -> 1, 1 -> 2, 2 -> 4
    return base * time.Duration(mult)
}

// redial performs a single redial attempt. The caller has already
// checked shouldReconnect and slept nextDelay.
func redial(ctx context.Context, dialer Dialer, url string, headers http.Header) (Conn, error) {
    c, _, err := dialer.Dial(ctx, url, headers)
    if err != nil {
        return nil, fmt.Errorf("%w: %w", ErrDialFailed, err)
    }
    return c, nil
}

// isReconnectableErr reports whether err represents a connection-level drop
// that a reconnect attempt could recover from. Assertion failures, context
// cancellation, and expect timeouts are deliberately excluded.
func isReconnectableErr(err error) bool {
    if err == nil {
        return false
    }
    if isTimeoutErr(err) {
        return false // expect timeouts are deterministic test failures
    }
    // Any other read/write error is considered a drop (EOF, unexpected close,
    // broken pipe, connection reset, etc.).
    return true
}
```

#### Executor change (sketch)
```go
// In Execute, after the dial block:
rstate := &reconnectState{cfg: req.WebSocket.Reconnect}

for i, step := range req.WebSocket.Steps {
    // ... context check ...
    sr := runStep(ctx, conn, buf, step, scope)

    // Reconnect branch: only for expect steps that failed on a connection
    // drop (sr.Err is ErrExpectTimeout wrapping a non-timeout error, OR
    // sr.Err is ErrDialFailed during a send — treated as reconnect trigger).
    if !sr.Passed && rstate.shouldReconnect(sr.Err) {
        if wErr := waitReconnect(ctx, rstate); wErr != nil {
            result.Err = wErr
            return result
        }
        newConn, rErr := redial(ctx, dialer, req.URL, headers)
        if rErr != nil {
            rstate.attempts++
            continue // loop will re-check shouldReconnect via the failure path
        }
        _ = conn.Close()
        conn = newConn
        rstate.attempts++
        // Re-run the same step on the new connection.
        sr = runStep(ctx, conn, buf, step, scope)
    }

    // ... (original success/failure handling) ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecute_reconnect_succeedsAfterDrop(t *testing.T) {
    // Dialer returns a conn that reads a drop error on its first ReadMessage,
    // then a second conn that returns the expected frame.
    // expect step passes, result.Passed == true.
}

func TestExecute_reconnect_exhaustsAttempts(t *testing.T) {
    // Dialer's second/third conns also drop.
    // Expect result.Err to wrap ErrReconnectExhausted.
}

func TestExecute_reconnect_disabledLeavesBehaviourUnchanged(t *testing.T) {
    // reconnect.enabled=false: first drop fails the step exactly as today.
}

func TestReconnectState_nextDelay_exponential(t *testing.T) {
    tests := []struct {
        name     string
        attempt  int
        initMs   int
        wantMs   int
    }{
        {"first attempt = initial", 0, 1000, 1000},
        {"second attempt = 2x", 1, 1000, 2000},
        {"third attempt = 4x", 2, 1000, 4000},
        {"zero initial defaults to 1s", 0, 0, 1000},
    }
    // ...
}
```

#### Impact on Existing Tests
- `TestExecute_*` that rely on read failures propagating to `ErrExpectTimeout`
  unchanged, because their requests do not enable reconnect.
- Integration tests unchanged: reconnect defaults to disabled.

---

### Step 4: Heartbeat loop

**Rationale:** Builds on the Conn interface, isolated to a new file in
`internal/websocket`. Cross-cuts `Execute` only by starting/stopping a
goroutine and wiring a shared error channel.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/websocket/conn.go` | modify | Extend `Conn` interface with `SetPongHandler(func(string) error)` |
| `internal/websocket/heartbeat.go` | create | `startHeartbeat(ctx, conn, cfg) (stop func(), errCh <-chan error)` |
| `internal/websocket/heartbeat_test.go` | create | Unit tests using a fakeConn that records pings and replays pongs |
| `internal/websocket/executor.go` | modify | Start heartbeat after dial, select on errCh in step loop to fail fast on heartbeat timeout |
| `internal/websocket/executor_test.go` | modify | Add tests: heartbeat timeout fails test; heartbeat disabled → no pings |

#### Current Code (conn.go)
```go
type Conn interface {
    WriteMessage(messageType int, data []byte) error
    ReadMessage() (messageType int, data []byte, err error)
    SetReadDeadline(t time.Time) error
    Close() error
    WriteControl(messageType int, data []byte, deadline time.Time) error
}
```

#### New Code (conn.go)
```go
type Conn interface {
    WriteMessage(messageType int, data []byte) error
    ReadMessage() (messageType int, data []byte, err error)
    SetReadDeadline(t time.Time) error
    Close() error
    WriteControl(messageType int, data []byte, deadline time.Time) error
    SetPongHandler(h func(appData string) error)
}

func (g *gorillaConn) SetPongHandler(h func(string) error) {
    g.c.SetPongHandler(h)
}
```

#### Heartbeat sketch (heartbeat.go)
```go
package websocket

import (
    "context"
    "fmt"
    "sync/atomic"
    "time"

    gws "github.com/gorilla/websocket"
    "github.com/peterlindqvist/apitest/internal/parser"
)

// startHeartbeat launches a goroutine that sends ping frames every
// cfg.IntervalMs and fails if no pong is received within one interval.
// The returned stop() cancels the loop; errCh receives at most one error.
func startHeartbeat(ctx context.Context, conn Conn, cfg *parser.HeartbeatConfig) (func(), <-chan error) {
    errCh := make(chan error, 1)
    if cfg == nil || !cfg.Enabled {
        close(errCh)
        return func() {}, errCh
    }
    interval := time.Duration(cfg.IntervalMs) * time.Millisecond
    if interval <= 0 {
        interval = 30 * time.Second
    }

    var pongReceived atomic.Bool
    conn.SetPongHandler(func(string) error {
        pongReceived.Store(true)
        return nil
    })
    pongReceived.Store(true) // prime so the first interval doesn't spuriously fail

    hbCtx, cancel := context.WithCancel(ctx)
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-hbCtx.Done():
                return
            case <-ticker.C:
                if !pongReceived.Swap(false) {
                    select {
                    case errCh <- fmt.Errorf("%w after %v", ErrHeartbeatTimeout, interval):
                    default:
                    }
                    return
                }
                if err := conn.WriteControl(gws.PingMessage, nil, time.Now().Add(interval)); err != nil {
                    select {
                    case errCh <- fmt.Errorf("%w: %w", ErrHeartbeatFailed, err):
                    default:
                    }
                    return
                }
            }
        }
    }()
    return cancel, errCh
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestStartHeartbeat_sendsPingsAtInterval(t *testing.T) {
    // fakeConn records control frames; simulate pongs via the pong handler.
    // After 3 ticker fires, expect 3 ping control frames recorded.
}

func TestStartHeartbeat_timeoutWhenNoPong(t *testing.T) {
    // Never invoke the pong handler; expect errCh to receive ErrHeartbeatTimeout.
}

func TestStartHeartbeat_disabled_noPings(t *testing.T) {
    // cfg.Enabled=false: fakeConn sees zero control frames, errCh closed.
}

func TestExecute_heartbeatFailureFailsTest(t *testing.T) {
    // Enable heartbeat with short interval, use a conn that never pongs.
    // Expect Execute to return result.Err wrapping ErrHeartbeatTimeout.
}
```

#### Impact on Existing Tests
- `fakeConn` in `executor_test.go` must grow a `SetPongHandler` method
  (trivial no-op, satisfies interface).
- All existing non-heartbeat tests continue to pass because default
  `Heartbeat` is nil and `startHeartbeat` is a no-op.

---

### Step 5: Parallel executor injection point for WebSocket

**Rationale:** Pure extension of `parallel.Config`. Existing callers pass nil
and retain current behaviour. The runner passes a closure that calls
`websocket.Execute`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | modify | Add `WebSocketFunc` field on `Config`; add branch in wave loop that treats WebSocket items as "atomic concurrent" alongside HTTP and data-driven items |
| `internal/parallel/executor_test.go` | modify | Add tests: (a) two WebSocket items run in same wave and complete concurrently, (b) sequential-step semantics preserved within one connection, (c) nil WebSocketFunc falls through to HTTP path (unchanged) |

#### Config change
```go
type WebSocketFunc func(ctx context.Context, item parser.RequestItem, scope *variable.Scope, waveIdx int) RequestOutcome

type Config struct {
    // existing fields ...
    WebSocketFunc WebSocketFunc // nil = fall back to HTTP ExecFunc
}
```

#### Wave loop change (sketch)
```go
var wsReady []readyRequest
for _, rr := range ready {
    switch {
    case cfg.Items[rr.index].Request.Protocol == "websocket" && cfg.WebSocketFunc != nil:
        wsReady = append(wsReady, rr)
    case rr.dataDriven:
        ddReady = append(ddReady, rr)
    default:
        regularReady = append(regularReady, rr)
    }
}

// Launch WS goroutines alongside regularReady and ddReady
wsOutcomes := make([]RequestOutcome, len(wsReady))
for i, rr := range wsReady {
    wg.Add(1)
    go func(i int, rr readyRequest) {
        defer wg.Done()
        wsOutcomes[i] = cfg.WebSocketFunc(ctx, cfg.Items[rr.index], rr.scope, waveIdx)
    }(i, rr)
}
// After wg.Wait, append wsOutcomes to wr.Outcomes and update failedIndices.
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWaves_websocketItemsRunConcurrently(t *testing.T) {
    // Build two items with Protocol=websocket. WebSocketFunc records start
    // times and sleeps 50ms each. After execution, elapsed < 100ms (proving
    // overlap) and both outcomes in wave 0.
}

func TestExecuteWaves_websocketFailurePropagatesSkip(t *testing.T) {
    // WebSocketFunc returns an outcome with AssertionResults.Passed=false.
    // A dependent HTTP item in wave 1 is skipped with the dependency reason.
}

func TestExecuteWaves_nilWebSocketFunc_fallsThrough(t *testing.T) {
    // Protocol=websocket but WebSocketFunc=nil: item routed through HTTP
    // ExecFunc (current behaviour), producing whatever the mock returns.
}
```

#### Impact on Existing Tests
- No existing parallel tests set `WebSocketFunc`; all existing paths remain
  nil and behave exactly as today.
- The `readyRequest` struct gains no fields; dispatch is done at loop entry.

---

### Step 6: Runner wiring

**Rationale:** Final step — connects the new `WebSocketFunc` to the
`parallel.Config` and removes the need for the sequential runner to handle
parallel main-phase WebSocket items.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | In `executeParallelMain`, build a `WebSocketFunc` closure that mirrors the sequential path (feature gate, interpolate, `websocket.Execute`, synthesise `httpexec.Result`); pass it through `parallel.Config` |
| `internal/runner/runner.go` | modify | Factor the synthesis-of-RequestResult from the sequential path into a helper `buildWebSocketOutcome(...)` so both paths share the same fail/warning/assertion shape |
| `internal/runner/runner_test.go` | modify | Add tests: (a) two WebSocket requests run under `--parallel` concurrently, (b) `--parallel` with WS + HTTP mixes correctly, (c) URL auto-detection with `ws://` runs via WebSocket path |

#### New Code (sketch)
```go
// In executeParallelMain, before parallel.ExecuteWaves:
wsFunc := func(ctx context.Context, item parser.RequestItem, reqScope *variable.Scope, waveIdx int) parallel.RequestOutcome {
    // Feature gate
    reg := vars.Registry
    if reg == nil { reg = auth.DefaultRegistry() }
    tier := vars.Tier
    if tier == "" { tier = auth.TierFree }
    if gateErr := auth.CheckFeature(reg, "protocol_websocket", tier); gateErr != nil {
        return parallel.RequestOutcome{
            Name: item.Name, Method: item.Request.Method, URL: item.Request.URL,
            WaveIndex: waveIdx, Err: gateErr,
        }
    }

    req := item.Request
    interpolated, interpErr := requtil.InterpolateRequest(reqScope, &req)
    if interpErr != nil {
        return parallel.RequestOutcome{..., Err: fmt.Errorf("request %q: %w", item.Name, interpErr)}
    }
    req = *interpolated

    dialer := vars.WebSocketDialer
    if dialer == nil { dialer = websocket.DefaultDialer }
    wsRes := websocket.Execute(ctx, &req, reqScope, dialer)

    return buildWSOutcome(item, req, wsRes, waveIdx)
}

execResult, err := parallel.ExecuteWaves(ctx, parallel.Config{
    // existing fields ...
    WebSocketFunc: wsFunc,
})
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_websocket_parallel_two_connections_concurrent(t *testing.T) {
    // Build two websocket items; dialer records connection times.
    // Run with vars.Parallel=true.
    // Assert both WS connections established and completed overlapped.
}

func TestRun_websocket_parallel_mixed_with_http(t *testing.T) {
    // One HTTP request and one WebSocket request in main phase, no deps.
    // Both in wave 0 when parallel.
}

func TestRun_websocket_autodetect_ws_scheme(t *testing.T) {
    // Collection with url: ws://... and no explicit protocol.
    // Parser sets Protocol=websocket; runner dispatches to wsDialer.
}
```

#### Impact on Existing Tests
- `TestRun_websocket_*` (sequential) continue to pass because the sequential
  `executePhase` WebSocket branch is retained.
- The new helper `buildWSOutcome` centralises result synthesis so both paths
  stay in sync.

---

### Step 7: Smoke test + help text

**Rationale:** Completeness contract — observable capability visible in the
smoke harness and any updated help text.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add a section that parses a `ws://` collection without explicit protocol (tier-gated) and asserts the parse succeeds / shows Professional tier message; add a section that parses a collection with `reconnect` / `heartbeat` config and asserts no parse error |
| `CHANGELOG.md` | modify | Add M2-034 entry |

No help-text changes are required — flags are unchanged; the new capability
is configured entirely via YAML.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | existing WebSocket cases | none | — |
| `internal/websocket/executor_test.go` | fakeConn interface | breaks (missing SetPongHandler) | add no-op stub |
| `internal/websocket/executor_test.go` | existing send/expect/close cases | none | — |
| `internal/parallel/executor_test.go` | existing data-driven and regular cases | none | — |
| `internal/runner/runner_test.go` | sequential WebSocket cases | none | — |
| `internal/requtil/requtil_test.go` | WS interpolation | none | — |

## Risks and Edge Cases

- **Risk:** Reconnecting after partial state (e.g. authentication already
  performed in step 1) leaves the new connection stateless.
  → **Mitigation:** Document this as a limitation in the executor doc
  comments; advise users to put auth/subscribe messages inside a shared
  YAML anchor or early steps, and note that reconnect replays the *current*
  failed step, not the whole sequence. Add an integration test asserting the
  documented semantics.

- **Risk:** Heartbeat goroutine leaks if `Execute` returns before the loop
  stops.
  → **Mitigation:** `defer stopHeartbeat()` immediately after the goroutine
  is launched; verified by a `runtime.Gosched` + leak-check assertion in
  tests using `t.Cleanup` + goroutine count before/after.

- **Risk:** Running WebSocket through `parallel.ExecuteWaves` uses
  `requtil.InterpolateRequest` via the closure, which calls
  `BeginRequest/EndRequest` on the scope snapshot. Need to confirm no double
  begin/end collision with the sequential websocket path.
  → **Mitigation:** `snapshotScope` already creates an independent scope,
  identical to HTTP behaviour. The closure uses that snapshot exclusively.

- **Edge case:** `count > 1` expect step experiencing a drop after
  `collected == 2 / 5`: behaviour is to lose the collected frames and restart
  the expect cleanly on the new connection (documented; initial MVP chose the
  simpler semantics).

- **Edge case:** `ws://` URL with explicit `protocol: http`. Auto-detect must
  not override an explicit setting. Covered by test case
  `explicit_http_with_ws_url_is_honoured`.

- **Edge case:** Heartbeat ping collides with an in-flight expect
  `ReadMessage`. `SetReadDeadline` still applies to expect; gorilla's
  implementation handles control frames transparently on read, so the pong
  handler fires without consuming a data frame.

- **Edge case:** Context cancellation during reconnect backoff. `redial`
  receives `ctx`; `waitReconnect` uses `select { case <-ctx.Done(); case
  <-time.After(delay) }`.

## Verification

```bash
go build ./cmd/apitest
go test ./internal/parser/... ./internal/websocket/... ./internal/parallel/... ./internal/runner/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):
```bash
# Reconnect
go test ./internal/websocket/... -run TestExecute_reconnect

# Heartbeat
go test ./internal/websocket/... -run TestStartHeartbeat -run TestExecute_heartbeatFailureFailsTest

# Parallel
go test ./internal/parallel/... -run TestExecuteWaves_websocketItemsRunConcurrently
go test ./internal/runner/... -run TestRun_websocket_parallel

# Auto-detection
go test ./internal/parser/... -run TestParse_websocketAutoDetectURLScheme
```
