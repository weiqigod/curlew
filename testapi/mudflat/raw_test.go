package mudflat

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The raw layer is the part of mudflat that does not use net/http at all.
//
// That is the whole point of it. A Go server and a Go client agree by
// construction about framing, chunking and header syntax, so a malformed
// response cannot be produced by net.http.Server — it will not emit a
// Content-Length that disagrees with the body, and it will not write a chunk
// size that is not hex. Those bytes have to be written by hand, and this is
// where language choice stops mattering.

func startRawServer(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewRaw(RawOptions{})
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

// rawGet sends a bare GET and reads until the server closes. It returns the
// literal bytes and whether the connection ended with a reset rather than a
// clean close, because for several endpoints that distinction is the assertion.
func rawGet(t *testing.T, addr, path string) (body []byte, reset bool) {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: mudflat.test\r\n\r\n", path); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, err := io.ReadAll(conn)
	if err != nil {
		// A reset surfaces as ECONNRESET rather than a clean EOF.
		return out, strings.Contains(err.Error(), "reset")
	}
	return out, false
}

func TestRaw_ContentLengthOverstatesBody(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/content-length/over")

	declared, body := splitRawResponse(t, got)
	if declared <= len(body) {
		t.Errorf("Content-Length %d does not exceed the %d bytes sent; "+
			"this endpoint exists to truncate", declared, len(body))
	}
}

func TestRaw_ContentLengthUnderstatesBody(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/content-length/under")

	declared, body := splitRawResponse(t, got)
	if declared >= len(body) {
		t.Errorf("Content-Length %d is not below the %d bytes sent", declared, len(body))
	}
}

func TestRaw_DuplicateContentLength(t *testing.T) {
	// RFC 9112 §6.3: a message with two conflicting Content-Length values is
	// unprocessable and must be rejected.
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/content-length/duplicate")

	if n := bytes.Count(got, []byte("Content-Length:")); n != 2 {
		t.Errorf("found %d Content-Length headers, want 2", n)
	}
}

func TestRaw_ContentLengthWithChunked(t *testing.T) {
	// Both framing mechanisms at once — the classic request-smuggling shape.
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/content-length/with-chunked")

	if !bytes.Contains(got, []byte("Content-Length:")) {
		t.Error("no Content-Length header")
	}
	if !bytes.Contains(got, []byte("Transfer-Encoding: chunked")) {
		t.Error("no Transfer-Encoding: chunked header")
	}
}

func TestRaw_ChunkedBadSize(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/chunked/bad-size")

	_, body := splitHeadersBody(t, got)
	firstLine, _, _ := strings.Cut(string(body), "\r\n")
	if isHex(firstLine) {
		t.Errorf("chunk size %q is valid hex; this endpoint must emit an invalid one", firstLine)
	}
}

func TestRaw_ChunkedNoTerminator(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/chunked/no-terminator")

	if bytes.HasSuffix(got, []byte("0\r\n\r\n")) {
		t.Error("response ends with the final chunk; this endpoint must omit it")
	}
	if !bytes.Contains(got, []byte("Transfer-Encoding: chunked")) {
		t.Error("not a chunked response")
	}
}

func TestRaw_ChunkedWithTrailersIsLegal(t *testing.T) {
	// The one well-formed member of the family. A client that cannot read a
	// chunked body with trailers is broken; a client that rejects the malformed
	// members is correct. Having both here keeps the distinction honest.
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/chunked/trailers")

	if !bytes.Contains(got, []byte("Trailer:")) {
		t.Error("no Trailer header announcing the trailing field")
	}
	if !bytes.HasSuffix(got, []byte("\r\n\r\n")) {
		t.Errorf("response does not terminate correctly: %q", tail(got, 40))
	}
	if !bytes.Contains(got, []byte("X-Checksum:")) {
		t.Error("no trailer field after the final chunk")
	}

	// Neither chunk is valid JSON on its own, so a client that mishandles
	// reassembly cannot accidentally pass the collection assertion.
	if !bytes.Contains(got, []byte(`8`+"\r\n"+`{"ok":tr`)) {
		t.Errorf("first chunk is not the split-mid-token payload: %s", escape(got))
	}
}

func TestRaw_NoStatusLine(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/no-status-line")

	if bytes.HasPrefix(got, []byte("HTTP/")) {
		t.Errorf("response starts with a status line: %q", head(got, 40))
	}
	if len(got) == 0 {
		t.Error("no bytes at all; the endpoint should send a body with no status line")
	}
}

func TestRaw_BareLFLineEndings(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/bare-lf")

	if bytes.Contains(got, []byte("\r\n")) {
		t.Error("response contains CRLF; this endpoint must use bare LF throughout")
	}
	if !bytes.Contains(got, []byte("\n")) {
		t.Error("response contains no LF at all")
	}
}

func TestRaw_HugeHeaderValue(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/header/huge")

	headers, _ := splitHeadersBody(t, got)
	longest := 0
	for _, line := range strings.Split(headers, "\r\n") {
		if len(line) > longest {
			longest = len(line)
		}
	}
	if longest < 8000 {
		t.Errorf("longest header line is %d bytes, want at least 8000", longest)
	}
}

func TestRaw_ManyHeaders(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/header/many")

	headers, _ := splitHeadersBody(t, got)
	count := strings.Count(headers, "\r\n")
	if count < 1000 {
		t.Errorf("got %d header lines, want at least 1000", count)
	}
}

func TestRaw_NulByteInHeaderValue(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/header/nul")

	headers, _ := splitHeadersBody(t, got)
	if !strings.ContainsRune(headers, 0) {
		t.Error("no NUL byte in the header block")
	}
}

func TestRaw_HeaderLineWithoutColon(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/header/no-colon")

	headers, _ := splitHeadersBody(t, got)
	lines := strings.Split(headers, "\r\n")
	found := false
	for _, line := range lines[1:] { // skip the status line
		if line != "" && !strings.Contains(line, ":") {
			found = true
		}
	}
	if !found {
		t.Errorf("every header line contains a colon; one must not:\n%s", headers)
	}
}

func TestRaw_TrailingGarbageAfterCompleteResponse(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/trailing-garbage")

	declared, body := splitRawResponse(t, got)
	if len(body) <= declared {
		t.Errorf("body is %d bytes with Content-Length %d; "+
			"this endpoint must append junk after a complete response", len(body), declared)
	}
}

func TestRaw_ResetAfterNBytes(t *testing.T) {
	addr := startRawServer(t)
	got, reset := rawGet(t, addr, "/raw/reset-after/20")

	if !reset {
		t.Errorf("connection closed cleanly after %d bytes; want a reset", len(got))
	}
	if len(got) > 40 {
		t.Errorf("received %d bytes, want the connection cut near 20", len(got))
	}
}

func TestRaw_HTTP09StyleBareBody(t *testing.T) {
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/http09")

	if bytes.Contains(got, []byte("\r\n\r\n")) {
		t.Error("response has a header block; HTTP/0.9 is a bare body")
	}
	if bytes.HasPrefix(got, []byte("HTTP/")) {
		t.Error("response has a status line; HTTP/0.9 has none")
	}
}

func TestRaw_UnknownPathIsAPlainError(t *testing.T) {
	// Even the raw layer's own misuse path should be legible: a typo in a
	// collection must not look like one of the deliberate malformations.
	addr := startRawServer(t)
	got, _ := rawGet(t, addr, "/raw/no-such-thing")

	if !bytes.HasPrefix(got, []byte("HTTP/1.1 404")) {
		t.Errorf("unknown raw path did not return a well-formed 404: %q", head(got, 60))
	}
}

func TestRaw_IndexListsEveryEndpoint(t *testing.T) {
	srv := NewRaw(RawOptions{})
	entries := srv.Index()
	if len(entries) < 15 {
		t.Fatalf("raw index lists %d endpoints, want at least 15", len(entries))
	}
	for _, e := range entries {
		if e.Exercises == "" {
			t.Errorf("raw endpoint %q has no Exercises citation (§P3)", e.Pattern)
		}
		if e.Summary == "" {
			t.Errorf("raw endpoint %q has no Summary", e.Pattern)
		}
		if !strings.HasPrefix(e.Pattern, "/raw/") {
			t.Errorf("raw endpoint %q is not under /raw/", e.Pattern)
		}
	}
}

// --- Golden transcripts (§13.2) ---------------------------------------------

// goldenPaths are the endpoints whose bytes are fixed and therefore comparable
// byte for byte. reset-after is excluded: its response is cut mid-flight, so
// what arrives depends on TCP timing rather than on what the server wrote.
var goldenPaths = []string{
	"/raw/content-length/over",
	"/raw/content-length/under",
	"/raw/content-length/duplicate",
	"/raw/content-length/with-chunked",
	"/raw/chunked/bad-size",
	"/raw/chunked/no-terminator",
	"/raw/chunked/trailers",
	"/raw/no-status-line",
	"/raw/bare-lf",
	"/raw/header/huge",
	"/raw/header/many",
	"/raw/header/nul",
	"/raw/header/no-colon",
	"/raw/trailing-garbage",
	"/raw/http09",
}

// TestRaw_MatchesGoldenTranscripts is drift detection, not a correctness proof.
//
// The correctness argument for these bytes is the one-time hand review against
// RFC text recorded in testapi/golden/README.md. What this test adds is that
// the server cannot change what it emits without a reviewer seeing the diff.
//
// Run with MUDFLAT_UPDATE_GOLDEN=1 to rewrite them, and then read the diff
// rather than trusting it.
func TestRaw_MatchesGoldenTranscripts(t *testing.T) {
	addr := startRawServer(t)
	update := os.Getenv("MUDFLAT_UPDATE_GOLDEN") == "1"

	for _, path := range goldenPaths {
		t.Run(path, func(t *testing.T) {
			got, _ := rawGet(t, addr, path)
			file := goldenFile(path)

			if update {
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(file, got, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("wrote %s — review the diff before committing", file)
				return
			}

			want, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read golden: %v\nRun MUDFLAT_UPDATE_GOLDEN=1 go test ./testapi/... to create it, "+
					"then review it against the RFC before committing.", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("wire bytes changed.\n got: %s\nwant: %s", escape(got), escape(want))
			}
		})
	}
}

func goldenFile(path string) string {
	name := strings.TrimPrefix(path, "/raw/")
	name = strings.ReplaceAll(name, "/", "-")
	return filepath.Join("..", "golden", "raw", name+".txt")
}

// --- helpers ----------------------------------------------------------------

// splitHeadersBody accepts either terminator. The raw layer varies line endings
// on purpose — /raw/bare-lf uses LF throughout — so a helper that only knows
// CRLF reports "no header terminator" for a response that has one.
func splitHeadersBody(t *testing.T, raw []byte) (headers string, body []byte) {
	t.Helper()
	if idx := bytes.Index(raw, []byte("\r\n\r\n")); idx >= 0 {
		return string(raw[:idx]), raw[idx+4:]
	}
	if idx := bytes.Index(raw, []byte("\n\n")); idx >= 0 {
		return string(raw[:idx]), raw[idx+2:]
	}
	t.Fatalf("no header terminator in response: %s", escape(raw))
	return "", nil
}

func splitRawResponse(t *testing.T, raw []byte) (declaredLength int, body []byte) {
	t.Helper()
	headers, body := splitHeadersBody(t, raw)
	for _, line := range headerLines(headers) {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &declaredLength); err != nil {
				t.Fatalf("bad Content-Length %q: %v", value, err)
			}
			return declaredLength, body
		}
	}
	t.Fatalf("no Content-Length header in:\n%s", headers)
	return 0, nil
}

