package mudflat

import (
	"net/http"
	"strconv"
	"time"
)

// registerEcho mounts family A (§9.A). These endpoints report what arrived,
// which makes them the only way to check what curlew actually sent — as opposed
// to what it believes it sent.
func (s *Server) registerEcho() {
	s.register(Endpoint{
		Pattern:   "/echo",
		Family:    "A",
		Summary:   "Returns the echo envelope for any method.",
		Exercises: "Request construction end to end: method, URL building, query merging, header assembly, body serialisation, variable interpolation, faker output, request signing.",
		Handler:   s.handleEcho,
	})

	s.register(Endpoint{
		Pattern:   "/echo/status/{code}",
		Family:    "A",
		Summary:   "Returns the echo envelope with the given status code.",
		Exercises: "Status assertions paired with request inspection, so a failing status can be correlated with what was sent.",
		Handler:   s.handleEchoStatus,
	})

	s.register(Endpoint{
		Pattern:   "/echo/delay/{ms}",
		Family:    "A",
		Summary:   "Returns the echo envelope after a delay, bounded at 120s.",
		Exercises: "timing.max_duration_ms against a known lower bound.",
		Handler:   s.handleEchoDelay,
	})

	s.register(Endpoint{
		Pattern:   "/anything/{rest...}",
		Family:    "A",
		Summary:   "Returns the echo envelope regardless of path depth.",
		Exercises: "URL construction with path variables, and imported OpenAPI paths that do not exist elsewhere in the registry.",
		Handler:   s.handleEcho,
	})
}

func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.envelope(r))
}

func (s *Server) handleEchoStatus(w http.ResponseWriter, r *http.Request) {
	code, err := statusCodeFrom(r.PathValue("code"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, code, s.envelope(r))
}

func (s *Server) handleEchoDelay(w http.ResponseWriter, r *http.Request) {
	d, err := boundedDuration(r.PathValue("ms"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if !sleepOrCancel(r, d) {
		return
	}
	writeJSON(w, http.StatusOK, s.envelope(r))
}

// envelope assembles the response for the request in flight, resolving the
// session lazily: touching it here would create a session for every bare echo
// call and make the LRU meaningless.
func (s *Server) envelope(r *http.Request) Envelope {
	return buildEnvelope(r, bodyFromContext(r.Context()), endpointFromContext(r.Context()).Pattern, sessionID(r))
}

// sleepOrCancel waits for d, or returns false if the client went away first.
// Returning false means no response should be written.
func sleepOrCancel(r *http.Request, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-r.Context().Done():
		return false
	}
}

// boundedDuration parses a millisecond count and clamps it to the §6.4 ceiling.
// Clamping rather than rejecting keeps a mistyped delay from failing a suite for
// the wrong reason, and the ceiling still guarantees termination.
func boundedDuration(raw string) (time.Duration, error) {
	ms, err := strconv.Atoi(raw)
	if err != nil {
		return 0, invalidParam("delay", raw, "expected a whole number of milliseconds")
	}
	if ms < 0 {
		return 0, invalidParam("delay", raw, "must not be negative")
	}
	d := time.Duration(ms) * time.Millisecond
	if d > hardResponseCeiling {
		d = hardResponseCeiling
	}
	return d, nil
}

func statusCodeFrom(raw string) (int, error) {
	code, err := strconv.Atoi(raw)
	if err != nil {
		return 0, invalidParam("status", raw, "expected a whole number")
	}
	if code < 100 || code > 599 {
		return 0, invalidParam("status", raw, "must be between 100 and 599")
	}
	return code, nil
}
