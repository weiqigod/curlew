package retry

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	"github.com/peterlindqvist/apitest/internal/httpexec"
)

func TestIsRetriable(t *testing.T) {
	tests := []struct {
		name   string
		method string
		result *httpexec.Result
		err    error
		want   bool
	}{
		{"GET 503 is retriable", "GET", &httpexec.Result{StatusCode: 503}, nil, true},
		{"GET 502 is retriable", "GET", &httpexec.Result{StatusCode: 502}, nil, true},
		{"GET 429 is retriable", "GET", &httpexec.Result{StatusCode: 429}, nil, true},
		{"GET 504 is retriable", "GET", &httpexec.Result{StatusCode: 504}, nil, true},
		{"GET 400 not retriable", "GET", &httpexec.Result{StatusCode: 400}, nil, false},
		{"GET 401 not retriable", "GET", &httpexec.Result{StatusCode: 401}, nil, false},
		{"GET 404 not retriable", "GET", &httpexec.Result{StatusCode: 404}, nil, false},
		{"GET 200 not retriable", "GET", &httpexec.Result{StatusCode: 200}, nil, false},
		{"POST 503 not retriable", "POST", &httpexec.Result{StatusCode: 503}, nil, false},
		{"PATCH 502 not retriable", "PATCH", &httpexec.Result{StatusCode: 502}, nil, false},
		{"PUT 503 not retriable", "PUT", &httpexec.Result{StatusCode: 503}, nil, false},
		{"DELETE 502 not retriable", "DELETE", &httpexec.Result{StatusCode: 502}, nil, false},
		{"GET network error retriable", "GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS, Message: "dns fail"}, true},
		{"GET connection refused retriable", "GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkConnectionRefused, Message: "refused"}, true},
		{"GET timeout retriable", "GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkTimeout, Message: "timeout"}, true},
		{"POST network error not retriable", "POST", nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS, Message: "dns fail"}, false},
		{"HEAD 503 is retriable", "HEAD", &httpexec.Result{StatusCode: 503}, nil, true},
		{"OPTIONS 502 is retriable", "OPTIONS", &httpexec.Result{StatusCode: 502}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetriable(tt.method, tt.result, tt.err)
			if got != tt.want {
				t.Errorf("IsRetriable(%q, %+v, %v) = %v, want %v", tt.method, tt.result, tt.err, got, tt.want)
			}
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	tests := []struct {
		name           string
		attempt        int
		initialDelayMs int
		want           time.Duration
	}{
		{"attempt 0 = initial delay", 0, 1000, 1 * time.Second},
		{"attempt 1 = 2x initial", 1, 1000, 2 * time.Second},
		{"attempt 2 = 4x initial", 2, 1000, 4 * time.Second},
		{"attempt 3 = 8x initial", 3, 1000, 8 * time.Second},
		{"capped at 30s", 10, 1000, 30 * time.Second},
		{"small initial", 0, 100, 100 * time.Millisecond},
		{"small initial attempt 3", 3, 100, 800 * time.Millisecond},
		{"extreme attempt does not overflow", 63, 1000, 30 * time.Second},
		{"very large attempt returns maxBackoff", 100, 1000, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BackoffDelay(tt.attempt, tt.initialDelayMs)
			if got != tt.want {
				t.Errorf("BackoffDelay(%d, %d) = %v, want %v", tt.attempt, tt.initialDelayMs, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantDur time.Duration
		wantOK  bool
	}{
		{"integer seconds", "5", 5 * time.Second, true},
		{"zero means immediate retry", "0", 0, true},
		{"missing header", "", 0, false},
		{"invalid value", "abc", 0, false},
		{"capped at 30s", "120", 30 * time.Second, true},
		{"negative value", "-5", 0, false},
		{"HTTP-date in the past", "Thu, 01 Jan 2020 00:00:00 GMT", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(http.Header)
			if tt.value != "" {
				h.Set("Retry-After", tt.value)
			}
			gotDur, gotOK := ParseRetryAfter(h)
			if gotOK != tt.wantOK {
				t.Errorf("ParseRetryAfter(%q) ok = %v, want %v", tt.value, gotOK, tt.wantOK)
			}
			if gotDur != tt.wantDur {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.value, gotDur, tt.wantDur)
			}
		})
	}

	t.Run("HTTP-date in the future", func(t *testing.T) {
		future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
		h := make(http.Header)
		h.Set("Retry-After", future)
		gotDur, gotOK := ParseRetryAfter(h)
		if !gotOK {
			t.Error("expected ok=true for HTTP-date in future")
		}
		if gotDur < 8*time.Second || gotDur > 11*time.Second {
			t.Errorf("expected ~10s for HTTP-date 10s in future, got %v", gotDur)
		}
	})

	t.Run("HTTP-date far future capped at 30s", func(t *testing.T) {
		future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
		h := make(http.Header)
		h.Set("Retry-After", future)
		gotDur, gotOK := ParseRetryAfter(h)
		if !gotOK {
			t.Error("expected ok=true for HTTP-date far in future")
		}
		if gotDur != 30*time.Second {
			t.Errorf("expected 30s cap for far-future date, got %v", gotDur)
		}
	})
}

// noSleep is a sleep function that returns immediately, for testing.
func noSleep(_ context.Context, _ time.Duration) error { return nil }

func TestExecuteWithRetry(t *testing.T) {
	t.Run("disabled calls exec once", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 503}, nil
		}
		cfg := Config{Enabled: false}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
		if out.Attempts != 1 {
			t.Errorf("expected attempts=1, got %d", out.Attempts)
		}
		if out.Result.StatusCode != 503 {
			t.Errorf("expected status 503, got %d", out.Result.StatusCode)
		}
	})

	t.Run("succeeds first attempt - attempts=1", func(t *testing.T) {
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			return &httpexec.Result{StatusCode: 200}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if out.Attempts != 1 {
			t.Errorf("expected attempts=1, got %d", out.Attempts)
		}
		if out.Result.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", out.Result.StatusCode)
		}
		if out.Err != nil {
			t.Errorf("expected no error, got %v", out.Err)
		}
	})

	t.Run("retries on 503 then succeeds - attempts=2", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				return &httpexec.Result{StatusCode: 503}, nil
			}
			return &httpexec.Result{StatusCode: 200}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if out.Attempts != 2 {
			t.Errorf("expected attempts=2, got %d", out.Attempts)
		}
		if out.Result.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", out.Result.StatusCode)
		}
		if out.Err != nil {
			t.Errorf("expected no error, got %v", out.Err)
		}
	})

	t.Run("all attempts fail - returns last error after max_attempts", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 503}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if out.Attempts != 3 {
			t.Errorf("expected attempts=3, got %d", out.Attempts)
		}
		if out.Result.StatusCode != 503 {
			t.Errorf("expected status 503, got %d", out.Result.StatusCode)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("retries on network error", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				return nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS, Message: "dns fail"}
			}
			return &httpexec.Result{StatusCode: 200}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if out.Attempts != 2 {
			t.Errorf("expected attempts=2, got %d", out.Attempts)
		}
		if out.Err != nil {
			t.Errorf("expected no error, got %v", out.Err)
		}
		if out.Result.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", out.Result.StatusCode)
		}
	})

	t.Run("Retry-After header respected", func(t *testing.T) {
		calls := 0
		var sleepDurations []time.Duration
		sleepFn := func(_ context.Context, d time.Duration) error {
			sleepDurations = append(sleepDurations, d)
			return nil
		}
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				h := make(http.Header)
				h.Set("Retry-After", "5")
				return &httpexec.Result{StatusCode: 429, Headers: h}, nil
			}
			return &httpexec.Result{StatusCode: 200}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
		if out.Attempts != 2 {
			t.Errorf("expected attempts=2, got %d", out.Attempts)
		}
		if len(sleepDurations) != 1 {
			t.Fatalf("expected 1 sleep, got %d", len(sleepDurations))
		}
		if sleepDurations[0] != 5*time.Second {
			t.Errorf("expected sleep 5s (Retry-After), got %v", sleepDurations[0])
		}
	})

	t.Run("context cancelled stops retrying", func(t *testing.T) {
		calls := 0
		ctx, cancel := context.WithCancel(context.Background())
		sleepFn := func(_ context.Context, _ time.Duration) error {
			cancel()
			return context.Canceled
		}
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 503}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 5, InitialDelayMs: 100}
		out := ExecuteWithRetry(ctx, cfg, "GET", exec, sleepFn)
		// Should stop after first sleep returns error
		if calls > 2 {
			t.Errorf("expected at most 2 calls after cancel, got %d", calls)
		}
		if out.Attempts < 1 {
			t.Errorf("expected at least 1 attempt, got %d", out.Attempts)
		}
	})

	t.Run("POST with 503 no retry", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 503}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "POST", exec, noSleep)
		if calls != 1 {
			t.Errorf("expected 1 call (POST not retried), got %d", calls)
		}
		if out.Attempts != 1 {
			t.Errorf("expected attempts=1, got %d", out.Attempts)
		}
	})

	t.Run("enabled with zero MaxAttempts defaults to 3 attempts", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 503}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 0, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if out.Attempts != 3 {
			t.Errorf("expected attempts=3 (spec default), got %d", out.Attempts)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("enabled with zero InitialDelayMs defaults to 1000ms delay", func(t *testing.T) {
		calls := 0
		var sleepDurations []time.Duration
		sleepFn := func(_ context.Context, d time.Duration) error {
			sleepDurations = append(sleepDurations, d)
			return nil
		}
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls < 2 {
				return &httpexec.Result{StatusCode: 503}, nil
			}
			return &httpexec.Result{StatusCode: 200}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 2, InitialDelayMs: 0}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
		if out.Attempts != 2 {
			t.Errorf("expected attempts=2, got %d", out.Attempts)
		}
		if len(sleepDurations) != 1 {
			t.Fatalf("expected 1 sleep, got %d", len(sleepDurations))
		}
		if sleepDurations[0] != 1000*time.Millisecond {
			t.Errorf("expected sleep 1000ms (spec default), got %v", sleepDurations[0])
		}
	})

	t.Run("GET 400 no retry", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 400}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if calls != 1 {
			t.Errorf("expected 1 call (400 not retriable), got %d", calls)
		}
		if out.Attempts != 1 {
			t.Errorf("expected attempts=1, got %d", out.Attempts)
		}
	})
}

