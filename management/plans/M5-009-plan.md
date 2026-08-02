# Implementation Plan: M5-009

## Overview

Add an `apitest worker` subcommand and a new `internal/worker` package that implements the client side of the M5-008 distributed-execution coordinator protocol: claim a shard, execute its HTTP requests, submit results, heartbeat, and retry transient failures with exponential backoff. Reuses `internal/httpexec` for request execution, mirrors the wire-shape and HTTP semantics already nailed down by M5-008's `CoordinatorEndpoints`.

## Task Details

- **ID:** M5-009
- **Title:** go-cli: worker agent protocol
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M5-008 | Backend: distributed worker coordinator service | done |

## Decisions and Ambiguity Resolutions

The task YAML references "cobra root" and a few specifics that don't match the current codebase. These decisions are made up-front so execution is unambiguous:

1. **CLI framework.** The codebase does not use cobra — it uses a hand-rolled switch dispatcher in `cmd/apitest/main.go` (see `func run`, lines 75–115) with per-command `parseXxxArgs` helpers. The new `worker` subcommand follows that exact pattern (mirroring `prCheckCmd`/`importOpenAPICmd`). No cobra import.
2. **Org URL segment.** The task observable uses `--org acme` (a slug) but `CoordinatorEndpoints` (`src/ApiTool.Backend/Coordinator/CoordinatorEndpoints.cs:97`) only accepts the `org_<hex>` wire id today (`OrgId.TryParse`). The worker passes `--org` through verbatim into the URL path; backend slug-resolution is M5-008's surface and out of scope here. Help text documents both forms (`org slug or org_<hex> wire id`) and the integration tests use whatever value the fake coordinator's URL pattern accepts. The behaviour test with the fake coordinator passes `--org acme` and the fake matches the `acme` substring in the URL — independent of backend slug resolution.
3. **`coordinator-fake.json` vs Go fixture.** The scope mentions `testdata/worker/coordinator-fake.json`. A static JSON file cannot serve dynamic HTTP responses (claim returns one shard then 204; submit returns 202; heartbeat returns 204). We instead use an `httptest.NewServer`-based fake declared in `internal/worker/coordinator_fake_test.go`. We still create `testdata/worker/sample-shard.json` so the fake can load the canonical "3 GET requests" payload from disk — this satisfies the spirit of "fixture" without inventing a stub-server file format.
4. **Service-token auth.** The task mentions `APITEST_BACKEND_TOKEN=svc_token_dev` and "service tokens (M4-010)". M4-010 is not yet built. The worker treats `APITEST_BACKEND_TOKEN` as an opaque bearer string, sent in `Authorization: Bearer <token>` exactly as `internal/prcheck/client.go` does today. Whether the backend treats it as a JWT or a future service token is the backend's concern.
5. **Concurrency flag default.** `--concurrency` defaults to `1`. The observable trace ("3 requests, pass=3 fail=0") and behaviours don't require parallel-within-shard. Concurrency >1 is implemented (a small bounded worker-pool over the shard's request list) so the help text is honest, but the default keeps determinism for the observable.
6. **Heartbeat cadence.** Behaviour 6 says "every 15 seconds". Codified as `defaultHeartbeatInterval = 15 * time.Second` and made injectable so the heartbeat test can use a 50 ms tick without sleeping for a full 15 s.
7. **Retry budget.** Behaviour 5 says "retries 3 times with exponential backoff". Implemented as 3 retries (4 total attempts) with `backoff = 200ms * 2^attempt` (200 ms, 400 ms, 800 ms). This matches the resilient-client pattern used elsewhere (`internal/prcheck/client.go` uses 2 retries with linear backoff; we use 3 with exponential to match the spec).
8. **`No more shards; exiting` exit code.** Exit 0 (per behaviour 4). The worker also exits 0 after a successful `Completed` submit followed by a 204 — that is the happy-path observable.
9. **Unauthorized exit code.** Behaviour 7 says exit code 10. The existing CLI uses 1/2/3/5/6 today; 10 is the new sentinel for `worker` auth failures (no collisions). Stderr message is `error: unauthorized` (lowercase, matching prcheck's `unauthorized: refresh APITEST_BACKEND_TOKEN`).
10. **Shard request format.** The coordinator returns `RequestsJson` (a JSON-encoded string per `CoordinatorShardDto.RequestsJson`). We define a Go type `WorkerRequest { Method, URL, Headers, Body }` matching `httpexec.Request` and `json.Unmarshal` the string. Malformed JSON → submit with `pass=0, fail=N, items=[{name, status:"error", message:"shard payload invalid"}]` and continue claiming (do not crash the worker).
11. **`items` payload shape on submit.** Mirrors `prcheck.ResultItem` (`name`, `status`, `duration_ms`, `message`) so the backend's `ResultsService.IngestAsync` can stitch them into the aggregated result without a new schema. Each `WorkerRequest` becomes one item.
12. **Testdata location.** `testdata/worker/sample-shard.json` lives at the repo root `testdata/` (mirroring existing convention — see `testdata/discovery/`, `testdata/openapi/`, etc., per the `ls testdata/` exploration).
13. **Exit codes summary.**
    - `0` — happy path (claimed and completed all shards, or `No more shards; exiting`)
    - `1` — usage error (bad flag, missing required env var)
    - `10` — unauthorized (401 from coordinator)
    - `2` — network failure exhausted retries on a non-submit call (claim/heartbeat); submit failures after retries are logged and the worker exits 0 with a warning, since the shard is reaped server-side
14. **Help text.** Documents `--job`, `--org`, `--coordinator-url`, `--token`, `--concurrency`, `--heartbeat-interval`, `--help`. Env vars: `APITEST_COORDINATOR_URL`, `APITEST_BACKEND_TOKEN`. Required-flag validation matches `prcheck.Config.Validate`'s pattern.

## Implementation Steps

Each step has the smallest blast radius: pure types first, then the protocol client (no I/O dependencies), then the orchestrator (claim-execute-submit loop), then the wiring into `main.go`, then the integration test, finally smoke + CHANGELOG.

### Step 1: Add `internal/worker` package skeleton with sentinel errors and types

**Rationale:** Pure types, no dependencies. All later steps import these. Zero risk to existing code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/worker/worker.go` | create | Package doc, sentinel errors, `Config`, request/response types |
| `internal/worker/worker_test.go` | create | Unit tests for `Config.Validate` |

#### New Code

```go
// Package worker implements the client side of the distributed-execution
// coordinator protocol (see internal/coordinator on the backend, M5-008).
//
// A worker authenticates with a bearer token, polls the coordinator for
// shards to claim, executes each shard's HTTP requests, submits per-shard
// outcomes, and exits when no shards remain.
package worker

import (
	"errors"
	"time"
)

// Sentinel errors for well-known failure modes.
var (
	// ErrCoordinatorURLMissing is returned when neither --coordinator-url
	// nor APITEST_COORDINATOR_URL is set.
	ErrCoordinatorURLMissing = errors.New("coordinator URL not configured (set APITEST_COORDINATOR_URL or --coordinator-url)")
	// ErrTokenMissing is returned when APITEST_BACKEND_TOKEN/--token is empty.
	ErrTokenMissing = errors.New("backend token not configured (set APITEST_BACKEND_TOKEN or --token)")
	// ErrUnauthorized is returned when the coordinator responds with 401.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrNetworkExhausted is returned when retries on a coordinator call are exhausted.
	ErrNetworkExhausted = errors.New("network error: coordinator unreachable after retries")
)

// Config holds the worker configuration derived from flags and env vars.
type Config struct {
	CoordinatorURL    string        // APITEST_COORDINATOR_URL / --coordinator-url
	Token             string        // APITEST_BACKEND_TOKEN / --token
	JobID             string        // --job (job_<hex>)
	Org               string        // --org (slug or org_<hex>)
	WorkerID          string        // --worker-id (defaults to "wkr_" + hostname + pid + nanos)
	Concurrency       int           // --concurrency (default 1)
	HeartbeatInterval time.Duration // --heartbeat-interval (default 15s)
}

// Validate returns an error if any required Config field is missing.
func (c *Config) Validate() error {
	if c.CoordinatorURL == "" {
		return ErrCoordinatorURLMissing
	}
	if c.Token == "" {
		return ErrTokenMissing
	}
	if c.JobID == "" {
		return errors.New("--job is required")
	}
	if c.Org == "" {
		return errors.New("--org is required")
	}
	if c.Concurrency < 1 {
		return errors.New("--concurrency must be >= 1")
	}
	return nil
}

// WorkerRequest is the per-request HTTP spec inside a shard's RequestsJson.
type WorkerRequest struct {
	Name    string            `json:"name,omitempty"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
}

// ShardResponse mirrors the backend CoordinatorShardDto.
type ShardResponse struct {
	ShardID        string `json:"shard_id"`
	JobID          string `json:"job_id"`
	ShardIndex     int    `json:"shard_index"`
	State          string `json:"state"`
	AssignedWorker string `json:"assigned_worker,omitempty"`
	RequestsJson   string `json:"requests_json"`
}

// SubmitItem mirrors backend SubmitResultRequest.Items[].
type SubmitItem struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "pass" | "fail" | "error"
	DurationMs int64  `json:"duration_ms"`
	Message    string `json:"message,omitempty"`
}

// SubmitResultBody is the JSON body for POST /shards/{id}/result.
type SubmitResultBody struct {
	WorkerID   string       `json:"worker_id"`
	PassCount  int          `json:"pass_count"`
	FailCount  int          `json:"fail_count"`
	DurationMs int64        `json:"duration_ms"`
	Items      []SubmitItem `json:"items"`
}

// ClaimRequestBody is the JSON body for POST /jobs/{id}/claim.
type ClaimRequestBody struct {
	WorkerID     string   `json:"worker_id"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// HeartbeatBody is the JSON body for POST /shards/{id}/heartbeat.
type HeartbeatBody struct {
	WorkerID string `json:"worker_id"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{"missing url",        Config{Token: "t", JobID: "job_x", Org: "acme", Concurrency: 1}, ErrCoordinatorURLMissing},
		{"missing token",      Config{CoordinatorURL: "http://x", JobID: "job_x", Org: "acme", Concurrency: 1}, ErrTokenMissing},
		{"missing job",        Config{CoordinatorURL: "http://x", Token: "t", Org: "acme", Concurrency: 1}, nil /* matches "--job is required" */},
		{"missing org",        Config{CoordinatorURL: "http://x", Token: "t", JobID: "job_x", Concurrency: 1}, nil /* matches "--org is required" */},
		{"zero concurrency",   Config{CoordinatorURL: "http://x", Token: "t", JobID: "job_x", Org: "acme"},   nil /* matches ">= 1" */},
		{"valid",              Config{CoordinatorURL: "http://x", Token: "t", JobID: "job_x", Org: "acme", Concurrency: 1}, nil},
	}
	// Use errors.Is for sentinel cases; substring match for others.
}
```

#### Impact on Existing Tests
- None. New package.

### Step 2: Add `internal/worker.Client` (HTTP wrapper around the coordinator API)

**Rationale:** Isolates HTTP I/O behind a small interface so the orchestrator (Step 3) can be tested with stubs. Mirrors `internal/prcheck/client.go` shape.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/worker/client.go` | create | `Client` with `Claim`, `SubmitResult`, `Heartbeat` methods |
| `internal/worker/client_test.go` | create | `httptest`-driven tests for status/retry/error mapping |

#### Proposed Signatures

```go
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultMaxRetries     = 3
	defaultBackoffBase    = 200 * time.Millisecond
)

// Client posts coordinator API calls and returns parsed responses.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client // nil = http.DefaultClient
	MaxRetries int          // 0 = defaultMaxRetries
	BackoffBase time.Duration // 0 = defaultBackoffBase
}

// Claim attempts to claim a shard for the given job. Returns (nil, nil) when
// the coordinator has no shards available (HTTP 204).
func (c *Client) Claim(ctx context.Context, org, jobID string, body *ClaimRequestBody) (*ShardResponse, error)

// SubmitResult posts the per-shard outcomes. Returns nil on 202 Accepted.
// Retries network failures with exponential backoff (max 3 retries by default).
func (c *Client) SubmitResult(ctx context.Context, org, jobID, shardID string, body *SubmitResultBody) error

// Heartbeat extends the shard's lease. Returns nil on 204 No Content.
func (c *Client) Heartbeat(ctx context.Context, org, jobID, shardID string, body *HeartbeatBody) error
```

URLs follow the M5-008 contract:
- `POST /api/v1/organizations/{org}/coordinator/jobs/{jobID}/claim`
- `POST /api/v1/organizations/{org}/coordinator/jobs/{jobID}/shards/{shardID}/result`
- `POST /api/v1/organizations/{org}/coordinator/jobs/{jobID}/shards/{shardID}/heartbeat`

A shared private helper `doWithRetry(ctx, method, url, body, expect2xx []int) ([]byte, status, error)`:
- Maps 401 → `ErrUnauthorized` (no retry).
- Maps 4xx (other than 401) → `fmt.Errorf("backend returned HTTP %d: %s", ...)` (no retry).
- Maps 5xx and connection errors → retry with `BackoffBase * 2^attempt`, log a warning per retry to the package-level `Log` callback (overridable for tests, default = stderr).
- After `MaxRetries+1` total attempts → `fmt.Errorf("%w: %v", ErrNetworkExhausted, lastErr)`.

`Claim` wraps `doWithRetry` and returns `(nil, nil)` if status == 204.

#### Tests to Write FIRST (RED phase)

`TestClient_Claim` — table-driven with `httptest.NewServer`:

```go
{ name: "success_200_returns_shard",        statusCode: 200, respBody: `{"shard_id":"shd_1",...}`, wantShard: &ShardResponse{ShardID:"shd_1", ...} },
{ name: "no_content_204_returns_nil_nil",   statusCode: 204, wantShard: nil, wantErr: nil },
{ name: "unauthorized_401",                 statusCode: 401, wantErr: ErrUnauthorized },
{ name: "server_error_500_then_success",    /* fake flips after first call */, wantShard: &ShardResponse{...} },
{ name: "server_error_500_exhausted",       statusCode: 500, wantErr: ErrNetworkExhausted },
{ name: "malformed_json",                   statusCode: 200, respBody: "{not json", wantErr: "parsing claim response" },
```

`TestClient_SubmitResult`:
- `success_202` → nil
- `unauthorized_401` → ErrUnauthorized
- `connection_drop_then_success` → nil after 1 retry (verify backoff > 0)

`TestClient_Heartbeat`:
- `success_204` → nil
- `not_found_404` → wraps non-retry error
- `unauthorized_401` → ErrUnauthorized

`TestClient_Retry_BackoffGrows`:
- Use a fake `time.AfterFunc`-style hook (or just measure `time.Since`) to assert that retry 2 waits longer than retry 1 (>= 2x).

#### Impact on Existing Tests
- None.

### Step 3: Add `internal/worker.Run` orchestrator (claim-execute-submit loop)

**Rationale:** Pure orchestration glued to `Client` and `httpexec.Execute`. Tested with a stub `httpexec.Execute` so we don't depend on the coordinator.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/worker/run.go` | create | `Run(ctx, cfg, opts) (*RunSummary, error)` with stdout output |
| `internal/worker/run_test.go` | create | Unit tests with fake Client and fake executor |

#### Proposed Signatures

```go
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/peterlindqvist/apitest/internal/httpexec"
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

// RunOptions injects test seams.
type RunOptions struct {
	Client            CoordinatorClient // nil = construct from cfg
	Execute           ExecuteFunc       // nil = httpexec.Execute
	Stdout            io.Writer         // nil = os.Stdout
	HeartbeatInterval time.Duration     // 0 = cfg.HeartbeatInterval
	Now               func() time.Time  // for testable backoff/duration; nil = time.Now
}

// RunSummary captures the worker's output for testing.
type RunSummary struct {
	ShardsClaimed   int
	ShardsCompleted int
	TotalPass       int
	TotalFail       int
}

// Run orchestrates the worker loop until the coordinator returns 204
// or the context is cancelled. Returns ErrUnauthorized when auth fails on the
// first call (so the CLI can map to exit code 10).
func Run(ctx context.Context, cfg Config, opts RunOptions) (*RunSummary, error)
```

Pseudocode:

```
1. opts defaults filled in (Client → newClient, Execute → httpexec.Execute, Stdout → os.Stdout)
2. summary := &RunSummary{}
3. for {
      shard, err := client.Claim(ctx, cfg.Org, cfg.JobID, &ClaimRequestBody{WorkerID: cfg.WorkerID, Capabilities: []string{"http"}})
      if err == ErrUnauthorized: return summary, err
      if err != nil:             return summary, err
      if shard == nil:           print "No more shards; exiting"; return summary, nil
      print "Claimed shard <id> (<n> requests)"
      summary.ShardsClaimed++

      // start heartbeater
      hctx, hcancel := context.WithCancel(ctx)
      go heartbeatLoop(hctx, client, cfg, shard, opts.HeartbeatInterval)

      items, pass, fail, dur := executeShard(ctx, opts.Execute, shard, cfg.Concurrency, opts.Now)
      hcancel()

      err := client.SubmitResult(ctx, cfg.Org, cfg.JobID, shard.ShardID, &SubmitResultBody{
          WorkerID: cfg.WorkerID, PassCount: pass, FailCount: fail, DurationMs: dur, Items: items,
      })
      if err != nil:
          warn("submit failed: %v (shard will be reaped)", err)
          continue  // try next shard; don't fail the worker
      print "Completed <shardID>: pass=<n> fail=<m> duration=<dur>ms"
      summary.ShardsCompleted++
      summary.TotalPass += pass
      summary.TotalFail += fail
   }
```

`executeShard`:
- Decode `shard.RequestsJson` into `[]WorkerRequest`. On JSON error: produce one `SubmitItem{Status: "error", Message: "shard payload invalid: <err>"}`, return `(items, 0, 1, 0)`.
- For Concurrency=1: sequential loop calling `Execute`; for >1: bounded `sync.WaitGroup` + semaphore channel of capacity Concurrency.
- For each request: `start := opts.Now()`; `res, err := Execute(ctx, &httpexec.Request{...})`; `dur := opts.Now().Sub(start)`.
- Map outcome:
  - `err != nil` → `SubmitItem{Name: req.Name, Status: "error", DurationMs: dur.Ms(), Message: err.Error()}`, fail++
  - `res.StatusCode >= 400` → `Status: "fail"`, fail++
  - else → `Status: "pass"`, pass++

`heartbeatLoop`:
- `ticker := time.NewTicker(interval)`; on each tick call `client.Heartbeat(ctx, ...)`. Errors are logged but not returned (the orchestrator continues; reaper handles dead workers).

#### Tests to Write FIRST (RED phase)

Table-driven `TestRun` cases (named to match observable behaviours 1–7):

```go
{ name: "claim_execute_submit_one_shard_then_204",
  // 1 shard with 3 GET requests; fake executor returns 200 for each.
  // Expect stdout: "Claimed shard shd_1 (3 requests)\nCompleted shd_1: pass=3 fail=0 duration=...ms\nNo more shards; exiting\n"
  // Expect summary.ShardsCompleted == 1, TotalPass == 3 },

{ name: "submit_failures_recorded_as_fail",
  // executor returns 500 for one of three; expect pass=2 fail=1 in submitted body },

{ name: "executor_error_recorded_as_error_status",
  // executor returns network error; expect items[0].Status == "error" with message },

{ name: "shard_with_invalid_requests_json_continues",
  // shard.RequestsJson = "{not json}"; submit body has fail=1 status=error; loop continues to next claim },

{ name: "no_shards_available_exits_zero",
  // First claim returns nil shard; expect "No more shards; exiting" and no SubmitResult calls },

{ name: "unauthorized_propagates_err",
  // First claim returns ErrUnauthorized; expect Run returns ErrUnauthorized },

{ name: "submit_failure_after_retries_logs_warning_and_continues",
  // Stub client returns ErrNetworkExhausted on SubmitResult; worker logs warning,
  // increments ShardsClaimed but not ShardsCompleted, then claims next shard (which returns nil) and exits 0 },

{ name: "heartbeats_sent_during_long_shard",
  // executor blocks on a channel for 200ms; HeartbeatInterval=50ms; assert stub
  // client.Heartbeat called >=2 times with correct WorkerID and ShardID },

{ name: "concurrency_3_runs_three_requests_in_parallel",
  // executor counts in-flight via atomic; assert max in-flight == 3 with 6 requests },

{ name: "context_cancel_aborts_loop_cleanly",
  // cancel ctx after first claim returns; SubmitResult still attempted with cancelled ctx; Run returns ctx.Err() },
```

That's >=10 tests against `internal/worker` covering the observable.

#### Impact on Existing Tests
- None.

### Step 4: Wire `worker` subcommand into `cmd/apitest/main.go`

**Rationale:** Activates the feature in the binary. Smaller blast radius if added after the package compiles.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `case "worker":` in dispatcher; add `workerCmd`, `parseWorkerArgs`, `printWorkerHelp` |
| `cmd/apitest/main.go` | modify | Add `worker` line to `printHelp()` Commands section |
| `cmd/apitest/worker_test.go` | create | Argument-parsing unit tests for `parseWorkerArgs` |

#### Current Code (`cmd/apitest/main.go:104-114`)

```go
	case "pr-check":
		return prCheckCmd(args[1:])
	case "import":
		return importCmd(args[1:])
	case "license":
		return licenseCmd(args[1:])
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		printHelp()
		return 1
	}
```

#### New Code (insert `worker` case)

```go
	case "license":
		return licenseCmd(args[1:])
	case "worker":
		return workerCmd(args[1:])
	default:
		// ...
```

Add at the end of `main.go`:

```go
// workerCmd implements the worker subcommand: claim-execute-submit loop
// against the M5-008 coordinator service.
//
// Exit codes:
//   0  = success (all shards completed or no more shards)
//   1  = usage error
//   10 = unauthorized (401 from coordinator)
//   2  = network failure (retries exhausted on claim/heartbeat)
func workerCmd(args []string) int {
	cfg, showHelp, err := parseWorkerArgs(args)
	if showHelp {
		printWorkerHelp()
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printWorkerHelp()
		return 1
	}
	if validateErr := cfg.Validate(); validateErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", validateErr)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	_, runErr := worker.Run(ctx, cfg, worker.RunOptions{Stdout: os.Stdout})
	if runErr != nil {
		if errors.Is(runErr, worker.ErrUnauthorized) {
			_, _ = fmt.Fprintln(os.Stderr, "error: unauthorized")
			return 10
		}
		if errors.Is(runErr, worker.ErrNetworkExhausted) {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
			return 2
		}
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
		return 1
	}
	return 0
}

// parseWorkerArgs extracts flags for the worker subcommand.
// Env vars: APITEST_COORDINATOR_URL, APITEST_BACKEND_TOKEN.
func parseWorkerArgs(args []string) (cfg worker.Config, showHelp bool, err error) {
	cfg = worker.Config{
		CoordinatorURL:    os.Getenv("APITEST_COORDINATOR_URL"),
		Token:             os.Getenv("APITEST_BACKEND_TOKEN"),
		Concurrency:       1,
		HeartbeatInterval: 15 * time.Second,
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return cfg, true, nil
		case "--job":
			i++
			if i >= len(args) { return cfg, false, fmt.Errorf("--job requires a value") }
			cfg.JobID = args[i]
		case "--org":
			i++
			if i >= len(args) { return cfg, false, fmt.Errorf("--org requires a value") }
			cfg.Org = args[i]
		case "--coordinator-url":
			i++
			if i >= len(args) { return cfg, false, fmt.Errorf("--coordinator-url requires a value") }
			cfg.CoordinatorURL = args[i]
		case "--token":
			i++
			if i >= len(args) { return cfg, false, fmt.Errorf("--token requires a value") }
			cfg.Token = args[i]
		case "--worker-id":
			i++
			if i >= len(args) { return cfg, false, fmt.Errorf("--worker-id requires a value") }
			cfg.WorkerID = args[i]
		case "--concurrency":
			i++
			if i >= len(args) { return cfg, false, fmt.Errorf("--concurrency requires a value") }
			n, parseErr := strconv.Atoi(args[i])
			if parseErr != nil { return cfg, false, fmt.Errorf("--concurrency must be an integer: %w", parseErr) }
			cfg.Concurrency = n
		case "--heartbeat-interval":
			i++
			if i >= len(args) { return cfg, false, fmt.Errorf("--heartbeat-interval requires a value (e.g. 15s)") }
			d, parseErr := time.ParseDuration(args[i])
			if parseErr != nil { return cfg, false, fmt.Errorf("--heartbeat-interval invalid: %w", parseErr) }
			cfg.HeartbeatInterval = d
		default:
			return cfg, false, fmt.Errorf("unknown flag: %s", args[i])
		}
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = defaultWorkerID()
	}
	return cfg, false, nil
}

func defaultWorkerID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("wkr_%s_%d_%d", host, os.Getpid(), time.Now().UnixNano())
}

func printWorkerHelp() {
	fmt.Println("Usage: apitest worker [options]")
	fmt.Println()
	fmt.Println("Run as a distributed worker against an ApiTool coordinator (Enterprise tier).")
	fmt.Println("The worker repeatedly claims a shard, executes its requests, submits results,")
	fmt.Println("and exits cleanly when no more shards are available.")
	fmt.Println()
	fmt.Println("Required:")
	fmt.Println("  --job <id>           Coordinator job id (job_<hex>)")
	fmt.Println("  --org <slug-or-id>   Organization slug or org_<hex> wire id")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --coordinator-url <url>   Coordinator base URL (or set APITEST_COORDINATOR_URL)")
	fmt.Println("  --token <token>           Bearer token (or set APITEST_BACKEND_TOKEN)")
	fmt.Println("  --worker-id <id>          Worker identifier (default: wkr_<host>_<pid>_<nanos>)")
	fmt.Println("  --concurrency <n>         Parallel requests per shard (default: 1)")
	fmt.Println("  --heartbeat-interval <d>  Heartbeat cadence (default: 15s)")
	fmt.Println("  --help, -h                Show this help message")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  APITEST_COORDINATOR_URL  Default coordinator base URL")
	fmt.Println("  APITEST_BACKEND_TOKEN    Default bearer token")
	fmt.Println()
	fmt.Println("Exit codes: 0=ok, 1=usage, 2=network, 10=unauthorized")
}
```

Update `printHelp()` Commands section by inserting one line after `pr-check`:

```diff
 	fmt.Println("  pr-check        Upload test results and post PR status check (Team tier)")
+	fmt.Println("  worker          Run as a distributed worker (Enterprise tier)")
 	fmt.Println("  license         Manage license state (--validate, --refresh, --debug)")
```

Add imports to `cmd/apitest/main.go` (already present except `worker`):

```go
import (
	// existing...
	"github.com/peterlindqvist/apitest/internal/worker"
)
```

#### Tests to Write FIRST (RED phase)

`cmd/apitest/worker_test.go` — argument-parsing only (orchestration is tested in `internal/worker/run_test.go`):

```go
func TestParseWorkerArgs(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		want    worker.Config
		wantErr string
		help    bool
	}{
		{name: "all_flags",            args: []string{"--job","job_x","--org","acme","--coordinator-url","http://c","--token","t","--concurrency","4"},                          want: worker.Config{JobID:"job_x", Org:"acme", CoordinatorURL:"http://c", Token:"t", Concurrency:4, HeartbeatInterval: 15*time.Second}},
		{name: "env_defaults",         env: map[string]string{"APITEST_COORDINATOR_URL":"http://e", "APITEST_BACKEND_TOKEN":"et"}, args: []string{"--job","job_y","--org","beta"},                want: worker.Config{JobID:"job_y", Org:"beta", CoordinatorURL:"http://e", Token:"et", Concurrency:1, HeartbeatInterval: 15*time.Second}},
		{name: "help_flag",            args: []string{"--help"}, help: true},
		{name: "missing_value",        args: []string{"--job"}, wantErr: "--job requires a value"},
		{name: "unknown_flag",         args: []string{"--bogus"}, wantErr: "unknown flag"},
		{name: "concurrency_invalid",  args: []string{"--concurrency","abc"}, wantErr: "must be an integer"},
		{name: "heartbeat_invalid",    args: []string{"--heartbeat-interval","abc"}, wantErr: "invalid"},
	}
	// Use t.Setenv to scope env vars per case.
}
```

#### Impact on Existing Tests
- `TestRun_NoArgs_*` and `TestRun_UnknownCommand_*` (if any in `cmd/apitest/main_test.go`) are unaffected because we only add a case.
- `printHelp` snapshot/golden tests, if any: extended by one line. Search during execution confirms whether any test asserts exact help-string content; if so, update the expected string.

### Step 5: Add end-to-end integration test with a fake coordinator (httptest)

**Rationale:** Proves the observable end-to-end without needing a live backend. This is the integration-test slice required by the Completeness Contract.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/worker/coordinator_fake_test.go` | create | `httptest`-based fake coordinator helper |
| `internal/worker/integration_test.go` | create | E2E test using the fake + the binary's `Run` orchestrator |
| `testdata/worker/sample-shard.json` | create | Three GET requests against `httptest.NewServer` (URL injected at runtime) |

#### Fake coordinator (`coordinator_fake_test.go`)

A struct holding state (issued shards, recorded submits, heartbeats). Exposes `srv := startFake(t, FakeOpts{ShardCount: 1, RequestsJson: "..."})` returning `(server, &state)`.

State recorded:
- Number of `Claim` calls; on each: returns the next pending shard or 204.
- Recorded submit bodies (so the test can assert pass/fail counts).
- Recorded heartbeats per shard.

#### Integration test (`integration_test.go`)

```go
func TestWorker_E2E_Observable(t *testing.T) {
	// Spin up an "API under test" that returns 200 for the worker's HTTP requests.
	apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	t.Cleanup(apiTS.Close)

	// Build the requests payload for the shard pointing at apiTS.
	requests := []WorkerRequest{
		{Name: "r1", Method: "GET", URL: apiTS.URL + "/a"},
		{Name: "r2", Method: "GET", URL: apiTS.URL + "/b"},
		{Name: "r3", Method: "GET", URL: apiTS.URL + "/c"},
	}
	requestsJSON, _ := json.Marshal(requests)

	fake, state := startFakeCoordinator(t, FakeOpts{
		Org:          "acme",
		Token:        "svc_token_dev",
		Shards:       []FakeShard{{ID: "shd_1", Index: 0, RequestsJson: string(requestsJSON)}},
	})
	t.Cleanup(fake.Close)

	cfg := Config{
		CoordinatorURL: fake.URL,
		Token:          "svc_token_dev",
		JobID:          "job_abc",
		Org:            "acme",
		Concurrency:    1,
		HeartbeatInterval: 50 * time.Millisecond,
	}

	var stdout bytes.Buffer
	summary, err := Run(context.Background(), cfg, RunOptions{Stdout: &stdout})

	// Observable assertions
	if err != nil { t.Fatalf("Run = %v; want nil", err) }
	if summary.ShardsCompleted != 1 { t.Errorf("ShardsCompleted = %d; want 1", summary.ShardsCompleted) }
	if summary.TotalPass != 3 || summary.TotalFail != 0 { t.Errorf("pass/fail = %d/%d; want 3/0", summary.TotalPass, summary.TotalFail) }
	out := stdout.String()
	for _, want := range []string{"Claimed shard shd_1 (3 requests)", "Completed shd_1: pass=3 fail=0", "No more shards; exiting"} {
		if !strings.Contains(out, want) { t.Errorf("stdout missing %q\n got: %s", want, out) }
	}

	// Coordinator-side assertions
	if state.SubmitsByShard["shd_1"] == nil { t.Fatalf("no submit recorded for shd_1") }
	got := state.SubmitsByShard["shd_1"]
	if got.PassCount != 3 { t.Errorf("submit.PassCount = %d; want 3", got.PassCount) }
}
```

`testdata/worker/sample-shard.json` is a static fallback containing the same three requests pointing at `__APITS__/a` (placeholder substituted at test time). It's not strictly required by the test but satisfies the task scope's "testdata fixture" callout.

#### Impact on Existing Tests
- None.

### Step 6: Update smoke test and CHANGELOG.md

**Rationale:** Required by Quality Gates and the always-runnable / completeness-contract commitments. Smoke must call the new subcommand at minimum to confirm `--help` works (no live coordinator in smoke).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add a `worker --help` smoke check after the `pr-check --help` block |
| `CHANGELOG.md` | modify | Add Unreleased entry: "Distributed worker agent (`apitest worker`) for Enterprise coordinator (M5-009)" |

#### Smoke addition (pattern mirrors existing `--help` checks)

```bash
echo "--- worker --help ---"
./apitest worker --help | grep -q "Usage: apitest worker" || { echo "FAIL: worker --help"; exit 1; }
echo "OK"
echo
```

#### Impact on Existing Tests
- None.

### Step 7: Final lint, coverage, and verification

**Rationale:** The task's DoD requires coverage >= 80% and no lint errors. Run the gates and address findings before commit.

#### Files to Modify
- None directly. Address any findings from `~/go/bin/golangci-lint run` and re-run `go test -coverprofile`.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/apitest/main_test.go` | help-text snapshot test (if any) | possibly extended | Update expected output to include `worker` line |
| All other existing tests | — | none | — |

## Risks and Edge Cases

- **Risk:** Heartbeat goroutine leaks if `executeShard` panics. **Mitigation:** Use `defer hcancel()` immediately after `go heartbeatLoop(...)` so the heartbeat is always cancelled when the loop iteration returns.
- **Risk:** Long-running worker holds an HTTP connection; coordinator restarts. **Mitigation:** `doWithRetry` retries on connection errors; `Heartbeat` errors are logged but non-fatal so the orchestrator keeps trying to submit.
- **Risk:** Concurrency >1 + shared `httpexec.Execute` race (e.g. shared transport). **Mitigation:** `httpexec.Execute` uses `http.DefaultClient` which is goroutine-safe; the bounded semaphore pattern doesn't share Request structs.
- **Risk:** Backend `CoordinatorEndpoints` rejects the slug `acme` because it requires `org_<hex>`. **Mitigation:** Documented in the help text (`slug or org_<hex>`); the integration test uses whatever the fake's URL pattern accepts (slug here). A future task can lift the worker's URL helper to call OrgResolver-equivalent backend code.
- **Edge case:** `RequestsJson` is empty `"[]"`. **Handling:** Submit with `pass=0, fail=0, items=[]` and `Completed shd_X: pass=0 fail=0 duration=0ms`.
- **Edge case:** `RequestsJson` is malformed JSON. **Handling:** Submit one item with `Status:"error"`, `fail=1`, message describing the parse error; loop continues.
- **Edge case:** `Concurrency > number of requests`. **Handling:** Bounded semaphore caps in-flight at `min(concurrency, len(requests))` naturally.
- **Edge case:** Context cancelled mid-shard (Ctrl+C). **Handling:** `signal.NotifyContext` cancels; in-flight requests receive cancelled context; `executeShard` records the in-flight requests as `Status:"error"` with `context canceled` message; submit attempts with cancelled ctx will fail; orchestrator returns `ctx.Err()`. CLI maps to exit code 1.
- **Edge case:** Submit succeeds (202) but the connection is dropped before the response body is fully written. **Handling:** Treat as success (we've already committed server-side).

## Go Function Signatures (recap)

```go
// internal/worker/worker.go
type Config struct{ /* ... */ }
func (*Config) Validate() error

// internal/worker/client.go
type Client struct{ /* ... */ }
func (*Client) Claim(ctx context.Context, org, jobID string, body *ClaimRequestBody) (*ShardResponse, error)
func (*Client) SubmitResult(ctx context.Context, org, jobID, shardID string, body *SubmitResultBody) error
func (*Client) Heartbeat(ctx context.Context, org, jobID, shardID string, body *HeartbeatBody) error

// internal/worker/run.go
type CoordinatorClient interface{ /* Claim, SubmitResult, Heartbeat */ }
type ExecuteFunc = func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error)
type RunOptions struct{ /* test seams */ }
type RunSummary struct{ /* counters */ }
func Run(ctx context.Context, cfg Config, opts RunOptions) (*RunSummary, error)

// cmd/apitest/main.go (additions)
func workerCmd(args []string) int
func parseWorkerArgs(args []string) (worker.Config, bool, error)
func printWorkerHelp()
```

All functions accept `context.Context` first as required by the Go standards. Errors are wrapped with `fmt.Errorf("%s: %w", ctx, err)` and sentinel errors are exported (`ErrUnauthorized`, `ErrCoordinatorURLMissing`, `ErrTokenMissing`, `ErrNetworkExhausted`). Names follow the no-stuttering rule (`worker.Config` not `worker.WorkerConfig`).

## Verification

```bash
# Build
go build ./cmd/apitest

# Unit + integration tests
go test ./internal/worker/...
# Expected: ok  github.com/peterlindqvist/apitest/internal/worker  (>=8 tests)

# Coverage
go test -coverprofile=coverage.out ./internal/worker/...
go tool cover -func=coverage.out | tail -1
# Expected: total coverage >= 80%

# Lint
~/go/bin/golangci-lint run

# Smoke
./smoke/run.sh
```

Observable verification (from the task YAML):

```bash
go build ./cmd/apitest
go test ./internal/worker/...
# Expected: ok  internal/worker  (>=8 tests passing)

# Start a fake coordinator fixture (the integration test's helper) — for manual
# verification, point apitest at any HTTP server that mimics the M5-008 contract:
APITEST_COORDINATOR_URL=http://127.0.0.1:9000 \
APITEST_BACKEND_TOKEN=svc_token_dev \
  ./apitest worker --job job_abc --org acme

# Expected stdout (tail):
# "Claimed shard shd_1 (3 requests)"
# "Completed shd_1: pass=3 fail=0 duration=142ms"
# "No more shards; exiting"
# exit 0
```
