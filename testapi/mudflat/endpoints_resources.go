package mudflat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// registerResources mounts family H (§9.H). These endpoints exist so variable
// chaining has an id that came from somewhere: extraction, dependency analysis,
// and phase ordering all need a value produced by one request and consumed by a
// later one.
func (s *Server) registerResources() {
	s.register(Endpoint{
		Pattern:   sessionPrefix + "/resources",
		Methods:   []string{http.MethodPost},
		Family:    "H",
		Summary:   "Creates a resource and returns it with a deterministic id.",
		Exercises: "Extraction of a created id, and chaining it into a later request item.",
		Handler:   s.handleResourceCreate,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/resources",
		Methods:   []string{http.MethodGet},
		Family:    "H",
		Summary:   "Lists resources with offset or cursor pagination and a Link header.",
		Exercises: "Extraction from arrays, length and contains operators, and cursor loops driven by an extracted value.",
		Handler:   s.handleResourceList,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/resources/{id}",
		Methods:   []string{http.MethodGet},
		Family:    "H",
		Summary:   "Fetches one resource; 404 once deleted.",
		Exercises: "A request whose URL is built from a variable extracted earlier in the run.",
		Handler:   s.handleResourceGet,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/resources/{id}",
		Methods:   []string{http.MethodPut, http.MethodPatch},
		Family:    "H",
		Summary:   "Replaces a resource body and recomputes its ETag.",
		Exercises: "Request bodies built from variables, and assertions on a changed ETag.",
		Handler:   s.handleResourceUpdate,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/resources/{id}",
		Methods:   []string{http.MethodDelete},
		Family:    "H",
		Summary:   "Deletes a resource; 204 the first time, 404 after.",
		Exercises: "Phase ordering: a delete must not run before the fetch that depends on it.",
		Handler:   s.handleResourceDelete,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/etag",
		Methods:   []string{http.MethodGet},
		Family:    "H",
		Summary:   "Returns a document with an ETag; 304 when If-None-Match matches.",
		Exercises: "Conditional requests, and status assertions against 304 — a status with no body.",
		Handler:   s.handleETagGet,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/etag",
		Methods:   []string{http.MethodPut},
		Family:    "H",
		Summary:   "Replaces the document; 412 when If-Match does not match.",
		Exercises: "Precondition failures, which are a distinct retry and assertion case from 4xx generally.",
		Handler:   s.handleETagPut,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/idempotency",
		Methods:   []string{http.MethodPost},
		Family:    "H",
		Summary:   "Replays the first response for a given Idempotency-Key.",
		Exercises: "The non-idempotent-method retry warning: a retried POST that is safe because the server deduplicates.",
		Handler:   s.handleIdempotency,
	})
}

// resourceView is the wire shape. Body is passed through as raw JSON rather than
// being decoded and re-encoded, for the same reason the envelope uses base64.
type resourceView struct {
	ID   string          `json:"id"`
	Seq  int             `json:"seq"`
	Body json.RawMessage `json:"body"`
	ETag string          `json:"etag"`
}

func viewOf(r Resource) resourceView {
	body := r.Body
	if !json.Valid(body) {
		// A non-JSON body would make the response itself invalid. Quote it so
		// the value survives and the caller can see what was stored.
		quoted, err := json.Marshal(string(body))
		if err != nil {
			quoted = []byte(`""`)
		}
		body = quoted
	}
	if len(body) == 0 {
		body = []byte("null")
	}
	return resourceView{ID: r.ID, Seq: r.Seq, Body: body, ETag: r.ETag}
}

func (s *Server) handleResourceCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	created, err := sess.CreateResource(bodyFromContext(r.Context()))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "create: "+err.Error())
		return
	}

	w.Header().Set("Location", strings.TrimSuffix(r.URL.Path, "/")+"/"+created.ID)
	w.Header().Set("ETag", created.ETag)
	writeJSON(w, http.StatusCreated, viewOf(created))
}

func (s *Server) handleResourceGet(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	found, ok := sess.GetResource(id)
	if !ok {
		writeProblem(w, http.StatusNotFound, fmt.Sprintf("no resource %q in session %q", id, sess.ID))
		return
	}

	w.Header().Set("ETag", found.ETag)
	writeJSON(w, http.StatusOK, viewOf(found))
}

