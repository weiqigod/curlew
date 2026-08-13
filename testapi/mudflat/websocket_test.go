package mudflat

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
)

// WebSocket (§9.L).
//
// These tests read mudflat's frames with gorilla/websocket, and that choice is
// the point. The server side is written by hand from RFC 6455 (wsframe.go), so
// gorilla here is a second, independent implementation checking the bytes —
// the same role curl plays for the raw layer in crosscheck.sh. A hand-rolled
// server read by a hand-rolled client would agree with itself and prove
// nothing.
//
// It is also why the endpoints can misbehave at all: gorilla will not send an
// empty continuation frame, decline to answer a ping, or close with a code it
// dislikes, and those are exactly the cases /ws/fragmented, /ws/no-pong and
// /ws/close/{code} exist to produce.

func wsDial(t *testing.T, base, path string, header http.Header) *gws.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(base, "http") + path
	dialer := gws.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, resp, err := dialer.Dial(url, header)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial %s: %v (status %d)", path, err, resp.StatusCode)
		}
		t.Fatalf("dial %s: %v", path, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	return conn
}

func wsReadJSON(t *testing.T, c *gws.Conn) map[string]any {
	t.Helper()
	_, raw, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return out
}

// TestWS_AcceptValueMatchesTheRFCExample pins the handshake against RFC 6455
// §1.3's worked example. Everything else in the family depends on the accept
// value being right, and this is the one place it can be checked against a
// published constant rather than against my own arithmetic.
func TestWS_AcceptValueMatchesTheRFCExample(t *testing.T) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	const want = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := wsAccept(key); got != want {
		t.Errorf("wsAccept(%q) = %q, want %q (RFC 6455 §1.3)", key, got, want)
	}
}

func TestWS_EchoReturnsAnEnvelopePerMessage(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/echo", nil)

	for i := 1; i <= 3; i++ {
		if err := c.WriteMessage(gws.TextMessage, []byte("hello")); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		msg := wsReadJSON(t, c)
		if msg["type"] != "echo" {
			t.Errorf("type = %v, want echo", msg["type"])
		}
		if msg["text"] != "hello" {
			t.Errorf("text = %v, want hello", msg["text"])
		}
		// Distinct sequence numbers are what make `count: 3` honest: three
		// matching messages have three numbers, one matched three times does
		// not.
		if seq, ok := msg["seq"].(float64); !ok || int(seq) != i {
			t.Errorf("seq = %v, want %d", msg["seq"], i)
		}
	}
}

