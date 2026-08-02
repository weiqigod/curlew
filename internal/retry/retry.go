package retry

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
)

const maxBackoff = 30 * time.Second

// AttemptDetail holds the outcome of a single retry attempt.
type AttemptDetail struct {
	Number     int           // 1-based attempt number
	StatusCode int           // HTTP status code (0 if network error)
	Duration   time.Duration // how long the attempt took
	Delay      time.Duration // backoff delay waited before this attempt (0 for first)
	Err        error         // error if attempt failed with non-HTTP error
}

// Outcome holds the result of a retried execution.
type Outcome struct {
	Result         *httpexec.Result
	Err            error
	Attempts       int             // total attempts (1 = no retries)
	Warnings       []string        // warnings (e.g., non-idempotent method retry)
	AttemptDetails []AttemptDetail // per-attempt details (populated when retry is enabled)
}

// SleepFunc is an injectable sleep for testing.
type SleepFunc func(ctx context.Context, d time.Duration) error

// DefaultSleep sleeps for d or until ctx is cancelled.
func DefaultSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsRetriable determines whether a failed request should be retried
// based on the HTTP method, response, and error.
func IsRetriable(method string, result *httpexec.Result, err error) bool {
	if !DefaultIdempotentMethods()[method] {
		return false
	}
	if err != nil {
		var netErr *apierrors.NetworkError
		return errors.As(err, &netErr)
	}
	if result != nil {
		return DefaultRetriableStatusCodes()[result.StatusCode]
	}
	return false
}

// ClassifyForRetry builds a ShouldRetryInput from the method, result, and error.
func ClassifyForRetry(method string, result *httpexec.Result, err error) ShouldRetryInput {
	input := ShouldRetryInput{Method: method}
	if result != nil {
		input.StatusCode = result.StatusCode
	}
	if err != nil {
		var netErr *apierrors.NetworkError
		if errors.As(err, &netErr) {
			if netErr.Kind == apierrors.NetworkTimeout {
				input.IsTimeout = true
			} else {
				input.IsNetworkErr = true
			}
		}
	}
	return input
}

// ParseRetryAfter extracts delay from Retry-After header.
// Returns (0, false) if absent or unparseable.
// Returns (duration, true) if present and valid.
// Supports integer-seconds and HTTP-date format (RFC 9110 §10.2.3).
// Caps at 30s. Retry-After: 0 returns (0, true) for immediate retry.
func ParseRetryAfter(headers http.Header) (time.Duration, bool) {
	val := headers.Get("Retry-After")
	if val == "" {
		return 0, false
	}

	// Try integer-seconds first.
	if secs, err := strconv.Atoi(val); err == nil {
		if secs < 0 {
			return 0, false
		}
		d := time.Duration(secs) * time.Second
		if d > maxBackoff {
			d = maxBackoff
		}
		return d, true
	}

	// Try HTTP-date format.
	t, err := time.Parse(http.TimeFormat, val)
	if err != nil {
		return 0, false
	}
	delta := time.Until(t)
	if delta <= 0 {
		return 0, true
	}
	if delta > maxBackoff {
		delta = maxBackoff
	}
	return delta, true
}

// BackoffDelay computes exponential backoff: initialDelayMs * 2^attempt, capped at 30s.
func BackoffDelay(attempt, initialDelayMs int) time.Duration {
	maxMs := float64(maxBackoff / time.Millisecond)
	ms := float64(initialDelayMs) * math.Pow(2, float64(attempt))
	if ms >= maxMs {
		return maxBackoff
	}
	return time.Duration(ms) * time.Millisecond
}

// ExecuteWithRetry wraps exec with retry logic.
// If cfg.Enabled is false, exec is called exactly once.
func ExecuteWithRetry(
	ctx context.Context,
	cfg Config,
	method string,
	exec func(ctx context.Context) (*httpexec.Result, error),
	sleep SleepFunc,
) *Outcome {
	result, err := exec(ctx)
	if !cfg.Enabled {
		return &Outcome{Result: result, Err: err, Attempts: 1}
	}

	attempts := 1
	maxAttempts := cfg.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = DefaultMaxAttempts
	}
	initialDelayMs := cfg.InitialDelayMs
	if initialDelayMs <= 0 {
		initialDelayMs = DefaultInitialDelayMs
	}

	strategy := Strategy(cfg.BackoffStrategy)
	if strategy == "" {
		strategy = StrategyExponential
	}
	maxDelay := cfg.MaxDelayMs
	if maxDelay <= 0 {
		maxDelay = DefaultMaxDelayMs
	}

	// Record the first attempt detail.
	details := make([]AttemptDetail, 1, maxAttempts)
	details[0] = buildAttemptDetail(1, 0, result, err)

	var warnings []string
	for attempts < maxAttempts {
		input := ClassifyForRetry(method, result, err)
		decision := ShouldRetry(input, cfg.RetryOn, cfg.DoNotRetryOn)
		if !decision.Retry {
			break
		}
		if decision.Warning != "" {
			warnings = append(warnings, decision.Warning)
		}

		// Determine delay: Retry-After header takes priority over backoff.
		delay := CalculateDelay(strategy, attempts-1, initialDelayMs, maxDelay)
		if cfg.Jitter && cfg.JitterFactor > 0 {
			delay = ApplyJitter(delay, cfg.JitterFactor, DefaultJitter)
		}
		if result != nil && result.Headers != nil {
			if ra, ok := ParseRetryAfter(result.Headers); ok {
				delay = ra
			}
		}

		if sleepErr := sleep(ctx, delay); sleepErr != nil {
			// Context cancelled or sleep interrupted — return what we have.
			return &Outcome{Result: result, Err: err, Attempts: attempts, Warnings: warnings, AttemptDetails: details}
		}

		result, err = exec(ctx)
		attempts++
		details = append(details, buildAttemptDetail(attempts, delay, result, err))
	}

	return &Outcome{Result: result, Err: err, Attempts: attempts, Warnings: warnings, AttemptDetails: details}
}

// buildAttemptDetail constructs an AttemptDetail from a single attempt's outcome.
func buildAttemptDetail(number int, delay time.Duration, result *httpexec.Result, err error) AttemptDetail {
	d := AttemptDetail{
		Number: number,
		Delay:  delay,
		Err:    err,
	}
	if result != nil {
		d.StatusCode = result.StatusCode
		d.Duration = result.Duration
	}
	return d
}
