package mudflat

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// startTestServer runs a real server on a real socket. The envelope's whole
// purpose is to report what arrived on the wire, so testing it through
// httptest's client would assert on bytes Go's client had already normalised.
func startTestServer(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := New(Options{})
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

// rawExchange writes literal bytes to the server and returns the literal
// response. No HTTP client is involved in either direction, so header casing
// and order survive the trip.
func rawExchange(t *testing.T, addr, request string) string {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(out)
}

// bodyOf splits a raw HTTP response at the header terminator.
func bodyOf(t *testing.T, raw string) string {
	t.Helper()
	_, body, ok := strings.Cut(raw, "\r\n\r\n")
	if !ok {
		t.Fatalf("no header terminator in response:\n%s", raw)
	}
	return body
}

func decodeEnvelope(t *testing.T, raw string) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal([]byte(bodyOf(t, raw)), &env); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\nbody:\n%s", err, bodyOf(t, raw))
	}
	return env
}

func TestEnvelope_PreservesOriginalHeaderCasing(t *testing.T) {
	addr := startTestServer(t)

	raw := rawExchange(t, addr, "GET /echo HTTP/1.1\r\n"+
		"Host: mudflat.test\r\n"+
		"x-lower-case: 1\r\n"+
		"X-UPPER-CASE: 2\r\n"+
		"X-mIxEd-CaSe: 3\r\n"+
		"Connection: close\r\n\r\n")

	env := decodeEnvelope(t, raw)

	want := map[string]bool{"x-lower-case": false, "X-UPPER-CASE": false, "X-mIxEd-CaSe": false}
	for _, pair := range env.Request.Headers {
		if _, ok := want[pair[0]]; ok {
			want[pair[0]] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("header %q not reported with its original casing; got %v",
				name, env.Request.Headers)
		}
	}
}

func TestEnvelope_PreservesHeaderOrder(t *testing.T) {
	addr := startTestServer(t)

	raw := rawExchange(t, addr, "GET /echo HTTP/1.1\r\n"+
		"Host: mudflat.test\r\n"+
		"A-First: 1\r\n"+
		"B-Second: 2\r\n"+
		"C-Third: 3\r\n"+
		"Connection: close\r\n\r\n")

	env := decodeEnvelope(t, raw)

	var got []string
	for _, name := range env.Request.HeaderNamesInOrder {
		if strings.HasPrefix(name, "A-") || strings.HasPrefix(name, "B-") || strings.HasPrefix(name, "C-") {
			got = append(got, name)
		}
	}
	want := []string{"A-First", "B-Second", "C-Third"}
	if len(got) != len(want) {
		t.Fatalf("ordered names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d = %q, want %q (full order: %v)",
				i, got[i], want[i], env.Request.HeaderNamesInOrder)
		}
	}
}

func TestEnvelope_ReportsDuplicateHeadersSeparately(t *testing.T) {
	addr := startTestServer(t)

	raw := rawExchange(t, addr, "GET /echo HTTP/1.1\r\n"+
		"Host: mudflat.test\r\n"+
		"X-Dup: a\r\n"+
		"X-Dup: b\r\n"+
		"Connection: close\r\n\r\n")

	env := decodeEnvelope(t, raw)

	var values []string
	for _, pair := range env.Request.Headers {
		if pair[0] == "X-Dup" {
			values = append(values, pair[1])
		}
	}
	if len(values) != 2 || values[0] != "a" || values[1] != "b" {
		t.Errorf("X-Dup values = %v, want [a b] as separate entries", values)
	}
}

func TestEnvelope_BodyIsBase64AndNeverDecoded(t *testing.T) {
	addr := startTestServer(t)

	// Latin-1 bytes that are not valid UTF-8. A decode-and-re-encode round trip
	// would replace them with U+FFFD and the test would not notice.
	body := "\xe9\xe8\xff\x00binary"
	raw := rawExchange(t, addr, fmt.Sprintf("POST /echo HTTP/1.1\r\n"+
		"Host: mudflat.test\r\n"+
		"Content-Length: %d\r\n"+
		"Connection: close\r\n\r\n%s", len(body), body))

	env := decodeEnvelope(t, raw)

	decoded, err := base64.StdEncoding.DecodeString(env.Request.BodyBase64)
	if err != nil {
		t.Fatalf("body_base64 does not decode: %v", err)
	}
	if string(decoded) != body {
		t.Errorf("round-tripped body = %q, want %q — bytes were altered in transit", decoded, body)
	}
	if env.Request.BodyLen != len(body) {
		t.Errorf("body_len = %d, want %d", env.Request.BodyLen, len(body))
	}
	if len(env.Request.BodySHA256) != 64 {
		t.Errorf("body_sha256 = %q, want 64 hex characters", env.Request.BodySHA256)
	}
}

func TestEnvelope_ReportsQueryPairsIncludingRepeats(t *testing.T) {
	addr := startTestServer(t)

	raw := rawExchange(t, addr, "GET /echo?x=1&x=2&y=3 HTTP/1.1\r\n"+
		"Host: mudflat.test\r\nConnection: close\r\n\r\n")

	env := decodeEnvelope(t, raw)

	want := [][2]string{{"x", "1"}, {"x", "2"}, {"y", "3"}}
	if len(env.Request.Query) != len(want) {
		t.Fatalf("query = %v, want %v", env.Request.Query, want)
	}
	for i := range want {
		if env.Request.Query[i] != want[i] {
			t.Errorf("query[%d] = %v, want %v", i, env.Request.Query[i], want[i])
		}
	}
}

