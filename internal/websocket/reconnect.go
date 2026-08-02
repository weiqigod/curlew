package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/weiqigod/curlew/internal/parser"
)

// reconnectState holds per-request reconnection bookkeeping owned by the
// step loop. It is not safe for concurrent use.
type reconnectState struct {
	cfg      *parser.ReconnectConfig
	attempts int
}

// shouldReconnect reports whether the executor should attempt a redial in
// response to err and whether the attempt budget allows it.
func (r *reconnectState) shouldReconnect(err error) bool {
	if r == nil || r.cfg == nil || !r.cfg.Enabled {
		return false
	}
	if err == nil {
		return false
	}
	// Context cancellation and deadline are deterministic non-recoverable
	// failures — retrying on a cancelled context wastes resources and produces
	// misleading errors.
	if errors.Is(err, ErrContextCanceled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if isTimeoutErr(err) {
		return false // expect timeouts are deterministic test failures
	}
	max := r.cfg.MaxAttempts
	if max <= 0 {
		max = 3
	}
	return r.attempts < max
}

// nextDelay computes the delay before the next reconnect attempt.
// Exponential: delay = initial * 2^attempts. Initial defaults to 1s.
func (r *reconnectState) nextDelay() time.Duration {
	base := time.Duration(r.cfg.InitialDelayMs) * time.Millisecond
	if base <= 0 {
		base = time.Second
	}
	// exponential: attempt 0 -> 1x, 1 -> 2x, 2 -> 4x
	mult := 1 << r.attempts
	return base * time.Duration(mult)
}

// waitReconnect sleeps for the backoff delay, honouring context cancellation.
func waitReconnect(ctx context.Context, r *reconnectState) error {
	delay := r.nextDelay()
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrContextCanceled, ctx.Err())
	case <-time.After(delay):
		return nil
	}
}

// redial performs a single redial attempt returning the new connection.
func redial(ctx context.Context, dialer Dialer, url string, headers http.Header) (Conn, error) {
	c, _, err := dialer.Dial(ctx, url, headers)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDialFailed, err)
	}
	return c, nil
}
