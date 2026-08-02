# Implementation Plan: M2-032

## Overview
Add a WebSocket protocol adapter (`protocol: websocket`) that establishes a persistent connection, drives a sequential list of `send` / `expect` / `wait` / `close` step actions, buffers inbound messages, evaluates JSONPath assertions on received JSON frames, and extracts variables from matched messages. The adapter is gated as a Professional-tier feature and counts all of a request's steps as a single request against the guard rail.

## Task Details
- **ID:** M2-032
- **Title:** WebSocket protocol adapter (connect, send, expect, close)
- **Phase:** M2: WebSocket
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-001 | CLI scaffold and basic `run` command | done |
| M1-005 | JSONPath body assertions | done |
| M1-028 | Feature gate framework | done |

## Architecture Decisions

### Adapter pattern: reuse the pre-processing model from M2-029 where possible, but branch execution

For GraphQL the request was rewritten into an HTTP POST and reused the existing HTTP execution path. WebSocket cannot reuse that path — the runner must perform a full connection lifecycle instead of a single `exec(ctx, req)` call. The plan therefore:

1. **Extends parser types** with `WebSocket *WebSocketConfig` on `parser.Request`, mirroring how `GraphQL` was added.
2. **Creates `internal/websocket/` package** that owns the connection lifecycle and step execution (`Connect`, `Run`, `Dialer`). The package takes a `Dialer` interface so tests can inject a fake that does not need a real server while integration tests can use the real `gorilla/websocket` dialer against `httptest.NewServer` + `websocket.Upgrader`.
3. **Adds a branch in `runner.Run`** — parallel to the existing `if req.Protocol == "graphql"` block — that feature-gates `protocol_websocket`, delegates to `websocket.Execute`, and produces a single `RequestResult` whose `Result` is a synthetic `httpexec.Result` (StatusCode 101 on success, Duration = total wall-clock) so downstream output formatters behave correctly without schema changes. Assertions on the collection-level `assertions:` block are **not** evaluated for WebSocket requests (assertions live inside `expect` steps); status is `Passed` iff every step succeeded.
4. **Registers `protocol_websocket`** in `auth.DefaultRegistry()` as `TierProfessional`.
5. **Adds `gorilla/websocket v1.5.3`** to `go.mod` (permissive BSD-2 license, zero transitive deps besides stdlib).

### Scope reduction for M2-032
The spec describes many WebSocket features; this slice delivers the MVP explicitly listed in the task's `behaviors`:
- **In scope:** connect, send (JSON map `message:` and raw string `message_raw:`), expect (single message, `timeout_ms`, JSONPath assertions via `assertion.BodyInput`, variable extraction via `variable.Extract`), wait (`duration_ms`), close (code/reason), message buffering (FIFO), variable interpolation for URL/headers/message fields, professional feature gate.
- **Deferred (explicitly not in this slice, logged as follow-up):** reconnection, heartbeat, `any_of`, `count:` multi-message expect, `message_template:` external files, binary messages, subscription (GraphQL) integration, the buffer-100 warning, and partial HTML/JUnit output enrichment for per-step traces. Terminal output treats a WebSocket request like any other request (name, method shown as `WS`, URL shown as `ws://...`).

### Open questions (resolved)
- **Q:** How should `stop_on_failure` / `required:` behave mid-stream? **A:** A WebSocket request is a single unit from the runner's perspective — either all steps succeed (Passed) or the first failing step terminates the connection and the request is marked failed. This matches how GraphQL works.
- **Q:** Should the runner count every step individually against `MaxRequests`? **A:** No. Spec section 2845 is explicit: "each WebSocket request (all steps) counts as 1 request." We bump `*counter` once before dialing.
- **Q:** Method display? **A:** Report `req.Method` as `"WS"` when `req.Protocol == "websocket"` and method is empty, so existing terminal/JSON output formats show something sensible. Parser fills this in automatically (analogous to GraphQL → POST).
- **Q:** How do `headers:` on the request get forwarded to the upgrade? **A:** `websocket.Dialer.Dial` accepts `http.Header`; we copy `req.Headers` into it. Variable interpolation is already applied upstream by `requtil.InterpolateRequest`.
- **Q:** Variable extraction scope? **A:** Variables extracted inside an `expect` step are written to the collection `scope` immediately, so later steps in the same WebSocket request — and later requests in the collection — can use them. Matches the spec's "Connection State Management" section.

## Implementation Steps

