package mudflat

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Streaming (§9.M).
//
// curlew is not a streaming client: it reads a response body to completion and
// asserts on the whole thing. That is the point of this family rather than an
// obstacle to it — the questions are whether a chunked body arrives intact,
// whether a text/event-stream is handled as a body at all, and what an
// unbounded response does to a client with no request timeout (§11.1).

func TestSSE_SendsWellFormedEvents(t *testing.T) {
	base, client := startServer(t)
	resp, body := get(t, client, base+"/sse?events=3&ms=0")

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if got := resp.Header.Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q, want it absent — a stream has no length", got)
	}
	if resp.Header.Get("Cache-Control") == "" {
		t.Error("no Cache-Control; an SSE stream that gets cached is not a stream")
	}

	text := string(body)
	if !strings.HasPrefix(text, "retry: ") {
		t.Errorf("stream does not open with a retry hint:\n%s", text)
	}
	for i := 1; i <= 3; i++ {
		for _, want := range []string{
			"id: " + itoa(i),
			"event: tick",
			`"seq":` + itoa(i),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("event %d: missing %q in:\n%s", i, want, text)
			}
		}
	}
	// Events are separated by a blank line (RFC-ish: the WHATWG SSE spec's
	// dispatch rule). Without it every event is one event.
	if n := strings.Count(text, "\n\n"); n < 3 {
		t.Errorf("found %d event terminators, want at least 3:\n%s", n, text)
	}
}

func TestSSE_IsChunkedRatherThanBuffered(t *testing.T) {
	// The events must arrive as they are produced. A server that buffered the
	// whole stream and sent it at the end would still pass the content checks
	// above while testing nothing about streaming.
	base, client := startServer(t)

	resp, err := client.Get(base + "/sse?events=3&ms=120")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !contains(resp.TransferEncoding, "chunked") && resp.Header.Get("Content-Length") != "" {
		t.Errorf("TransferEncoding = %v, want chunked", resp.TransferEncoding)
	}

	start := time.Now()
	reader := bufio.NewReader(resp.Body)
	var firstEventAt time.Duration
	for {
		line, readErr := reader.ReadString('\n')
		if strings.HasPrefix(line, "data: ") {
			firstEventAt = time.Since(start)
			break
		}
		if readErr != nil {
			t.Fatalf("read: %v", readErr)
		}
	}
	// The first event is available long before the last one is written.
	if firstEventAt > 300*time.Millisecond {
		t.Errorf("first event took %s; the stream looks buffered", firstEventAt)
	}
}

func TestSSE_ReconnectResumesFromLastEventID(t *testing.T) {
	base, client := startServer(t)

	_, first := get(t, client, base+"/sse/reconnect")
	if !strings.Contains(string(first), "id: 2") {
		t.Fatalf("first leg should end at id 2:\n%s", first)
	}
	if strings.Contains(string(first), "id: 3") {
		t.Fatalf("first leg should stop before id 3:\n%s", first)
	}

	req, err := http.NewRequest(http.MethodGet, base+"/sse/reconnect", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", "2")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	second, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	text := string(second)
	if !strings.Contains(text, "id: 3") || !strings.Contains(text, "id: 4") {
		t.Errorf("resumed leg should carry ids 3 and 4:\n%s", text)
	}
	if strings.Contains(text, "id: 1") {
		t.Errorf("resumed leg replayed an event the client already had:\n%s", text)
	}
	if !strings.Contains(text, `"resumed":true`) {
		t.Errorf("resumed leg does not say it resumed:\n%s", text)
	}
}

func TestStream_SendsNJSONObjects(t *testing.T) {
	base, client := startServer(t)
	_, body := get(t, client, base+"/stream/4?ms=0")

	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), body)
	}
	for i, line := range lines {
		var obj struct {
			Seq   int  `json:"seq"`
			Total int  `json:"total"`
			Last  bool `json:"last"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not JSON: %v (%q)", i+1, err, line)
			continue
		}
		if obj.Seq != i+1 {
			t.Errorf("line %d: seq = %d", i+1, obj.Seq)
		}
		if want := i == 3; obj.Last != want {
			t.Errorf("line %d: last = %v, want %v", i+1, obj.Last, want)
		}
	}
}

func TestStream_RejectsACountThatWouldOutlastTheCeiling(t *testing.T) {
	base, client := startServer(t)
	resp, _ := get(t, client, base+"/stream/1000?ms=1000")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestStream_InfiniteKeepsProducingAndIsBounded(t *testing.T) {
	// The endpoint curlew cannot survive: it has no request timeout (§11.1), so
	// a collection request here would sit for the full server-side ceiling.
	// This test reads a few objects and hangs up, which is what a client with a
	// timeout would do.
	base, client := startServer(t)

	resp, err := client.Get(base + "/stream/infinite?ms=5")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)
	for i := 1; i <= 5; i++ {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("object %d: %v", i, readErr)
		}
		var obj struct {
			Seq int `json:"seq"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("object %d is not JSON: %v (%q)", i, err, line)
		}
		if obj.Seq != i {
			t.Errorf("object %d: seq = %d", i, obj.Seq)
		}
	}
	// Hanging up here must not wedge the server: the handler notices the
	// closed connection on its next write.
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}
