package websocket

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// fakeConn is an in-memory Conn implementation for unit tests.
type fakeConn struct {
	mu            sync.Mutex
	writes        [][]byte
	controlWrites []controlFrame
	incoming      [][]byte
	readErr       error
	writeErr      error
	closed        bool
	readDelay     time.Duration // optional artificial delay before returning a message
	deadline      time.Time
	pongHandler   func(appData string) error // set by SetPongHandler
}

type controlFrame struct {
	messageType int
	data        []byte
}

func (f *fakeConn) WriteMessage(messageType int, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return f.writeErr
	}
	cpy := make([]byte, len(data))
	copy(cpy, data)
	f.writes = append(f.writes, cpy)
	return nil
}

func (f *fakeConn) ReadMessage() (int, []byte, error) {
	f.mu.Lock()
	if f.readErr != nil {
		err := f.readErr
		f.mu.Unlock()
		return 0, nil, err
	}
	if len(f.incoming) > 0 {
		msg := f.incoming[0]
		f.incoming = f.incoming[1:]
		delay := f.readDelay
		deadline := f.deadline
		f.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return 0, nil, &timeoutError{}
		}
		return gws.TextMessage, msg, nil
	}
	deadline := f.deadline
	f.mu.Unlock()
	if deadline.IsZero() {
		// No deadline: fail fast rather than hanging a misconfigured test.
		return 0, nil, &timeoutError{}
	}
	wait := time.Until(deadline)
	if wait > 0 {
		time.Sleep(wait)
	}
	return 0, nil, &timeoutError{}
}

func (f *fakeConn) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deadline = t
	return nil
}

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeConn) WriteControl(messageType int, data []byte, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cpy := make([]byte, len(data))
	copy(cpy, data)
	f.controlWrites = append(f.controlWrites, controlFrame{messageType: messageType, data: cpy})
	return nil
}

func (f *fakeConn) SetPongHandler(h func(appData string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pongHandler = h
}

// timeoutError satisfies net.Error so gorilla-style timeout checks work.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type fakeDialer struct {
	conn     Conn
	dialErr  error
	lastURL  string
	lastHdrs http.Header
}

func (f *fakeDialer) Dial(_ context.Context, url string, headers http.Header) (Conn, *http.Response, error) {
	f.lastURL = url
	f.lastHdrs = headers
	if f.dialErr != nil {
		return nil, nil, f.dialErr
	}
	return f.conn, nil, nil
}

func newTestRequest(steps ...parser.WebSocketStep) *parser.Request {
	return &parser.Request{
		Protocol: "websocket",
		Method:   "WS",
		URL:      "ws://fake/ws",
		WebSocket: &parser.WebSocketConfig{
			Steps: steps,
		},
	}
}

func TestExecute_DialFailure(t *testing.T) {
	dialer := &fakeDialer{dialErr: errors.New("connection refused")}
	req := newTestRequest(parser.WebSocketStep{Action: "close"})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("result should not be Passed on dial failure")
	}
	if !errors.Is(result.Err, ErrDialFailed) {
		t.Errorf("result.Err = %v, want wrapped ErrDialFailed", result.Err)
	}
}

func TestExecute_SendJSONMessage(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:  "send",
		Message: map[string]any{"type": "ping", "id": float64(1)},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result.Passed = false, err = %v", result.Err)
	}
	if len(fc.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fc.writes))
	}
	var got map[string]any
	if err := json.Unmarshal(fc.writes[0], &got); err != nil {
		t.Fatalf("write not JSON: %v", err)
	}
	if got["type"] != "ping" {
		t.Errorf("got[type] = %v, want ping", got["type"])
	}
}

func TestExecute_SendRawMessage(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:     "send",
		MessageRaw: "PING",
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result.Passed = false, err = %v", result.Err)
	}
	if string(fc.writes[0]) != "PING" {
		t.Errorf("write = %q, want PING", fc.writes[0])
	}
}

