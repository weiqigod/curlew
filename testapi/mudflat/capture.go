package mudflat

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

// maxPreambleBytes bounds the capture buffer. A client that opens a connection
// and never sends a header terminator would otherwise grow it without limit.
const maxPreambleBytes = 1 << 20

type connCtxKey struct{}

// capturedConn tees the request preamble on its way to net/http's parser.
//
// This exists because http.Request.Header is a canonicalised map: it has already
// lost the original casing and the order the client sent, and it has collapsed
// duplicates into a slice keyed by the canonical name. Those three losses are
// exactly what §8 refuses to accept, so the bytes are captured before the parser
// sees them.
//
// Capture is a state machine with two states. While capturing, bytes read from
// the socket are accumulated until CRLFCRLF appears; the preamble is then stored
// and capture stops, so the request body that follows is not mistaken for header
// bytes. The middleware resumes capture once the handler has finished and the
// body is drained, ready for the next request on a keep-alive connection.
type capturedConn struct {
	net.Conn

	id       string
	requests atomic.Int64

	mu        sync.Mutex
	capturing bool
	buf       []byte
	preamble  []byte
	have      bool
	truncated bool
}

func (c *capturedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n <= 0 {
		return n, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.capturing {
		return n, err
	}

	c.buf = append(c.buf, p[:n]...)
	if idx := bytes.Index(c.buf, []byte("\r\n\r\n")); idx >= 0 {
		c.preamble = append([]byte(nil), c.buf[:idx+4]...)
		c.have = true
		c.capturing = false
		c.buf = nil
		return n, err
	}

	if len(c.buf) > maxPreambleBytes {
		// Give up rather than grow without bound. The envelope reports the
		// fallback so a confusing result is attributable.
		c.capturing = false
		c.truncated = true
		c.buf = nil
	}
	return n, err
}

// takePreamble returns the captured preamble for the request now being handled.
func (c *capturedConn) takePreamble() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.have {
		return nil, false
	}
	out := c.preamble
	c.have = false
	c.preamble = nil
	return out, true
}

// resume re-arms capture for the next request on this connection. It must be
// called only after the current request's body has been consumed, or body bytes
// would be captured as though they were the next preamble.
func (c *capturedConn) resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capturing = true
	c.buf = nil
	c.have = false
	c.preamble = nil
}

func (c *capturedConn) nextRequestNumber() int {
	return int(c.requests.Add(1))
}

// capturingListener wraps accepted connections and assigns each a stable id, so
// connection reuse is observable from the client side.
type capturingListener struct {
	net.Listener
	counter *atomic.Int64
}

func (l *capturingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	wrapped := &capturedConn{
		Conn:      conn,
		id:        fmt.Sprintf("c%d", l.counter.Add(1)),
		capturing: true,
	}
	return wrapped, nil
}

func connFromContext(ctx context.Context) (*capturedConn, bool) {
	c, ok := ctx.Value(connCtxKey{}).(*capturedConn)
	return c, ok
}

// parsePreamble splits captured bytes into the request line and the header
// pairs, preserving order, original casing, and duplicates.
//
// Obsolete line folding (a header value continued on a line starting with space
// or tab, RFC 7230 §3.2.4) is appended to the previous value rather than being
// treated as a new header, which is what a folded value means.
func parsePreamble(raw []byte) (requestLine string, headers [][2]string) {
	text := string(raw)
	text = strings.TrimSuffix(text, "\r\n\r\n")

	lines := strings.Split(text, "\r\n")
	if len(lines) == 0 {
		return "", nil
	}
	requestLine = lines[0]

	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(headers) > 0 {
				headers[len(headers)-1][1] += " " + strings.TrimSpace(line)
			}
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			// A header line without a colon is malformed. Report it with an
			// empty value rather than dropping it: the raw layer produces this
			// deliberately, and silently discarding it would hide the case.
			headers = append(headers, [2]string{line, ""})
			continue
		}
		headers = append(headers, [2]string{name, strings.TrimSpace(value)})
	}
	return requestLine, headers
}
