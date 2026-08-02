package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/ratelimit"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		rps     int
		wantNil bool
	}{
		{"positive rps returns limiter", 10, false},
		{"zero rps returns nil (unlimited)", 0, true},
		{"negative rps returns nil (unlimited)", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := ratelimit.New(tt.rps)
			if tt.wantNil && l != nil {
				t.Fatalf("New(%d) = %v, want nil", tt.rps, l)
			}
			if !tt.wantNil && l == nil {
				t.Fatalf("New(%d) = nil, want non-nil", tt.rps)
			}
		})
	}
}

func TestLimiter_Wait_NilReceiverIsNoop(t *testing.T) {
	var l *ratelimit.Limiter
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("nil Wait returned error: %v", err)
	}
}

func TestLimiter_Wait_Throttles(t *testing.T) {
	// 100 rps → 10ms interval; 5 Waits should take >= ~30ms (4 intervals × 10ms)
	l := ratelimit.New(100)
	const calls = 5
	start := time.Now()
	for i := 0; i < calls; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait[%d] error: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// 4 intervals × 10ms × 80% tolerance = 32ms
	want := time.Duration(calls-1) * 10 * time.Millisecond * 80 / 100
	if elapsed < want {
		t.Errorf("elapsed %v < %v (expected throttle)", elapsed, want)
	}
}

func TestLimiter_Wait_ContextCancellation(t *testing.T) {
	// 1 rps; first Wait should succeed; cancel context; second Wait must return ctx.Err()
	l := ratelimit.New(1)

	ctx, cancel := context.WithCancel(context.Background())
	// First wait sets l.last to now.
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("first Wait error: %v", err)
	}
	cancel()
	// Second wait: interval is 1s, context already cancelled.
	err := l.Wait(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestLimiter_Wait_ConcurrentSharesBucket(t *testing.T) {
	// 10 rps → 100ms interval. 4 goroutines × 5 waits = 20 total.
	// Wall-clock must be >= 19 intervals × 100ms × 80% = 1.52s.
	const rps = 10
	const goroutines = 4
	const waitsPerGoroutine = 5
	const total = goroutines * waitsPerGoroutine

	l := ratelimit.New(rps)
	ctx := context.Background()

	var wg sync.WaitGroup
	start := time.Now()
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < waitsPerGoroutine; i++ {
				if err := l.Wait(ctx); err != nil {
					return
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// (total-1) intervals × (1s/rps) × 80% tolerance
	interval := time.Second / time.Duration(rps)
	want := time.Duration(total-1) * interval * 80 / 100
	if elapsed < want {
		t.Errorf("elapsed %v < %v; concurrent goroutines are not sharing the bucket", elapsed, want)
	}
}