func TestExecuteWithRetry_linearBackoff(t *testing.T) {
	calls := 0
	var sleepDurations []time.Duration
	sleepFn := func(_ context.Context, d time.Duration) error {
		sleepDurations = append(sleepDurations, d)
		return nil
	}
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503}, nil
	}
	cfg := Config{
		Enabled:         true,
		MaxAttempts:     4,
		InitialDelayMs:  100,
		BackoffStrategy: "linear",
		MaxDelayMs:      30000,
	}
	ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
	if len(sleepDurations) != 3 {
		t.Fatalf("expected 3 sleeps, got %d", len(sleepDurations))
	}
	// Linear: 100ms * (attempt+1) => 100ms, 200ms, 300ms
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond}
	for i, w := range want {
		if sleepDurations[i] != w {
			t.Errorf("sleep[%d] = %v, want %v", i, sleepDurations[i], w)
		}
	}
}

func TestExecuteWithRetry_constantBackoff(t *testing.T) {
	calls := 0
	var sleepDurations []time.Duration
	sleepFn := func(_ context.Context, d time.Duration) error {
		sleepDurations = append(sleepDurations, d)
		return nil
	}
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503}, nil
	}
	cfg := Config{
		Enabled:         true,
		MaxAttempts:     4,
		InitialDelayMs:  200,
		BackoffStrategy: "constant",
		MaxDelayMs:      30000,
	}
	ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
	if len(sleepDurations) != 3 {
		t.Fatalf("expected 3 sleeps, got %d", len(sleepDurations))
	}
	// Constant: all delays are 200ms
	for i, d := range sleepDurations {
		if d != 200*time.Millisecond {
			t.Errorf("sleep[%d] = %v, want 200ms", i, d)
		}
	}
}

