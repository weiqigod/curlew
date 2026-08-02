package websocket

import (
	"errors"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"

	gws "github.com/gorilla/websocket"
)

func TestStartHeartbeat_disabled_noPings(t *testing.T) {
	fc := &fakeConn{}
	cfg := &parser.HeartbeatConfig{Enabled: false}

	stop, errCh := startHeartbeat(t.Context(), fc, cfg)
	defer stop()

	// Channel should be closed immediately (no error sent).
	select {
	case err, ok := <-errCh:
		if ok {
			t.Errorf("disabled heartbeat: expected closed channel, got error: %v", err)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("disabled heartbeat: errCh not closed within 50ms")
	}

	fc.mu.Lock()
	pings := len(fc.controlWrites)
	fc.mu.Unlock()
	if pings != 0 {
		t.Errorf("disabled heartbeat: control writes = %d, want 0", pings)
	}
}

func TestStartHeartbeat_sendsPingsAtInterval(t *testing.T) {
	fc := &fakeConn{}

	cfg := &parser.HeartbeatConfig{
		Enabled:    true,
		IntervalMs: 20, // short interval for test speed
	}

	stop, _ := startHeartbeat(t.Context(), fc, cfg)
	defer stop()

	// Simulate pong for each interval so heartbeat doesn't time out.
	// We need to invoke the pong handler that heartbeat registered.
	// Access via the fakeConn's pong handler hook.
	for i := 0; i < 3; i++ {
		time.Sleep(25 * time.Millisecond)
		fc.mu.Lock()
		handler := fc.pongHandler
		fc.mu.Unlock()
		if handler != nil {
			_ = handler("")
		}
	}

	fc.mu.Lock()
	pings := len(fc.controlWrites)
	fc.mu.Unlock()

	if pings == 0 {
		t.Error("expected at least one ping control frame, got 0")
	}
	// Verify they were ping frames.
	fc.mu.Lock()
	for i, cf := range fc.controlWrites {
		if cf.messageType != gws.PingMessage {
			t.Errorf("controlWrites[%d].messageType = %d, want PingMessage (%d)", i, cf.messageType, gws.PingMessage)
		}
	}
	fc.mu.Unlock()
}

func TestStartHeartbeat_timeoutWhenNoPong(t *testing.T) {
	fc := &fakeConn{}

	cfg := &parser.HeartbeatConfig{
		Enabled:    true,
		IntervalMs: 20, // short for test speed
	}

	_, errCh := startHeartbeat(t.Context(), fc, cfg)

	// Do NOT invoke pong handler — heartbeat should detect timeout.
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrHeartbeatTimeout) {
			t.Errorf("expected ErrHeartbeatTimeout, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout: heartbeat did not send error within 500ms")
	}
}

func TestExecute_heartbeatFailureFailsTest(t *testing.T) {
	// Set a read delay much longer than two heartbeat intervals so that the
	// heartbeat timer fires (and detects no pong) before the expect step reads
	// the message. This makes the outcome deterministic: the test must fail
	// with ErrHeartbeatTimeout.
	//
	// Heartbeat sequence:
	//  t=0:      heartbeat started, pongReceived primed to true
	//  t=10ms:   first tick — pongReceived=true (primed), clears to false, sends ping
	//  t=20ms:   second tick — pongReceived=false (no pong handler invoked), fires ErrHeartbeatTimeout
	//  t=500ms:  readDelay expires (read would happen here, but Execute already returned)
	fc := &fakeConn{
		incoming:  [][]byte{[]byte(`{"type":"ok"}`)},
		readDelay: 500 * time.Millisecond, // much longer than two heartbeat ticks (2×10ms)
	}
	dialer := &fakeDialer{conn: fc}

	req := &parser.Request{
		Protocol: "websocket",
		Method:   "WS",
		URL:      "ws://fake/ws",
		WebSocket: &parser.WebSocketConfig{
			Heartbeat: &parser.HeartbeatConfig{
				Enabled:    true,
				IntervalMs: 10, // 10ms interval → timeout at ~20ms
			},
			Steps: []parser.WebSocketStep{
				{
					Action:    "expect",
					TimeoutMs: 5000,
					ExpectAssertions: parser.BodyAssertions{
						Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
					},
				},
			},
		},
	}

	result := Execute(t.Context(), req, variable.NewScope(nil), dialer)

	if result.Passed {
		t.Error("result.Passed = true, want false: heartbeat should have timed out before message was read")
	}
	if !errors.Is(result.Err, ErrHeartbeatTimeout) {
		t.Errorf("result.Err = %v, want to wrap ErrHeartbeatTimeout", result.Err)
	}
}

// TestExecute_heartbeatRestartedAfterReconnect verifies that when a reconnect
// succeeds, the heartbeat goroutine is restarted on the new connection so that
// the old (closed) connection's heartbeat failure does not produce a spurious
// error after the reconnect.
func TestExecute_heartbeatRestartedAfterReconnect(t *testing.T) {
	// dropConn triggers a reconnect: its ReadMessage returns a non-timeout error.
	// It also does NOT invoke the pong handler, but the heartbeat interval is
	// long enough (200ms) that it won't fire during the 1ms reconnect delay.
	dropConn := &fakeConn{
		readErr: errors.New("connection reset"),
	}
	// successConn returns the expected message immediately.
	successConn := &fakeConn{
		incoming: [][]byte{[]byte(`{"type":"pong"}`)},
	}
	md := &multiDialer{conns: []Conn{dropConn, successConn}}

	req := &parser.Request{
		Protocol: "websocket",
		Method:   "WS",
		URL:      "ws://fake/ws",
		WebSocket: &parser.WebSocketConfig{
			Reconnect: &parser.ReconnectConfig{
				Enabled:        true,
				MaxAttempts:    2,
				InitialDelayMs: 1,
			},
			Heartbeat: &parser.HeartbeatConfig{
				Enabled:    true,
				IntervalMs: 200, // interval long enough not to fire during 1ms reconnect delay
			},
			Steps: []parser.WebSocketStep{
				{
					Action:    "expect",
					TimeoutMs: 2000,
					ExpectAssertions: parser.BodyAssertions{
						Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "pong"}},
					},
				},
			},
		},
	}

	result := Execute(t.Context(), req, variable.NewScope(nil), md)

	// After reconnect, the test should pass — no spurious ErrHeartbeatFailed
	// from the closed connection's goroutine.
	if !result.Passed {
		t.Errorf("result.Passed = false, err = %v: expected success after reconnect with heartbeat", result.Err)
	}
}
