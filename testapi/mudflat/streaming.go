package mudflat

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Streaming (§9.M).
//
// curlew is not a streaming client: it reads a body to completion and asserts
// on the whole thing. That is what this family is for rather than an obstacle
// to it. The questions are whether a chunked body survives intact, whether a
// text/event-stream is handled as a body at all, and what an unbounded response
// does to a client that has no request timeout (§11.1).
//
// Every endpoint here flushes as it writes. A server that buffered the whole
// stream and sent it at the end would satisfy every content assertion while
// testing nothing about streaming, which is why TestSSE_IsChunkedRatherThanBuffered
// measures when the first event arrives rather than only what it says.

const (
	// sseRetryMs is the reconnection hint every stream opens with. Fixed, like
	// every other value here, because §6.1 makes determinism an invariant.
	sseRetryMs = 3000

	// streamMaxObjects bounds /stream/{n}.
	streamMaxObjects = 10000
)

func (s *Server) registerStreaming() {
	s.register(Endpoint{
		Pattern:   "/sse",
		Methods:   []string{http.MethodGet},
		Family:    "M",
		Summary:   "Server-Sent Events: a retry hint, then `events` events with ids, ms apart.",
		Exercises: "Whether a text/event-stream body is read at all, and whether a response with no Content-Length is handled. curlew buffers it whole, so what a collection can assert is the accumulated stream — which is the honest description of what curlew does with SSE today.",
		Handler:   s.handleSSE,
	})

	s.register(Endpoint{
		Pattern:   "/sse/reconnect",
		Methods:   []string{http.MethodGet},
		Family:    "M",
		Summary:   "Ends after event 2; resumes at 3 when Last-Event-ID says so.",
		Exercises: "Request headers driving server state across two requests, and extraction feeding the second from the first — the SSE resumption protocol expressed with the tools curlew actually has.",
		Handler:   s.handleSSEReconnect,
	})

	s.register(Endpoint{
		Pattern:   "/stream/{n}",
		Methods:   []string{http.MethodGet},
		Family:    "M",
		Summary:   "n newline-delimited JSON objects under chunked transfer encoding.",
		Exercises: "Chunked transfer with a body that is not a single JSON document. $. addresses nothing here, so it also checks that a JSONPath assertion against non-JSON fails legibly rather than silently.",
		Handler:   s.handleStream,
	})

	s.register(Endpoint{
		Pattern:   "/stream/json/{n}",
		Methods:   []string{http.MethodGet},
		Family:    "M",
		Summary:   "One JSON array of n objects, written and flushed element by element.",
		Exercises: "Chunked transfer where the body IS assertable. curlew can only assert on a JSON body (§7.3), and its CEL escape hatch sees nil for anything else — so without this endpoint the whole family could check status and headers and nothing about the content that arrived.",
		Handler:   s.handleStreamJSON,
	})

	s.register(Endpoint{
		Pattern:   "/stream/infinite",
		Methods:   []string{http.MethodGet},
		Family:    "M",
		Summary:   "Never stops producing, until the §6.4 ceiling cuts it off.",
		Exercises: "Memory behaviour on an unbounded response, and §11.1 — curlew has no request timeout, so it reads until the server stops. The ceiling exists so that gap fails CI in two minutes rather than wedging it forever.",
		Handler:   s.handleStreamInfinite,
	})
}

// flusherFor returns the ResponseWriter's flusher. Without one nothing here is
// a stream, so this is an error rather than a silent degradation to a buffered
// response that still passes every content assertion.
func flusherFor(w http.ResponseWriter) (http.Flusher, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError,
			"this response writer cannot flush, so nothing here would actually stream")
		return nil, false
	}
	return f, true
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	events, err := intParam(r, "events", 3, 1, 1000)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	gap, err := intParam(r, "ms", 0, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if total := time.Duration(events) * time.Duration(gap) * time.Millisecond; total > hardResponseCeiling {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf(
			"events=%d at ms=%d would stream for %s, past the %s ceiling", events, gap, total, hardResponseCeiling))
		return
	}

	flusher, ok := flusherFor(w)
	if !ok {
		return
	}
	sseHeaders(w)
	w.WriteHeader(http.StatusOK)

	// The retry hint opens the stream: a client that reconnects is supposed to
	// wait this long, and it is the one SSE field with no data of its own.
	_, _ = fmt.Fprintf(w, "retry: %d\n\n", sseRetryMs)
	flusher.Flush()

	for i := 1; i <= events; i++ {
		if gap > 0 {
			time.Sleep(time.Duration(gap) * time.Millisecond)
		}
		writeSSEEvent(w, i, "tick", fmt.Sprintf(`{"seq":%d,"total":%d,"resumed":false}`, i, events))
		flusher.Flush()
	}
}