func TestExecuteWithRetry_maxDelayMs(t *testing.T) {
	var sleepDurations []time.Duration
	sleepFn := func(_ context.Context, d time.Duration) error {
		sleepDurations = append(sleepDurations, d)
		return nil
	}
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503}, nil
	}
	cfg := Config{
		Enabled:         true,
		MaxAttempts:     5,
		InitialDelayMs:  1000,
		BackoffStrategy: "exponential",
		MaxDelayMs:      3000,
	}
	ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
	// Exponential: 1000, 2000, 4000->capped to 3000, 8000->capped to 3000
	want := []time.Duration{1000 * time.Millisecond, 2000 * time.Millisecond, 3000 * time.Millisecond, 3000 * time.Millisecond}
	if len(sleepDurations) != len(want) {
		t.Fatalf("expected %d sleeps, got %d", len(want), len(sleepDurations))
	}
	for i, w := range want {
		if sleepDurations[i] != w {
			t.Errorf("sleep[%d] = %v, want %v", i, sleepDurations[i], w)
		}
	}
}

func TestExecuteWithRetry_jitter(t *testing.T) {
	var sleepDurations []time.Duration
	sleepFn := func(_ context.Context, d time.Duration) error {
		sleepDurations = append(sleepDurations, d)
		return nil
	}
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503}, nil
	}
	cfg := Config{
		Enabled:         true,
		MaxAttempts:     3,
		InitialDelayMs:  1000,
		BackoffStrategy: "constant",
		MaxDelayMs:      30000,
		Jitter:          true,
		JitterFactor:    0.1,
	}
	ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
	if len(sleepDurations) != 2 {
		t.Fatalf("expected 2 sleeps, got %d", len(sleepDurations))
	}
	// With jitter_factor=0.1, delays should be between 900ms and 1100ms
	for i, d := range sleepDurations {
		if d < 900*time.Millisecond || d > 1100*time.Millisecond {
			t.Errorf("sleep[%d] = %v, want between 900ms and 1100ms", i, d)
		}
	}
}

