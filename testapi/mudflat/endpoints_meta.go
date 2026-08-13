package mudflat

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/weiqigod/curlew/testapi/openapi"
)

// registerMeta mounts the self-description surface (§6.5). The index is derived
// from the same registry that drives routing, so it cannot describe an endpoint
// that does not exist or omit one that does.
func (s *Server) registerMeta() {
	s.register(Endpoint{
		Pattern:   "/{$}",
		Methods:   []string{http.MethodGet},
		Family:    "meta",
		Summary:   "Index of every endpoint with its contract and cited curlew behaviour.",
		Exercises: "Self-description (§6.5). Consumed by the anti-bloat parity test (§16).",
		Handler:   s.handleIndex,
	})

	s.register(Endpoint{
		Pattern:   "/openapi.json",
		Methods:   []string{http.MethodGet},
		Family:    "P",
		Summary:   "The hand-written OpenAPI 3.1 document describing a slice of this server.",
		Exercises: "`curlew import openapi` end to end: import this document, run what comes out, and require it to pass against the server it describes. The document is hand-written rather than generated from the handlers, so the round trip is evidence rather than a tautology (§9.P).",
		Handler:   s.handleOpenAPI,
	})

	s.register(Endpoint{
		Pattern:   "/capabilities",
		Methods:   []string{http.MethodGet},
		Family:    "meta",
		Summary:   "Families enabled in this instance, and session-store state.",
		Exercises: "Self-description (§6.5). Reports session evictions so a suite that trips the cap can find out.",
		Handler:   s.handleCapabilities,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix,
		Methods:   []string{http.MethodDelete},
		Family:    "meta",
		Summary:   "Resets a session to empty. Idempotent.",
		Exercises: "Session isolation (§7): lets a collection start from a known-empty namespace.",
		Handler:   s.handleSessionReset,
	})
}

// IndexEntry is the public description of one endpoint.
type IndexEntry struct {
	Pattern         string   `json:"pattern"`
	Methods         []string `json:"methods"`
	Family          string   `json:"family"`
	Summary         string   `json:"summary"`
	Exercises       string   `json:"exercises"`
	SessionScoped   bool     `json:"session_scoped"`
	SessionOptional bool     `json:"session_optional"`
}

// Index returns the registry in a stable order. Sorted by pattern so two runs
// produce identical bytes, per §6.1.
func (s *Server) Index() []IndexEntry {
	entries := make([]IndexEntry, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		methods := ep.Methods
		if len(methods) == 0 {
			methods = []string{"ANY"}
		}
		entries = append(entries, IndexEntry{
			Pattern:         ep.Pattern,
			Methods:         methods,
			Family:          ep.Family,
			Summary:         ep.Summary,
			Exercises:       ep.Exercises,
			SessionScoped:   ep.SessionScoped(),
			SessionOptional: !ep.SessionScoped(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Pattern != entries[j].Pattern {
			return entries[i].Pattern < entries[j].Pattern
		}
		return fmt.Sprint(entries[i].Methods) < fmt.Sprint(entries[j].Methods)
	})
	return entries
}

type indexDocument struct {
	Name      string       `json:"name"`
	Version   string       `json:"version"`
	Spec      string       `json:"spec"`
	Endpoints []IndexEntry `json:"endpoints"`
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, indexDocument{
		Name:      "mudflat",
		Version:   Version,
		Spec:      "docs/TESTAPI_SPECIFICATION.md",
		Endpoints: s.Index(),
	})
}

type capabilitiesDocument struct {
	Name     string         `json:"name"`
	Version  string         `json:"version"`
	Families []string       `json:"families"`
	Phase    int            `json:"phase"`
	Sessions sessionsReport `json:"sessions"`
	Absent   map[string]string
}

type sessionsReport struct {
	Active    int `json:"active"`
	Evictions int `json:"evictions"`
}

func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	seen := map[string]bool{}
	var families []string
	for _, ep := range s.endpoints {
		if !seen[ep.Family] {
			seen[ep.Family] = true
			families = append(families, ep.Family)
		}
	}
	sort.Strings(families)

	writeJSON(w, http.StatusOK, capabilitiesDocument{
		Name:     "mudflat",
		Version:  Version,
		Families: families,
		Phase:    1,
		Sessions: sessionsReport{
			Active:    s.sessions.Len(),
			Evictions: s.sessions.Evictions(),
		},
		Absent: map[string]string{
			"raw":       "Phase 2 — adversarial framing (§9.E)",
			"verify":    "Phase 2 — signature verification (§9.G)",
			"barrier":   "Phase 2 — concurrency observability (§9.J)",
			"tls":       "Phase 3 — certificate postures (§9.N)",
			"websocket": "Phase 3 — §9.L",
			"graphql":   "Phase 3 — §9.K",
			"streaming": "Phase 3 — §9.M",
		},
	})
}

func (s *Server) handleSessionReset(w http.ResponseWriter, r *http.Request) {
	sid := sessionID(r)
	if err := ValidateSessionID(sid); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	s.sessions.Reset(sid)
	w.WriteHeader(http.StatusNoContent)
}

// invalidParam is the single shape for a rejected path or query parameter, so
// every such failure names the parameter, the value, and the rule.
func invalidParam(name, value, rule string) error {
	return fmt.Errorf("invalid %s %q: %s", name, value, rule)
}

// handleOpenAPI serves the document verbatim. Verbatim matters: the round trip
// in §9.P compares what an import produces against the server it describes, and
// a document rewritten on the way out would be describing something else.
func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprint(len(openapi.Document)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapi.Document)
}
