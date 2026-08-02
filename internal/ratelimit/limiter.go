// Package ratelimit provides a shared token-bucket rate limiter used to
// throttle HTTP dispatch across all request phases of a collection run.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter is a simple single-token bucket limiter. Construct with New; a nil
// *Limiter is safe to Wait on and is treated as "no throttling".
type Limiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

// New returns a Limiter that paces requests to at most rps per second. When
// rps <= 0 it returns nil, which represents "unlimited" (Wait on a nil
// receiver is a no-op).
func New(rps int) *Limiter {
	if rps <= 0 {
		return nil
	}
	return &Limiter{interval: time.Second / time.Duration(rps)}
}

// Wait blocks until the next token is available, or until ctx is cancelled.
// It is safe to call from multiple goroutines concurrently; tokens are
// awarded in the order goroutines acquire the mutex.
func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	now := time.Now()
	next := l.last.Add(l.interval)
	if next.After(now) {
		delay := next.Sub(now)
		l.last = next
		l.mu.Unlock()
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-t.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	l.last = now
	l.mu.Unlock()
	return nil
}
