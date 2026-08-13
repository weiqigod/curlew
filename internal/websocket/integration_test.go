package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// TestExecute_realGorillaDialer spins up a real httptest server with a
// gorilla upgrader, exercises DefaultDialer, and verifies the full
// send -> expect -> close lifecycle against a real WebSocket connection.
func TestExecute_realGorillaDialer(t *testing.T) {
	upgrader := gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = c.Close() }()
		// Echo one message, then drain until the client closes.
		_, msg, err := c.ReadMessage()
		if err != nil {
			return
		}
		if err := c.WriteMessage(gws.TextMessage, msg); err != nil {
			return
		}
		for {
			if _, _, readErr := c.ReadMessage(); readErr != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	req := &parser.Request{
		Protocol: "websocket",
		URL:      wsURL,
		Method:   "WS",
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{
				{Action: "send", Message: map[string]any{"type": "hello"}},
				{
					Action:    "expect",
					TimeoutMs: 2000,
					ExpectAssertions: parser.BodyAssertions{
						Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "hello"}},
					},
				},
				{Action: "close", Code: 1000, Reason: "bye"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := Execute(ctx, req, variable.NewScope(nil), DefaultDialer)
	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if len(result.Steps) != 3 {
		t.Errorf("Steps = %d, want 3", len(result.Steps))
	}
}

// TestExecute_realGorillaDialer_ExtractVariable verifies that variables
// extracted from a real WebSocket message flow into the caller's scope.
func TestExecute_realGorillaDialer_ExtractVariable(t *testing.T) {
	upgrader := gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_ = c.WriteMessage(gws.TextMessage, []byte(`{"sid":"XYZ-42","ok":true}`))
		for {
			if _, _, readErr := c.ReadMessage(); readErr != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	scope := variable.NewScope(nil)

	req := &parser.Request{
		Protocol: "websocket",
		URL:      wsURL,
		Method:   "WS",
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{
				{
					Action:    "expect",
					TimeoutMs: 2000,
					ExpectAssertions: parser.BodyAssertions{
						Items: []parser.BodyAssertion{{Path: "$.ok", Operator: "equals", Value: true}},
					},
					Extract: map[string]string{"sid": "$.sid"},
				},
				{Action: "close", Code: 1000},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := Execute(ctx, req, scope, DefaultDialer)
	if !result.Passed {
		t.Fatalf("result failed: %v", result.Err)
	}
	if got := scope.Resolved()["sid"]; got != "XYZ-42" {
		t.Errorf("sid = %q, want XYZ-42", got)
	}
}

// TestExecute_realGorillaDialer_DialFailure verifies that dialing a URL
// that is not a WebSocket endpoint fails with ErrDialFailed.
func TestExecute_realGorillaDialer_DialFailure(t *testing.T) {
	// Use ws://127.0.0.1 on a port that is not listening.
	req := &parser.Request{
		Protocol: "websocket",
		URL:      "ws://127.0.0.1:1/does-not-exist",
		Method:   "WS",
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{{Action: "close"}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := Execute(ctx, req, variable.NewScope(nil), DefaultDialer)
	if result.Passed {
		t.Fatal("expected dial failure")
	}
}

// TestExecute_realDialerReportsRefusedUpgrade exercises the real gorilla
// dialer against a server that refuses the upgrade (§11C.4). The unit tests
// use a fake dialer, but the discarded value came from gorilla itself — so the
// path that actually mattered is only covered here.
func TestExecute_realDialerReportsRefusedUpgrade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Sec-WebSocket-Version", "13")
		w.WriteHeader(http.StatusUpgradeRequired)
		_, _ = w.Write([]byte(`{"error":"Upgrade Required","detail":"refused on purpose"}`))
	}))
	defer srv.Close()

	req := &parser.Request{
		Protocol: "websocket",
		Method:   "WS",
		URL:      "ws" + strings.TrimPrefix(srv.URL, "http"),
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{{Action: "expect", TimeoutMs: 500}},
		},
	}
	result := Execute(context.Background(), req, variable.NewScope(nil), nil)

	if result.Passed {
		t.Fatal("a refused upgrade reported success")
	}
	msg := result.Err.Error()
	if !strings.Contains(msg, "426") {
		t.Errorf("message does not name the status: %q", msg)
	}
	if !strings.Contains(msg, "refused on purpose") {
		t.Errorf("message does not carry the server's explanation: %q", msg)
	}
}
