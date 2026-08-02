package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
)

// ExecuteFunc is the request-execution function (overridable for tests).
type ExecuteFunc func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error)

// CoordinatorClient is the subset of *Client used by Run, exposed as an
// interface for test stubbing.
type CoordinatorClient interface {
	Claim(ctx context.Context, org, jobID string, body *ClaimRequestBody) (*ShardResponse, error)
	SubmitResult(ctx context.Context, org, jobID, shardID string, body *SubmitResultBody) error
	Heartbeat(ctx context.Context, org, jobID, shardID string, body *HeartbeatBody) error
}

// RunOptions injects test seams into Run.
type RunOptions struct {
	Client            CoordinatorClient // nil = construct from cfg
	Execute           ExecuteFunc       // nil = httpexec.Execute
	Stdout            io.Writer         // nil is accepted; reserved for a future stdout payload (e.g. --report JSON) and is not currently read by Run.
	Stderr            io.Writer         // nil = os.Stderr — carries progress and warnings
	HeartbeatInterval time.Duration     // 0 = cfg.HeartbeatInterval
}

// RunSummary captures the worker's output for testing and logging.
type RunSummary struct {
	ShardsClaimed   int
	ShardsCompleted int
	TotalPass       int
	TotalFail       int
}

// Run orchestrates the worker loop until the coordinator returns 204 (no more
// shards) or the context is cancelled. Returns ErrUnauthorized when auth fails
// on the first coordinator call (so the CLI can map to exit code 10).
func Run(ctx context.Context, cfg Config, opts RunOptions) (*RunSummary, error) {
	// Fill in defaults for injectable seams.
	client := opts.Client
	if client == nil {
		client = &Client{
			BaseURL: cfg.CoordinatorURL,
			Token:   cfg.Token,
		}
	}
	execute := opts.Execute
	if execute == nil {
		execute = httpexec.Execute
	}
	// stdout is reserved for a future declared result payload (e.g. --report JSON).
	// Progress and warnings are routed to stderr.
	_ = opts.Stdout // seam retained for API stability
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	hbInterval := opts.HeartbeatInterval
	if hbInterval == 0 {
		hbInterval = cfg.HeartbeatInterval
	}
	if hbInterval == 0 {
		hbInterval = 15 * time.Second
	}

	summary := &RunSummary{}

	for {
		// Check context before each claim.
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}

		shard, err := client.Claim(ctx, cfg.Org, cfg.JobID, &ClaimRequestBody{
			WorkerID:     cfg.WorkerID,
			Capabilities: []string{"http"},
		})
		if err != nil {
			return summary, err
		}
		if shard == nil {
			_, _ = fmt.Fprintln(stderr, "No more shards; exiting")
			return summary, nil
		}

		// Decode requests to get count for logging.
		var requests []WorkerRequest
		jsonErr := json.Unmarshal([]byte(shard.RequestsJson), &requests)
		if jsonErr == nil {
			_, _ = fmt.Fprintf(stderr, "Claimed shard %s (%d requests)\n", shard.ShardID, len(requests))
		} else {
			_, _ = fmt.Fprintf(stderr, "Claimed shard %s (payload error)\n", shard.ShardID)
		}
		summary.ShardsClaimed++

		// Start heartbeater; hcancel is called explicitly after executeShard
		// returns to ensure the goroutine is stopped per shard (not deferred,
		// which would accumulate cancels across shard iterations).
		hctx, hcancel := context.WithCancel(ctx)
		go heartbeatLoop(hctx, client, cfg, shard.ShardID, hbInterval, stderr)

		// Execute the shard.
		var items []SubmitItem
		var pass, fail int
		var durMs int64

		if jsonErr != nil {
			// Malformed RequestsJson: submit a single error item.
			items = []SubmitItem{{
				Name:    shard.ShardID,
				Status:  "error",
				Message: fmt.Sprintf("shard payload invalid: %v", jsonErr),
			}}
			fail = 1
		} else {
			items, pass, fail, durMs = executeShard(ctx, execute, requests, cfg.Concurrency)
		}

		hcancel()

		submitErr := client.SubmitResult(ctx, cfg.Org, cfg.JobID, shard.ShardID, &SubmitResultBody{
			WorkerID:   cfg.WorkerID,
			PassCount:  pass,
			FailCount:  fail,
			DurationMs: durMs,
			Items:      items,
		})
		if submitErr != nil {
			_, _ = fmt.Fprintf(stderr, "warning: submit failed for %s: %v (shard will be reaped)\n", shard.ShardID, submitErr)
			continue
		}

		_, _ = fmt.Fprintf(stderr, "Completed %s: pass=%d fail=%d duration=%dms\n", shard.ShardID, pass, fail, durMs)
		summary.ShardsCompleted++
		summary.TotalPass += pass
		summary.TotalFail += fail
	}
}

