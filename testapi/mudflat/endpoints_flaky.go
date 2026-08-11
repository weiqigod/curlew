package mudflat

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// registerFlaky mounts family I (§9.I): deterministic failure injection.
//
// "Deterministic" is the load-bearing word. Attempt counters are keyed on
// (session, endpoint, query), so a fresh session restarts every sequence and a
// rerun reproduces the previous run exactly. A test API that is itself flaky
// does not test retry; it relocates the doubt.
func (s *Server) registerFlaky() {
	s.register(Endpoint{
		Pattern:   sessionPrefix + "/flaky/fail-then-succeed",
		Family:    "I",
		Summary:   "Fails the first `times` attempts with `status`, then succeeds.",
		Exercises: "retry max_attempts, all three backoff strategies, and jitter — the delays between attempts are observable in the events stream.",
		Handler:   s.handleFailThenSucceed,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/flaky/fail-on",
		Family:    "I",
		Summary:   "Fails on the listed attempt numbers, succeeds on the rest.",
		Exercises: "Retry sequences that recover and fail again, which a simple fail-N-times endpoint cannot express.",
		Handler:   s.handleFailOn,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/flaky/reset-then-succeed",
		Family:    "I",
		Summary:   "Resets the connection for the first `times` attempts, then succeeds.",
		Exercises: "retry_on.network_errors. A reset is classified differently from an HTTP status, and this is the only endpoint that produces one.",
		Handler:   s.handleResetThenSucceed,
	})

	s.register(Endpoint{
		Pattern:   "/retry-after/seconds",
		Methods:   []string{http.MethodGet, http.MethodPost},
		Family:    "I",
		Summary:   "429 with Retry-After as a whole number of seconds.",
		Exercises: "respect_retry_after with the delta-seconds form.",
		Handler:   s.handleRetryAfterSeconds,
	})

	s.register(Endpoint{
		Pattern:   "/retry-after/date",
		Methods:   []string{http.MethodGet, http.MethodPost},
		Family:    "I",
		Summary:   "429 with Retry-After as an HTTP-date.",
		Exercises: "respect_retry_after with the HTTP-date form, which is the branch implementations forget.",
		Handler:   s.handleRetryAfterDate,
	})

	s.register(Endpoint{
		Pattern:   "/retry-after/huge",
		Methods:   []string{http.MethodGet},
		Family:    "I",
		Summary:   "429 with Retry-After: 3600.",
		Exercises: "The 30-second Retry-After clamp (CLI_SPECIFICATION §9.4). Without the clamp this waits an hour, which is unmistakable rather than merely slow.",
		Handler:   s.handleRetryAfterHuge,
	})

	s.register(Endpoint{
		Pattern:   "/retry-after/malformed",
		Methods:   []string{http.MethodGet},
		Family:    "I",
		Summary:   "429 with an unparseable Retry-After value.",
		Exercises: "Fallback to computed backoff when the header cannot be parsed, rather than to no delay at all.",
		Handler:   s.handleRetryAfterMalformed,
	})
}

// attemptKey identifies a failure-injection sequence. The query is sorted so
// that two requests differing only in parameter order share a counter, and two
// differing in any value do not.
func attemptKey(r *http.Request) string {
	values := r.URL.Query()
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(endpointFromContext(r.Context()).Pattern)
	for _, name := range names {
		vs := append([]string(nil), values[name]...)
		sort.Strings(vs)
		fmt.Fprintf(&b, "|%s=%s", name, strings.Join(vs, ","))
	}
	return b.String()
}

// intParam reads a whole-number query parameter, applying fallback when absent.
func intParam(r *http.Request, name string, fallback, min, max int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, invalidParam(name, raw, "expected a whole number")
	}
	if n < min || n > max {
		return 0, invalidParam(name, raw, fmt.Sprintf("must be between %d and %d", min, max))
	}
	return n, nil
}

type flakyResult struct {
	Attempt  int    `json:"attempt"`
	Failing  bool   `json:"failing"`
	Endpoint string `json:"endpoint"`
	Note     string `json:"note"`
}

