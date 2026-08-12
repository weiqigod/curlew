package mudflat

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The raw layer (§5.2).
//
// Everything here writes literal bytes to a net.Conn. No HTTP library is
// involved on the server side, which is the only way to produce the responses
// below: net/http will not emit a Content-Length that disagrees with the body,
// will not write a chunk size that is not hex, and will not put a NUL in a
// header value. Those are precisely the cases a client has never been tested
// against, because the server that would have produced them cannot.
//
// This is also the layer where the language mudflat is written in stops
// mattering (§3, §17.1): the bytes are the contract.

const (
	// rawReadLimit bounds how much of a request preamble is read before giving
	// up. The raw layer only needs the request line to route.
	rawReadLimit = 16 << 10

	// rawConnDeadline bounds a single raw exchange. Every response is written
	// immediately and the connection closed, so this only catches a client that
	// connects and never speaks.
	rawConnDeadline = 30 * time.Second
)

// rawResponse is what an endpoint produces: the exact bytes to write, and
// whether to abort the connection partway through.
type rawResponse struct {
	Bytes []byte
	// ResetAfter, when >= 0, writes only that many bytes and then aborts the
	// connection with RST instead of a clean close.
	ResetAfter int
}

// RawEndpoint is one entry in the raw registry. It mirrors Endpoint so both
// registries feed the same index and the same parity test (§16).
type RawEndpoint struct {
	Pattern   string
	Summary   string
	Exercises string
	Family    string
	// Respond builds the bytes. It takes the request path so the few endpoints
	// with a parameter can read it.
	Respond func(path string) rawResponse
}

// RawOptions configures a RawServer. Present for symmetry with Options; the
// raw layer has no state to configure yet.
type RawOptions struct{}

// RawServer is the adversarial listener.
type RawServer struct {
	endpoints []RawEndpoint
	byPath    map[string]RawEndpoint

	closed atomic.Bool
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	ln     net.Listener
}

func NewRaw(RawOptions) *RawServer {
	s := &RawServer{
		byPath: make(map[string]RawEndpoint),
		conns:  make(map[net.Conn]struct{}),
	}
	s.register()
	return s
}

// Index returns the raw registry as index entries, so /capabilities and the
// parity test see one endpoint list rather than two.
func (s *RawServer) Index() []IndexEntry {
	out := make([]IndexEntry, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		out = append(out, IndexEntry{
			Pattern:   ep.Pattern,
			Methods:   []string{"ANY"},
			Family:    ep.Family,
			Summary:   ep.Summary,
			Exercises: ep.Exercises,
		})
	}
	return out
}

// Serve accepts connections until Close.
func (s *RawServer) Serve(ln net.Listener) error {
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("raw accept: %w", err)
		}
		s.track(conn)
		go s.handle(conn)
	}
}

func (s *RawServer) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.mu.Lock()
	ln := s.ln
	conns := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}
	if ln != nil {
		return ln.Close()
	}
	return nil
}

func (s *RawServer) track(conn net.Conn) {
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
}

func (s *RawServer) untrack(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

// handle reads just enough of the request to route, writes the response bytes,
// and closes. There is no keep-alive: several endpoints leave the connection in
// a state no client could reuse, and closing makes truncation observable rather
// than leaving a client waiting for bytes that will never come.
func (s *RawServer) handle(conn net.Conn) {
	defer func() {
		s.untrack(conn)
		_ = conn.Close()
	}()

	if err := conn.SetDeadline(time.Now().Add(rawConnDeadline)); err != nil {
		return
	}

	path, err := readRequestPath(conn)
	if err != nil {
		_, _ = conn.Write(rawProblem(400, "could not read the request line: "+err.Error()))
		return
	}

	ep, ok := s.byPath[routeKey(path)]
	if !ok {
		// A well-formed 404, deliberately. A typo in a collection must not look
		// like one of the malformations this layer exists to produce.
		_, _ = conn.Write(rawProblem(404, fmt.Sprintf("no raw endpoint %q", path)))
		return
	}

	resp := ep.Respond(path)
	if resp.ResetAfter >= 0 {
		s.writeThenReset(conn, resp)
		return
	}
	_, _ = conn.Write(resp.Bytes)
}

// writeThenReset sends a prefix and aborts with RST rather than FIN, so the
// client sees a connection reset instead of a clean truncation.
func (s *RawServer) writeThenReset(conn net.Conn, resp rawResponse) {
	n := min(resp.ResetAfter, len(resp.Bytes))
	_, _ = conn.Write(resp.Bytes[:n])

	if tcp, ok := conn.(*net.TCPConn); ok {
		// SO_LINGER 0 is what turns Close into RST.
		_ = tcp.SetLinger(0)
	}
}

// readRequestPath reads the request line and discards the rest of the preamble.
func readRequestPath(conn net.Conn) (string, error) {
	buf := make([]byte, 0, 1024)
	chunk := make([]byte, 512)

	for {
		n, err := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if idx := bytes.Index(buf, []byte("\r\n")); idx >= 0 {
				return parseRequestLine(string(buf[:idx]))
			}
			if len(buf) > rawReadLimit {
				return "", fmt.Errorf("no request line within %d bytes", rawReadLimit)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf) == 0 {
				return "", fmt.Errorf("client sent nothing")
			}
			return "", err
		}
	}
}

