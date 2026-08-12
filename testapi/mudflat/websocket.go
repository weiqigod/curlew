package mudflat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// WebSocket endpoints (§9.L).
//
// Every one of these holds the connection for the life of the session, so every
// one sets a deadline: §6.4 caps any endpoint that can stall at 120 s, and a
// WebSocket that waits forever is the easiest way to wedge CI rather than fail
// it.
//
// The frame layer is in wsframe.go, written from RFC 6455 rather than taken
// from gorilla — see the note there.

const (
	wsDefaultDeadline = 30 * time.Second
	wsMaxPush         = 1000
)

func (s *Server) registerWebSocket() {
	s.register(Endpoint{
		Pattern:   "/ws/echo",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Echoes every text and binary message, wrapped in an envelope.",
		Exercises: "The send and expect step actions, JSONPath assertions over a received message, and extract: from one. The envelope numbers each message, so `count: 3` satisfied by one message matching three times is visible as three identical sequence numbers.",
		Handler:   s.handleWSEcho,
	})

	s.register(Endpoint{
		Pattern:   "/ws/push",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Pushes n server-initiated messages, ms apart, without being asked.",
		Exercises: "expect with count: and timeout_ms: against messages the client did not solicit — a subscription, which is what WebSocket testing is mostly for.",
		Handler:   s.handleWSPush,
	})

	s.register(Endpoint{
		Pattern:   "/ws/ping",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Sends protocol pings every ms, then reports how many pongs came back.",
		Exercises: "The client half of the heartbeat contract: does curlew answer a protocol ping? The count arrives as a data message, because a collection can assert on a message and cannot assert on a close reason.",
		Handler:   s.handleWSPing,
	})

	s.register(Endpoint{
		Pattern:   "/ws/no-pong",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Reads pings and never answers them; the connection stays open.",
		Exercises: "The reconnect path and heartbeat timeouts. A dead peer that still holds the socket open is the failure a heartbeat exists to detect, and it cannot be simulated by closing the connection.",
		Handler:   s.handleWSNoPong,
	})

	s.register(Endpoint{
		Pattern:   "/ws/close/{code}",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Closes with the requested code after one message.",
		Exercises: "Whether a close code reaches the user. 1000 is normal; 1011 and 4000 are not, and a client that reports them all as 'connection closed' has lost the only diagnostic the protocol offers.",
		Handler:   s.handleWSClose,
	})

	s.register(Endpoint{
		Pattern:   "/ws/fragmented",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Sends one JSON message split across three continuation frames.",
		Exercises: "Message reassembly. Each fragment is invalid JSON alone, so a client that treats a frame as a message fails to parse rather than quietly asserting on a fragment.",
		Handler:   s.handleWSFragmented,
	})

	s.register(Endpoint{
		Pattern:   "/ws/subprotocol",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Negotiates mudflat.v2 or mudflat.v1, and rejects a client offering neither.",
		Exercises: "Sec-WebSocket-Protocol negotiation, and whether curlew can send the header at all — the dialer takes headers from the request, so this proves they survive the upgrade.",
		Handler:   s.handleWSSubprotocol,
	})

	s.register(Endpoint{
		Pattern:   "/ws/reject",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Refuses the upgrade with 426 and a JSON body.",
		Exercises: "Whether a refused upgrade produces a legible error naming the status, rather than a bare dial failure. The body explains why, and a client that discards the response cannot show it.",
		Handler:   s.handleWSReject,
	})

	s.register(Endpoint{
		Pattern:   "/ws/slow-accept",
		Methods:   []string{http.MethodGet},
		Family:    "L",
		Summary:   "Delays the 101 by ms, leaving the client waiting on the handshake.",
		Exercises: "Handshake timeouts, which are a different code path from read timeouts: the connection is established and the request sent, and only the upgrade response is missing.",
		Handler:   s.handleWSSlowAccept,
	})
}

// wsEnvelope is what the echo and push endpoints send. Sequence numbering is
// what makes `count: 3` honest — three matching messages have three different
// numbers, one message matched three times does not.
type wsEnvelope struct {
	Type     string `json:"type"`
	Seq      int    `json:"seq"`
	Text     string `json:"text,omitempty"`
	Binary   bool   `json:"binary"`
	Bytes    int    `json:"bytes"`
	Endpoint string `json:"endpoint"`
}

