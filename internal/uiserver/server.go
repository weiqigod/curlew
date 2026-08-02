// Package uiserver implements the localhost HTTP server behind `curlew ui`:
// a thin facade over the existing parser/validator/runner contracts that
// serves the embedded single-page app, a JSON API under /api/v1/, and a
// WebSocket event stream. See docs/UI_SPECIFICATION.md.
//
// Security model (spec §9): the caller binds loopback-only; this package
// enforces Host-header validation (DNS-rebinding defense), a per-start
// session token on every /api/* request, JSON-only writes, and always-on
// redaction. The SPA and its static assets are served without the token.
package uiserver

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/uiserver/assets"
)

// Body-size limits (spec terminology table).
const (
	// InlineBodyLimit is the per-body cap for inline REST detail content.
	InlineBodyLimit = 256 * 1024
	// StoredBodyLimit is the per-body cap for the persisted history store.
	StoredBodyLimit = 1024 * 1024
	// MemoryRuns is the size of the in-memory ring of completed runs.
	MemoryRuns = 5
	// DefaultMaxRuns is the default persisted-history retention.
	DefaultMaxRuns = 50
	// APIVersion is the /api/v1 protocol version reported by /meta.
	APIVersion = 1
	// DefaultPort is the built-in listen port.
	DefaultPort = 8765
)

// Options configures a Server.
type Options struct {
	Root             string // absolute project root (directory of curlew.yaml)
	ProjectName      string
	Version          string             // curlew binary version
	DefaultEnv       string             // --env preselection surfaced via /meta
	CollectionFilter string             // root-relative collection restriction, or ""
	Token            string             // required session token (32 hex chars)
	HistoryEnabled   bool               // config ui.history.enabled
	MaxRuns          int                // history retention; 0 = DefaultMaxRuns
	Exec             runner.ExecuteFunc // nil = httpexec.Execute (injectable for tests)
	EditorCommand    string             // editor template for /open ($CURLEW_EDITOR > ui.editor)
	DevProxy         string             // CURLEW_UI_DEV_PROXY target, or ""
	Diagnostics      func(format string, args ...any)
}

// Server is the curlew ui HTTP server. Construct with NewServer; serve via
// Handler().
type Server struct {
	opts      Options
	startedAt time.Time
	mux       *http.ServeMux
	hub       *Hub
	orch      *Orchestrator
	store     *Store // nil when history is config-disabled
	watcher   *Watcher
}

// NewServer validates opts and builds the route table.
func NewServer(opts Options) (*Server, error) {
	if opts.Root == "" {
		return nil, fmt.Errorf("uiserver: Options.Root is required")
	}
	if opts.Token == "" {
		return nil, fmt.Errorf("uiserver: Options.Token is required")
	}
	if opts.MaxRuns <= 0 {
		opts.MaxRuns = DefaultMaxRuns
	}
	if opts.Diagnostics == nil {
		opts.Diagnostics = func(string, ...any) {}
	}
	s := &Server{
		opts:      opts,
		startedAt: time.Now().UTC(),
	}
	s.hub = newHub()
	if opts.HistoryEnabled {
		store, err := newStore(opts.Root, opts.MaxRuns)
		if err != nil {
			return nil, fmt.Errorf("uiserver: history store: %w", err)
		}
		s.store = store
	}
	s.orch = newOrchestrator(s)
	if w, err := newWatcher(s); err == nil {
		s.watcher = w
	} else {
		opts.Diagnostics("curlew ui: file watcher unavailable: %v", err)
	}
	s.mux = s.routes()
	return s, nil
}