func TestExecute_SendRawMessageNotJSONEncoded(t *testing.T) {
	// A literal JSON object sent via message_raw must arrive byte-for-byte
	// identical to the input. It must NOT be JSON-serialised again (which
	// would yield a double-encoded string like "{\"raw\":\"value\"}").
	input := `{"raw":"value"}`
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:     "send",
		MessageRaw: input,
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result.Passed = false, err = %v", result.Err)
	}
	if len(fc.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fc.writes))
	}
	got := string(fc.writes[0])
	if got != input {
		t.Errorf("message_raw was re-serialised: got %q, want %q", got, input)
	}
	// Double-encoded form would be a JSON string with escaped quotes.
	doubleEncoded := `"{\"raw\":\"value\"}"`
	if got == doubleEncoded {
		t.Errorf("message_raw was JSON-marshalled: got %q (double-encoded)", got)
	}
}

func TestExecute_ExpectMatchesFromBuffer(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"ok"}`)}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 100,
		ExpectAssertions: parser.BodyAssertions{
			Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result.Passed = false, err = %v", result.Err)
	}
	if len(result.Steps) != 1 || !result.Steps[0].Passed {
		t.Errorf("step not passed: %+v", result.Steps)
	}
}

func TestExecute_ExpectAssertionMismatch(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"nope"}`)}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 100,
		ExpectAssertions: parser.BodyAssertions{
			Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("expected assertion failure")
	}
	if result.Err == nil {
		t.Fatal("expected non-nil Err")
	}
}

func TestExecute_ExpectTimeout(t *testing.T) {
	fc := &fakeConn{} // no incoming, deadline-based timeout
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 50,
		ExpectAssertions: parser.BodyAssertions{
			Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
		},
	})

	start := time.Now()
	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)
	elapsed := time.Since(start)

	if result.Passed {
		t.Fatal("expected timeout failure")
	}
	if !errors.Is(result.Err, ErrExpectTimeout) {
		t.Errorf("err = %v, want ErrExpectTimeout", result.Err)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 50ms", elapsed)
	}
}

func TestExecute_ExpectExtractsVariable(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"token":"T42","type":"ok"}`)}}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 100,
		ExpectAssertions: parser.BodyAssertions{
			Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
		},
		Extract: map[string]string{"tok": "$.token"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if got := scope.Resolved()["tok"]; got != "T42" {
		t.Errorf("tok = %q, want T42", got)
	}
}

func TestExecute_WaitPausesForDuration(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{Action: "wait", DurationMs: 30})

	start := time.Now()
	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)
	elapsed := time.Since(start)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if elapsed < 30*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 30ms", elapsed)
	}
	if len(fc.writes) != 0 {
		t.Errorf("wait should not write: got %d writes", len(fc.writes))
	}
}

func TestExecute_WaitRespectsCancellation(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{Action: "wait", DurationMs: 10000})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	result := Execute(ctx, req, variable.NewScope(nil), dialer)
	elapsed := time.Since(start)

	if result.Passed {
		t.Fatal("result should not be Passed when context canceled")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("elapsed = %v, want < 500ms", elapsed)
	}
}

func TestExecute_CloseWritesCloseFrame(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{Action: "close", Code: 1000, Reason: "done"})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if len(fc.controlWrites) != 1 {
		t.Fatalf("control writes = %d, want 1", len(fc.controlWrites))
	}
	if fc.controlWrites[0].messageType != gws.CloseMessage {
		t.Errorf("messageType = %d, want CloseMessage", fc.controlWrites[0].messageType)
	}
	// Close frame payload: 2-byte code + reason
	payload := fc.controlWrites[0].data
	if len(payload) < 2 {
		t.Fatalf("close payload too short: %v", payload)
	}
	gotCode := binary.BigEndian.Uint16(payload[:2])
	if gotCode != 1000 {
		t.Errorf("close code = %d, want 1000", gotCode)
	}
}

func TestExecute_CloseDefaultsCodeToNormalClosure(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{Action: "close"})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	payload := fc.controlWrites[0].data
	gotCode := binary.BigEndian.Uint16(payload[:2])
	if gotCode != 1000 {
		t.Errorf("default code = %d, want 1000", gotCode)
	}
}

func TestExecute_StopsOnFirstFailure(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"nope"}`)}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(
		parser.WebSocketStep{Action: "send", MessageRaw: "first"},
		parser.WebSocketStep{
			Action:    "expect",
			TimeoutMs: 100,
			ExpectAssertions: parser.BodyAssertions{
				Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
			},
		},
		parser.WebSocketStep{Action: "send", MessageRaw: "never"},
	)

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("expected failure")
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(result.Steps))
	}
	if len(fc.writes) != 1 || string(fc.writes[0]) != "first" {
		t.Errorf("writes = %v, want only [first]", fc.writes)
	}
}