### Step 1: Add WebSocket parser types and validation
**Rationale:** YAML must round-trip first. This is a pure additive change to `parser.Request` with an optional pointer field; no existing behavior changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `WebSocketConfig`, `WebSocketStep`, `WebSocket *WebSocketConfig` field on `Request` |
| `internal/parser/parser.go` | modify | Accept `"websocket"` in protocol validation; require non-empty steps; default `Method = "WS"` |
| `internal/parser/errors.go` | modify (if needed) | Reuse existing sentinels (`ErrUnsupportedProtocol`, `ErrMissingRequiredField`, `ErrInvalidFieldValue`) |
| `internal/parser/parser_test.go` | modify | Add `TestParseFile_websocket` with subtests mirroring `TestParseFile_graphql` |
| `internal/parser/testdata/websocket_basic.yaml` | create | Minimal collection with one step |
| `internal/parser/testdata/websocket_full_lifecycle.yaml` | create | send + expect + wait + close |
| `internal/parser/testdata/websocket_invalid_action.yaml` | create | Fixture for bad action name |
| `internal/parser/testdata/websocket_no_steps.yaml` | create | Fixture with empty steps |
| `internal/parser/testdata/websocket_no_config.yaml` | create | `protocol: websocket` with no `websocket:` block |

#### Current Code (parser/collection.go)
```go
// Request defines the HTTP request to execute.
type Request struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body any `yaml:"body,omitempty"`
	QueryParams map[string]string `yaml:"query,omitempty"`
	Protocol string `yaml:"protocol,omitempty"`
	GraphQL *GraphQLConfig `yaml:"graphql,omitempty"`
}
```

#### New Code (parser/collection.go)
```go
// WebSocketConfig holds the WebSocket-specific request configuration.
type WebSocketConfig struct {
	Steps []WebSocketStep `yaml:"steps"`
}

// WebSocketStep is a single action within a WebSocket request lifecycle.
// Exactly one of Message / MessageRaw is populated for send actions.
// ExpectAssertions holds JSONPath assertions for expect actions.
type WebSocketStep struct {
	Action           string            `yaml:"action"` // "send" | "expect" | "wait" | "close"
	Message          map[string]any    `yaml:"message,omitempty"`
	MessageRaw       string            `yaml:"message_raw,omitempty"`
	TimeoutMs        int               `yaml:"timeout_ms,omitempty"`
	ExpectAssertions BodyAssertions    `yaml:"-"` // populated via custom UnmarshalYAML on expect
	Extract          map[string]string `yaml:"extract,omitempty"`
	DurationMs       int               `yaml:"duration_ms,omitempty"`
	Code             int               `yaml:"code,omitempty"`
	Reason           string            `yaml:"reason,omitempty"`
}

// Request ... (append fields)
type Request struct {
	// ... existing fields ...
	WebSocket *WebSocketConfig `yaml:"websocket,omitempty"`
}
```

Because the `expect` action's `message:` field is a **JSONPath assertion map** (`$.type: { equals: "x" }`) while the `send` action's `message:` is a **literal JSON object**, `WebSocketStep` needs a custom `UnmarshalYAML` that peeks at `action` first, then decodes `message:` differently:

```go
func (s *WebSocketStep) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("step: expected mapping")
	}
	// First pass: read action
	for i := 0; i+1 < len(value.Content); i += 2 {
		if value.Content[i].Value == "action" {
			s.Action = value.Content[i+1].Value
			break
		}
	}
	// Second pass: decode remaining fields based on action
	for i := 0; i+1 < len(value.Content); i += 2 {
		k, v := value.Content[i].Value, value.Content[i+1]
		switch k {
		case "action":
			// already captured
		case "message":
			if s.Action == "expect" {
				if err := v.Decode(&s.ExpectAssertions); err != nil {
					return fmt.Errorf("expect message: %w", err)
				}
			} else {
				if err := v.Decode(&s.Message); err != nil {
					return fmt.Errorf("send message: %w", err)
				}
			}
		case "message_raw":
			s.MessageRaw = v.Value
		case "timeout_ms":
			_ = v.Decode(&s.TimeoutMs)
		case "duration_ms":
			_ = v.Decode(&s.DurationMs)
		case "code":
			_ = v.Decode(&s.Code)
		case "reason":
			s.Reason = v.Value
		case "extract":
			_ = v.Decode(&s.Extract)
		}
	}
	return nil
}
```