func (s *Server) handleSSEReconnect(w http.ResponseWriter, r *http.Request) {
	flusher, ok := flusherFor(w)
	if !ok {
		return
	}

	// Last-Event-ID is the client's half of the SSE resumption protocol. curlew
	// has no SSE support, so it sets the header itself — which is the point:
	// the resumption contract is expressible with the tools it does have.
	last := 0
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 0 {
			writeProblem(w, http.StatusBadRequest,
				invalidParam("Last-Event-ID", raw, "expected the id of the last event the client saw").Error())
			return
		}
		last = n
	}

	sseHeaders(w)
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "retry: %d\n\n", sseRetryMs)
	flusher.Flush()

	// Two events per leg, starting after whatever the client already has. A
	// client that replays from 1 gets the same two events twice and can prove
	// it; one that resumes correctly never sees id 1 again.
	start := last + 1
	end := min(start+1, 4)
	for i := start; i <= end; i++ {
		writeSSEEvent(w, i, "tick",
			fmt.Sprintf(`{"seq":%d,"resumed":%t,"last_event_id_seen":%d}`, i, last > 0, last))
		flusher.Flush()
	}
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 || n > streamMaxObjects {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("n", r.PathValue("n"),
				fmt.Sprintf("expected a whole number between 1 and %d", streamMaxObjects)).Error())
		return
	}
	gap, err := intParam(r, "ms", 0, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if total := time.Duration(n) * time.Duration(gap) * time.Millisecond; total > hardResponseCeiling {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf(
			"n=%d at ms=%d would stream for %s, past the %s ceiling", n, gap, total, hardResponseCeiling))
		return
	}

	flusher, ok := flusherFor(w)
	if !ok {
		return
	}
	// application/x-ndjson, not application/json: the body is a sequence of
	// documents, and a client that treats it as one will fail to parse it.
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)

	for i := 1; i <= n; i++ {
		if gap > 0 {
			time.Sleep(time.Duration(gap) * time.Millisecond)
		}
		if _, writeErr := fmt.Fprintf(w, `{"seq":%d,"total":%d,"last":%t}`+"\n", i, n, i == n); writeErr != nil {
			return // the client hung up
		}
		flusher.Flush()
	}
}

func (s *Server) handleStreamInfinite(w http.ResponseWriter, r *http.Request) {
	gap, err := intParam(r, "ms", 50, 1, 10000)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	flusher, ok := flusherFor(w)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)

	// §6.4's ceiling is what stops this endpoint from being the thing that
	// wedges CI. curlew has no request timeout (§11.1), so it reads until the
	// server stops; the deadline turns "forever" into "two minutes", which is a
	// failing gate rather than a hung one.
	deadline := time.Now().Add(hardResponseCeiling)
	for seq := 1; time.Now().Before(deadline); seq++ {
		if _, writeErr := fmt.Fprintf(w, `{"seq":%d,"endpoint":"/stream/infinite"}`+"\n", seq); writeErr != nil {
			return // the client hung up, which is the sane thing for it to do
		}
		flusher.Flush()
		time.Sleep(time.Duration(gap) * time.Millisecond)
	}
}

// sseHeaders sets the three headers an event stream needs. No Content-Length:
// the length is not known, and declaring one would end the stream early.
func sseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// writeSSEEvent emits one event. The blank line is the dispatch terminator:
// without it every event in the stream is one event.
func writeSSEEvent(w http.ResponseWriter, id int, name, data string) {
	_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, name, data)
}

func (s *Server) handleStreamJSON(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 || n > streamMaxObjects {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("n", r.PathValue("n"),
				fmt.Sprintf("expected a whole number between 1 and %d", streamMaxObjects)).Error())
		return
	}
	gap, err := intParam(r, "ms", 0, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if total := time.Duration(n) * time.Duration(gap) * time.Millisecond; total > hardResponseCeiling {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf(
			"n=%d at ms=%d would stream for %s, past the %s ceiling", n, gap, total, hardResponseCeiling))
		return
	}

	flusher, ok := flusherFor(w)
	if !ok {
		return
	}
	// A single JSON document, delivered in pieces. The brackets are written
	// separately from the elements, so the body is only parseable once the last
	// chunk lands — which is what makes this a test of reading to completion
	// rather than of parsing a chunk.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte(`{"items":[`))
	flusher.Flush()
	for i := 1; i <= n; i++ {
		if gap > 0 {
			time.Sleep(time.Duration(gap) * time.Millisecond)
		}
		sep := ","
		if i == 1 {
			sep = ""
		}
		if _, writeErr := fmt.Fprintf(w, `%s{"seq":%d,"last":%t}`, sep, i, i == n); writeErr != nil {
			return
		}
		flusher.Flush()
	}
	_, _ = fmt.Fprintf(w, `],"total":%d,"streamed":true}`, n)
	flusher.Flush()
}