func TestExecute_BufferFIFO(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"a"}`),
		[]byte(`{"type":"b"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(
		parser.WebSocketStep{
			Action:    "expect",
			TimeoutMs: 100,
			ExpectAssertions: parser.BodyAssertions{
				Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "a"}},
			},
		},
		parser.WebSocketStep{
			Action:    "expect",
			TimeoutMs: 100,
			ExpectAssertions: parser.BodyAssertions{
				Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "b"}},
			},
		},
	)

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(result.Steps))
	}
}

func TestExecute_ExpectSkipsStrayMessagesBeforeMatch(t *testing.T) {
	// First two frames are unrelated heartbeats; the third matches the assertion.
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"heartbeat"}`),
		[]byte(`{"type":"noise"}`),
		[]byte(`{"type":"ok"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 200,
		ExpectAssertions: parser.BodyAssertions{
			Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result.Passed = false, err = %v", result.Err)
	}
	if len(result.Steps) != 1 || !result.Steps[0].Passed {
		t.Errorf("step not passed: %+v", result.Steps)
	}
}

func TestExecute_ExpectTimesOutAfterOnlyStrayMessages(t *testing.T) {
	// Only stray frames; the expect step must time out, not be satisfied
	// by the last unrelated frame.
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"noise"}`),
		[]byte(`{"type":"heartbeat"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 60,
		ExpectAssertions: parser.BodyAssertions{
			Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("expected timeout failure when no frame matches")
	}
	if !errors.Is(result.Err, ErrExpectTimeout) {
		t.Errorf("err = %v, want ErrExpectTimeout", result.Err)
	}
}