func (s *Server) handleResourceUpdate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	updated, ok := sess.UpdateResource(id, bodyFromContext(r.Context()))
	if !ok {
		writeProblem(w, http.StatusNotFound, fmt.Sprintf("no resource %q in session %q", id, sess.ID))
		return
	}

	w.Header().Set("ETag", updated.ETag)
	writeJSON(w, http.StatusOK, viewOf(updated))
}

func (s *Server) handleResourceDelete(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	if !sess.DeleteResource(id) {
		writeProblem(w, http.StatusNotFound, fmt.Sprintf("no resource %q in session %q", id, sess.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type resourcePage struct {
	Items      []resourceView `json:"items"`
	Total      int            `json:"total"`
	Limit      int            `json:"limit"`
	Offset     int            `json:"offset"`
	NextCursor string         `json:"next_cursor"`
}

func (s *Server) handleResourceList(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	all := sess.ListResources()

	limit, err := intParam(r, "limit", 20, 1, 1000)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	// Cursor pagination takes precedence when a cursor is supplied: the cursor
	// is the sequence number of the last item already seen, so a concurrent
	// insert cannot make the walk skip or repeat an item the way an offset can.
	start := 0
	offset := 0
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		after, err := strconv.Atoi(raw)
		if err != nil || after < 0 {
			writeProblem(w, http.StatusBadRequest,
				invalidParam("cursor", raw, "expected the sequence number of the last item seen").Error())
			return
		}
		for i, item := range all {
			if item.Seq > after {
				start = i
				break
			}
			start = i + 1
		}
	} else {
		offset, err = intParam(r, "offset", 0, 0, 1_000_000)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		start = min(offset, len(all))
	}

	end := min(start+limit, len(all))
	window := all[start:end]

	page := resourcePage{
		Items:  make([]resourceView, 0, len(window)),
		Total:  len(all),
		Limit:  limit,
		Offset: offset,
	}
	for _, item := range window {
		page.Items = append(page.Items, viewOf(item))
	}

	if end < len(all) && len(window) > 0 {
		page.NextCursor = strconv.Itoa(window[len(window)-1].Seq)
		next := fmt.Sprintf("%s?limit=%d&cursor=%s", r.URL.Path, limit, page.NextCursor)
		w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"next\"", next))
	}

	writeJSON(w, http.StatusOK, page)
}

// etagDocSlot is the well-known session document the ETag endpoints operate on.
const etagDocSlot = "etag"

func (s *Server) etagDoc(sess *Session) []byte {
	if doc, ok := sess.Doc(etagDocSlot); ok {
		return doc
	}
	// The initial document is fixed rather than generated, so a first GET is
	// byte-identical across runs and sessions.
	initial := []byte(`{"version":1,"note":"initial document"}` + "\n")
	sess.SetDoc(etagDocSlot, initial)
	return initial
}

func (s *Server) handleETagGet(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	doc := s.etagDoc(sess)
	etag := etagFor(doc)

	if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, etag) {
		// 304 carries no body, and must carry the ETag so a client can keep
		// using it for the next conditional request.
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(doc)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(doc)
}

func (s *Server) handleETagPut(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	current := etagFor(s.etagDoc(sess))
	if match := r.Header.Get("If-Match"); match != "" && !etagMatches(match, current) {
		writeProblem(w, http.StatusPreconditionFailed,
			fmt.Sprintf("If-Match %s does not match the current entity tag %s", match, current))
		return
	}

	body := bodyFromContext(r.Context())
	sess.SetDoc(etagDocSlot, body)

	w.Header().Set("ETag", etagFor(body))
	writeJSON(w, http.StatusOK, map[string]any{
		"updated": true,
		"bytes":   len(body),
	})
}

// etagMatches implements the comma-separated list form and the "*" wildcard.
// Weak comparison is not implemented: no endpoint here emits a weak tag.
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

func (s *Server) handleIdempotency(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeProblem(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}

	requestBody := bodyFromContext(r.Context())
	body, replayed := sess.Idempotent(key, func() []byte {
		stored := requestBody
		if !json.Valid(stored) {
			stored = []byte(`null`)
		}
		out, err := json.MarshalIndent(map[string]any{
			"key":      key,
			"received": json.RawMessage(stored),
			"note":     "the first body for this key is what every later call replays",
		}, "", "  ")
		if err != nil {
			return []byte(`{"error":"marshal failed"}`)
		}
		return append(out, '\n')
	})

	w.Header().Set("Idempotent-Replay", strconv.FormatBool(replayed))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