func TestWS_EchoHandlesBinary(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/echo", nil)

	payload := []byte{0x00, 0xFF, 0x10, 0x00}
	if err := c.WriteMessage(gws.BinaryMessage, payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	msg := wsReadJSON(t, c)
	if msg["binary"] != true {
		t.Errorf("binary = %v, want true", msg["binary"])
	}
	if n, ok := msg["bytes"].(float64); !ok || int(n) != len(payload) {
		t.Errorf("bytes = %v, want %d", msg["bytes"], len(payload))
	}
	if msg["text"] != nil {
		t.Errorf("text = %v, want it absent for a binary frame", msg["text"])
	}
}

func TestWS_PushSendsUnsolicitedMessages(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/push?n=4&ms=1", nil)

	for i := 1; i <= 4; i++ {
		msg := wsReadJSON(t, c)
		if msg["type"] != "push" {
			t.Errorf("message %d: type = %v, want push", i, msg["type"])
		}
		if seq, ok := msg["seq"].(float64); !ok || int(seq) != i {
			t.Errorf("message %d: seq = %v", i, msg["seq"])
		}
	}
	// The fifth read must be the close, not a fifth message.
	if _, _, err := c.ReadMessage(); !gws.IsCloseError(err, gws.CloseNormalClosure) {
		t.Errorf("after n messages: err = %v, want a normal close", err)
	}
}

func TestWS_PushRejectsAScheduleThatWouldOutlastTheCeiling(t *testing.T) {
	// §6.4: an endpoint that can stall must be bounded. n × ms is the one place
	// a caller can ask this family to run for a week.
	base, client := startServer(t)
	resp, _ := get(t, client, base+"/ws/push?n=1000&ms=1000")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestWS_FragmentedIsOneMessageInThreeFrames(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/fragmented", nil)

	// gorilla reassembles, so a correct server yields exactly one message that
	// parses. A server that sent three messages would fail the JSON decode.
	msg := wsReadJSON(t, c)
	if msg["type"] != "fragmented" {
		t.Errorf("type = %v, want fragmented", msg["type"])
	}
	if msg["text"] != "three frames, one message" {
		t.Errorf("text = %v", msg["text"])
	}
	if _, _, err := c.ReadMessage(); !gws.IsCloseError(err, gws.CloseNormalClosure) {
		t.Errorf("want exactly one message then a close; got err = %v", err)
	}
}

func TestWS_CloseUsesTheRequestedCode(t *testing.T) {
	for _, code := range []int{1000, 1011, 4000} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			base, _ := startServer(t)
			c := wsDial(t, base, "/ws/close/"+strconv.Itoa(code), nil)

			wsReadJSON(t, c) // the pre-close message
			_, _, err := c.ReadMessage()
			if !gws.IsCloseError(err, code) {
				t.Errorf("err = %v, want close code %d", err, code)
			}
			if ce, ok := err.(*gws.CloseError); ok && !strings.Contains(ce.Text, strconv.Itoa(code)) {
				t.Errorf("close reason %q does not name the code", ce.Text)
			}
		})
	}
}

func TestWS_CloseRejectsACodeOutsideTheProtocolRange(t *testing.T) {
	base, client := startServer(t)
	for _, code := range []string{"999", "5000", "abc"} {
		resp, _ := get(t, client, base+"/ws/close/"+code)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("code %s: status = %d, want 400", code, resp.StatusCode)
		}
	}
}

func TestWS_PingIsAnsweredAndCounted(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/ping?n=2&ms=20", nil)

	// gorilla answers pings from inside ReadMessage, so simply reading is
	// enough to be a well-behaved peer. The count arrives as a data message so
	// a collection can assert on it, and in the close reason for a reader that
	// is already at the end.
	report := wsReadJSON(t, c)
	if report["type"] != "ping-report" {
		t.Fatalf("type = %v, want ping-report", report["type"])
	}
	if n, ok := report["pongs"].(float64); !ok || int(n) != 2 {
		t.Errorf("pongs = %v, want 2 — a well-behaved client answers every ping", report["pongs"])
	}

	_, _, err := c.ReadMessage()
	ce, ok := err.(*gws.CloseError)
	if !ok {
		t.Fatalf("err = %v, want a close carrying the pong count", err)
	}
	if !strings.Contains(ce.Text, "got 2 pongs") {
		t.Errorf("close reason = %q, want it to report 2 pongs", ce.Text)
	}
}

func TestWS_NoPongLeavesThePingUnanswered(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/no-pong?hold_ms=600", nil)

	answered := make(chan struct{}, 1)
	c.SetPongHandler(func(string) error {
		answered <- struct{}{}
		return nil
	})

	msg := wsReadJSON(t, c)
	if msg["type"] != "alive" {
		t.Fatalf("type = %v, want alive — the connection must prove it is up first", msg["type"])
	}
	if err := c.WriteControl(gws.PingMessage, []byte("hi"), time.Now().Add(time.Second)); err != nil {
		t.Fatalf("ping: %v", err)
	}

	_ = c.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	_, _, _ = c.ReadMessage() // drives the pong handler; a timeout is the expected outcome

	select {
	case <-answered:
		t.Error("the ping was answered; this endpoint must never pong")
	default:
	}
}