func (s *Server) handleFailThenSucceed(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	times, err := intParam(r, "times", 1, 0, 100)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := intParam(r, "status", http.StatusServiceUnavailable, 100, 599)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	attempt := sess.Attempt(attemptKey(r))
	failing := attempt <= times

	code := http.StatusOK
	note := fmt.Sprintf("attempt %d of a sequence that fails %d time(s)", attempt, times)
	if failing {
		code = status
	}

	writeJSON(w, code, flakyResult{
		Attempt:  attempt,
		Failing:  failing,
		Endpoint: endpointFromContext(r.Context()).Pattern,
		Note:     note,
	})
}

func (s *Server) handleFailOn(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	raw := r.URL.Query().Get("attempts")
	if raw == "" {
		raw = "1"
	}
	failOn := map[int]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			writeProblem(w, http.StatusBadRequest,
				invalidParam("attempts", raw, "expected a comma-separated list of positive whole numbers").Error())
			return
		}
		failOn[n] = true
	}

	status, err := intParam(r, "status", http.StatusInternalServerError, 100, 599)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	attempt := sess.Attempt(attemptKey(r))
	failing := failOn[attempt]

	code := http.StatusOK
	if failing {
		code = status
	}
	writeJSON(w, code, flakyResult{
		Attempt:  attempt,
		Failing:  failing,
		Endpoint: endpointFromContext(r.Context()).Pattern,
		Note:     "failing on attempts " + raw,
	})
}

func (s *Server) handleResetThenSucceed(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	times, err := intParam(r, "times", 1, 0, 100)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	attempt := sess.Attempt(attemptKey(r))
	if attempt > times {
		writeJSON(w, http.StatusOK, flakyResult{
			Attempt:  attempt,
			Failing:  false,
			Endpoint: endpointFromContext(r.Context()).Pattern,
			Note:     fmt.Sprintf("connection was reset on the first %d attempt(s)", times),
		})
		return
	}

	if err := resetConnection(w); err != nil {
		writeProblem(w, http.StatusInternalServerError, "cannot reset this connection: "+err.Error())
	}
}

// resetConnection aborts the TCP connection with an RST rather than a clean
// FIN, so the client sees a network error instead of a truncated response.
//
// SetLinger(0) is what makes Close send RST. The connection has to be unwrapped
// first: Hijack hands back the capturedConn wrapper, not the *net.TCPConn.
func resetConnection(w http.ResponseWriter) error {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return fmt.Errorf("ResponseWriter does not support hijacking")
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		return fmt.Errorf("hijack: %w", err)
	}
	defer func() { _ = conn.Close() }()

	underlying := conn
	if wrapped, ok := conn.(*capturedConn); ok {
		underlying = wrapped.Conn
	}
	tcp, ok := underlying.(*net.TCPConn)
	if !ok {
		return fmt.Errorf("connection is %T, not *net.TCPConn", underlying)
	}
	if err := tcp.SetLinger(0); err != nil {
		return fmt.Errorf("set linger: %w", err)
	}
	return nil
}

func writeRetryAfter(w http.ResponseWriter, value string, note string) {
	w.Header().Set("Retry-After", value)
	writeJSON(w, http.StatusTooManyRequests, map[string]string{
		"error":       "Too Many Requests",
		"retry_after": value,
		"note":        note,
	})
}

func (s *Server) handleRetryAfterSeconds(w http.ResponseWriter, r *http.Request) {
	n, err := intParam(r, "n", 1, 0, 86400)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeRetryAfter(w, strconv.Itoa(n), "delta-seconds form")
}

func (s *Server) handleRetryAfterDate(w http.ResponseWriter, r *http.Request) {
	ms, err := intParam(r, "ms", 1000, 0, 86400000)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	// This endpoint's contract is about time, which is the §6.1 exemption that
	// lets it read the clock. Truncated to the second because HTTP-date has no
	// finer resolution.
	at := time.Now().UTC().Add(time.Duration(ms) * time.Millisecond).Truncate(time.Second)
	writeRetryAfter(w, at.Format(http.TimeFormat), "HTTP-date form")
}

func (s *Server) handleRetryAfterHuge(w http.ResponseWriter, _ *http.Request) {
	writeRetryAfter(w, "3600", "must be clamped to 30s by the client (CLI_SPECIFICATION §9.4)")
}

func (s *Server) handleRetryAfterMalformed(w http.ResponseWriter, _ *http.Request) {
	writeRetryAfter(w, "soon", "unparseable: the client should fall back to computed backoff, not to no delay")
}
