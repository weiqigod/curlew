package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

func TestReconnectState_nextDelay_exponential(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		initMs  int
		wantMs  int
	}{
		{"first attempt = initial", 0, 1000, 1000},
		{"second attempt = 2x", 1, 1000, 2000},
		{"third attempt = 4x", 2, 1000, 4000},
		{"zero initial defaults to 1s", 0, 0, 1000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &reconnectState{
				cfg: &parser.ReconnectConfig{
					Enabled:        true,
					InitialDelayMs: tc.initMs,
				},
				attempts: tc.attempt,
			}
			got := r.nextDelay()
			want := time.Duration(tc.wantMs) * time.Millisecond
			if got != want {
				t.Errorf("nextDelay() = %v, want %v", got, want)
			}
		})
	}
}

func TestReconnectState_shouldReconnect(t *testing.T) {
	netErr := errors.New("connection reset by peer")

	t.Run("nil state returns false", func(t *testing.T) {
		var r *reconnectState
		if r.shouldReconnect(netErr) {
			t.Error("nil state: shouldReconnect = true, want false")
		}
	})

	t.Run("disabled returns false", func(t *testing.T) {
		r := &reconnectState{cfg: &parser.ReconnectConfig{Enabled: false}}
		if r.shouldReconnect(netErr) {
			t.Error("disabled: shouldReconnect = true, want false")
		}
	})

	t.Run("returns true on connection error within max attempts", func(t *testing.T) {
		r := &reconnectState{cfg: &parser.ReconnectConfig{Enabled: true, MaxAttempts: 3}}
		if !r.shouldReconnect(netErr) {
			t.Error("shouldReconnect = false, want true")
		}
	})

	t.Run("returns false when attempts exhausted", func(t *testing.T) {
		r := &reconnectState{
			cfg:      &parser.ReconnectConfig{Enabled: true, MaxAttempts: 2},
			attempts: 2,
		}
		if r.shouldReconnect(netErr) {
			t.Error("shouldReconnect = true after exhaustion, want false")
		}
	})

	t.Run("timeout error returns false", func(t *testing.T) {
		r := &reconnectState{cfg: &parser.ReconnectConfig{Enabled: true, MaxAttempts: 3}}
		if r.shouldReconnect(&timeoutError{}) {
			t.Error("timeout: shouldReconnect = true, want false")
		}
	})

	t.Run("nil error returns false", func(t *testing.T) {
		r := &reconnectState{cfg: &parser.ReconnectConfig{Enabled: true, MaxAttempts: 3}}
		if r.shouldReconnect(nil) {
			t.Error("nil err: shouldReconnect = true, want false")
		}
	})

	t.Run("context canceled error returns false", func(t *testing.T) {
		r := &reconnectState{cfg: &parser.ReconnectConfig{Enabled: true, MaxAttempts: 3}}
		cancelErr := fmt.Errorf("%w: %w", ErrContextCanceled, context.Canceled)
		if r.shouldReconnect(cancelErr) {
			t.Error("context canceled: shouldReconnect = true, want false")
		}
	})

	t.Run("context deadline exceeded error returns false", func(t *testing.T) {
		r := &reconnectState{cfg: &parser.ReconnectConfig{Enabled: true, MaxAttempts: 3}}
		deadlineErr := fmt.Errorf("%w: %w", ErrContextCanceled, context.DeadlineExceeded)
		if r.shouldReconnect(deadlineErr) {
			t.Error("context deadline: shouldReconnect = true, want false")
		}
	})
}

func TestExecute_reconnect_succeedsAfterDrop(t *testing.T) {
	// First connection returns a drop error on ReadMessage.
	// Second connection (after reconnect) returns the expected frame.
	dropConn := &fakeConn{
		readErr: errors.New("connection reset"),
	}
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
				MaxAttempts:    3,
				InitialDelayMs: 1,
			},
			Steps: []parser.WebSocketStep{
				{
					Action:  "send",
					Message: map[string]any{"type": "ping"},
				},
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

	if !result.Passed {
		t.Errorf("result.Passed = false, err = %v", result.Err)
	}
}

func TestExecute_reconnect_exhaustsAttempts(t *testing.T) {
	// All connections produce a drop error.
	md := &multiDialer{conns: []Conn{
		&fakeConn{readErr: errors.New("drop 1")},
		&fakeConn{readErr: errors.New("drop 2")},
		&fakeConn{readErr: errors.New("drop 3")},
		&fakeConn{readErr: errors.New("drop 4")},
	}}

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
			Steps: []parser.WebSocketStep{
				{
					Action:    "expect",
					TimeoutMs: 500,
					ExpectAssertions: parser.BodyAssertions{
						Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "pong"}},
					},
				},
			},
		},
	}

	result := Execute(t.Context(), req, variable.NewScope(nil), md)

	if result.Passed {
		t.Error("result.Passed = true, want false after exhausting reconnect attempts")
	}
	if !errors.Is(result.Err, ErrReconnectExhausted) {
		t.Errorf("result.Err = %v, want to wrap ErrReconnectExhausted", result.Err)
	}
}

func TestExecute_reconnect_disabledLeavesBehaviourUnchanged(t *testing.T) {
	dropConn := &fakeConn{
		readErr: errors.New("connection reset"),
	}
	md := &multiDialer{conns: []Conn{dropConn}}

	req := &parser.Request{
		Protocol: "websocket",
		Method:   "WS",
		URL:      "ws://fake/ws",
		WebSocket: &parser.WebSocketConfig{
			// No Reconnect set (disabled by default).
			Steps: []parser.WebSocketStep{
				{
					Action:    "expect",
					TimeoutMs: 500,
					ExpectAssertions: parser.BodyAssertions{
						Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "pong"}},
					},
				},
			},
		},
	}

	result := Execute(t.Context(), req, variable.NewScope(nil), md)

	if result.Passed {
		t.Error("result.Passed = true, want false on read error without reconnect")
	}
	if errors.Is(result.Err, ErrReconnectExhausted) {
		t.Error("result.Err wraps ErrReconnectExhausted, but reconnect was disabled")
	}
}

// multiDialer returns conns in sequence; each call to Dial returns the next conn.
type multiDialer struct {
	conns []Conn
	idx   int
}

func (m *multiDialer) Dial(_ context.Context, _ string, _ http.Header) (Conn, *http.Response, error) {
	if m.idx >= len(m.conns) {
		return nil, nil, errors.New("no more connections")
	}
	c := m.conns[m.idx]
	m.idx++
	return c, nil, nil
}