Parser validation additions (`parser.go`, extend the existing protocol loop):
```go
if protocol != "" && protocol != "http" && protocol != "graphql" && protocol != "websocket" {
	return nil, &apierrors.Structured{ /* ... Allowed protocols: http, graphql, websocket */ }
}
if protocol == "websocket" {
	if (*section)[i].Request.WebSocket == nil || len((*section)[i].Request.WebSocket.Steps) == 0 {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryParse,
			FilePath: path,
			Message:  fmt.Sprintf("websocket request %q must have websocket.steps with at least one action", (*section)[i].Name),
			Hint:     "Add websocket: steps: [...] to your request",
			Inner:    ErrMissingRequiredField,
		}
	}
	// Default method for display purposes
	if (*section)[i].Request.Method == "" {
		(*section)[i].Request.Method = "WS"
	}
	for j, step := range (*section)[i].Request.WebSocket.Steps {
		switch step.Action {
		case "send", "expect", "wait", "close":
		default:
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: path,
				Message:  fmt.Sprintf("unsupported websocket action %q in request %q step %d", step.Action, (*section)[i].Name, j+1),
				Hint:     "Allowed actions: send, expect, wait, close",
				Inner:    ErrInvalidFieldValue,
			}
		}
	}
}
```

`validateRequests` already enforces non-empty `URL`, which applies equally to `ws://` and `wss://` schemes — no change needed there.

#### Tests to Write FIRST (RED phase) — `parser_test.go`

```go
func TestParseFile_websocket(t *testing.T) {
	t.Run("basic websocket parses", func(t *testing.T) {
		col, err := ParseFile("testdata/websocket_basic.yaml")
		// assert: Protocol == "websocket", Method == "WS", len(Steps) == 1
	})
	t.Run("full lifecycle parses", func(t *testing.T) {
		col, err := ParseFile("testdata/websocket_full_lifecycle.yaml")
		// assert: steps[0].Action=="send", steps[1].Action=="expect", ...
		// assert: expect.ExpectAssertions.Items has JSONPath entries
	})
	t.Run("invalid action rejected", func(t *testing.T) {
		_, err := ParseFile("testdata/websocket_invalid_action.yaml")
		// assert: errors.Is(err, ErrInvalidFieldValue)
	})
	t.Run("empty steps rejected", func(t *testing.T) {
		_, err := ParseFile("testdata/websocket_no_steps.yaml")
		// assert: errors.Is(err, ErrMissingRequiredField)
	})
	t.Run("missing websocket block rejected", func(t *testing.T) {
		_, err := ParseFile("testdata/websocket_no_config.yaml")
		// assert: errors.Is(err, ErrMissingRequiredField)
	})
}
```

#### Impact on Existing Tests
- `TestParseFile_graphql` → no change; the existing "invalid protocol value" test still passes because `"websocket"` is now valid but the fixture uses a different bad value.
- The error `Hint` message for unsupported protocols changes from `"Allowed protocols: http, graphql"` to `"Allowed protocols: http, graphql, websocket"` — any test that asserts on the exact hint string must be updated. (A grep for the string will confirm none exist today; if found, update.)

---

### Step 2: Create `internal/websocket/` package with mockable dialer
**Rationale:** Pure unit testable layer before wiring into the runner. Smallest blast radius — no imports elsewhere yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `go.mod` / `go.sum` | modify | `go get github.com/gorilla/websocket@v1.5.3` |
| `internal/websocket/errors.go` | create | Sentinel errors |
| `internal/websocket/conn.go` | create | `Conn` interface + `Dialer` interface + `gorillaDialer` default |
| `internal/websocket/executor.go` | create | `Execute(ctx, cfg, dialer, scope)` — runs all steps |
| `internal/websocket/executor_test.go` | create | Table-driven tests using a fake in-memory `Conn` |

