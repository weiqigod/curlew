// Package distributed orchestrates the distributed execution path for apitest run.
// When --workers N is specified (N ≥ 2), Run creates a coordinator job, waits for
// N workers to join, polls until all shards complete, and returns aggregated results
// compatible with the existing terminal/json/tap/junit printers.
//
// Only the main-phase requests are sharded. Setup and teardown are always run locally
// on the CLI host, and any extracted variables are propagated to the coordinator job
// via the PreExecVars field.
package distributed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/runner"
	"github.com/peterlindqvist/apitest/internal/runner/shard"
	"github.com/peterlindqvist/apitest/internal/worker"
)

// Sentinel errors.
var (
	// ErrWorkerJoinTimeout is returned when workers fail to join within JoinTimeout.
	ErrWorkerJoinTimeout = errors.New("timed out waiting for workers to join")
)

// Config is the distributed-run request.
type Config struct {
	CoordinatorURL string // required
	Token          string // required (Bearer)
	Org            string // --org (slug or org_<hex>)
	Workers        int    // --workers (N, must be ≥ 2 here; caller validates)
	CollectionSha  string // opaque id (hash of collection file bytes)
	Collection     *parser.Collection
	PreExecVars    map[string]string // variables already resolved locally
	JoinTimeout    time.Duration     // default 60s
	PollInterval   time.Duration     // default 500ms
	JobTimeout     time.Duration     // default 30m
	Stdout         io.Writer         // default os.Stdout — progress logs
}

// Deps are injection seams for tests.
type Deps struct {
	Client CoordinatorClient // nil → construct from cfg
	Now    func() time.Time  // nil → time.Now
}

// CoordinatorClient is the extended client interface used by Run.
type CoordinatorClient interface {
	CreateJob(ctx context.Context, org string, body *CreateJobBody) (*JobResponse, error)
	GetJob(ctx context.Context, org, jobID string) (*JobResponse, error)
}

// CreateJobBody is the wire body sent to the coordinator.
// The Shards extension field is honoured by the fake coordinator and
// by future backend versions (System.Text.Json ignores unknown properties).
type CreateJobBody struct {
	CollectionSha string            `json:"collection_sha"`
	ShardCount    int               `json:"shard_count"`
	Shards        []ShardPayload    `json:"shards,omitempty"` // forward-compat
	Variables     map[string]string `json:"variables,omitempty"`
}

// ShardPayload carries per-shard requests when creating a job.
type ShardPayload struct {
	Index        int    `json:"index"`
	RequestsJson string `json:"requests_json"` // JSON array of worker.WorkerRequest
}

// JobResponse mirrors the backend CoordinatorJobDto plus aggregated shard outcomes.
type JobResponse struct {
	JobID       string        `json:"job_id"`
	State       string        `json:"state"` // pending | running | completed
	ShardCount  int           `json:"shard_count"`
	Shards      []ShardStatus `json:"shards"`
	WorkerCount int           `json:"worker_count,omitempty"`
}

// ShardStatus describes a single shard's state.
type ShardStatus struct {
	ShardID        string              `json:"shard_id"`
	Index          int                 `json:"shard_index"`
	State          string              `json:"state"` // pending | running | completed | reassigned
	AssignedWorker string              `json:"assigned_worker,omitempty"`
	Items          []worker.SubmitItem `json:"items,omitempty"`
	PassCount      int                 `json:"pass_count,omitempty"`
	FailCount      int                 `json:"fail_count,omitempty"`
}

const (
	defaultJoinTimeout  = 60 * time.Second
	defaultPollInterval = 500 * time.Millisecond
	defaultJobTimeout   = 30 * time.Minute
)