// headerLines splits a header block on whichever terminator it uses.
func headerLines(headers string) []string {
	if strings.Contains(headers, "\r\n") {
		return strings.Split(headers, "\r\n")
	}
	return strings.Split(headers, "\n")
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// escape renders bytes readably for a failure message: CRLF stays visible and
// non-printables become hex, so a diff on a NUL byte is legible in test output.
func escape(b []byte) string {
	var out strings.Builder
	for _, c := range b {
		switch {
		case c == '\r':
			out.WriteString("\\r")
		case c == '\n':
			out.WriteString("\\n\n")
		case c < 0x20 || c > 0x7e:
			fmt.Fprintf(&out, "\\x%02x", c)
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

func head(b []byte, n int) string {
	if len(b) > n {
		b = b[:n]
	}
	return escape(b)
}

func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return escape(b)
}

// TestRaw_OnlyTheIntendedMalformationIsPresent guards against a second,
// accidental defect riding along in an endpoint that is about something else.
//
// A wrong Content-Length in the NUL-byte endpoint would make that endpoint test
// two things at once, and the failure it produced would be attributed to the
// wrong cause. The content-length family is exempt because a mismatch there is
// the entire point.
func TestRaw_OnlyTheIntendedMalformationIsPresent(t *testing.T) {
	deliberate := map[string]bool{
		"/raw/content-length/over":         true,
		"/raw/content-length/under":        true,
		"/raw/content-length/duplicate":    true,
		"/raw/content-length/with-chunked": true,
		"/raw/trailing-garbage":            true,
	}

	addr := startRawServer(t)
	for _, path := range goldenPaths {
		if deliberate[path] {
			continue
		}
		t.Run(path, func(t *testing.T) {
			got, _ := rawGet(t, addr, path)
			if !bytes.Contains(got, []byte("Content-Length:")) {
				return // chunked or HTTP/0.9: no length to check
			}
			declared, body := splitRawResponse(t, got)
			if declared != len(body) {
				t.Errorf("Content-Length %d but %d body bytes — this endpoint is not "+
					"supposed to be testing length framing:\n%s", declared, len(body), escape(got))
			}
		})
	}
}