func TestExecute_ExtractedVariableAvailableInLaterStep(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"sid":"S42","type":"ok"}`)}}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(
		parser.WebSocketStep{
			Action:    "expect",
			TimeoutMs: 100,
			ExpectAssertions: parser.BodyAssertions{
				Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
			},
			Extract: map[string]string{"sid": "$.sid"},
		},
		parser.WebSocketStep{
			Action:     "send",
			MessageRaw: "subscribe {{sid}}",
		},
	)

	// runner.go applies request-level interpolation before Execute is called,
	// but per-step placeholders referencing variables extracted by an earlier
	// step must be resolved at send time. This test pins that contract.
	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if len(fc.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fc.writes))
	}
	if string(fc.writes[0]) != "subscribe S42" {
		t.Errorf("write = %q, want %q", fc.writes[0], "subscribe S42")
	}
}

func TestExecute_EmptyStepsErrors(t *testing.T) {
	dialer := &fakeDialer{conn: &fakeConn{}}
	req := &parser.Request{
		Protocol:  "websocket",
		Method:    "WS",
		URL:       "ws://fake/ws",
		WebSocket: &parser.WebSocketConfig{Steps: nil},
	}

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("expected failure for empty-steps request")
	}
	if !errors.Is(result.Err, ErrNoSteps) {
		t.Errorf("err = %v, want ErrNoSteps", result.Err)
	}
}

func TestExecute_UnknownActionErrors(t *testing.T) {
	dialer := &fakeDialer{conn: &fakeConn{}}
	req := newTestRequest(parser.WebSocketStep{Action: "bogus"})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("expected failure for unknown action")
	}
	if !errors.Is(result.Err, ErrUnknownAction) {
		t.Errorf("err = %v, want ErrUnknownAction", result.Err)
	}
}

func TestExecute_ConnectionClosed(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{Action: "close"})

	_ = Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !fc.closed {
		t.Error("Close was not called on conn")
	}
}

func TestExecute_SendMessageTemplate(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(parser.WebSocketStep{
		Action:             "send",
		MessageRawTemplate: `{"channel":"{{channel}}"}`,
		StepVariables:      map[string]string{"channel": "news"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if len(fc.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fc.writes))
	}
	if string(fc.writes[0]) != `{"channel":"news"}` {
		t.Errorf("write = %q, want {\"channel\":\"news\"}", fc.writes[0])
	}
}

func TestExecute_SendMessageTemplate_UsesScopeVars(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(map[string]string{"token": "tok123"})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(parser.WebSocketStep{
		Action:             "send",
		MessageRawTemplate: `{"auth":"{{token}}","ch":"{{channel}}"}`,
		StepVariables:      map[string]string{"channel": "sports"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if string(fc.writes[0]) != `{"auth":"tok123","ch":"sports"}` {
		t.Errorf("write = %q", fc.writes[0])
	}
}

func TestExecute_SendMessageTemplate_StepVarsDoNotLeak(t *testing.T) {
	fc := &fakeConn{}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(parser.WebSocketStep{
		Action:             "send",
		MessageRawTemplate: `{"channel":"{{channel}}"}`,
		StepVariables:      map[string]string{"channel": "tech"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	// "channel" step variable must NOT appear in the parent scope.
	if _, ok := scope.Resolved()["channel"]; ok {
		t.Error("step variable 'channel' leaked into parent scope")
	}
}

func TestExecute_ExpectAnyOfMatchesFirst(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"success"}`)}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 100,
		AnyOf: []parser.BodyAssertions{
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "success"}}},
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "error"}}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result.Passed = false, err = %v", result.Err)
	}
}