// executeShard runs all requests in a shard and collects results.
// Returns (items, passCount, failCount, totalDurationMs).
func executeShard(ctx context.Context, execute ExecuteFunc, requests []WorkerRequest, concurrency int) ([]SubmitItem, int, int, int64) {
	if concurrency <= 1 {
		return executeShardSequential(ctx, execute, requests)
	}
	return executeShardConcurrent(ctx, execute, requests, concurrency)
}

func executeShardSequential(ctx context.Context, execute ExecuteFunc, requests []WorkerRequest) ([]SubmitItem, int, int, int64) {
	items := make([]SubmitItem, 0, len(requests))
	var pass, fail int
	var totalMs int64

	for i, req := range requests {
		name := req.Name
		if name == "" {
			name = fmt.Sprintf("request_%d", i+1)
		}
		item, p, f, ms := executeOne(ctx, execute, name, &req)
		items = append(items, item)
		pass += p
		fail += f
		totalMs += ms
	}
	return items, pass, fail, totalMs
}

func executeShardConcurrent(ctx context.Context, execute ExecuteFunc, requests []WorkerRequest, concurrency int) ([]SubmitItem, int, int, int64) {
	type result struct {
		idx  int
		item SubmitItem
		pass int
		fail int
		ms   int64
	}

	results := make([]result, len(requests))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		i, req := i, req // capture
		name := req.Name
		if name == "" {
			name = fmt.Sprintf("request_%d", i+1)
		}
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			item, p, f, ms := executeOne(ctx, execute, name, &req)
			results[i] = result{idx: i, item: item, pass: p, fail: f, ms: ms}
		}()
	}
	wg.Wait()

	items := make([]SubmitItem, len(requests))
	var pass, fail int
	var totalMs int64
	for _, r := range results {
		items[r.idx] = r.item
		pass += r.pass
		fail += r.fail
		totalMs += r.ms
	}
	return items, pass, fail, totalMs
}

// executeOne runs a single request and returns a SubmitItem plus pass/fail counts.
func executeOne(ctx context.Context, execute ExecuteFunc, name string, req *WorkerRequest) (SubmitItem, int, int, int64) {
	start := time.Now()
	res, err := execute(ctx, &httpexec.Request{
		Method:  req.Method,
		URL:     req.URL,
		Headers: req.Headers,
		Body:    req.Body,
	})
	ms := time.Since(start).Milliseconds()

	item := SubmitItem{Name: name, DurationMs: ms}
	if err != nil {
		item.Status = "error"
		item.Message = err.Error()
		return item, 0, 1, ms
	}
	if res.StatusCode >= 400 {
		item.Status = "fail"
		item.Message = fmt.Sprintf("HTTP %d", res.StatusCode)
		return item, 0, 1, ms
	}
	item.Status = "pass"
	return item, 1, 0, ms
}

// heartbeatLoop sends periodic heartbeat POSTs until the context is cancelled.
// stderr receives non-fatal heartbeat warnings via the injected writer so tests
// can capture them through the RunOptions.Stderr seam.
func heartbeatLoop(ctx context.Context, client CoordinatorClient, cfg Config, shardID string, interval time.Duration, stderr io.Writer) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Heartbeat(ctx, cfg.Org, cfg.JobID, shardID, &HeartbeatBody{WorkerID: cfg.WorkerID}); err != nil {
				// Heartbeat errors are non-fatal; the server's reaper handles dead workers.
				_, _ = fmt.Fprintf(stderr, "warning: heartbeat failed for %s: %v\n", shardID, err)
			}
		}
	}
}