func (s *Server) handleWSEcho(w http.ResponseWriter, r *http.Request) {
	c := wsUpgrade(w, r, wsHandshakeOptions{})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	c.SetDeadline(time.Now().Add(wsDefaultDeadline))
	for seq := 1; ; seq++ {
		msg, err := c.ReadMessage(true)
		if err != nil {
			// A client close is the ordinary end of this endpoint.
			if msg.Opcode == opClose {
				_ = c.WriteClose(1000, "echoed")
			}
			return
		}
		env := wsEnvelope{
			Type:     "echo",
			Seq:      seq,
			Binary:   msg.Opcode == opBinary,
			Bytes:    len(msg.Payload),
			Endpoint: "/ws/echo",
		}
		if msg.Opcode == opText {
			env.Text = string(msg.Payload)
		}
		if err := c.WriteText(mustJSON(env)); err != nil {
			return
		}
	}
}

func (s *Server) handleWSPush(w http.ResponseWriter, r *http.Request) {
	n, err := intParam(r, "n", 3, 1, wsMaxPush)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	gap, err := intParam(r, "ms", 10, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	// n messages gap apart must fit inside the ceiling, or the endpoint becomes
	// the thing that wedges CI.
	if total := time.Duration(n) * time.Duration(gap) * time.Millisecond; total > hardResponseCeiling {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf(
			"n=%d at ms=%d would run for %s, past the %s ceiling", n, gap, total, hardResponseCeiling))
		return
	}

	c := wsUpgrade(w, r, wsHandshakeOptions{})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	defer c.StartControlPump()()
	c.SetDeadline(time.Now().Add(hardResponseCeiling))
	for i := 1; i <= n; i++ {
		if gap > 0 {
			// Not r.Context(): see the note in handleWSNoPong. The write below
			// is what detects a client that has gone away.
			time.Sleep(time.Duration(gap) * time.Millisecond)
		}
		env := wsEnvelope{Type: "push", Seq: i, Bytes: 0, Endpoint: "/ws/push"}
		env.Text = fmt.Sprintf("push %d of %d", i, n)
		if err := c.WriteText(mustJSON(env)); err != nil {
			return
		}
	}
	_ = c.WriteClose(1000, fmt.Sprintf("pushed %d", n))
}

func (s *Server) handleWSPing(w http.ResponseWriter, r *http.Request) {
	gap, err := intParam(r, "ms", 100, 1, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := intParam(r, "n", 3, 1, 100)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	c := wsUpgrade(w, r, wsHandshakeOptions{})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	pongs := make(chan struct{}, count)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			f, readErr := c.ReadFrame()
			if readErr != nil {
				return
			}
			if f.Opcode == opPong {
				select {
				case pongs <- struct{}{}:
				default:
				}
			}
			if f.Opcode == opClose {
				return
			}
		}
	}()

	c.SetDeadline(time.Now().Add(hardResponseCeiling))
	for i := range count {
		if err := c.WriteFrame(opPing, true, []byte(fmt.Sprintf("mudflat-ping-%d", i+1))); err != nil {
			return
		}
		time.Sleep(time.Duration(gap) * time.Millisecond)
	}

	// Wait for the outstanding pongs rather than sampling immediately. This is
	// a rendezvous, not a sleep: it returns as soon as the client has answered,
	// and reports the shortfall when it never does. Sampling right after the
	// last ping would make the count depend on scheduling, which §6.1 forbids.
	deadline := time.Now().Add(2 * time.Second)
	for len(pongs) < count && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := len(pongs)

	// The count goes into a data message as well as the close reason. A
	// collection can assert on a message and cannot assert on a close reason,
	// and this is the one place the client half of the heartbeat contract —
	// does curlew answer a protocol ping? — is observable at all.
	_ = c.WriteText(mustJSON(map[string]any{
		"type":     "ping-report",
		"seq":      1,
		"pings":    count,
		"pongs":    got,
		"endpoint": "/ws/ping",
	}))
	_ = c.WriteClose(1000, fmt.Sprintf("sent %d pings, got %d pongs", count, got))
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