func TestExecuteWithRetry_exponentialDefault(t *testing.T) {
	var sleepDurations []time.Duration
	sleepFn := func(_ context.Context, d time.Duration) error {
		sleepDurations = append(sleepDurations, d)
		return nil
	}
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503}, nil
	}
	// Empty BackoffStrategy should default to exponential
	cfg := Config{
		Enabled:        true,
		MaxAttempts:    4,
		InitialDelayMs: 100,
	}
	ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
	// Exponential: 100ms, 200ms, 400ms
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	if len(sleepDurations) != len(want) {
		t.Fatalf("expected %d sleeps, got %d", len(want), len(sleepDurations))
	}
	for i, w := range want {
		if sleepDurations[i] != w {
			t.Errorf("sleep[%d] = %v, want %v", i, sleepDurations[i], w)
		}
	}
}

func TestClassifyForRetry(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		result    *httpexec.Result
		err       error
		wantInput ShouldRetryInput
	}{
		{
			"GET 503",
			"GET", &httpexec.Result{StatusCode: 503}, nil,
			ShouldRetryInput{Method: "GET", StatusCode: 503},
		},
		{
			"network error",
			"GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS},
			ShouldRetryInput{Method: "GET", IsNetworkErr: true},
		},
		{
			"timeout error",
			"GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkTimeout},
			ShouldRetryInput{Method: "GET", IsTimeout: true},
		},
		{
			"nil result nil error",
			"GET", nil, nil,
			ShouldRetryInput{Method: "GET"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyForRetry(tt.method, tt.result, tt.err)
			if got != tt.wantInput {
				t.Errorf("ClassifyForRetry() = %+v, want %+v", got, tt.wantInput)
			}
		})
	}
}

func TestExecuteWithRetry_statusRangeRetry(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		if calls < 3 {
			return &httpexec.Result{StatusCode: 503}, nil
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}
	cfg := Config{
		Enabled:     true,
		MaxAttempts: 5,
		RetryOn: &RetryOnConfig{
			StatusRanges: []string{"500-599"},
			Methods:      []string{"GET"},
		},
	}
	out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
	if out.Attempts != 3 {
		t.Errorf("expected attempts=3, got %d", out.Attempts)
	}
	if out.Result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", out.Result.StatusCode)
	}
}

func TestExecuteWithRetry_doNotRetryOnExclusion(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 501}, nil
	}
	cfg := Config{
		Enabled:     true,
		MaxAttempts: 5,
		RetryOn: &RetryOnConfig{
			StatusRanges: []string{"500-599"},
			Methods:      []string{"GET"},
		},
		DoNotRetryOn: &DoNotRetryOnConfig{
			StatusCodes: []int{501},
		},
	}
	out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
	if out.Attempts != 1 {
		t.Errorf("expected attempts=1 (501 excluded), got %d", out.Attempts)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestExecuteWithRetry_methodRestriction(t *testing.T) {
	t.Run("POST not allowed by methods list", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 503}, nil
		}
		cfg := Config{
			Enabled:     true,
			MaxAttempts: 3,
			RetryOn: &RetryOnConfig{
				StatusCodes: []int{503},
				Methods:     []string{"GET", "HEAD"},
			},
		}
		out := ExecuteWithRetry(context.Background(), cfg, "POST", exec, noSleep)
		if out.Attempts != 1 {
			t.Errorf("expected attempts=1 (POST not in methods), got %d", out.Attempts)
		}
	})

	t.Run("POST allowed when explicitly in methods list", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				return &httpexec.Result{StatusCode: 503}, nil
			}
			return &httpexec.Result{StatusCode: 200}, nil
		}
		cfg := Config{
			Enabled:     true,
			MaxAttempts: 3,
			RetryOn: &RetryOnConfig{
				StatusCodes: []int{503},
				Methods:     []string{"GET", "POST"},
			},
		}
		out := ExecuteWithRetry(context.Background(), cfg, "POST", exec, noSleep)
		if out.Attempts != 2 {
			t.Errorf("expected attempts=2 (POST retried), got %d", out.Attempts)
		}
	})
}