func parseRequestLine(line string) (string, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", fmt.Errorf("malformed request line %q", line)
	}
	return parts[1], nil
}

// routeKey maps a concrete path onto a registered pattern, collapsing the one
// parameterised endpoint.
func routeKey(path string) string {
	if q := strings.IndexByte(path, '?'); q >= 0 {
		path = path[:q]
	}
	if strings.HasPrefix(path, "/raw/reset-after/") {
		return "/raw/reset-after/{n}"
	}
	return path
}

// rawProblem builds a well-formed response for the raw layer's own errors.
func rawProblem(status int, detail string) []byte {
	body := fmt.Sprintf("{\"error\":%q,\"detail\":%q,\"layer\":\"raw\"}\n",
		statusTextFor(status), detail)
	return fmt.Appendf(nil,
		"HTTP/1.1 %d %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		status, statusTextFor(status), len(body), body)
}

func statusTextFor(status int) string {
	switch status {
	case 400:
		return "Bad Request"
	case 404:
		return "Not Found"
	default:
		return "Error"
	}
}

func (s *RawServer) add(ep RawEndpoint) {
	if ep.Exercises == "" {
		panic(fmt.Sprintf("mudflat: raw endpoint %q has no Exercises citation (§P3)", ep.Pattern))
	}
	s.endpoints = append(s.endpoints, ep)
	s.byPath[ep.Pattern] = ep
}

// fixed returns an endpoint whose bytes never vary.
func fixed(b []byte) func(string) rawResponse {
	return func(string) rawResponse { return rawResponse{Bytes: b, ResetAfter: -1} }
}

func (s *RawServer) register() {
	s.add(RawEndpoint{
		Pattern:   "/raw/content-length/over",
		Family:    "E",
		Summary:   "Declares more bytes than it sends, then closes.",
		Exercises: "Truncated-response handling. The connection closing mid-body must surface as an error naming the request, not as a short body silently accepted.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Content-Length: 1000\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"only these twenty-nine bytes\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/content-length/under",
		Family:    "E",
		Summary:   "Declares fewer bytes than it sends.",
		Exercises: "Whether a body is read to the declared length or to the close. The excess is silently dropped by a correct client, which is the more dangerous direction: nothing errors.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Content-Length: 5\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"hello and then a great deal more that was never declared\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/content-length/duplicate",
		Family:    "E",
		Summary:   "Two conflicting Content-Length headers.",
		Exercises: "RFC 9112 §6.3 requires rejecting this. A client that picks one and proceeds is a request-smuggling liability.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Content-Length: 10\r\n" +
			"Content-Length: 20\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"0123456789\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/content-length/with-chunked",
		Family:    "E",
		Summary:   "Content-Length and Transfer-Encoding: chunked together.",
		Exercises: "RFC 9112 §6.1: Transfer-Encoding overrides Content-Length, and a proxy that disagrees with its origin about which wins is how requests get smuggled.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Content-Length: 10\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"5\r\nhello\r\n0\r\n\r\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/chunked/bad-size",
		Family:    "E",
		Summary:   "Chunk size line that is not hexadecimal.",
		Exercises: "Chunked decoding error legibility. The failure should name the framing, not surface as a generic read error.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"ZZZZ\r\nhello\r\n0\r\n\r\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/chunked/no-terminator",
		Family:    "E",
		Summary:   "Chunked body with no final zero-length chunk.",
		Exercises: "Detecting an incomplete chunked stream. A client that returns the partial body as a success has lost data silently.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"5\r\nhello\r\n5\r\nworld\r\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/chunked/trailers",
		Family:    "E",
		Summary:   "Well-formed chunked body with a trailer field. This one is legal.",
		Exercises: "The control case. A client that cannot read chunked-with-trailers is broken; having it beside the malformed members is what stops the family from only proving strictness.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Trailer: X-Checksum\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"5\r\nhello\r\n0\r\nX-Checksum: 5d41402abc4b2a76b9719d911017c592\r\n\r\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/no-status-line",
		Family:    "E",
		Summary:   "Header block and body with no status line at all.",
		Exercises: "Whether a missing status line is reported as such, or misread as a header.",
		Respond: fixed([]byte("Content-Type: text/plain\r\n" +
			"Content-Length: 24\r\n" +
			"\r\n" +
			"no status line preceded\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/bare-lf",
		Family:    "E",
		Summary:   "Bare LF line endings throughout instead of CRLF.",
		Exercises: "RFC 9112 §2.2 permits a recipient to accept bare LF. Whether curlew does — and agrees with curl about it — is the question.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\n" +
			"Content-Type: text/plain\n" +
			"Content-Length: 18\n" +
			"Connection: close\n" +
			"\n" +
			"bare LF throughout")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/header/huge",
		Family:    "E",
		Summary:   "A single header value of 8 KB.",
		Exercises: "Header size limits. Go's default MaxHeaderBytes is 1 MB on the server side; the client side has its own bound, and this sits above the common 8 KB proxy limit.",
		Respond:   fixed(buildHugeHeaderResponse()),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/header/many",
		Family:    "E",
		Summary:   "One thousand distinct headers.",
		Exercises: "Header count limits, and whether the echo envelope's ordered-pair representation survives a large block.",
		Respond:   fixed(buildManyHeadersResponse()),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/header/nul",
		Family:    "E",
		Summary:   "A NUL byte inside a header value.",
		Exercises: "RFC 9110 §5.5 forbids NUL in a field value. A client that passes it through into a log or a report has a header-injection problem.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"X-Nul-Value: before\x00after\r\n" +
			"Content-Length: 4\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"nul\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/header/no-colon",
		Family:    "E",
		Summary:   "A header line with no colon separator.",
		Exercises: "Malformed field-line handling. The line is neither a header nor a body, and the error should say which line it choked on.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"ThisLineHasNoColonAtAll\r\n" +
			"Content-Length: 10\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"no colon\r\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/trailing-garbage",
		Family:    "E",
		Summary:   "A complete response followed by junk bytes on the same connection.",
		Exercises: "Whether a client stops at Content-Length or keeps reading to close. Reading past the declared length is how a response-splitting payload gets treated as data.",
		Respond: fixed([]byte("HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/plain\r\n" +
			"Content-Length: 9\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"complete\n" +
			"GARBAGE THAT FOLLOWS A COMPLETE RESPONSE\r\n")),
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/reset-after/{n}",
		Family:    "E",
		Summary:   "Sends n bytes of a valid response, then aborts the connection with RST.",
		Exercises: "retry_on.network_errors against a mid-response reset — the case that must be retriable, and was not classified as such until the Phase 1 fixes.",
		Respond:   respondResetAfter,
	})

	s.add(RawEndpoint{
		Pattern:   "/raw/http09",
		Family:    "E",
		Summary:   "A bare body with no status line and no headers, HTTP/0.9 style.",
		Exercises: "The oldest ambiguity in HTTP. Modern clients reject it; one that guesses at a status code is inventing information.",
		Respond:   fixed([]byte("a bare HTTP/0.9 body with no status line and no headers\n")),
	})
}

func buildHugeHeaderResponse() []byte {
	var b bytes.Buffer
	b.WriteString("HTTP/1.1 200 OK\r\n")
	b.WriteString("Content-Type: text/plain\r\n")
	b.WriteString("X-Huge: " + strings.Repeat("v", 8192) + "\r\n")
	b.WriteString("Content-Length: 5\r\n")
	b.WriteString("Connection: close\r\n")
	b.WriteString("\r\n")
	b.WriteString("huge\n")
	return b.Bytes()
}

func buildManyHeadersResponse() []byte {
	var b bytes.Buffer
	b.WriteString("HTTP/1.1 200 OK\r\n")
	b.WriteString("Content-Type: text/plain\r\n")
	for i := range 1000 {
		fmt.Fprintf(&b, "X-Many-%04d: %d\r\n", i, i)
	}
	b.WriteString("Content-Length: 5\r\n")
	b.WriteString("Connection: close\r\n")
	b.WriteString("\r\n")
	b.WriteString("many\n")
	return b.Bytes()
}

// respondResetAfter cuts a valid response at the byte offset in the path.
func respondResetAfter(path string) rawResponse {
	full := []byte("HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 40\r\n" +
		"Connection: close\r\n" +
		"\r\n" +
		"this response is cut off partway through")

	after := 20
	if trimmed := strings.TrimPrefix(path, "/raw/reset-after/"); trimmed != path {
		if q := strings.IndexByte(trimmed, '?'); q >= 0 {
			trimmed = trimmed[:q]
		}
		if n, err := strconv.Atoi(trimmed); err == nil && n >= 0 {
			after = n
		}
	}
	return rawResponse{Bytes: full, ResetAfter: after}
}
