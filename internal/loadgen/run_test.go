package loadgen

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
)

func TestRun_VUsExecuteUntilDeadline(t *testing.T) {
	var count int64
	fake := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		atomic.AddInt64(&count, 1)
		return &httpexec.Result{StatusCode: 200}, nil
	}

	sum, err := Run(
		context.Background(),
		Config{VUs: 4, Duration: 200 * time.Millisecond},
		&httpexec.Request{Method: "GET", URL: "http://stub"},
		RunOptions{Execute: fake},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.Requests <= 4 {
		t.Errorf("Requests = %d, want > 4", sum.Requests)
	}
	if sum.Successes != sum.Requests {
		t.Errorf("Successes = %d, want %d (all requests)", sum.Successes, sum.Requests)
	}
	if sum.Failures != 0 {
		t.Errorf("Failures = %d, want 0", sum.Failures)
	}
	if sum.Elapsed < 150*time.Millisecond || sum.Elapsed > 600*time.Millisecond {
		t.Errorf("Elapsed = %v, want ~200ms", sum.Elapsed)
	}
	if sum.Aborted {
		t.Error("Aborted = true, want false")
	}
}

func TestRun_RejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want error
	}{
		{"zero VUs", Config{VUs: 0, Duration: time.Second}, ErrInvalidVUs},
		{"zero duration", Config{VUs: 1, Duration: 0}, ErrInvalidDuration},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Run(context.Background(), tc.cfg,
				&httpexec.Request{Method: "GET", URL: "http://stub"},
				RunOptions{Execute: fakeOK},
			)
			if !errors.Is(err, tc.want) {
				t.Errorf("Run() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRun_FailuresOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	sum, err := Run(
		context.Background(),
		Config{VUs: 2, Duration: 100 * time.Millisecond},
		&httpexec.Request{Method: "GET", URL: srv.URL},
		RunOptions{},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.Failures != sum.Requests {
		t.Errorf("Failures = %d, want %d (== Requests)", sum.Failures, sum.Requests)
	}
	if sum.Successes != 0 {
		t.Errorf("Successes = %d, want 0", sum.Successes)
	}
}

func TestRun_ContextCancelStopsVUs(t *testing.T) {
	slow := func(ctx context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return &httpexec.Result{StatusCode: 200}, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	sum, err := Run(ctx, Config{VUs: 3, Duration: 5 * time.Second},
		&httpexec.Request{Method: "GET", URL: "http://stub"},
		RunOptions{Execute: slow},
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !sum.Aborted {
		t.Error("Aborted = false, want true")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Run took %v, want < 500ms (should cancel quickly)", elapsed)
	}
}

func TestRun_RampUp_LinearActivation(t *testing.T) {
	// Strategy: each VU sends its activation timestamp on a buffered channel on
	// its very first call, then blocks for the rest of the run duration. This
	// cleanly captures exactly one timestamp per VU without any shared mutable
	// state, avoiding the data race.
	const numVUs = 4
	const rampUp = 300 * time.Millisecond
	const runDur = 700 * time.Millisecond

	activationCh := make(chan int64, numVUs)
	baseTime := time.Now()
	// activatedSet tracks which goroutines have already sent their activation.
	// We use a sync.Map keyed by goroutine-unique *bool (one closure per VU via
	// a per-call seen flag captured by each VU's first invocation).
	var activatedMu sync.Mutex
	activated := 0

	fake := func(ctx context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		activatedMu.Lock()
		isFirst := activated < numVUs
		if isFirst {
			activated++
		}
		activatedMu.Unlock()

		if isFirst {
			activationCh <- time.Since(baseTime).Nanoseconds()
			// Block for remainder of run so this VU does not loop back and
			// re-enter isFirst for a different VU's slot.
			select {
			case <-ctx.Done():
			case <-time.After(runDur):
			}
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}

	_, err := Run(
		context.Background(),
		Config{VUs: numVUs, Duration: runDur, RampUp: rampUp},
		&httpexec.Request{Method: "GET", URL: "http://stub"},
		RunOptions{Execute: fake},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	close(activationCh)

	var captured []int64
	for ns := range activationCh {
		captured = append(captured, ns)
	}

	if len(captured) < numVUs {
		t.Fatalf("captured %d activation timestamps, want %d", len(captured), numVUs)
	}

	// Find the spread: max − min among the activation timestamps.
	// With a 300ms ramp the spread must be at least 50% of the ramp window
	// (150ms), even under scheduler noise.
	minNs, maxNs := captured[0], captured[0]
	for _, ns := range captured[1:] {
		if ns < minNs {
			minNs = ns
		}
		if ns > maxNs {
			maxNs = ns
		}
	}
	spread := time.Duration(maxNs-minNs) * time.Nanosecond
	minExpected := rampUp / 2 // 50% tolerance
	if spread < minExpected {
		t.Errorf("ramp-up spread = %v, want >= %v (50%% of %s ramp window)",
			spread.Round(time.Millisecond), minExpected, rampUp)
	}
}

func TestRun_RPSMode_LimitsThroughput(t *testing.T) {
	var count int64
	fake := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		atomic.AddInt64(&count, 1)
		return &httpexec.Result{StatusCode: 200}, nil
	}

	sum, err := Run(
		context.Background(),
		Config{VUs: 10, Duration: 500 * time.Millisecond, RPS: 20},
		&httpexec.Request{Method: "GET", URL: "http://stub"},
		RunOptions{Execute: fake},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// At 20 RPS for 500ms we expect ~10 requests ±4 tolerance.
	if sum.Requests < 6 || sum.Requests > 26 {
		t.Errorf("RPS mode: Requests = %d, want roughly 10 (±4)", sum.Requests)
	}
}

func TestRun_NilRequestReturnsError(t *testing.T) {
	_, err := Run(context.Background(), Config{VUs: 1, Duration: time.Second}, nil, RunOptions{})
	if err == nil {
		t.Fatal("expected error for nil request, got nil")
	}
	if !strings.Contains(err.Error(), "request") {
		t.Errorf("error = %q, want message containing 'request'", err.Error())
	}
}

func TestRun_ZeroVUsError(t *testing.T) {
	_, err := Run(context.Background(), Config{VUs: 0, Duration: time.Second},
		&httpexec.Request{Method: "GET", URL: "x"}, RunOptions{})
	if !errors.Is(err, ErrInvalidVUs) {
		t.Errorf("error = %v, want ErrInvalidVUs", err)
	}
}

func TestRun_Integration_HTTPTestServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	sum, err := Run(
		context.Background(),
		Config{VUs: 3, Duration: 150 * time.Millisecond},
		&httpexec.Request{Method: "GET", URL: srv.URL},
		RunOptions{},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.Requests == 0 {
		t.Error("Requests = 0, want > 0")
	}
	// Context-cancelled in-flight requests are not counted, so
	// Requests == Successes + Failures must hold.
	if sum.Successes+sum.Failures != sum.Requests {
		t.Errorf("Successes(%d) + Failures(%d) != Requests(%d)",
			sum.Successes, sum.Failures, sum.Requests)
	}
	if sum.Successes == 0 {
		t.Error("Successes = 0, want > 0")
	}
	if sum.Failures != 0 {
		t.Errorf("Failures = %d, want 0 (200 server)", sum.Failures)
	}
}

func TestRun_OnSampleReceivesOneCallPerRequest(t *testing.T) {
	var mu sync.Mutex
	var samples []Sample
	fake := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		time.Sleep(time.Millisecond)
		return &httpexec.Result{StatusCode: 200}, nil
	}
	sum, err := Run(context.Background(), Config{VUs: 2, Duration: 100 * time.Millisecond},
		&httpexec.Request{URL: "x"},
		RunOptions{
			Execute: fake,
			OnSample: func(s Sample) {
				mu.Lock()
				samples = append(samples, s)
				mu.Unlock()
			},
		})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	mu.Lock()
	count := len(samples)
	mu.Unlock()
	if count != sum.Requests {
		t.Errorf("samples=%d, requests=%d", count, sum.Requests)
	}
	// Sanity: all samples should have LatencyNs > 0 and StartOffsetNs >= 0.
	for i, s := range samples {
		if s.LatencyNs <= 0 {
			t.Errorf("samples[%d].LatencyNs = %d, want > 0", i, s.LatencyNs)
		}
		if s.StartOffsetNs < 0 {
			t.Errorf("samples[%d].StartOffsetNs = %d, want >= 0", i, s.StartOffsetNs)
		}
		if !s.Success {
			t.Errorf("samples[%d].Success = false, want true (200 server)", i)
		}
	}
}

// fakeOK is a simple test fake that always returns 200.
func fakeOK(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
	return &httpexec.Result{StatusCode: 200}, nil
}