func TestExecuteWithRetry_nonIdempotentWarning(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		if calls == 1 {
			return &httpexec.Result{StatusCode: 503}, nil
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}
	cfg := Config{
		Enabled:     true,
		MaxAttempts: 3,
		RetryOn: &RetryOnConfig{
			StatusCodes: []int{503},
			Methods:     []string{"GET", "POST"},
		},
	}
	out := ExecuteWithRetry(context.Background(), cfg, "POST", exec, noSleep)
	if len(out.Warnings) == 0 {
		t.Fatal("expected warnings for non-idempotent POST retry")
	}
	found := false
	for _, w := range out.Warnings {
		if strings.Contains(w, "non-idempotent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning containing 'non-idempotent', got %v", out.Warnings)
	}
}

func TestExecuteWithRetry_networkErrorCondition(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		if calls == 1 {
			return nil, &apierrors.NetworkError{Kind: apierrors.NetworkConnectionRefused, Message: "refused"}
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}
	cfg := Config{
		Enabled:     true,
		MaxAttempts: 3,
		RetryOn: &RetryOnConfig{
			NetworkErrors: BoolPtr(true),
			Methods:       []string{"GET"},
		},
	}
	out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
	if out.Attempts != 2 {
		t.Errorf("expected attempts=2, got %d", out.Attempts)
	}
	if out.Result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", out.Result.StatusCode)
	}
}

func TestExecuteWithRetry_timeoutCondition(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context) (*httpexec.Result, error) {
		calls++
		if calls == 1 {
			return nil, &apierrors.NetworkError{Kind: apierrors.NetworkTimeout, Message: "timeout"}
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}
	cfg := Config{
		Enabled:     true,
		MaxAttempts: 3,
		RetryOn: &RetryOnConfig{
			Timeouts: BoolPtr(true),
			Methods:  []string{"GET"},
		},
	}
	out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
	if out.Attempts != 2 {
		t.Errorf("expected attempts=2, got %d", out.Attempts)
	}
	if out.Result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", out.Result.StatusCode)
	}
}

func TestExecuteWithRetry_defaultBehaviorUnchanged(t *testing.T) {
	// With nil RetryOn/DoNotRetryOn, should behave exactly like before
	t.Run("GET 503 retried with defaults", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				return &httpexec.Result{StatusCode: 503}, nil
			}
			return &httpexec.Result{StatusCode: 200}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if out.Attempts != 2 {
			t.Errorf("expected attempts=2, got %d", out.Attempts)
		}
	})

	t.Run("POST 503 not retried with defaults", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			return &httpexec.Result{StatusCode: 503}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "POST", exec, noSleep)
		if out.Attempts != 1 {
			t.Errorf("expected attempts=1, got %d", out.Attempts)
		}
	})
}