func TestEnvelope_ReportsMethodTargetAndVersion(t *testing.T) {
	addr := startTestServer(t)

	raw := rawExchange(t, addr, "PATCH /echo?a=1 HTTP/1.1\r\n"+
		"Host: mudflat.test\r\nConnection: close\r\n\r\n")

	env := decodeEnvelope(t, raw)

	if env.Request.Method != "PATCH" {
		t.Errorf("method = %q, want PATCH", env.Request.Method)
	}
	if env.Request.Target != "/echo?a=1" {
		t.Errorf("target = %q, want /echo?a=1", env.Request.Target)
	}
	if env.Request.HTTPVersion != "HTTP/1.1" {
		t.Errorf("http_version = %q, want HTTP/1.1", env.Request.HTTPVersion)
	}
}

func TestEnvelope_CarriesNoTimestamp(t *testing.T) {
	// §6.1: the envelope carries no wall-clock field, which is what makes
	// byte-identical reruns possible. A timestamp added for convenience would
	// break reproducibility silently.
	addr := startTestServer(t)

	raw := rawExchange(t, addr, "GET /echo HTTP/1.1\r\nHost: m\r\nConnection: close\r\n\r\n")
	body := bodyOf(t, raw)

	for _, banned := range []string{"timestamp", "\"time\"", "received_at", "date"} {
		if strings.Contains(strings.ToLower(body), banned) {
			t.Errorf("envelope contains %q; §6.1 forbids wall-clock fields\nbody: %s", banned, body)
		}
	}
}

func TestEnvelope_TwoRequestsProduceIdenticalBodies(t *testing.T) {
	// The determinism invariant, at its smallest scale: the same request twice
	// yields the same envelope except for the connection fields §6.1 exempts.
	addr := startTestServer(t)

	req := "GET /echo?k=v HTTP/1.1\r\nHost: m\r\nX-Same: 1\r\nConnection: close\r\n\r\n"
	first := decodeEnvelope(t, rawExchange(t, addr, req))
	second := decodeEnvelope(t, rawExchange(t, addr, req))

	first.Connection = ConnectionInfo{}
	second.Connection = ConnectionInfo{}

	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Errorf("envelopes differ across identical requests:\n%s\n%s", a, b)
	}
}

func TestEnvelope_ReportsConnectionReuse(t *testing.T) {
	addr := startTestServer(t)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	reader := bufio.NewReader(conn)
	var connIDs []string

	for i := range 3 {
		last := i == 2
		req := "GET /echo HTTP/1.1\r\nHost: m\r\n"
		if last {
			req += "Connection: close\r\n"
		}
		req += "\r\n"

		if _, err := io.WriteString(conn, req); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}

		env := readOneEnvelope(t, reader)
		connIDs = append(connIDs, env.Connection.ID)

		if got := env.Connection.RequestsOnConn; got != i+1 {
			t.Errorf("request %d: requests_on_conn = %d, want %d", i, got, i+1)
		}
	}

	for i := 1; i < len(connIDs); i++ {
		if connIDs[i] != connIDs[0] {
			t.Errorf("connection id changed across a reused connection: %v", connIDs)
			break
		}
	}
}

// readOneEnvelope reads a single chunked-or-sized response off a keep-alive
// connection and decodes its body.
func readOneEnvelope(t *testing.T, r *bufio.Reader) Envelope {
	t.Helper()

	var contentLen int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read header line: %v", err)
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}
		if name, value, ok := strings.Cut(trimmed, ":"); ok {
			if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
				if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &contentLen); err != nil {
					t.Fatalf("bad Content-Length %q: %v", value, err)
				}
			}
		}
	}
	if contentLen == 0 {
		t.Fatal("response had no Content-Length; keep-alive read needs one")
	}

	buf := make([]byte, contentLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("read body: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(buf, &env); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, buf)
	}
	return env
}

func TestEnvelope_ReportsServerContext(t *testing.T) {
	addr := startTestServer(t)

	raw := rawExchange(t, addr, "GET /s/abc123/echo HTTP/1.1\r\nHost: m\r\nConnection: close\r\n\r\n")
	env := decodeEnvelope(t, raw)

	if env.Server.Name != "mudflat" {
		t.Errorf("server.name = %q, want mudflat", env.Server.Name)
	}
	if env.Server.Session != "abc123" {
		t.Errorf("server.session = %q, want abc123", env.Server.Session)
	}
	if env.Server.Endpoint == "" {
		t.Error("server.endpoint is empty; a failing assertion should name the endpoint it hit")
	}
}

func TestEnvelope_HeaderCaptureSurvivesLargePreamble(t *testing.T) {
	// The capture buffer reads until the header terminator. A preamble split
	// across several TCP reads is the ordinary case for anything non-trivial.
	addr := startTestServer(t)

	var b strings.Builder
	b.WriteString("GET /echo HTTP/1.1\r\nHost: m\r\n")
	for i := range 200 {
		fmt.Fprintf(&b, "X-Filler-%03d: %s\r\n", i, strings.Repeat("v", 40))
	}
	b.WriteString("X-Last: sentinel\r\nConnection: close\r\n\r\n")

	env := decodeEnvelope(t, rawExchange(t, addr, b.String()))

	var found bool
	for _, pair := range env.Request.Headers {
		if pair[0] == "X-Last" && pair[1] == "sentinel" {
			found = true
		}
	}
	if !found {
		t.Errorf("last header of a large preamble was not captured (%d headers seen)",
			len(env.Request.Headers))
	}
}