#### New Code (`internal/websocket/conn.go`)
```go
// Package websocket provides WebSocket protocol support for the API testing tool.
// It owns the connection lifecycle (dial → send/expect/wait/close) and integrates
// with variable interpolation, JSONPath assertions, and variable extraction.
package websocket

import (
	"context"
	"net/http"
	"time"

	gws "github.com/gorilla/websocket"
)

// Conn is the minimal connection interface required by the step executor.
// It is implemented by gorilla/websocket's *Conn (wrapped in gorillaConn)
// and by fakes used in tests.
type Conn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, data []byte, err error)
	SetReadDeadline(t time.Time) error
	Close() error
	WriteControl(messageType int, data []byte, deadline time.Time) error
}

// Dialer abstracts WebSocket connection establishment.
type Dialer interface {
	Dial(ctx context.Context, url string, headers http.Header) (Conn, *http.Response, error)
}

// DefaultDialer is a package-level Dialer backed by gorilla/websocket.
var DefaultDialer Dialer = gorillaDialer{}

type gorillaDialer struct{}

func (gorillaDialer) Dial(ctx context.Context, url string, headers http.Header) (Conn, *http.Response, error) {
	d := *gws.DefaultDialer
	c, resp, err := d.DialContext(ctx, url, headers)
	if err != nil {
		return nil, resp, err
	}
	return &gorillaConn{c: c}, resp, nil
}

type gorillaConn struct{ c *gws.Conn }

func (g *gorillaConn) WriteMessage(t int, d []byte) error { return g.c.WriteMessage(t, d) }
func (g *gorillaConn) ReadMessage() (int, []byte, error)  { return g.c.ReadMessage() }
func (g *gorillaConn) SetReadDeadline(t time.Time) error  { return g.c.SetReadDeadline(t) }
func (g *gorillaConn) Close() error                        { return g.c.Close() }
func (g *gorillaConn) WriteControl(t int, d []byte, dl time.Time) error {
	return g.c.WriteControl(t, d, dl)
}
```

#### New Code (`internal/websocket/executor.go`)
```go
package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// StepResult records the outcome of one step.
type StepResult struct {
	Action     string
	Passed     bool
	Err        error
	Assertions *assertion.Results // populated for expect steps
	Extracted  map[string]string  // populated for expect steps
}

// Result is the aggregate outcome of running all WebSocket steps.
type Result struct {
	Steps    []StepResult
	Duration time.Duration
	Passed   bool
	Err      error // first non-assertion error (connection/timeout/close)
}

// Execute dials the URL, runs each step sequentially, and returns the aggregate
// result. Variables extracted in expect steps are applied to scope immediately
// so subsequent steps can reference them.
func Execute(
	ctx context.Context,
	req *parser.Request,
	scope *variable.Scope,
	dialer Dialer,
) *Result {
	if dialer == nil {
		dialer = DefaultDialer
	}
	start := time.Now()
	result := &Result{}

	headers := http.Header{}
	for k, v := range req.Headers {
		headers.Set(k, v)
	}
	conn, _, err := dialer.Dial(ctx, req.URL, headers)
	if err != nil {
		result.Err = fmt.Errorf("%w: %v", ErrDialFailed, err)
		result.Duration = time.Since(start)
		return result
	}
	defer func() {
		_ = conn.Close()
		result.Duration = time.Since(start)
	}()

	buffer := newMessageBuffer()
	for i, step := range req.WebSocket.Steps {
		sr := runStep(ctx, conn, buffer, step, scope)
		result.Steps = append(result.Steps, sr)
		if !sr.Passed {
			if sr.Err != nil {
				result.Err = fmt.Errorf("step %d (%s): %w", i+1, step.Action, sr.Err)
			} else {
				result.Err = fmt.Errorf("step %d (%s): assertions failed", i+1, step.Action)
			}
			return result
		}
		// close step is always final
		if step.Action == "close" {
			break
		}
	}
	result.Passed = true
	return result
}

func runStep(ctx context.Context, conn Conn, buf *messageBuffer, step parser.WebSocketStep, scope *variable.Scope) StepResult {
	switch step.Action {
	case "send":
		return runSend(conn, step)
	case "expect":
		return runExpect(ctx, conn, buf, step, scope)
	case "wait":
		return runWait(ctx, step)
	case "close":
		return runClose(conn, step)
	default:
		return StepResult{Action: step.Action, Err: fmt.Errorf("%w: %q", ErrUnknownAction, step.Action)}
	}
}

// ... runSend, runExpect, runWait, runClose, messageBuffer implemented below
```

