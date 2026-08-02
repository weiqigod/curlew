package schedule

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/peterlindqvist/apitest/internal/backend"
)

// CollectionExecutor executes a resolved collection and returns the outcome.
// The default implementation is runnerExecutor (see executor.go). Tests inject
// a fake to avoid pulling in the real parser + runner.
//
// Implementations must return a non-nil *ExecutionOutcome whenever err is nil.
// Returning (nil, nil) is a contract violation and will be treated by the runner
// as a single-item error result rather than causing a panic.
type CollectionExecutor interface {
	Execute(ctx context.Context, collectionPath string, envVars map[string]string) (*ExecutionOutcome, error)
}

// ExecutionOutcome is the result of running a collection. It is mapped onto
// ResultRequest by the orchestration loop. Using our own type prevents the
// schedule package from leaking runner.Summary upward.
type ExecutionOutcome struct {
	CollectionName string
	StartedAt      time.Time
	DurationMs     int64
	PassCount      int
	FailCount      int
	SkippedCount   int
	Items          []ResultItem
}

// Runner is the schedule-pull orchestration loop. It polls for a new run,
// resolves the collection ref, executes it, sends heartbeats, and posts
// results. On post failure it writes the payload to the Queue.
//
// All fields except Client, Queue, and Executor have sensible defaults (applied
// by RunOnce when zero). The Now and Sleep fields are injectable for testing;
// both default to real-time behaviour when nil.
type Runner struct {
	Client            *Client
	Queue             *Queue
	Executor          CollectionExecutor
	PollInterval      time.Duration // default 30s
	HeartbeatInterval time.Duration // default 30s
	PostMaxElapsed    time.Duration // default 1h (exp backoff ceiling)
	WorkingDir        string        // resolved from os.Getwd() when empty
	Stdout            io.Writer     // "claimed run …" and "posted result for …" lines
	Stderr            io.Writer     // warnings, retry chatter, queue notices
	Now               func() time.Time
	// Sleep pauses for d or until ctx is cancelled. Defaults to a
	// context-aware timer so cancellation is immediate.
	Sleep func(ctx context.Context, d time.Duration) error
}

