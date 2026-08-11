package mudflat

import (
	"net/http"
)

// registerStatus mounts the status portion of family B (§9.B). Redirects are
// Phase 2; §11.2 records why they need their own pass.
func (s *Server) registerStatus() {
	s.register(Endpoint{
		Pattern:   "/status/{code}",
		Methods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions},
		Family:    "B",
		Summary:   "Returns the requested status code, 100–599. 1xx, 204 and 304 carry no body.",
		Exercises: "Status assertions, including the any-of form (status: [200, 201, 204]), and retry triggers keyed on status_codes and status_ranges.",
		Handler:   s.handleStatus,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	code, err := statusCodeFrom(r.PathValue("code"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	if isBodiless(code) {
		// Sending a body with 204 or 304 desynchronises a keep-alive connection
		// and teaches a client the wrong framing rule. Setting no Content-Type
		// and writing nothing is the whole handler.
		w.WriteHeader(code)
		return
	}

	writeJSON(w, code, map[string]any{
		"status":      code,
		"status_text": http.StatusText(code),
		"method":      r.Method,
	})
}

// isBodiless reports whether the protocol forbids a body for this status.
func isBodiless(code int) bool {
	return code == http.StatusNoContent ||
		code == http.StatusNotModified ||
		(code >= 100 && code < 200)
}
