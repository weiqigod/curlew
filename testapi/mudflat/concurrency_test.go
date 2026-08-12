package mudflat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Concurrency observability (§9.J).
//
// The barrier is the reason this family exists. "Is --parallel actually
// parallel?" was previously answered by comparing wall-clock durations, and this
// repository has already had to delete one such comparison as flaky (M21-003).
// A barrier turns it into a positive proof: with n set to the expected worker
// count, a serial implementation cannot pass — it times out, and the timeout
// message says how many workers actually showed up.

func TestBarrier_ReleasesWhenTheExpectedCountArrives(t *testing.T) {
	base, client := startServer(t)
	url := base + "/s/barrier-ok/barrier/4?timeout_ms=5000"

	type outcome struct {
		status int
		body   barrierResult
	}
	results := make([]outcome, 4)

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("request %d: %v", i, err)
				return
			}
			defer func() { _ = resp.Body.Close() }()

			var out barrierResult
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Errorf("request %d decode: %v", i, err)
				return
			}
			results[i] = outcome{status: resp.StatusCode, body: out}
		}(i)
	}
	wg.Wait()

	seen := map[int]bool{}
	for i, r := range results {
		if r.status != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i, r.status)
		}
		if r.body.Expected != 4 {
			t.Errorf("request %d: expected = %d, want 4", i, r.body.Expected)
		}
		if r.body.Arrived != 4 {
			t.Errorf("request %d: arrived = %d, want 4 — all four should be released together",
				i, r.body.Arrived)
		}
		seen[r.body.ArrivalIndex] = true
	}
	for want := 1; want <= 4; want++ {
		if !seen[want] {
			t.Errorf("no request reported arrival index %d; indices were %v", want, seen)
		}
	}
}

func TestBarrier_TimesOutAndSaysHowManyArrived(t *testing.T) {
	// The diagnostic that matters. A serial client reaches the barrier alone,
	// and the response must say so rather than just failing.
	base, client := startServer(t)

	resp, body := get(t, client, base+"/s/barrier-lonely/barrier/8?timeout_ms=300")

	if resp.StatusCode != http.StatusRequestTimeout {
		t.Errorf("status = %d, want 408", resp.StatusCode)
	}

	var out barrierResult
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if out.Expected != 8 {
		t.Errorf("expected = %d, want 8", out.Expected)
	}
	if out.Arrived != 1 {
		t.Errorf("arrived = %d, want 1", out.Arrived)
	}
	if out.Detail == "" {
		t.Error("no detail explaining the timeout; this is the message that tells a "+
			"developer their run was serial", out.Detail)
	}
}

func TestBarrier_IsScopedToASession(t *testing.T) {
	base, client := startServer(t)

	// Two requests to the same barrier size in different sessions must not
	// satisfy each other.
	done := make(chan int, 2)
	for _, sid := range []string{"barrier-a", "barrier-b"} {
		go func(sid string) {
			resp, _ := get(t, client, fmt.Sprintf("%s/s/%s/barrier/2?timeout_ms=300", base, sid))
			done <- resp.StatusCode
		}(sid)
	}

	for range 2 {
		select {
		case status := <-done:
			if status != http.StatusRequestTimeout {
				t.Errorf("status = %d, want 408 — sessions must not share a barrier", status)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("barrier did not time out")
		}
	}
}

func TestBarrier_RejectsAnUnusableSize(t *testing.T) {
	base, client := startServer(t)

	for _, n := range []string{"0", "-1", "abc", "1001"} {
		t.Run(n, func(t *testing.T) {
			resp, _ := get(t, client, base+"/s/barrier-bad/barrier/"+n+"?timeout_ms=200")
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestConcurrency_ReportsMaximumObserved(t *testing.T) {
	base, client := startServer(t)
	url := base + "/s/conc-max/concurrency?hold_ms=120"

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			_, _ = resp.Body.Read(make([]byte, 1))
		}()
	}
	wg.Wait()

	_, body := get(t, client, base+"/s/conc-max/concurrency")
	var out concurrencyResult
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if out.MaxObserved < 2 {
		t.Errorf("max_observed = %d after five overlapping requests, want at least 2",
			out.MaxObserved)
	}
	if out.Total < 6 {
		t.Errorf("total = %d, want at least 6", out.Total)
	}
}

func TestConcurrency_IsScopedToASession(t *testing.T) {
	base, client := startServer(t)

	get(t, client, base+"/s/conc-one/concurrency")
	get(t, client, base+"/s/conc-one/concurrency")

	_, body := get(t, client, base+"/s/conc-two/concurrency")
	var out concurrencyResult
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 1 {
		t.Errorf("total = %d in a fresh session, want 1", out.Total)
	}
}

func TestSerialize_RejectsOverlappingRequests(t *testing.T) {
	base, client := startServer(t)
	url := base + "/s/serial/serialize?hold_ms=200"

	statuses := make(chan int, 2)
	for range 2 {
		go func() {
			resp, err := client.Get(url)
			if err != nil {
				statuses <- -1
				return
			}
			defer func() { _ = resp.Body.Close() }()
			statuses <- resp.StatusCode
		}()
	}

	var ok, conflict int
	for range 2 {
		switch <-statuses {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		}
	}

	if ok != 1 || conflict != 1 {
		t.Errorf("got %d OK and %d conflict, want exactly one of each", ok, conflict)
	}
}

func TestRateLimit_AllowsBurstThenRejects(t *testing.T) {
	// rate=0 means no refill, which makes the sequence deterministic: exactly
	// `burst` requests succeed and every later one is refused. A wall-clock
	// refill would make this endpoint's own test flaky, which §6.1 forbids.
	base, client := startServer(t)
	url := base + "/s/rl-burst/ratelimit?rate=0&burst=3"

	var codes []int
	for range 5 {
		resp, _ := get(t, client, url)
		codes = append(codes, resp.StatusCode)
	}

	want := []int{200, 200, 200, 429, 429}
	for i := range want {
		if codes[i] != want[i] {
			t.Errorf("request %d: status = %d, want %d (sequence %v)", i+1, codes[i], want[i], codes)
		}
	}
}

func TestRateLimit_ReportsStandardHeaders(t *testing.T) {
	base, client := startServer(t)

	resp, _ := get(t, client, base+"/s/rl-headers/ratelimit?rate=0&burst=2")
	for _, name := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining"} {
		if resp.Header.Get(name) == "" {
			t.Errorf("no %s header", name)
		}
	}
	if got := resp.Header.Get("X-RateLimit-Limit"); got != "2" {
		t.Errorf("X-RateLimit-Limit = %q, want 2", got)
	}
	if got := resp.Header.Get("X-RateLimit-Remaining"); got != "1" {
		t.Errorf("X-RateLimit-Remaining = %q, want 1 after the first of two", got)
	}
}

func TestRateLimit_RejectionCarriesRetryAfter(t *testing.T) {
	base, client := startServer(t)
	url := base + "/s/rl-retry/ratelimit?rate=2&burst=1"

	get(t, client, url) // consume the burst
	resp, _ := get(t, client, url)

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("a 429 with a known refill rate should say when to come back")
	}
}

func TestRateLimit_IsScopedToASession(t *testing.T) {
	base, client := startServer(t)

	for range 3 {
		get(t, client, base+"/s/rl-one/ratelimit?rate=0&burst=1")
	}

	resp, _ := get(t, client, base+"/s/rl-two/ratelimit?rate=0&burst=1")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d in a fresh session, want 200 — buckets must not be shared",
			resp.StatusCode)
	}
}