// routes builds the method+pattern route table (Go 1.22 ServeMux).
func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", s.handleMeta)
	mux.HandleFunc("GET /api/v1/tree", s.handleTree)
	mux.HandleFunc("GET /api/v1/environments", s.handleEnvironments)
	mux.HandleFunc("GET /api/v1/validate", s.handleValidate)
	mux.HandleFunc("GET /api/v1/files", s.handleFiles)
	mux.HandleFunc("POST /api/v1/runs", s.handleRunStart)
	mux.HandleFunc("GET /api/v1/runs", s.handleRunsList)
	mux.HandleFunc("GET /api/v1/runs/current", s.handleRunCurrent)
	mux.HandleFunc("GET /api/v1/runs/{run_id}", s.handleRunGet)
	mux.HandleFunc("DELETE /api/v1/runs/{run_id}", s.handleRunDelete)
	mux.HandleFunc("POST /api/v1/runs/{run_id}/cancel", s.handleRunCancel)
	mux.HandleFunc("GET /api/v1/runs/{run_id}/events", s.handleRunEvents)
	mux.HandleFunc("GET /api/v1/runs/{run_id}/requests", s.handleRunRequests)
	mux.HandleFunc("GET /api/v1/runs/{run_id}/requests/{request_id}", s.handleRequestDetail)
	mux.HandleFunc("GET /api/v1/runs/{run_id}/requests/{request_id}/body", s.handleRequestBody)
	mux.HandleFunc("GET /api/v1/compare", s.handleCompare)
	mux.HandleFunc("POST /api/v1/open", s.handleOpen)
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)
	// JSON 404 for anything else under /api.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown API path", "", nil)
	})
	mux.Handle("/", s.assetHandler())
	return mux
}

// Handler returns the full middleware-wrapped handler.
func (s *Server) Handler() http.Handler {
	return s.hostCheckMiddleware(s.tokenMiddleware(s.contentTypeMiddleware(s.mux)))
}

// Shutdown cancels any active run, closes WS connections, and stops the
// watcher. Safe to call once during server termination.
func (s *Server) Shutdown() {
	if s.watcher != nil {
		s.watcher.Close()
	}
	s.orch.Shutdown()
	s.hub.CloseAll()
}

// --- middleware ---

// loopbackHostname reports whether host (no port) is a loopback literal.
func loopbackHostname(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// hostnameOf strips an optional port from a Host header value.
func hostnameOf(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}

// hostCheckMiddleware rejects requests whose Host (and Origin, when present)
// is not loopback — the DNS-rebinding defense (spec §9.2). Applies to every
// request including static assets.
func (s *Server) hostCheckMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHostname(hostnameOf(r.Host)) {
			writeAPIError(w, http.StatusForbidden, "forbidden_origin",
				"request Host is not loopback", "curlew ui only serves localhost", nil)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !loopbackHostname(u.Hostname()) {
				writeAPIError(w, http.StatusForbidden, "forbidden_origin",
					"request Origin is not loopback", "", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// tokenMiddleware requires the session token on every /api/* request.
// Accepted carriers: Authorization: Bearer <t>, X-Curlew-UI-Token, and —
// for the WebSocket upgrade only — the token query parameter (browsers cannot
// set headers on WS upgrades). Static assets are served without the token.
func (s *Server) tokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		token := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if h := r.Header.Get("X-Curlew-UI-Token"); h != "" {
			token = h
		} else if r.URL.Path == "/api/v1/ws" {
			token = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.opts.Token)) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized",
				"missing or invalid session token",
				"restart curlew ui and open the printed URL", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// contentTypeMiddleware enforces application/json on mutating /api requests
// that carry a body (CSRF hardening on top of the token; spec §9.5).
func (s *Server) contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") &&
			(r.Method == http.MethodPost || r.Method == http.MethodDelete) && r.ContentLength != 0 {
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" {
				writeAPIError(w, http.StatusBadRequest, "bad_request",
					"request body must be application/json", "", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// --- JSON envelope ---

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Details any    `json:"details,omitempty"`
}

// writeAPIError writes the error envelope {"error": {...}}.
func writeAPIError(w http.ResponseWriter, status int, code, message, hint string, details any) {
	writeJSON(w, status, map[string]apiErrorBody{"error": {
		Code: code, Message: message, Hint: hint, Details: details,
	}})
}

// writeJSON writes v with the canonical content type.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- assets ---

// assetHandler serves the embedded SPA: hashed assets immutable, index.html
// no-cache, any non-API path without a file extension falls back to
// index.html (the SPA routes via URL hash). In dev-proxy mode every non-API
// path is reverse-proxied instead (spec §3.4).
func (s *Server) assetHandler() http.Handler {
	if s.opts.DevProxy != "" {
		target, err := url.Parse(s.opts.DevProxy)
		if err == nil {
			return newDevProxy(target)
		}
		s.opts.Diagnostics("curlew ui: invalid CURLEW_UI_DEV_PROXY %q: %v", s.opts.DevProxy, err)
	}
	dist := assets.Dist()
	fileServer := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := dist.Open(p); err == nil {
			_ = f.Close()
			if p == "index.html" {
				w.Header().Set("Cache-Control", "no-cache")
			} else if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for unknown non-API paths.
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
