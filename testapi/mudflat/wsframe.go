package mudflat

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // RFC 6455 §4.2.2 specifies SHA-1 for the handshake accept value
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The WebSocket wire layer, written from RFC 6455 rather than taken from a
// library (§9.L, and the same argument as §5.2's raw layer).
//
// curlew's WebSocket client is gorilla/websocket. A mudflat that also used
// gorilla would agree with curlew by construction about masking, fragmentation,
// control-frame rules and close-code encoding — every bug in the shared reading
// invisible, which is §3's closed-loop problem wearing a different hat. So the
// frames here are assembled by hand, and the package's own tests read them with
// gorilla: a second implementation checking the bytes, exactly as curl checks
// the raw layer.
//
// It also buys the endpoints that matter. A library will not send a
// continuation frame with no data, answer nothing to a ping, or close with a
// code it considers invalid — and those are the cases /ws/fragmented,
// /ws/no-pong and /ws/close/{code} exist to produce.

// wsMagic is the GUID from RFC 6455 §1.3, concatenated with the client key
// before hashing.
const wsMagic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket opcodes (RFC 6455 §5.2).
const (
	opContinuation byte = 0x0
	opText         byte = 0x1
	opBinary       byte = 0x2
	opClose        byte = 0x8
	opPing         byte = 0x9
	opPong         byte = 0xA
)

// wsMaxPayload bounds a single inbound frame. mudflat is a test server on
// loopback; a frame larger than this is a client defect, not a workload.
const wsMaxPayload = 1 << 20

var errWSClosed = errors.New("websocket closed")

// wsFrame is one decoded frame.
type wsFrame struct {
	FIN     bool
	Opcode  byte
	Payload []byte
}

// wsConn is a hijacked connection carrying WebSocket frames.
type wsConn struct {
	conn    net.Conn
	br      *bufio.Reader
	writeMu sync.Mutex // serialize complete frames, including control-pump replies

	// Subprotocol is what the handshake negotiated, empty when none.
	Subprotocol string
}

// wsAccept computes the Sec-WebSocket-Accept value for a client key
// (RFC 6455 §4.2.2, step 5).
func wsAccept(key string) string {
	sum := sha1.Sum([]byte(key + wsMagic)) //nolint:gosec // specified by the RFC
	return base64.StdEncoding.EncodeToString(sum[:])
}

// wsHandshakeOptions tunes the upgrade for the endpoints that need it wrong.
type wsHandshakeOptions struct {
	// Subprotocols the server is willing to speak, in preference order. Empty
	// means the server offers none and answers without the header.
	Subprotocols []string

	// RequireSubprotocol rejects a client that offers none of the above.
	RequireSubprotocol bool

	// Delay pauses before the 101 is written — after the request is fully read,
	// so the client is left waiting on the handshake rather than on the accept.
	Delay time.Duration
}

// wsUpgrade validates the handshake and takes over the connection. It writes
// the error response itself and returns nil when the request is not a valid
// upgrade, so callers can simply return.
func wsUpgrade(w http.ResponseWriter, r *http.Request, opts wsHandshakeOptions) *wsConn {
	if !headerContainsToken(r.Header, "Connection", "upgrade") ||
		!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		writeProblem(w, http.StatusBadRequest,
			"not a WebSocket upgrade: RFC 6455 §4.1 requires Connection: Upgrade and Upgrade: websocket")
		return nil
	}
	if v := r.Header.Get("Sec-WebSocket-Version"); v != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		writeProblem(w, http.StatusUpgradeRequired,
			fmt.Sprintf("unsupported WebSocket version %q; this server speaks 13", v))
		return nil
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		writeProblem(w, http.StatusBadRequest, "no Sec-WebSocket-Key")
		return nil
	}

	chosen := ""
	if len(opts.Subprotocols) > 0 {
		offered := splitHeaderList(r.Header.Values("Sec-WebSocket-Protocol"))
		for _, want := range opts.Subprotocols {
			for _, have := range offered {
				if strings.EqualFold(want, have) {
					chosen = want
					break
				}
			}
			if chosen != "" {
				break
			}
		}
		if chosen == "" && opts.RequireSubprotocol {
			writeProblem(w, http.StatusBadRequest, fmt.Sprintf(
				"none of the offered subprotocols %v is supported; this endpoint speaks %v",
				offered, opts.Subprotocols))
			return nil
		}
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "connection cannot be hijacked")
		return nil
	}

	if opts.Delay > 0 {
		// The request has been read in full; the client is now blocked waiting
		// for the 101 that has not been written yet.
		select {
		case <-time.After(opts.Delay):
		case <-r.Context().Done():
			return nil
		}
	}

	conn, brw, err := hj.Hijack()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "hijack failed: "+err.Error())
		return nil
	}

	var resp strings.Builder
	resp.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	resp.WriteString("Upgrade: websocket\r\n")
	resp.WriteString("Connection: Upgrade\r\n")
	resp.WriteString("Sec-WebSocket-Accept: " + wsAccept(key) + "\r\n")
	if chosen != "" {
		resp.WriteString("Sec-WebSocket-Protocol: " + chosen + "\r\n")
	}
	resp.WriteString("\r\n")
	if _, err := conn.Write([]byte(resp.String())); err != nil {
		_ = conn.Close()
		return nil
	}

	return &wsConn{conn: conn, br: brw.Reader, Subprotocol: chosen}
}