func TestWS_SubprotocolNegotiatesThePreferredOne(t *testing.T) {
	base, _ := startServer(t)
	h := http.Header{}
	h.Set("Sec-WebSocket-Protocol", "mudflat.v1, mudflat.v2")
	c := wsDial(t, base, "/ws/subprotocol", h)

	// Server preference order wins, not the client's: v2 is listed first
	// server-side and second by the client.
	if got := c.Subprotocol(); got != "mudflat.v2" {
		t.Errorf("negotiated %q, want mudflat.v2", got)
	}
	msg := wsReadJSON(t, c)
	if msg["subprotocol"] != "mudflat.v2" {
		t.Errorf("body reports %v", msg["subprotocol"])
	}
}

func TestWS_SubprotocolRejectsAnUnofferedOne(t *testing.T) {
	base, _ := startServer(t)
	url := "ws" + strings.TrimPrefix(base, "http") + "/ws/subprotocol"
	h := http.Header{}
	h.Set("Sec-WebSocket-Protocol", "something.else")

	dialer := gws.Dialer{HandshakeTimeout: 5 * time.Second}
	_, resp, err := dialer.Dial(url, h)
	if err == nil {
		t.Fatal("dial succeeded; an unsupported subprotocol must be refused")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %v, want 400", resp)
	}
}

func TestWS_RejectRefusesTheUpgradeWithABody(t *testing.T) {
	base, client := startServer(t)
	resp, body := get(t, client, base+"/ws/reject")
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Errorf("status = %d, want 426", resp.StatusCode)
	}
	if !strings.Contains(string(body), "refuses every upgrade") {
		t.Errorf("body does not explain the refusal: %s", body)
	}
}

func TestWS_SlowAcceptDelaysThe101(t *testing.T) {
	base, _ := startServer(t)
	start := time.Now()
	c := wsDial(t, base, "/ws/slow-accept?ms=300", nil)
	elapsed := time.Since(start)

	if elapsed < 250*time.Millisecond {
		t.Errorf("handshake took %s, want at least ~300ms", elapsed)
	}
	msg := wsReadJSON(t, c)
	if msg["type"] != "late" {
		t.Errorf("type = %v, want late", msg["type"])
	}
}

func TestWS_RejectsANonUpgradeRequest(t *testing.T) {
	base, client := startServer(t)
	resp, body := get(t, client, base+"/ws/echo")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (%s)", resp.StatusCode, body)
	}
}

// TestWS_EchoAnswersAClientPing pins the server side of the heartbeat contract.
// RFC 6455 §5.5.2 requires a pong for every ping, and a mudflat that quietly
// failed to send one would make every client-side heartbeat finding
// unattributable — the server would be faking the failure it is meant to
// measure.
func TestWS_EchoAnswersAClientPing(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/echo", http.Header{})

	got := make(chan string, 8)
	c.SetPongHandler(func(data string) error { got <- data; return nil })

	// All pings first. A gorilla read error is permanent, so the connection
	// must be read exactly once, at the end.
	for range 3 {
		if err := c.WriteControl(gws.PingMessage, []byte("beat"), time.Now().Add(time.Second)); err != nil {
			t.Fatalf("ping: %v", err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	_ = c.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	_, _, _ = c.ReadMessage() // drives the pong handler; the timeout is expected

	if n := len(got); n != 3 {
		t.Errorf("got %d pongs, want 3", n)
	}
}

// Does /ws/no-pong actually hold the connection open for hold_ms? A dead peer
// that drops the socket is a different failure from one that keeps it.
func TestWS_NoPongHoldsTheConnectionOpen(t *testing.T) {
	base, _ := startServer(t)
	c := wsDial(t, base, "/ws/no-pong?hold_ms=1500", http.Header{})

	start := time.Now()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	wsReadJSON(t, c) // "alive"

	// Nothing more should arrive, and the socket must not close early.
	_, _, err := c.ReadMessage()
	held := time.Since(start)
	t.Logf("connection ended after %s with %v", held, err)
	if held < 1400*time.Millisecond {
		t.Errorf("connection ended after %s, want it held ~1500ms", held)
	}
}