func TestExecute_ExpectAnyOfMatchesSecond(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"error"}`)}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 100,
		AnyOf: []parser.BodyAssertions{
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "success"}}},
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "error"}}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result.Passed = false, err = %v", result.Err)
	}
}

func TestExecute_ExpectCountCollectsArray(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"update","data":"x"}`),
		[]byte(`{"type":"update","data":"y"}`),
		[]byte(`{"type":"update","data":"z"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 200,
		Count:     3,
		ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{
			{Path: "$.type", Operator: "equals", Value: "update"},
		}},
		Extract: map[string]string{"updates": "$.data"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	got := scope.Resolved()["updates"]
	if got != `["x","y","z"]` {
		t.Errorf("updates = %q, want [\"x\",\"y\",\"z\"]", got)
	}
}

func TestExecute_ExpectCountPartialTimesOut(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"update","data":"x"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 60,
		Count:     3,
		ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{
			{Path: "$.type", Operator: "equals", Value: "update"},
		}},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("expected timeout failure when count not reached")
	}
	if !errors.Is(result.Err, ErrExpectTimeout) {
		t.Errorf("err = %v, want ErrExpectTimeout", result.Err)
	}
	if !strings.Contains(result.Err.Error(), "1/3") {
		t.Errorf("err = %q, want to mention 1/3", result.Err.Error())
	}
}

func TestExecute_ExpectCountOne_BackwardCompat(t *testing.T) {
	// Count: 1 with extract yields a flat (non-array) variable value.
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"ok","token":"T99"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 100,
		Count:     1,
		ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{
			{Path: "$.type", Operator: "equals", Value: "ok"},
		}},
		Extract: map[string]string{"tok": "$.token"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if got := scope.Resolved()["tok"]; got != "T99" {
		t.Errorf("tok = %q, want T99", got)
	}
}

func TestExecute_ExpectCountWithAnyOf(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"success","data":"s1"}`),
		[]byte(`{"type":"error","data":"e1"}`),
		[]byte(`{"type":"success","data":"s2"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 200,
		Count:     3,
		AnyOf: []parser.BodyAssertions{
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "success"}}},
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "error"}}},
		},
		Extract: map[string]string{"results": "$.data"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	got := scope.Resolved()["results"]
	if got != `["s1","e1","s2"]` {
		t.Errorf("results = %q, want [\"s1\",\"e1\",\"s2\"]", got)
	}
}

func TestExecute_ExpectCountSkipsNonMatchingMessages(t *testing.T) {
	// Interleaved noise and matching frames; only matches count toward N=2.
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"noise"}`),
		[]byte(`{"type":"update","data":"a"}`),
		[]byte(`{"type":"noise"}`),
		[]byte(`{"type":"update","data":"b"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	scope := variable.NewScope(nil)
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 200,
		Count:     2,
		ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{
			{Path: "$.type", Operator: "equals", Value: "update"},
		}},
		Extract: map[string]string{"items": "$.data"},
	})

	result := Execute(context.Background(), req, scope, dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	got := scope.Resolved()["items"]
	if got != `["a","b"]` {
		t.Errorf("items = %q, want [\"a\",\"b\"]", got)
	}
}

func TestExecute_ExpectAnyOfNoneMatchTimesOut(t *testing.T) {
	fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"noise"}`)}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 60,
		AnyOf: []parser.BodyAssertions{
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "success"}}},
			{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "error"}}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Fatal("expected timeout when no frame matches any alternative")
	}
	if !errors.Is(result.Err, ErrExpectTimeout) {
		t.Errorf("err = %v, want ErrExpectTimeout", result.Err)
	}
}

func TestExecute_BufferCarriesFramesAcrossSteps(t *testing.T) {
	// Three frames arrive up-front. Steps expect them out of order:
	// step 1 wants "a", step 2 wants "c", step 3 wants "b".
	// The buffer must retain "b" and "c" after "a" is consumed so the
	// second expect can find "c" instantly from the buffer.
	fc := &fakeConn{incoming: [][]byte{
		[]byte(`{"type":"a"}`),
		[]byte(`{"type":"b"}`),
		[]byte(`{"type":"c"}`),
	}}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(
		parser.WebSocketStep{
			Action:    "expect",
			TimeoutMs: 100,
			ExpectAssertions: parser.BodyAssertions{
				Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "a"}},
			},
		},
		parser.WebSocketStep{
			Action:    "expect",
			TimeoutMs: 100,
			ExpectAssertions: parser.BodyAssertions{
				Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "c"}},
			},
		},
		parser.WebSocketStep{
			Action:    "expect",
			TimeoutMs: 100,
			ExpectAssertions: parser.BodyAssertions{
				Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "b"}},
			},
		},
	)

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(result.Steps))
	}
}

func TestExecute_BufferWarningSurfaced(t *testing.T) {
	// Build 101 stray frames then one matching frame.
	// The stray frames should trigger the buffer warning.
	stray := make([][]byte, 101)
	for i := range stray {
		stray[i] = []byte(`{"type":"noise"}`)
	}
	frames := append(stray, []byte(`{"type":"ok"}`))
	fc := &fakeConn{incoming: frames}
	dialer := &fakeDialer{conn: fc}
	req := newTestRequest(parser.WebSocketStep{
		Action:    "expect",
		TimeoutMs: 500,
		ExpectAssertions: parser.BodyAssertions{
			Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
		},
	})

	result := Execute(context.Background(), req, variable.NewScope(nil), dialer)

	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected buffer warning in result.Warnings, got none")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "buffer") || strings.Contains(w, "Buffer") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want one containing 'buffer'", result.Warnings)
	}
}