// headerContainsToken reports whether a comma-separated header carries a token.
func headerContainsToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// splitHeaderList flattens a repeated, comma-separated header into its tokens.
func splitHeaderList(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if t := strings.TrimSpace(part); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

// Close closes the underlying connection.
func (c *wsConn) Close() error { return c.conn.Close() }

// SetDeadline bounds a read or write. Every endpoint sets one: §6.4 caps any
// endpoint that can stall at 120 s so a client-side regression fails CI rather
// than wedging it.
func (c *wsConn) SetDeadline(t time.Time) { _ = c.conn.SetDeadline(t) }

// ReadFrame decodes one frame. Client-to-server frames must be masked
// (RFC 6455 §5.1); an unmasked one is a protocol error and says so.
func (c *wsConn) ReadFrame() (wsFrame, error) {
	var head [2]byte
	if _, err := io.ReadFull(c.br, head[:]); err != nil {
		return wsFrame{}, err
	}

	f := wsFrame{
		FIN:    head[0]&0x80 != 0,
		Opcode: head[0] & 0x0F,
	}
	masked := head[1]&0x80 != 0
	length := uint64(head[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > wsMaxPayload {
		return wsFrame{}, fmt.Errorf("frame payload %d exceeds the %d byte limit", length, wsMaxPayload)
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return wsFrame{}, err
		}
	} else {
		return wsFrame{}, errors.New("client frame is not masked; RFC 6455 §5.1 requires it")
	}

	f.Payload = make([]byte, length)
	if _, err := io.ReadFull(c.br, f.Payload); err != nil {
		return wsFrame{}, err
	}
	for i := range f.Payload {
		f.Payload[i] ^= mask[i%4]
	}
	return f, nil
}

// WriteFrame emits one frame. Server-to-client frames are never masked
// (RFC 6455 §5.1), so no mask bit is set here at all.
func (c *wsConn) WriteFrame(opcode byte, fin bool, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	var head []byte
	first := opcode
	if fin {
		first |= 0x80
	}
	head = append(head, first)

	switch n := len(payload); {
	case n <= 125:
		head = append(head, byte(n))
	case n <= 0xFFFF:
		head = append(head, 126, byte(n>>8), byte(n))
	default:
		head = append(head, 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		head = append(head, ext[:]...)
	}

	if _, err := c.conn.Write(head); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := c.conn.Write(payload)
	return err
}

// WriteText sends a complete text message.
func (c *wsConn) WriteText(s string) error { return c.WriteFrame(opText, true, []byte(s)) }

// WriteClose sends a close frame carrying a code and reason (RFC 6455 §5.5.1:
// a 2-byte big-endian code, then UTF-8 text).
func (c *wsConn) WriteClose(code int, reason string) error {
	if code == 0 {
		// A close frame with no payload at all is legal and means "no status".
		return c.WriteFrame(opClose, true, nil)
	}
	payload := make([]byte, 2, 2+len(reason))
	binary.BigEndian.PutUint16(payload, uint16(code)) //nolint:gosec // codes are bounded by the caller
	payload = append(payload, reason...)
	return c.WriteFrame(opClose, true, payload)
}

// StartControlPump answers pings in the background and returns a stop
// function.
//
// It exists because RFC 6455 §5.5.2 requires a pong in response to every ping,
// which means a server that only ever writes still has to read. Without it,
// /ws/push and friends look to a client exactly like a peer that has died —
// and a mudflat that fakes that failure by accident cannot be used to test a
// client's handling of the real one.
//
// Only for endpoints that never read in the foreground: two readers on one
// connection would interleave mid-frame and corrupt the stream.
func (c *wsConn) StartControlPump() (stop func()) {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			// ReadFrame answers nothing itself; the pong is written here.
			f, err := c.ReadFrame()
			if err != nil {
				return
			}
			switch f.Opcode {
			case opPing:
				if err := c.WriteFrame(opPong, true, f.Payload); err != nil {
					return
				}
			case opClose:
				return
			}
		}
	}()
	return func() { close(done) }
}

// ReadMessage reads one application message, answering control frames as the
// protocol requires and reassembling continuation frames.
//
// answerPing is the switch /ws/no-pong needs: with it false the server reads
// the ping and says nothing, which is a live connection that has stopped
// responding — the case a heartbeat exists to detect.
func (c *wsConn) ReadMessage(answerPing bool) (wsFrame, error) {
	var assembled []byte
	var msgOpcode byte

	for {
		f, err := c.ReadFrame()
		if err != nil {
			return wsFrame{}, err
		}

		switch f.Opcode {
		case opClose:
			return f, errWSClosed
		case opPing:
			if answerPing {
				if err := c.WriteFrame(opPong, true, f.Payload); err != nil {
					return wsFrame{}, err
				}
			}
			continue
		case opPong:
			continue
		case opText, opBinary:
			msgOpcode = f.Opcode
			assembled = append(assembled, f.Payload...)
		case opContinuation:
			assembled = append(assembled, f.Payload...)
		default:
			return wsFrame{}, fmt.Errorf("unknown opcode 0x%X", f.Opcode)
		}

		if f.FIN {
			return wsFrame{FIN: true, Opcode: msgOpcode, Payload: assembled}, nil
		}
	}
}
