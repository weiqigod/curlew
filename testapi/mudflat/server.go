package mudflat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// sessionPrefix is the path form of the session model (§7). Chosen over a
	// header so a failing dogfood assertion names its own session in terminal
	// output, in the events stream, and in an HTML report.
	sessionPrefix = "/s/{sid}"

	// sessionHeader is the alternative for cases where the path is fixed —
	// notably the OpenAPI round trip, whose paths come from the served document.
	sessionHeader = "X-Mudflat-Session"

	// maxRequestBody bounds what the middleware will buffer. The echo family
	// must read the whole body before the capture state machine can re-arm, so
	// this is a real ceiling rather than a formality.
	maxRequestBody = 10 << 20

	// hardResponseCeiling is the §6.4 bound on any endpoint that delays, stalls,
	// or hangs. Deliberately far above any legitimate test duration: its job is
	// to stop a client-side regression from wedging CI indefinitely.
	hardResponseCeiling = 120 * time.Second
)

type (
	bodyCtxKey     struct{}
	reqNumCtxKey   struct{}
	endpointCtxKey struct{}
)

// Endpoint is one entry in the registry. The registry is the single definition:
// routing, the self-description index (§6.5), and the anti-bloat parity test
// (§16) all derive from it, so an endpoint cannot exist without documenting
// itself or drift out of the index.
type Endpoint struct {
	// Pattern is the canonical path. Session-scoped endpoints include the
	// sessionPrefix; stateless ones do not, and are mounted at both the bare
	// path and the prefixed one.
	Pattern string

	// Methods restricts the endpoint. Empty means any method, which is the
	// right default for the echo family.
	Methods []string

	// Summary says what the endpoint does.
	Summary string

	// Exercises names the curlew behaviour this endpoint tests. §P3 requires it,
	// and §16 deletes any endpoint that cannot answer it.
	Exercises string

	// Family is the §9 letter grouping.
	Family string

	Handler http.HandlerFunc `json:"-"`
}

// SessionScoped reports whether the endpoint keeps per-session state.
func (e Endpoint) SessionScoped() bool {
	return strings.HasPrefix(e.Pattern, "/s/{sid}")
}

// Options configures a Server.
type Options struct {
	// Sessions may be supplied to control TTL, capacity, or the clock. A nil
	// value gets the specification's defaults.
	Sessions *Sessions

	// Now is the clock the rate limiter refills against. Injectable so a test
	// can drive refill without sleeping; nil means time.Now.
	Now func() time.Time
}

// Server is the structured layer (§5.1).
type Server struct {
	mux       *http.ServeMux
	endpoints []Endpoint
	sessions  *Sessions

	http     *http.Server
	connSeq  atomic.Int64
	closed   atomic.Bool
	shutdown chan struct{}
	now      func() time.Time
}

// New builds a server with every Phase 1 family registered.
func New(opts Options) *Server {
	sessions := opts.Sessions
	if sessions == nil {
		sessions = NewSessions(SessionOptions{})
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	s := &Server{
		mux:      http.NewServeMux(),
		sessions: sessions,
		shutdown: make(chan struct{}),
		now:      now,
	}

	s.registerMeta()
	s.registerEcho()
	s.registerStatus()
	s.registerEncoding()
	s.registerFlaky()
	s.registerResources()
	s.registerVerify()
	s.registerConcurrency()
	s.registerLeak()
	s.registerGraphQL()
	s.registerWebSocket()

	s.http = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 30 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			if cc, ok := c.(*capturedConn); ok {
				return context.WithValue(ctx, connCtxKey{}, cc)
			}
			return ctx
		},
	}

	return s
}

// Endpoints returns the registry, for the index and the parity test.
func (s *Server) Endpoints() []Endpoint {
	out := make([]Endpoint, len(s.endpoints))
	copy(out, s.endpoints)
	return out
}

// Sessions exposes the store, for tests and the capabilities endpoint.
func (s *Server) Sessions() *Sessions { return s.sessions }

// Serve accepts connections on ln. The listener is wrapped so each connection
// carries a stable id and tees its request preambles (§5.1).
func (s *Server) Serve(ln net.Listener) error {
	wrapped := &capturingListener{Listener: ln, counter: &s.connSeq}
	err := s.http.Serve(wrapped)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close stops the server immediately, dropping active connections.
func (s *Server) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	close(s.shutdown)
	return s.http.Close()
}