// Run creates a coordinator job, waits for workers to join, polls until all
// shards complete (or reassignments resolve), aggregates per-shard items back
// into []runner.RequestResult ordered to match cfg.Collection.Requests.Items,
// and returns a runner.Summary compatible with the terminal printer.
func Run(ctx context.Context, cfg Config, deps Deps) ([]runner.RequestResult, *runner.Summary, error) {
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	joinTimeout := cfg.JoinTimeout
	if joinTimeout == 0 {
		joinTimeout = defaultJoinTimeout
	}
	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}
	jobTimeout := cfg.JobTimeout
	if jobTimeout == 0 {
		jobTimeout = defaultJobTimeout
	}
	nowFn := deps.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	client := deps.Client
	if client == nil {
		client = &HTTPClient{
			BaseURL: cfg.CoordinatorURL,
			Token:   cfg.Token,
		}
	}

	items := cfg.Collection.Requests.Items
	n := cfg.Workers
	total := len(items)

	start := nowFn()
	_, _ = fmt.Fprintf(stdout, "Sharding %d requests across %d workers...\n", total, n)

	// Build shard plans.
	plans := shard.Split(items, n)

	// Encode each shard's requests as worker.WorkerRequest JSON.
	shardPayloads := make([]ShardPayload, len(plans))
	for i, plan := range plans {
		reqs := make([]worker.WorkerRequest, len(plan.Requests))
		for j, ri := range plan.Requests {
			reqs[j] = worker.WorkerRequest{
				Name:   ri.Name,
				Method: ri.Request.Method,
				URL:    ri.Request.URL,
			}
		}
		data, err := json.Marshal(reqs)
		if err != nil {
			return nil, nil, fmt.Errorf("encoding shard %d requests: %w", i, err)
		}
		shardPayloads[i] = ShardPayload{
			Index:        i,
			RequestsJson: string(data),
		}
	}

	// Create the job.
	jobResp, err := client.CreateJob(ctx, cfg.Org, &CreateJobBody{
		CollectionSha: cfg.CollectionSha,
		ShardCount:    n,
		Shards:        shardPayloads,
		Variables:     cfg.PreExecVars,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating coordinator job: %w", err)
	}
	jobID := jobResp.JobID

	// Poll for workers to join.
	lastWorkerCount := -1
	joinDeadline := nowFn().Add(joinTimeout)
	for {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if nowFn().After(joinDeadline) {
			return nil, nil, fmt.Errorf("%w: %d/%d joined", ErrWorkerJoinTimeout, lastWorkerCount, n)
		}

		resp, pollErr := client.GetJob(ctx, cfg.Org, jobID)
		if pollErr != nil {
			return nil, nil, fmt.Errorf("polling job status: %w", pollErr)
		}

		if resp.WorkerCount != lastWorkerCount {
			lastWorkerCount = resp.WorkerCount
			_, _ = fmt.Fprintf(stdout, "Waiting for workers... %d/%d joined\n", lastWorkerCount, n)
		}
		if resp.WorkerCount >= n {
			break
		}

		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	// Poll until all shards are complete.
	jobDeadline := nowFn().Add(jobTimeout)
	reported := make(map[string]bool) // tracks shards already reported
	reassigned := make(map[string]bool)

	for {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if nowFn().After(jobDeadline) {
			return nil, nil, fmt.Errorf("job timed out after %s", jobTimeout)
		}

		resp, pollErr := client.GetJob(ctx, cfg.Org, jobID)
		if pollErr != nil {
			return nil, nil, fmt.Errorf("polling job status: %w", pollErr)
		}

		allDone := true
		for _, s := range resp.Shards {
			switch s.State {
			case "completed":
				if !reported[s.ShardID] {
					reported[s.ShardID] = true
					_, _ = fmt.Fprintf(stdout, "Shard %s done (%d/%d pass)\n",
						s.ShardID, s.PassCount, s.PassCount+s.FailCount)
				}
			case "reassigned":
				if !reassigned[s.ShardID] {
					reassigned[s.ShardID] = true
					_, _ = fmt.Fprintf(stdout, "Shard %s reassigned (worker timeout)\n", s.ShardID)
				}
				allDone = false
			default:
				allDone = false
			}
		}

		if allDone {
			// Aggregate results.
			return aggregate(stdout, items, plans, resp, nowFn().Sub(start))
		}

		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// aggregate converts shards' SubmitItems back to runner.RequestResult slices
// ordered by the original item index and computes the summary.
func aggregate(stdout io.Writer, items []parser.RequestItem, plans []shard.Plan, resp *JobResponse, elapsed time.Duration) ([]runner.RequestResult, *runner.Summary, error) {
	results := make([]runner.RequestResult, len(items))
	summary := &runner.Summary{Total: len(items), Duration: elapsed}

	// Build a map from shardID → ShardStatus for fast lookup.
	shardByID := make(map[string]*ShardStatus, len(resp.Shards))
	for i := range resp.Shards {
		shardByID[resp.Shards[i].ShardID] = &resp.Shards[i]
	}

	for si, plan := range plans {
		shardID := shardIDForIndex(si)
		s := shardByID[shardID]
		for j, origIdx := range plan.OriginalIndex {
			name := items[origIdx].Name
			rr := runner.RequestResult{
				Name:  name,
				Phase: runner.PhaseMain,
			}
			if s != nil && j < len(s.Items) {
				item := s.Items[j]
				switch item.Status {
				case "pass":
					summary.Passed++
				default:
					summary.Failed++
					rr.Err = errors.New(item.Message)
				}
			} else {
				// No result for this item — treat as unknown failure.
				summary.Failed++
				rr.Err = fmt.Errorf("no result received for request %q", name)
			}
			results[origIdx] = rr
		}
	}

	totalPass := summary.Passed
	totalAll := summary.Total
	_, _ = fmt.Fprintf(stdout, "All shards complete: %d/%d pass\n", totalPass, totalAll)

	return results, summary, nil
}

// shardIDForIndex returns the shard ID for the given 0-based shard index.
// This must match what the fake and real coordinator use.
func shardIDForIndex(idx int) string {
	return fmt.Sprintf("shd_%d", idx+1)
}
