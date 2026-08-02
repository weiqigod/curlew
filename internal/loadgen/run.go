package loadgen

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
)

// ExecuteFunc is the injectable request-execution seam (tests swap this for
// a fake; production wires httpexec.Execute).
type ExecuteFunc func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error)

// RunOptions injects test seams. Zero value uses production defaults.
type RunOptions struct {
	Execute  ExecuteFunc      // nil = httpexec.Execute
	Now      func() time.Time // nil = time.Now
	OnSample func(Sample)     // nil = samples discarded
}

// Run executes the load generation loop and returns a summary. The caller
// owns ctx; ctx cancellation ends the run early with Aborted=true.
func Run(ctx context.Context, cfg Config, req *httpexec.Request, opts RunOptions) (*Summary, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if req == nil || req.URL == "" {
		return nil, fmt.Errorf("loadgen: request is nil or URL empty")
	}

	execute := opts.Execute
	if execute == nil {
		execute = httpexec.Execute
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	runCtx, cancel := context.WithDeadline(ctx, now().Add(cfg.Duration))
	defer cancel()

	var requests, successes, failures int64
	var ticker *time.Ticker
	var tickC <-chan time.Time
	if cfg.RPS > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(cfg.RPS))
		defer ticker.Stop()
		tickC = ticker.C
	}

	start := now()
	onSample := opts.OnSample
	var wg sync.WaitGroup
	for i := 0; i < cfg.VUs; i++ {
		wg.Add(1)
		vuIdx := i
		go func() {
			defer wg.Done()
			// Wait for this VU's ramp-in tick (linear).
			if cfg.RampUp > 0 && cfg.VUs > 1 {
				per := cfg.RampUp / time.Duration(cfg.VUs-1)
				delay := time.Duration(vuIdx) * per
				select {
				case <-time.After(delay):
				case <-runCtx.Done():
					return
				}
			}
			runVU(runCtx, execute, req, tickC, start, now, onSample, &requests, &successes, &failures)
		}()
	}
	wg.Wait()

	aborted := ctx.Err() != nil
	return &Summary{
		Requests:  int(atomic.LoadInt64(&requests)),
		Successes: int(atomic.LoadInt64(&successes)),
		Failures:  int(atomic.LoadInt64(&failures)),
		Elapsed:   now().Sub(start),
		Aborted:   aborted,
	}, nil
}

// runVU loops issuing requests until ctx is done. start and now are used to
// compute per-sample offsets and latencies for the OnSample hook.
func runVU(ctx context.Context, execute ExecuteFunc, req *httpexec.Request,
	tickC <-chan time.Time, start time.Time, now func() time.Time,
	onSample func(Sample),
	reqs, succ, fail *int64,
) {
	for {
		if ctx.Err() != nil {
			return
		}
		if tickC != nil {
			select {
			case <-ctx.Done():
				return
			case <-tickC:
			}
		}
		reqStart := now()
		atomic.AddInt64(reqs, 1)
		res, err := execute(ctx, req)
		reqEnd := now()
		ok := false
		switch {
		case err != nil:
			// Errors caused by context deadline or cancellation are artifacts of
			// the run ending, not actual server failures. Undo the request count
			// so that Requests == Successes + Failures always holds.
			if ctx.Err() != nil {
				atomic.AddInt64(reqs, -1)
				return
			}
			atomic.AddInt64(fail, 1)
		case res.StatusCode >= 400:
			atomic.AddInt64(fail, 1)
		default:
			atomic.AddInt64(succ, 1)
			ok = true
		}
		if onSample != nil {
			onSample(Sample{
				StartOffsetNs: reqStart.Sub(start).Nanoseconds(),
				LatencyNs:     reqEnd.Sub(reqStart).Nanoseconds(),
				Success:       ok,
			})
		}
	}
}