// Shutdown stops the server gracefully.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.closed.Swap(true) {
		return nil
	}
	close(s.shutdown)
	return s.http.Shutdown(ctx)
}

func (s *Server) register(ep Endpoint) {
	if ep.Exercises == "" {
		// A programming error, caught at construction rather than by the parity
		// test, so the failure names the endpoint instead of a count mismatch.
		panic(fmt.Sprintf("mudflat: endpoint %q has no Exercises citation (§P3)", ep.Pattern))
	}
	s.endpoints = append(s.endpoints, ep)

	s.mount(ep.Pattern, ep)
	if !ep.SessionScoped() {
		// Stateless endpoints are reachable bare and under a session prefix, so
		// a collection can use one base variable for every request.
		s.mount(sessionPrefix+ep.Pattern, ep)
	}
}

func (s *Server) mount(path string, ep Endpoint) {
	h := s.middleware(ep, ep.Handler)
	if len(ep.Methods) == 0 {
		s.mux.Handle(path, h)
		return
	}
	for _, method := range ep.Methods {
		s.mux.Handle(method+" "+path, h)
	}
}

// middleware reads the body, numbers the request on its connection, and re-arms
// preamble capture once the handler is done.
//
// Reading the body here rather than in each handler is load-bearing: the capture
// state machine may only resume after the body has been consumed, or the body's
// bytes would be captured as if they were the next request's preamble.
func (s *Server) middleware(ep Endpoint, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, hasConn := connFromContext(r.Context())

		reqNum := 0
		if hasConn {
			reqNum = conn.nextRequestNumber()
			defer conn.resume()
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
		if err != nil {
			writeProblem(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body exceeds the %d byte limit", maxRequestBody))
			return
		}

		ctx := context.WithValue(r.Context(), bodyCtxKey{}, body)
		ctx = context.WithValue(ctx, reqNumCtxKey{}, reqNum)
		ctx = context.WithValue(ctx, endpointCtxKey{}, ep)

		next(w, r.WithContext(ctx))
	}
}

func bodyFromContext(ctx context.Context) []byte {
	b, _ := ctx.Value(bodyCtxKey{}).([]byte)
	return b
}

func requestNumberFromContext(ctx context.Context) int {
	n, _ := ctx.Value(reqNumCtxKey{}).(int)
	return n
}

func endpointFromContext(ctx context.Context) Endpoint {
	ep, _ := ctx.Value(endpointCtxKey{}).(Endpoint)
	return ep
}

// sessionID returns the session for this request, from the path or the header.
// The empty string means the request was made to a bare, stateless path.
func sessionID(r *http.Request) string {
	if sid := r.PathValue("sid"); sid != "" {
		return sid
	}
	return r.Header.Get(sessionHeader)
}

// session resolves the request's session, creating it implicitly. Endpoints
// that hold state require one; the error path is the only place a caller learns
// that a malformed id was rejected rather than silently namespaced.
func (s *Server) session(w http.ResponseWriter, r *http.Request) (*Session, bool) {
	sid := sessionID(r)
	if sid == "" {
		writeProblem(w, http.StatusBadRequest,
			"this endpoint is session-scoped: use /s/{session}/… or the "+sessionHeader+" header")
		return nil, false
	}
	sess, err := s.sessions.Get(sid)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return sess, true
}

// writeJSON emits a deterministic JSON body. Indentation is fixed so golden
// transcripts (§13.2) compare byte for byte.
func writeJSON(w http.ResponseWriter, status int, v any) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, `{"error":"marshal failed"}`, http.StatusInternalServerError)
		return
	}
	buf = append(buf, '\n')

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprint(len(buf)))
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

type problem struct {
	Error   string `json:"error"`
	Detail  string `json:"detail,omitempty"`
	Mudflat string `json:"mudflat"`
}

// writeProblem is the single error shape. A test API that reports its own
// misuse ambiguously wastes the time it was built to save.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, problem{
		Error:   http.StatusText(status),
		Detail:  detail,
		Mudflat: Version,
	})
}