func (s *Server) handleWSNoPong(w http.ResponseWriter, r *http.Request) {
	hold, err := intParam(r, "hold_ms", 2000, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	c := wsUpgrade(w, r, wsHandshakeOptions{})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	// One message first, so the client knows the connection is live. Then
	// nothing: pings are read and never answered, which is a peer that has
	// stopped responding without dropping the socket — the failure a heartbeat
	// exists to detect, and one that closing the connection cannot simulate.
	c.SetDeadline(time.Now().Add(hardResponseCeiling))
	if err := c.WriteText(mustJSON(wsEnvelope{
		Type: "alive", Seq: 1, Endpoint: "/ws/no-pong",
		Text: "this connection will not answer a ping",
	})); err != nil {
		return
	}

	// r.Context() is deliberately not consulted here. Once the connection is
	// hijacked it belongs to this handler, and net/http cancels the request
	// context out from under it — an earlier version of this loop exited after
	// one read cycle instead of holding for hold_ms, which made the endpoint
	// simulate a peer that DROPS the socket rather than one that keeps it open
	// and stops answering. Those are different failures, and this endpoint
	// exists for the second.
	deadline := time.After(time.Duration(hold) * time.Millisecond)
	for {
		select {
		case <-deadline:
			return
		default:
		}
		c.SetDeadline(time.Now().Add(100 * time.Millisecond))
		// A read timeout is the expected outcome: nothing may ever arrive.
		_, _ = c.ReadMessage(false)
	}
}

func (s *Server) handleWSClose(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(r.PathValue("code"))
	if err != nil || code < 1000 || code > 4999 {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("code", r.PathValue("code"),
				"expected a WebSocket close code between 1000 and 4999 (RFC 6455 §7.4)").Error())
		return
	}

	c := wsUpgrade(w, r, wsHandshakeOptions{})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	defer c.StartControlPump()()
	c.SetDeadline(time.Now().Add(wsDefaultDeadline))
	if err := c.WriteText(mustJSON(wsEnvelope{
		Type: "closing", Seq: 1, Endpoint: "/ws/close",
		Text: fmt.Sprintf("closing with %d", code),
	})); err != nil {
		return
	}
	_ = c.WriteClose(code, fmt.Sprintf("mudflat closing with %d", code))
}

func (s *Server) handleWSFragmented(w http.ResponseWriter, r *http.Request) {
	c := wsUpgrade(w, r, wsHandshakeOptions{})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	defer c.StartControlPump()()
	c.SetDeadline(time.Now().Add(wsDefaultDeadline))

	// One message, three frames. Each fragment is invalid JSON on its own, so a
	// client that mistakes a frame for a message fails to parse rather than
	// silently asserting against a third of the document.
	fragments := []string{
		`{"type":"fragmented","seq":1,`,
		`"text":"three frames, one message",`,
		`"endpoint":"/ws/fragmented","binary":false,"bytes":0}`,
	}
	if err := c.WriteFrame(opText, false, []byte(fragments[0])); err != nil {
		return
	}
	if err := c.WriteFrame(opContinuation, false, []byte(fragments[1])); err != nil {
		return
	}
	if err := c.WriteFrame(opContinuation, true, []byte(fragments[2])); err != nil {
		return
	}
	_ = c.WriteClose(1000, "sent one message in three frames")
}

func (s *Server) handleWSSubprotocol(w http.ResponseWriter, r *http.Request) {
	c := wsUpgrade(w, r, wsHandshakeOptions{
		Subprotocols:       []string{"mudflat.v2", "mudflat.v1"},
		RequireSubprotocol: true,
	})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	defer c.StartControlPump()()
	c.SetDeadline(time.Now().Add(wsDefaultDeadline))
	// The negotiated protocol comes back in the body as well as the handshake
	// header, because a collection can assert on a message and cannot assert on
	// a handshake header.
	_ = c.WriteText(mustJSON(map[string]any{
		"type":        "negotiated",
		"seq":         1,
		"subprotocol": c.Subprotocol,
		"endpoint":    "/ws/subprotocol",
	}))
	_ = c.WriteClose(1000, "negotiated "+c.Subprotocol)
}

func (s *Server) handleWSReject(w http.ResponseWriter, _ *http.Request) {
	// 426 with a body that says why. RFC 6455 §4.2.2 allows any non-101
	// response; the interesting question is whether the client shows it.
	w.Header().Set("Sec-WebSocket-Version", "13")
	writeJSON(w, http.StatusUpgradeRequired, map[string]any{
		"error":  "Upgrade Required",
		"detail": "this endpoint refuses every upgrade on purpose",
		"note":   "a client should report the 426 and this body, not a bare dial failure",
	})
}

func (s *Server) handleWSSlowAccept(w http.ResponseWriter, r *http.Request) {
	delay, err := intParam(r, "ms", 500, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	c := wsUpgrade(w, r, wsHandshakeOptions{Delay: time.Duration(delay) * time.Millisecond})
	if c == nil {
		return
	}
	defer func() { _ = c.Close() }()

	defer c.StartControlPump()()
	c.SetDeadline(time.Now().Add(wsDefaultDeadline))
	_ = c.WriteText(mustJSON(wsEnvelope{
		Type: "late", Seq: 1, Endpoint: "/ws/slow-accept",
		Text: fmt.Sprintf("handshake was delayed %dms", delay),
	}))
	_ = c.WriteClose(1000, "late but present")
}

// mustJSON marshals an envelope. The values are all server-controlled, so a
// failure here is a mudflat bug and the string says so rather than hiding it.
func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return `{"type":"mudflat-error","text":"marshal failed"}`
	}
	return string(raw)
}