func TestExecuteWithRetry_AttemptDetails(t *testing.T) {
	t.Run("no retry - single attempt detail", func(t *testing.T) {
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			return &httpexec.Result{StatusCode: 200, Duration: 50 * time.Millisecond}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if len(out.AttemptDetails) != 1 {
			t.Fatalf("expected 1 attempt detail, got %d", len(out.AttemptDetails))
		}
		d := out.AttemptDetails[0]
		if d.Number != 1 {
			t.Errorf("expected attempt number 1, got %d", d.Number)
		}
		if d.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", d.StatusCode)
		}
		if d.Delay != 0 {
			t.Errorf("expected delay 0 for first attempt, got %v", d.Delay)
		}
	})

	t.Run("retry succeeds on second attempt - two details", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				return &httpexec.Result{StatusCode: 503, Duration: 20 * time.Millisecond}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 30 * time.Millisecond}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if len(out.AttemptDetails) != 2 {
			t.Fatalf("expected 2 attempt details, got %d", len(out.AttemptDetails))
		}
		if out.AttemptDetails[0].Number != 1 {
			t.Errorf("first attempt number should be 1, got %d", out.AttemptDetails[0].Number)
		}
		if out.AttemptDetails[0].StatusCode != 503 {
			t.Errorf("first attempt status should be 503, got %d", out.AttemptDetails[0].StatusCode)
		}
		if out.AttemptDetails[0].Delay != 0 {
			t.Errorf("first attempt delay should be 0, got %v", out.AttemptDetails[0].Delay)
		}
		if out.AttemptDetails[1].Number != 2 {
			t.Errorf("second attempt number should be 2, got %d", out.AttemptDetails[1].Number)
		}
		if out.AttemptDetails[1].StatusCode != 200 {
			t.Errorf("second attempt status should be 200, got %d", out.AttemptDetails[1].StatusCode)
		}
	})

	t.Run("all attempts fail - three details", func(t *testing.T) {
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			return &httpexec.Result{StatusCode: 503, Duration: 10 * time.Millisecond}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if len(out.AttemptDetails) != 3 {
			t.Fatalf("expected 3 attempt details, got %d", len(out.AttemptDetails))
		}
		for i, d := range out.AttemptDetails {
			if d.Number != i+1 {
				t.Errorf("detail[%d] number should be %d, got %d", i, i+1, d.Number)
			}
			if d.StatusCode != 503 {
				t.Errorf("detail[%d] status should be 503, got %d", i, d.StatusCode)
			}
		}
	})

	t.Run("retry disabled - no details", func(t *testing.T) {
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			return &httpexec.Result{StatusCode: 503, Duration: 10 * time.Millisecond}, nil
		}
		cfg := Config{Enabled: false}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if len(out.AttemptDetails) != 0 {
			t.Errorf("expected 0 attempt details when disabled, got %d", len(out.AttemptDetails))
		}
	})

	t.Run("attempt details include delay for retries", func(t *testing.T) {
		var sleepDurations []time.Duration
		sleepFn := func(_ context.Context, d time.Duration) error {
			sleepDurations = append(sleepDurations, d)
			return nil
		}
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls < 3 {
				return &httpexec.Result{StatusCode: 503, Duration: 10 * time.Millisecond}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 5, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, sleepFn)
		if len(out.AttemptDetails) != 3 {
			t.Fatalf("expected 3 attempt details, got %d", len(out.AttemptDetails))
		}
		// First attempt: no delay
		if out.AttemptDetails[0].Delay != 0 {
			t.Errorf("first attempt should have 0 delay, got %v", out.AttemptDetails[0].Delay)
		}
		// Second attempt: initial delay (100ms for exponential default)
		if out.AttemptDetails[1].Delay != 100*time.Millisecond {
			t.Errorf("second attempt delay should be 100ms, got %v", out.AttemptDetails[1].Delay)
		}
		// Third attempt: 200ms (exponential)
		if out.AttemptDetails[2].Delay != 200*time.Millisecond {
			t.Errorf("third attempt delay should be 200ms, got %v", out.AttemptDetails[2].Delay)
		}
	})

	t.Run("attempt details include status codes", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				return &httpexec.Result{StatusCode: 429, Duration: 10 * time.Millisecond}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if len(out.AttemptDetails) < 2 {
			t.Fatalf("expected at least 2 attempt details, got %d", len(out.AttemptDetails))
		}
		if out.AttemptDetails[0].StatusCode != 429 {
			t.Errorf("first attempt status should be 429, got %d", out.AttemptDetails[0].StatusCode)
		}
		if out.AttemptDetails[1].StatusCode != 200 {
			t.Errorf("second attempt status should be 200, got %d", out.AttemptDetails[1].StatusCode)
		}
	})

	t.Run("attempt details include error for network failure", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context) (*httpexec.Result, error) {
			calls++
			if calls == 1 {
				return nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS, Message: "dns fail"}
			}
			return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
		}
		cfg := Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100}
		out := ExecuteWithRetry(context.Background(), cfg, "GET", exec, noSleep)
		if len(out.AttemptDetails) < 2 {
			t.Fatalf("expected at least 2 attempt details, got %d", len(out.AttemptDetails))
		}
		if out.AttemptDetails[0].StatusCode != 0 {
			t.Errorf("network error attempt should have status 0, got %d", out.AttemptDetails[0].StatusCode)
		}
		if out.AttemptDetails[0].Err == nil {
			t.Error("network error attempt should have non-nil Err")
		}
		if out.AttemptDetails[1].Err != nil {
			t.Errorf("successful attempt should have nil Err, got %v", out.AttemptDetails[1].Err)
		}
	})
}
