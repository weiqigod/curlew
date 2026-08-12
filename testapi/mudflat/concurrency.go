package mudflat

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Concurrency observability and rate limiting (§9.J).
//
// The barrier is the important one. It converts "is --parallel actually
// parallel?" from a timing heuristic into a positive proof: with n set to the
// expected worker count, a serial implementation cannot pass — it blocks alone
// until the deadline, and the timeout response says how many workers arrived.
//
// A wall-clock speedup comparison cannot make that claim. This repository has
// already had to remove one such test as flaky (M21-003).

const (
	defaultBarrierTimeoutMs = 5000
	maxBarrierSize          = 1000
)

// barrier is a rendezvous for n requests. The last arrival closes release,
// which frees every waiter at once.
type barrier struct {
	expected int
	arrived  int
	release  chan struct{}
}

// concurrencyState is the per-session bookkeeping for this family. It lives
// beside the session rather than globally, so two sessions cannot see each
// other's counters (§6.2).
type concurrencyState struct {
	mu sync.Mutex

	barriers map[string]*barrier

	inFlight    int
	maxObserved int
	total       int

	serializeBusy bool

	buckets map[string]*tokenBucket
}

// tokenBucket is a plain token bucket. With rate 0 it never refills, which is
// what makes the dogfood assertion deterministic: exactly `burst` requests
// succeed and every later one is refused, with no clock involved.
type tokenBucket struct {
	limit      int
	tokens     float64
	ratePerSec float64
	last       time.Time
}

func newConcurrencyState() *concurrencyState {
	return &concurrencyState{
		barriers: make(map[string]*barrier),
		buckets:  make(map[string]*tokenBucket),
	}
}

type barrierResult struct {
	Expected     int    `json:"expected"`
	Arrived      int    `json:"arrived"`
	ArrivalIndex int    `json:"arrival_index"`
	Released     bool   `json:"released"`
	Session      string `json:"session"`
	Detail       string `json:"detail,omitempty"`
}

type concurrencyResult struct {
	Current     int    `json:"current"`
	MaxObserved int    `json:"max_observed"`
	Total       int    `json:"total"`
	Session     string `json:"session"`
}

func (s *Server) registerConcurrency() {
	s.register(Endpoint{
		Pattern:   sessionPrefix + "/barrier/{n}",
		Methods:   []string{http.MethodGet, http.MethodPost},
		Family:    "J",
		Summary:   "Blocks until n requests arrive concurrently, then releases them all.",
		Exercises: "--parallel worker counts and perf --vus, as a positive proof. A serial client cannot pass this: it blocks alone and the 408 reports how many arrived.",
		Handler:   s.handleBarrier,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/concurrency",
		Methods:   []string{http.MethodGet},
		Family:    "J",
		Summary:   "Reports current and maximum observed concurrency; hold_ms keeps the request in flight.",
		Exercises: "Whether a declared worker count is the count actually reached, without inferring it from durations.",
		Handler:   s.handleConcurrency,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/serialize",
		Methods:   []string{http.MethodGet, http.MethodPost},
		Family:    "J",
		Summary:   "Returns 409 if a second request overlaps the first.",
		Exercises: "The inverse of the barrier: dependency ordering and phase separation, where two items must NOT run at once.",
		Handler:   s.handleSerialize,
	})

	s.register(Endpoint{
		Pattern:   sessionPrefix + "/ratelimit",
		Methods:   []string{http.MethodGet, http.MethodPost},
		Family:    "J",
		Summary:   "Token bucket with X-RateLimit-* headers; rate=0 never refills.",
		Exercises: "internal/ratelimit against a server that actually enforces. A client-side limiter that does not throttle shows up as 429s the server should never have needed to send.",
		Handler:   s.handleRateLimit,
	})
}

func (s *Server) handleBarrier(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 || n > maxBarrierSize {
		writeProblem(w, http.StatusBadRequest,
			invalidParam("n", r.PathValue("n"),
				fmt.Sprintf("expected a whole number between 1 and %d", maxBarrierSize)).Error())
		return
	}

	timeoutMs, err := intParam(r, "timeout_ms", defaultBarrierTimeoutMs, 1, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	state := sess.Concurrency()
	b, index := state.arrive(strconv.Itoa(n), n)

	if index == n {
		// The last arrival releases everyone, itself included.
		close(b.release)
	}

	timer := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-b.release:
		writeJSON(w, http.StatusOK, barrierResult{
			Expected:     n,
			Arrived:      n,
			ArrivalIndex: index,
			Released:     true,
			Session:      sess.ID,
		})

	case <-timer.C:
		arrived := state.barrierArrived(strconv.Itoa(n))
		state.abandonBarrier(strconv.Itoa(n))
		writeJSON(w, http.StatusRequestTimeout, barrierResult{
			Expected:     n,
			Arrived:      arrived,
			ArrivalIndex: index,
			Released:     false,
			Session:      sess.ID,
			Detail: fmt.Sprintf(
				"waited %dms for %d concurrent requests and saw %d. "+
					"If this run was meant to be parallel, it was not: the barrier is a "+
					"positive proof of concurrency and cannot be satisfied one request at a time.",
				timeoutMs, n, arrived),
		})

	case <-r.Context().Done():
		state.abandonBarrier(strconv.Itoa(n))
	}
}