Key implementation details for each step function:
- **`runSend`**: If `MessageRaw != ""` → `WriteMessage(TextMessage, []byte(MessageRaw))`. Else marshal `Message` via `json.Marshal` and write. Interpolation is already done upstream by `requtil.InterpolateRequest`.
- **`runExpect`**: 1) Drain any already-queued messages into `buffer`. 2) Compute `deadline := time.Now().Add(timeout_ms)` (default 5000ms if zero). 3) First, walk the buffer FIFO and try to match each buffered entry against `ExpectAssertions` using `assertion.CheckBody`. If all assertions pass, consume the entry, run `variable.Extract` on it, update scope, and return `Passed: true`. 4) Otherwise enter a read loop: `conn.SetReadDeadline(deadline)`, `conn.ReadMessage()`, append to buffer, retry match. 5) On deadline: return `StepResult{Err: ErrExpectTimeout}`.
- **`runWait`**: `select { case <-ctx.Done(): ... ; case <-time.After(duration) }`.
- **`runClose`**: Write a close control frame using `gws.FormatCloseMessage(code, reason)`; code defaults to 1000 if zero. Returns `Passed: true` regardless of whether server sent a reply (spec: "clean shutdown").

#### New Code (`internal/websocket/errors.go`)
```go
package websocket

import "errors"

var (
	ErrDialFailed    = errors.New("websocket dial failed")
	ErrUnknownAction = errors.New("unknown websocket action")
	ErrExpectTimeout = errors.New("expect timed out")
	ErrSendFailed    = errors.New("send failed")
	ErrCloseFailed   = errors.New("close failed")
)
```

#### Tests to Write FIRST — `executor_test.go`

Table-driven tests using a fake `Conn`:

```go
type fakeConn struct {
	writes        [][]byte
	incoming      [][]byte // messages available to be "read"
	readErr       error
	writeErr      error
	closed        bool
	readDeadline  time.Time
}

func (f *fakeConn) WriteMessage(mt int, data []byte) error { /* append */ }
func (f *fakeConn) ReadMessage() (int, []byte, error) { /* pop or block-until-deadline */ }
// ... etc

type fakeDialer struct {
	conn    Conn
	dialErr error
}
func (f fakeDialer) Dial(ctx context.Context, url string, h http.Header) (Conn, *http.Response, error) {
	return f.conn, nil, f.dialErr
}
```

Test cases:

| Case | Setup | Assertion |
|---|---|---|
| `dial failure returns ErrDialFailed` | `fakeDialer{dialErr: net err}` | `result.Err` wraps `ErrDialFailed`, `Passed==false` |
| `send writes JSON message` | one `send` step with `Message{type: "ping"}` | `conn.writes[0]` unmarshals to `{"type":"ping"}` |
| `send writes raw message` | one `send` step with `MessageRaw: "PING"` | `conn.writes[0] == []byte("PING")` |
| `expect matches immediately from buffer` | incoming: `{"type":"ok"}` delivered before read | buffer match, no real `ReadMessage` call needed beyond drain |
| `expect reads message and matches` | incoming: `{"type":"ok"}` | step passes, assertion results populated |
| `expect assertion mismatch fails` | incoming: `{"type":"nope"}`, expect `type=ok` | step fails, `Passed==false`, assertion `Passed==false` |
| `expect timeout` | no incoming, `timeout_ms=50` | `sr.Err == ErrExpectTimeout`, duration ≥ 50ms |
| `expect extracts variable into scope` | incoming: `{"token":"T"}`, `extract: {tok: "$.token"}` | `scope.Get("tok") == "T"` |
| `wait pauses for duration` | `wait duration_ms=30` | elapsed ≥ 30ms, no conn writes |
| `wait respects context cancellation` | canceled ctx mid-wait | step fails with `ctx.Err()` |
| `close writes close frame` | `close code=1000 reason="done"` | `conn.WriteControl` called with CloseMessage |
| `default close code 1000 when zero` | `close` | verify WriteControl payload starts with 0x03, 0xE8 |
| `step ordering stops on first failure` | send → expect (fails) → send | only 2 StepResults, second send never executed |
| `variables extracted mid-stream available to later steps` | expect extracts `sid`; later send uses `sid` | send payload (after interpolation from caller's perspective) is what the caller passed — extraction updates scope only; interpolation is caller's job (assert scope state) |
| `buffer FIFO ordering` | push two messages; first matches, second matches next expect | both expects pass; writes accessed in order |

#### Impact on Existing Tests
None — new package.

---

### Step 3: Wire WebSocket execution into `runner.Run`
**Rationale:** Once the package is green and the parser emits `WebSocket` configs, the runner can branch to it. This step changes `runner.go`, which has the widest blast radius, so it comes after Steps 1–2 are green.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `protocol_websocket` branch next to graphql; synthesize `httpexec.Result` |
| `internal/runner/runner_test.go` | modify | Add `TestRun_websocket_*` tests using a fake dialer |
| `internal/auth/registry.go` | modify | Register `protocol_websocket` feature |
| `internal/auth/registry_test.go` | modify | Assert registration |

#### Current Code (runner.go around the graphql block)
```go
// GraphQL protocol handling: feature gate check and request transformation
if req.Protocol == "graphql" {
	// ... feature gate + BuildRequest ...
}
```

#### New Code (insert new branch + injectable dialer)
```go
// WebSocketDialer allows tests to inject a fake dialer. nil → websocket.DefaultDialer.
type WebSocketDialer = websocket.Dialer

// In VarSources:
//   WebSocketDialer WebSocketDialer // nil = use websocket.DefaultDialer

// Inside the request loop, after graphql block:
if req.Protocol == "websocket" {
	reg := vars.Registry
	if reg == nil {
		reg = auth.DefaultRegistry()
	}
	tier := vars.Tier
	if tier == "" {
		tier = auth.TierFree
	}
	if gateErr := auth.CheckFeature(reg, "protocol_websocket", tier); gateErr != nil {
		return results, requiredFailed, gateErr
	}

	// Count all steps as one request against the guard rail.
	*counter++
	if *counter > maxRequests {
		// existing limit handling – mirror the HTTP branch
		summary.LimitExceeded = true
		stopped = true
		continue
	}

	dialer := vars.WebSocketDialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	wsRes := websocket.Execute(ctx, &req, reqScope, dialer)

	// Synthesize an httpexec.Result so downstream output formatters keep working.
	synth := &httpexec.Result{
		StatusCode: 101,
		Duration:   wsRes.Duration,
		Body:       nil,
		Headers:    http.Header{"X-WebSocket-Steps": []string{fmt.Sprintf("%d", len(wsRes.Steps))}},
	}
	if !wsRes.Passed {
		synth.StatusCode = 0
	}

	rr := RequestResult{
		Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
		RequestHeaders: req.Headers, RequestBody: req.Body,
		Result:         synth,
		Err:            wsRes.Err,
		WaveIndex:      -1,
	}
	if wsRes.Passed {
		rr.AssertionResults = &assertion.Results{Passed: true}
	} else {
		rr.AssertionResults = &assertion.Results{
			Passed: false,
			Items: []assertion.Result{{
				Type:     "websocket",
				Expected: "all steps pass",
				Actual:   fmt.Sprint(wsRes.Err),
				Passed:   false,
			}},
		}
		if checkRequired && item.IsRequired() {
			requiredFailed = true
			stopped = true
		} else if stopOnFailure {
			stopped = true
		}
	}
	results = append(results, rr)
	continue
}
```

**Positioning:** this branch goes **after** GraphQL handling but **before** the existing `exec` call. The `continue` guarantees the HTTP path is not taken for WebSocket requests.

**Counter/MaxRequests parity:** the HTTP branch delegates counter increment to the `exec` closure; the WebSocket branch must increment manually because it does not call `exec`. Verify against the existing guard-rail test to confirm my reading of where the counter is incremented. If necessary, centralise in a helper.

#### Auth registry addition (`internal/auth/registry.go`)
```go
r.Register(FeatureDefinition{
	Name:         "protocol_websocket",
	RequiredTier: TierProfessional,
	Description:  "WebSocket protocol support requires Professional tier ($19/month)",
	Workaround:   "Use external tools like wscat or websocat for ad-hoc WebSocket testing",
})
```

#### Tests to Write FIRST — `runner_test.go`

```go
func TestRun_websocket_basic_lifecycle(t *testing.T) { /* send + expect + close with fake dialer, Tier=Professional, asserts Passed */ }
func TestRun_websocket_feature_gate_free_tier(t *testing.T) { /* Tier=Free → *auth.GateError */ }
func TestRun_websocket_expect_timeout_fails(t *testing.T) { /* fake conn never delivers, result failed, AssertionResults.Passed==false */ }
func TestRun_websocket_counts_as_one_request(t *testing.T) { /* 5 steps → *counter incremented by 1; MaxRequests=1 allows it, MaxRequests=0 trips guard rail */ }
func TestRun_websocket_variable_interpolation_url(t *testing.T) { /* URL = "ws://{{host}}/chat", scope host=localhost, fake dialer records dialed URL */ }
func TestRun_websocket_extract_variable_available_in_later_step(t *testing.T) { /* expect extracts sid; next send's Message includes {{sid}}; assert interpolated payload written */ }
func TestRun_websocket_stop_on_failure(t *testing.T) { /* first request fails, stop_on_failure=true → second request not executed */ }
```

The fake dialer is a test helper — define a minimal `wsFakeDialer` at the top of the new test block or in a shared `_test.go` helper.

#### Impact on Existing Tests
- `runner_test.go` — unchanged existing tests; new imports added.
- `TestRun_graphql_*` — unchanged.
- Adding `WebSocketDialer` field to `VarSources` is a struct-field addition → all existing zero-valued `VarSources{}` literals still compile.
- `auth/registry_test.go` — if it asserts the exact count of registered features, bump the expected count by 1. Otherwise no change.

---

### Step 4: End-to-end integration test with httptest + gorilla upgrader
**Rationale:** Prove the real gorilla dialer works against a real upgrader. Lower blast radius because it only *adds* a new test file.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/websocket/integration_test.go` | create | Spin up `httptest.NewServer` with `websocket.Upgrader`, run `Execute` against it, assert round-trip |
| `cmd/curlew/testdata/ws_basic.yaml` | create | Fixture for the smoke/integration test of the `run` command |
| `cmd/curlew/run_test.go` | modify | Add `TestRun_websocket_integration` that sets `runCommandWebSocketDialer` (or uses the real dialer against `httptest.NewServer`) |

Integration test outline:
```go
func TestExecute_real_gorilla_dialer(t *testing.T) {
	upgrader := gws.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil { t.Fatal(err) }
		defer c.Close()
		// echo once then accept close
		_, msg, _ := c.ReadMessage()
		_ = c.WriteMessage(gws.TextMessage, msg)
		for {
			if _, _, err := c.ReadMessage(); err != nil { return }
		}
	}))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	req := &parser.Request{
		Protocol: "websocket",
		URL: url,
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{
				{Action: "send", Message: map[string]any{"type": "hello"}},
				{Action: "expect", TimeoutMs: 2000, ExpectAssertions: /* $.type equals hello */},
				{Action: "close", Code: 1000},
			},
		},
	}
	scope := variable.NewScope(nil)
	result := Execute(context.Background(), req, scope, DefaultDialer)
	if !result.Passed { t.Fatalf("result failed: %v", result.Err) }
}
```

#### Impact on Existing Tests
None.

---

### Step 5: Help text, CHANGELOG, smoke test
**Rationale:** Non-behavioral polish; do last so we know what we actually delivered.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add M2-032 entry under `[Unreleased]` → `Added` |
| `smoke/run.sh` | modify (optional) | Add a WebSocket validation line if smoke currently exercises protocols — inspect first; if the smoke test runs `validate` on a fixture, add a websocket fixture and `validate` call. Do NOT add a run-against-real-server call (non-hermetic). |
| `cmd/curlew/main.go` | modify (if needed) | If `run --help` enumerates supported protocols anywhere, append `websocket`. Grep confirms no such enumeration today, but re-verify during execute. |

CHANGELOG entry (single line, keep-a-changelog style, matching M2-029's format):
> WebSocket protocol adapter: `protocol: websocket` with `websocket.steps` containing `send` (JSON `message:` or raw `message_raw:`), `expect` (JSONPath assertions on `message:`, `timeout_ms`, `extract:`), `wait` (`duration_ms`), and `close` (`code`, `reason`) actions; establishes a real WebSocket connection via HTTP upgrade using `gorilla/websocket`; FIFO message buffer so pre-delivered messages are matched before waiting; variable interpolation in URL/headers/message fields; variables extracted in `expect` steps are available to subsequent steps; Professional-tier feature gate (exit code 6 at Free tier); each WebSocket request counts as 1 request against the guard rail regardless of step count (M2-032)

#### Impact on Existing Tests
None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|---|---|---|---|
| `internal/parser/parser_test.go` | `TestParseFile_websocket` (new) | new | add |
| `internal/parser/parser_test.go` | `TestParseFile_graphql` "invalid protocol value" | none | fixture uses an unrelated bad value; still invalid |
| `internal/websocket/executor_test.go` | many (new) | new | add |
| `internal/websocket/integration_test.go` | `TestExecute_real_gorilla_dialer` (new) | new | add |
| `internal/runner/runner_test.go` | `TestRun_websocket_*` (new) | new | add |
| `internal/runner/runner_test.go` | existing graphql + http tests | none | `VarSources{}` literals still compile |
| `internal/auth/registry_test.go` | feature count assertion (if any) | possibly breaks | increment expected count by 1 |
| `cmd/curlew/run_test.go` | `TestRun_websocket_integration` (new) | new | add |

## Risks and Edge Cases

- **Risk:** `gorilla/websocket` is an external dependency the project currently has none of (only `fsnotify` and `yaml.v3`). **Mitigation:** the spec explicitly calls for it. License is BSD-2, single module, zero transitive deps beyond stdlib, last released 2024 — acceptable per the "minimise dependencies" rule because writing a WebSocket framer from scratch is out of scope.
- **Risk:** The `expect` action's `message:` field is overloaded (literal JSON for send, assertion map for expect). **Mitigation:** custom `UnmarshalYAML` on `WebSocketStep` that peeks `action` before decoding `message:`. Covered by parser tests.
- **Risk:** `SetReadDeadline` based timeout interacts poorly with repeated reads when multiple messages arrive before the match. **Mitigation:** recompute `time.Until(deadline)` and call `SetReadDeadline` before **each** `ReadMessage`; if deadline already elapsed, short-circuit with `ErrExpectTimeout`.
- **Risk:** Goroutine leak if dial succeeds but a step hangs and the context is canceled — gorilla's `ReadMessage` honors `SetReadDeadline` but not `ctx`. **Mitigation:** the `defer conn.Close()` path plus the `SetReadDeadline(deadline)` timeout bounds the blocking. For `wait`, use `select { <-ctx.Done() }` so cancellation is immediate.
- **Risk:** Guard rail counter semantics — off-by-one. **Mitigation:** the HTTP branch increments inside the exec closure *after* the call; match that ordering (increment before `Execute` call is simpler and spec-compliant because all steps count as one). Write `TestRun_websocket_counts_as_one_request` to lock the behavior.
- **Edge case:** `stop_on_failure` with `required: true` — a WebSocket failure must propagate like an HTTP failure. Mirror the `if ar != nil && !ar.Passed` branch semantics by populating `AssertionResults` with a synthetic failing item.
- **Edge case:** Variable interpolation touches WebSocket fields. `requtil.InterpolateRequest` must be extended to walk `req.WebSocket.Steps[*]` URL/headers are already handled; `message` and `message_raw` per step need interpolation. Add this in Step 3 alongside the runner wiring.
  - Concretely, extend `InterpolateRequest`:
    ```go
    if out.WebSocket != nil {
        ws := *out.WebSocket
        ws.Steps = append([]parser.WebSocketStep(nil), ws.Steps...)
        for i := range ws.Steps {
            if ws.Steps[i].MessageRaw != "" {
                if ws.Steps[i].MessageRaw, err = scope.Interpolate(ws.Steps[i].MessageRaw); err != nil {
                    return nil, fmt.Errorf("websocket step %d message_raw: %w", i+1, err)
                }
            }
            if ws.Steps[i].Message != nil {
                // walk map values; interpolate strings; leave other types
                ws.Steps[i].Message = interpolateAny(scope, ws.Steps[i].Message)
            }
        }
        out.WebSocket = &ws
    }
    ```
    Add a `requtil_test.go` case for this.
- **Edge case:** `ws://` URL normalization — should parser accept URLs with `ws://` scheme even when `Protocol: ""`? The spec says "Auto-Detection: `ws://`, `wss://` schemes auto-detect WebSocket." **Decision:** DEFER auto-detection to a follow-up; for M2-032 users must set `protocol: websocket` explicitly. Note in plan; do not add auto-detect logic here.
- **Edge case:** Extractions inside an expect step must flow into `scope` (not `reqScope`) so later collection requests benefit. **Mitigation:** pass `scope` through to `websocket.Execute`; `runExpect` writes via `scope.Set`.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Build
go build ./cmd/curlew

# Unit tests for the new package
go test ./internal/websocket/...

# End-to-end against a file-based collection
cat > /tmp/ws_tests.yaml <<'YAML'
name: WebSocket Smoke
requests:
  - name: echo
    request:
      protocol: websocket
      url: "ws://localhost:9999/echo"
      websocket:
        steps:
          - action: send
            message: { type: "hello" }
          - action: expect
            timeout_ms: 2000
            message:
              $.type: { equals: "hello" }
          - action: close
            code: 1000
YAML
# (Run against a test server — see internal/websocket/integration_test.go for the in-process equivalent.)
./curlew run /tmp/ws_tests.yaml
```