// defaults fills in zero values on r.
func (r *Runner) defaults() {
	if r.PollInterval == 0 {
		r.PollInterval = 30 * time.Second
	}
	if r.HeartbeatInterval == 0 {
		r.HeartbeatInterval = 30 * time.Second
	}
	if r.PostMaxElapsed == 0 {
		r.PostMaxElapsed = time.Hour
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Sleep == nil {
		r.Sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	if r.Stdout == nil {
		r.Stdout = io.Discard
	}
	if r.Stderr == nil {
		r.Stderr = io.Discard
	}
}

// Run polls forever (until ctx cancellation or a fatal error). On each cycle
// it first drains the pending-uploads queue, then attempts a fresh claim.
func (r *Runner) Run(ctx context.Context) error {
	r.defaults()
	for {
		if err := r.RunOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			// Non-fatal errors (ErrNoRunAvailable already handled inside RunOnce).
			_, _ = fmt.Fprintf(r.Stderr, "worker: error: %v\n", err)
		}
		if err := r.Sleep(ctx, r.PollInterval); err != nil {
			return err
		}
	}
}

// RunOnce performs one drain + one claim cycle.
// Returns nil when no run was available and the queue was empty.
func (r *Runner) RunOnce(ctx context.Context) error {
	r.defaults()

	// 1. Drain pending-uploads queue first.
	r.drainQueue(ctx)

	// 2. Poll for a new run.
	next, err := r.Client.PollNextRun(ctx)
	if errors.Is(err, ErrNoRunAvailable) {
		return nil
	}
	if errors.Is(err, backend.ErrNetworkFailure) {
		_, _ = fmt.Fprintf(r.Stderr, "worker: backend unreachable during poll; will retry\n")
		return nil
	}
	if err != nil {
		return err // 401, 5xx — caller decides
	}

	// 3. Resolve collection ref.
	_, _ = fmt.Fprintf(r.Stdout, "claimed run %s; executing collection %s\n", next.RunID, next.CollectionRef)
	started := r.Now()

	var outcome *ExecutionOutcome

	path, resolveErr := ResolveCollection(next.CollectionRef, r.WorkingDir)
	if resolveErr != nil {
		// For git: refs print the canonical stderr message; for other rejections
		// print the raw error. Both cases produce a fail result.
		if errors.Is(resolveErr, ErrUnsupportedCollectionRef) {
			_, _ = fmt.Fprintf(r.Stderr, "git: collection refs are not yet supported in M16; "+
				"please clone the repo on the worker host and use file: instead\n")
		} else {
			_, _ = fmt.Fprintf(r.Stderr, "worker: resolve collection: %v\n", resolveErr)
		}
		outcome = &ExecutionOutcome{
			StartedAt:  started,
			DurationMs: r.Now().Sub(started).Milliseconds(),
			FailCount:  1,
			Items: []ResultItem{{
				Name:    "collection_ref",
				Status:  "error",
				Message: resolveErr.Error(),
			}},
		}
	} else {
		// 4. Execute the collection with a heartbeat goroutine running alongside.
		abandonCh := make(chan struct{}, 1)
		hctx, hcancel := context.WithCancel(ctx)
		go r.heartbeatLoop(hctx, next.RunID, next.ClaimToken, abandonCh)

		var runErr error
		outcome, runErr = r.Executor.Execute(ctx, path, next.EnvVars)
		hcancel() // stop heartbeat regardless of outcome

		// Check if the claim was reaped while we were executing.
		select {
		case <-abandonCh:
			// Claim was reaped — skip posting entirely.
			_, _ = fmt.Fprintf(r.Stderr, "worker: claim reaped for run %s; discarding result\n", next.RunID)
			return nil
		default:
		}

		if runErr != nil {
			// Still post a fail result so the schedule run is recorded.
			outcome = &ExecutionOutcome{
				StartedAt:  started,
				DurationMs: r.Now().Sub(started).Milliseconds(),
				FailCount:  1,
				Items: []ResultItem{{
					Name:    "run",
					Status:  "error",
					Message: runErr.Error(),
				}},
			}
		}
		if outcome != nil && outcome.StartedAt.IsZero() {
			outcome.StartedAt = started
		}
	}

	// 5. Build ResultRequest and POST.
	// Guard against a contract-violating (nil, nil) return from Execute.
	if outcome == nil {
		outcome = &ExecutionOutcome{
			StartedAt:  started,
			DurationMs: r.Now().Sub(started).Milliseconds(),
			FailCount:  1,
			Items: []ResultItem{{
				Name:    "executor",
				Status:  "error",
				Message: "CollectionExecutor.Execute returned nil outcome with nil error (contract violation)",
			}},
		}
	}
	payload := buildResultRequest(next.ClaimToken, outcome)
	postErr := r.postWithRetry(ctx, next.RunID, payload)
	if errors.Is(postErr, ErrAlreadyCompleted) {
		// Server already has the result (idempotent guard).
		_, _ = fmt.Fprintf(r.Stdout, "posted result for %s (pass=%d, fail=%d)\n",
			next.RunID, payload.PassCount, payload.FailCount)
		return nil
	}
	if postErr != nil && errors.Is(postErr, ErrPostExhausted) {
		qPath, qErr := r.Queue.Enqueue(next.RunID, payload)
		_, _ = fmt.Fprintln(r.Stderr, "backend unreachable; queued result locally")
		if qErr != nil {
			_, _ = fmt.Fprintf(r.Stderr, "worker: queue error: %v\n", qErr)
		} else {
			_ = qPath // path available for logging if needed
		}
		return nil
	}
	if postErr != nil {
		_, _ = fmt.Fprintf(r.Stderr, "worker: post result error: %v\n", postErr)
		return nil
	}

	_, _ = fmt.Fprintf(r.Stdout, "posted result for %s (pass=%d, fail=%d)\n",
		next.RunID, payload.PassCount, payload.FailCount)
	return nil
}

// drainQueue iterates the pending-uploads queue and posts each entry. Network
// failures stop the drain (backend still down) but don't return an error — the
// entries stay on disk for the next cycle. Already-completed entries are removed.
func (r *Runner) drainQueue(ctx context.Context) {
	items, err := r.Queue.List()
	if err != nil {
		_, _ = fmt.Fprintf(r.Stderr, "worker: queue list error: %v\n", err)
		return
	}
	for _, p := range items {
		postErr := r.Client.PostResult(ctx, p.RunID, p.Payload)
		if postErr == nil {
			if err := r.Queue.Remove(p.RunID); err != nil {
				_, _ = fmt.Fprintf(r.Stderr, "worker: remove queued result %s: %v\n", p.RunID, err)
			}
			_, _ = fmt.Fprintf(r.Stdout, "drained queued result for %s\n", p.RunID)
			continue
		}
		if errors.Is(postErr, ErrAlreadyCompleted) {
			// Server already accepted; remove local copy.
			if err := r.Queue.Remove(p.RunID); err != nil {
				_, _ = fmt.Fprintf(r.Stderr, "worker: remove queued result %s: %v\n", p.RunID, err)
			}
			continue
		}
		if errors.Is(postErr, backend.ErrNetworkFailure) {
			// Still down — stop trying to drain this cycle.
			_, _ = fmt.Fprintf(r.Stderr, "worker: backend still unreachable; will retry drain next cycle\n")
			return
		}
		// Other error — warn and leave on disk.
		_, _ = fmt.Fprintf(r.Stderr, "worker: drain post error for %s: %v\n", p.RunID, postErr)
	}
}

// heartbeatLoop sends POST /heartbeat on HeartbeatInterval until ctx is done
// or ErrClaimReaped is received, in which case it sends on abandonCh.
func (r *Runner) heartbeatLoop(ctx context.Context, runID, claimToken string, abandonCh chan<- struct{}) {
	ticker := time.NewTicker(r.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.Client.Heartbeat(ctx, runID, claimToken); err != nil {
				if errors.Is(err, ErrClaimReaped) {
					select {
					case abandonCh <- struct{}{}:
					default:
					}
					return
				}
				_, _ = fmt.Fprintf(r.Stderr, "worker: heartbeat error for %s: %v\n", runID, err)
			}
		}
	}
}