func (s *Server) handleConcurrency(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	holdMs, err := intParam(r, "hold_ms", 0, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	state := sess.Concurrency()
	state.enter()
	defer state.leave()

	if !sleepOrCancel(r, time.Duration(holdMs)*time.Millisecond) {
		return
	}

	current, maxObserved, total := state.snapshot()
	writeJSON(w, http.StatusOK, concurrencyResult{
		Current:     current,
		MaxObserved: maxObserved,
		Total:       total,
		Session:     sess.ID,
	})
}

func (s *Server) handleSerialize(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	holdMs, err := intParam(r, "hold_ms", 100, 0, int(hardResponseCeiling/time.Millisecond))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	state := sess.Concurrency()
	if !state.claimSerialize() {
		writeProblem(w, http.StatusConflict,
			"another request to this endpoint is already in flight in this session")
		return
	}
	defer state.releaseSerialize()

	if !sleepOrCancel(r, time.Duration(holdMs)*time.Millisecond) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"serialized": true,
		"session":    sess.ID,
	})
}

func (s *Server) handleRateLimit(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r)
	if !ok {
		return
	}

	burst, err := intParam(r, "burst", 5, 1, 10000)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	rate, err := intParam(r, "rate", 0, 0, 10000)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	key := fmt.Sprintf("rate=%d,burst=%d", rate, burst)
	allowed, remaining, retryAfter := sess.Concurrency().take(key, burst, float64(rate), s.now())

	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(burst))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

	if !allowed {
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		}
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":     "Too Many Requests",
			"limit":     burst,
			"remaining": remaining,
			"note":      "rate=0 never refills, which is what makes this sequence reproducible",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"allowed":   true,
		"limit":     burst,
		"remaining": remaining,
		"session":   sess.ID,
	})
}

// --- state ------------------------------------------------------------------

// arrive registers this request at the barrier and returns it with the caller's
// 1-based arrival index.
func (c *concurrencyState) arrive(key string, expected int) (*barrier, int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, ok := c.barriers[key]
	if !ok {
		b = &barrier{expected: expected, release: make(chan struct{})}
		c.barriers[key] = b
	}
	b.arrived++
	return b, b.arrived
}

func (c *concurrencyState) barrierArrived(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if b, ok := c.barriers[key]; ok {
		return b.arrived
	}
	return 0
}

// abandonBarrier discards a barrier that timed out, so a later attempt in the
// same session starts from zero rather than inheriting stale arrivals.
func (c *concurrencyState) abandonBarrier(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.barriers, key)
}

func (c *concurrencyState) enter() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inFlight++
	c.total++
	if c.inFlight > c.maxObserved {
		c.maxObserved = c.inFlight
	}
}

func (c *concurrencyState) leave() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inFlight--
}

func (c *concurrencyState) snapshot() (current, maxObserved, total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inFlight, c.maxObserved, c.total
}

func (c *concurrencyState) claimSerialize() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serializeBusy {
		return false
	}
	c.serializeBusy = true
	return true
}

func (c *concurrencyState) releaseSerialize() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.serializeBusy = false
}

// take consumes one token, refilling first at ratePerSec. Returns whether the
// request is allowed, the tokens left, and how many seconds until the next one.
func (c *concurrencyState) take(key string, burst int, ratePerSec float64, now time.Time) (allowed bool, remaining, retryAfterSec int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, ok := c.buckets[key]
	if !ok {
		b = &tokenBucket{limit: burst, tokens: float64(burst), ratePerSec: ratePerSec, last: now}
		c.buckets[key] = b
	}

	if b.ratePerSec > 0 {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens = min(float64(b.limit), b.tokens+elapsed*b.ratePerSec)
	}
	b.last = now

	if b.tokens < 1 {
		retry := 1
		if b.ratePerSec > 0 {
			retry = max(1, int((1-b.tokens)/b.ratePerSec+0.999))
		}
		return false, 0, retry
	}

	b.tokens--
	return true, int(b.tokens), 0
}