// postWithRetry attempts to POST the result with exponential backoff up to
// PostMaxElapsed. Returns ErrPostExhausted if retries are exhausted and the
// backend remains unreachable.
func (r *Runner) postWithRetry(ctx context.Context, runID string, payload *ResultRequest) error {
	deadline := r.Now().Add(r.PostMaxElapsed)
	wait := 500 * time.Millisecond
	const maxWait = 30 * time.Second

	for {
		err := r.Client.PostResult(ctx, runID, payload)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrAlreadyCompleted) {
			return ErrAlreadyCompleted
		}
		if !errors.Is(err, backend.ErrNetworkFailure) {
			return err
		}
		// Network failure — retry with backoff if time remains.
		if !r.Now().Before(deadline) {
			return ErrPostExhausted
		}
		_, _ = fmt.Fprintf(r.Stderr, "worker: post failed (network); retrying in %v\n", wait)
		// Cap wait to remaining time so we don't overshoot deadline.
		// Use r.Now() for consistency with the injectable clock.
		remaining := deadline.Sub(r.Now())
		if wait > remaining {
			wait = remaining
		}
		if wait <= 0 {
			return ErrPostExhausted
		}
		if err := r.Sleep(ctx, wait); err != nil {
			return err
		}
		wait *= 2
		if wait > maxWait {
			wait = maxWait
		}
	}
}

// buildResultRequest maps an ExecutionOutcome onto a ResultRequest.
func buildResultRequest(claimToken string, outcome *ExecutionOutcome) *ResultRequest {
	return &ResultRequest{
		ClaimToken:     claimToken,
		CollectionName: outcome.CollectionName,
		RunAt:          outcome.StartedAt,
		DurationMs:     outcome.DurationMs,
		PassCount:      outcome.PassCount,
		FailCount:      outcome.FailCount,
		SkippedCount:   outcome.SkippedCount,
		TriggeredBy:    "schedule",
		Items:          outcome.Items,
	}
}
