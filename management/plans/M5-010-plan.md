# Implementation Plan: M5-010

## Overview
Add a `--workers N` flag to `curlew run` that, when >1, delegates execution to a remote coordinator (M5-008 backend + M5-009 workers). The CLI shards requests locally, creates a coordinator job, waits for workers to join, polls for completion, and renders the aggregated shard results in the existing terminal format. When the flag is omitted the run proceeds through the existing single-process pipeline with zero behavioural change.

## Task Details
- **ID:** M5-010
- **Title:** go-cli: --workers flag for distributed run execution
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task   | Title                                            | Status |
|--------|--------------------------------------------------|--------|
| M5-009 | go-cli: worker agent protocol                    | done   |
| M5-008 | Backend: distributed worker coordinator service  | done   |

## Key Findings From Code Exploration

### Relevant existing files (read in full)
- `cmd/curlew/main.go` (2876 lines) — command dispatch, `runCmd`, `runCmdInner`, `parseRunArgs`/`runFlags`, `printHelp` (line 2591). `run` commands route through `checkGraceExpired` → `runCmdInner` which calls `runner.Run` at line 660.
- `cmd/curlew/worker.go` — the existing `curlew worker` subcommand wiring (M5-009). Shows the flag-parsing style, env-var fallback, and exit-code mapping (0/1/2/10) that `--workers` must align with.
- `internal/worker/worker.go` — `Config`, `Client`, sentinel errors, `WorkerRequest`, `ShardResponse`, `ClaimRequestBody`, `SubmitItem`, `SubmitResultBody`, `HeartbeatBody`. All wire shapes are already defined.
- `internal/worker/client.go` — `Claim`, `SubmitResult`, `Heartbeat` with auth, retry and `ErrUnauthorized` / `ErrNetworkExhausted` sentinels.
- `internal/worker/run.go` — the per-worker loop and `CoordinatorClient` interface we can reuse in the CLI orchestrator for `GET /jobs/{job_id}`-style polling (we'll add a new method for that).
- `internal/worker/coordinator_fake_test.go` — `startFakeCoordinator` test helper + `FakeShard` / `FakeState`. We'll extend it in this task's tests (or mirror it in the new package) so the CLI integration test has the same fixture shape.
- `internal/runner/runner.go` — `runner.Run`, `VarSources`, `RequestResult`, `Summary`. We reuse `RequestResult` / `Summary` types for printing so the distributed path prints the same summary as the local path.
- `internal/parser/collection.go` — `Collection`, `RequestItem`, `Request`. Sharding will operate on `col.Requests.Items` (the main phase), leaving `Setup` and `Teardown` on the local path (see "Risks & edge cases").
- `src/ApiTool.Backend/Coordinator/CreateJobRequest.cs` — backend accepts only `collection_sha` and `shard_count`. **The coordinator does not currently ingest per-shard request payloads.** See "Ambiguities resolved" for how we handle this.
- `src/ApiTool.Backend/Coordinator/CoordinatorEndpoints.cs` — confirms URL shape the CLI must target: `/api/v1/organizations/{orgId}/coordinator/jobs[/{jobId}[/claim|/shards/{shardId}/result|/heartbeat]]`.
- `internal/auth/registry.go` — feature-gate registry. No `distributed_execution` feature currently exists; we must register it as `TierEnterprise`.
- `smoke/run.sh` — has a small `worker --help` block around line 1687. We extend it with a `run --workers` help + env-var validation smoke block.

### Call-site / signature impact
- `runner.Run` signature is **unchanged**. We do not alter `internal/runner` at the package boundary.
- `parseRunArgs` gains two fields (`workers int`, `coordinatorURL string`). No caller breaks because `runFlags` is `main`-package internal and assigned as a struct literal in a single place.
- `printHelp` gains two lines in the "Run Options" block plus an "Enterprise tier" section.
- Existing `curlew worker` subcommand is untouched.

### Ambiguities resolved
1. **How do shard payloads reach workers?** The M5-008 backend seeds every new shard with `RequestsJson = "[]"`; the worker fake in tests populates `RequestsJson` directly. Since this task is `track: go-cli` and backend changes are out of scope, we design the CLI to target the **existing M5-008 wire contract plus a forward-compatible extension field** (`shards` array on `CreateJobRequest`) that the production backend silently ignores today (System.Text.Json defaults to ignoring unknown properties). The observable command uses a local coordinator fake bound to `127.0.0.1:9000` which we extend to honour `shards` — matching the M5-009 observable style. Production rollout will require a follow-up backend task (tracked as M5-010-followup in the plan's "Risks" section) to persist the field. This keeps M5-010 strictly go-cli.
2. **Which phases are sharded?** Only `main` phase requests. `setup` and `teardown` typically mutate shared state and are not safe to shard (setup may populate variables used by all shards). They are executed **locally** on the CLI host and the extracted variables are propagated to the coordinator job via a `variables` map on the CLI's extension body. This keeps M5-010 tractable; full pipeline sharding is deferred.
3. **Feature-gating.** `--workers >1` requires Enterprise tier. We register a new feature `distributed_execution` in `DefaultRegistry()` and gate early (before any network call) so Free/Solo/Pro/Team users get a clean exit-6 gate error.
4. **--workers 1 fallback.** Per behaviour, it emits a warning to stderr and continues on the local pipeline.
5. **Coordinator URL precedence.** `--coordinator-url` > `CURLEW_COORDINATOR_URL`. Missing both → exit 2 with the exact message from behaviour #6.
6. **Terminal rendering.** The distributed path converts the coordinator's aggregated `SubmitItem` rows into `runner.RequestResult` + `runner.Summary` so the existing terminal/json/tap/junit printers handle the output with no duplication.

## Implementation Steps

Step ordering is smallest-blast-radius-first: (1) new `internal/runner/shard` package (pure function, no callers yet); (2) new `internal/runner/distributed` package that orchestrates against a `CoordinatorClient` interface (no callers yet); (3) wire through `cmd/curlew/main.go` + help + feature gate + smoke.

### Step 1: Add `distributed_execution` feature gate
**Rationale:** Tiny registry edit with a focused test. Enables the early-gate path step 4 will call.

#### Files to Modify

| File                                     | Action  | Description                                     |
|------------------------------------------|---------|-------------------------------------------------|
| `internal/auth/registry.go`              | modify  | Register `distributed_execution` → TierEnterprise. |
| `internal/auth/registry_test.go`         | modify  | Add table row asserting lookup + required tier. |

#### Current Code
```go
// registry.go — end of DefaultRegistry before `return r`:
r.Register(FeatureDefinition{
    Name:         "shared_vault_templates",
    RequiredTier: TierTeam,
    Description:  "Shared vault configuration templates require Team tier ($39/month)",
    Workaround:   "Use vault provider profiles (Solo tier) for single-user configurations",
})
return r
```

#### New Code
```go
r.Register(FeatureDefinition{
    Name:         "shared_vault_templates",
    RequiredTier: TierTeam,
    Description:  "Shared vault configuration templates require Team tier ($39/month)",
    Workaround:   "Use vault provider profiles (Solo tier) for single-user configurations",
})
r.Register(FeatureDefinition{
    Name:         "distributed_execution",
    RequiredTier: TierEnterprise,
    Description:  "Distributed --workers execution requires Enterprise tier",
    Workaround:   "Run without --workers for single-process execution, or use --parallel (Professional tier) for in-process concurrency",
})
return r
```

#### Tests to Write FIRST (RED phase)

```go
// internal/auth/registry_test.go — add to the existing table
{"distributed_execution requires enterprise", "distributed_execution", TierEnterprise, true},
```
(Asserts `Lookup` returns ok=true and `RequiredTier == TierEnterprise`.)

#### Impact on Existing Tests
- `TestDefaultRegistry` — adds one table row; no existing assertion breaks.

---

### Step 2: Shard planner — `internal/runner/shard/shard.go`
**Rationale:** Pure function with no callers; the smallest unit we can TDD independently. No code paths depend on it yet.

#### Files to Modify

| File                                      | Action | Description                                                      |
|-------------------------------------------|--------|------------------------------------------------------------------|
| `internal/runner/shard/shard.go`          | create | Round-robin splitter over `parser.RequestItem`.                  |
| `internal/runner/shard/shard_test.go`     | create | Table-driven coverage of split behaviour.                        |

#### New Code (signatures only — bodies in TDD)
```go
// Package shard splits a request list into N shards for distributed execution.
package shard

import "github.com/weiqigod/curlew/internal/parser"

// Plan is the per-shard slice + original index map.
type Plan struct {
    // Requests is the shard's slice of RequestItems (stable order within shard).
    Requests []parser.RequestItem
    // OriginalIndex[i] is the index of Requests[i] in the pre-shard list.
    // Used by the aggregator to re-key results back to original order.
    OriginalIndex []int
}

// Split returns n shards from items using round-robin assignment
// (items[i] → shard[i%n]). Returns empty shards when items is empty;
// trailing shards are empty when len(items) < n. Panics on n < 1
// because callers must validate first.
func Split(items []parser.RequestItem, n int) []Plan
```

#### Tests to Write FIRST (RED phase)

```go
func TestSplit(t *testing.T) {
    req := func(name string) parser.RequestItem {
        return parser.RequestItem{Name: name}
    }
    tests := []struct {
        name  string
        items []parser.RequestItem
        n     int
        want  [][]string // per-shard name order
    }{
        {"evenly divisible 6/3", []parser.RequestItem{req("a"), req("b"), req("c"), req("d"), req("e"), req("f")}, 3, [][]string{{"a", "d"}, {"b", "e"}, {"c", "f"}}},
        {"remainder — 7 items over 3 shards", []parser.RequestItem{req("a"), req("b"), req("c"), req("d"), req("e"), req("f"), req("g")}, 3, [][]string{{"a", "d", "g"}, {"b", "e"}, {"c", "f"}}},
        {"fewer items than shards", []parser.RequestItem{req("a"), req("b")}, 4, [][]string{{"a"}, {"b"}, {}, {}}},
        {"single shard", []parser.RequestItem{req("a"), req("b"), req("c")}, 1, [][]string{{"a", "b", "c"}}},
        {"empty input", nil, 3, [][]string{{}, {}, {}}},
        {"observable 12/4", makeItems(12), 4, [][]string{{"r1", "r5", "r9"}, {"r2", "r6", "r10"}, {"r3", "r7", "r11"}, {"r4", "r8", "r12"}}},
    }
    // ...
}

func TestSplit_OriginalIndexRoundTrip(t *testing.T) {
    // Verify that plans[i].OriginalIndex[j] = j*len(plans) + i.
}
```

#### Impact on Existing Tests
- None — new package, no callers.

---

### Step 3: Distributed runner — `internal/runner/distributed/`
**Rationale:** Encapsulates the coordinator client interaction behind an interface, so we can drive it with `startFakeCoordinator` without touching `cmd/`. Depends on Step 2 (shard) but not on main.

#### Files to Modify

| File                                                   | Action | Description                                                  |
|--------------------------------------------------------|--------|--------------------------------------------------------------|
| `internal/runner/distributed/distributed.go`           | create | `Run(ctx, Config, Deps) (results, summary, error)`.          |
| `internal/runner/distributed/client.go`                | create | Extended coordinator client (adds `CreateJob`, `GetJob`).    |
| `internal/runner/distributed/distributed_test.go`      | create | Orchestration table (join, progress, aggregation, reassignment). |
| `internal/runner/distributed/fake_coordinator_test.go` | create | In-package fake that honours the `shards` extension field.   |

#### New Code (signatures)

```go
package distributed

import (
    "context"
    "io"
    "time"

    "github.com/weiqigod/curlew/internal/parser"
    "github.com/weiqigod/curlew/internal/runner"
    "github.com/weiqigod/curlew/internal/worker"
)

// Sentinel errors.
var (
    ErrCoordinatorURLMissing = errors.New("--workers requires CURLEW_COORDINATOR_URL")
    ErrTokenMissing          = errors.New("--workers requires CURLEW_BACKEND_TOKEN")
    ErrWorkerJoinTimeout     = errors.New("timed out waiting for workers to join")
)

// Config is the distributed-run request.
type Config struct {
    CoordinatorURL string        // required
    Token          string        // required (Bearer)
    Org            string        // --org (slug or org_<hex>)
    Workers        int           // --workers (N, must be ≥ 2 here; caller validates)
    CollectionSha  string        // opaque id (hash of collection file bytes)
    Collection     *parser.Collection
    PreExecVars    map[string]string // variables already resolved locally
    JoinTimeout    time.Duration     // default 60s
    PollInterval   time.Duration     // default 500ms
    Stdout         io.Writer         // default os.Stdout — progress logs
}

// Deps are injection seams for tests.
type Deps struct {
    Client CoordinatorClient // nil → construct from cfg (using internal/worker.Client)
    Now    func() time.Time  // nil → time.Now
}

// CoordinatorClient is the extended client interface used by Run.
type CoordinatorClient interface {
    CreateJob(ctx context.Context, org string, body *CreateJobBody) (*JobResponse, error)
    GetJob(ctx context.Context, org, jobID string) (*JobResponse, error)
    worker.CoordinatorClient // Claim/SubmitResult/Heartbeat (unused here but makes mocking uniform)
}

// CreateJobBody is the wire body: the extension field `Shards` is honoured
// by the fake coordinator and by future backend versions.
type CreateJobBody struct {
    CollectionSha string                  `json:"collection_sha"`
    ShardCount    int                     `json:"shard_count"`
    Shards        []ShardPayload          `json:"shards,omitempty"` // forward-compat
    Variables     map[string]string       `json:"variables,omitempty"`
}

// ShardPayload carries the per-shard requests when creating a job.
type ShardPayload struct {
    Index        int                   `json:"index"`
    RequestsJson string                `json:"requests_json"` // JSON array of worker.WorkerRequest
}

// JobResponse mirrors the backend CoordinatorJobDto plus aggregated shard outcomes.
type JobResponse struct {
    JobID       string              `json:"job_id"`
    State       string              `json:"state"` // pending | running | completed
    ShardCount  int                 `json:"shard_count"`
    Shards      []ShardStatus       `json:"shards"`
    WorkerCount int                 `json:"worker_count,omitempty"`
}

type ShardStatus struct {
    ShardID      string                `json:"shard_id"`
    Index        int                   `json:"shard_index"`
    State        string                `json:"state"` // pending | running | completed | reassigned
    AssignedWorker string              `json:"assigned_worker,omitempty"`
    Items        []worker.SubmitItem   `json:"items,omitempty"` // populated once completed
    PassCount    int                   `json:"pass_count,omitempty"`
    FailCount    int                   `json:"fail_count,omitempty"`
}

// Run creates a coordinator job, waits for workers to join, polls until all
// shards complete (or reassignments resolve), aggregates per-shard items
// back into []runner.RequestResult ordered to match cfg.Collection.Requests.Items,
// and returns a runner.Summary compatible with the terminal printer.
func Run(ctx context.Context, cfg Config, deps Deps) (
    []runner.RequestResult, *runner.Summary, error)
```

#### Behavioural outline (from task YAML)
- Print `"Sharding N requests across M workers..."` once, before `CreateJob`.
- Call `CreateJob` with `ShardCount=Workers` and `Shards[]` built from `shard.Split(col.Requests.Items, Workers)` (each `RequestsJson` is a JSON-encoded array of `worker.WorkerRequest` built from the `RequestItem`'s `Method`, `URL`, headers, body — after CLI-side interpolation using `PreExecVars`).
- Poll `GetJob` every `PollInterval`, tracking `worker_count`; on each change emit `"Waiting for workers... X/N joined"` (exactly once per new value) until `X == N` or `JoinTimeout` elapses.
- Continue polling (no blank lines) until every shard is in `completed` or `reassigned` terminal state. When a shard transitions to `reassigned` print `"Shard shd_X reassigned (worker timeout)"` exactly once per shard.
- On each `completed` shard print `"Shard <id> done (<pass>/<total> pass)"`.
- After every shard is complete print `"All shards complete: N/M pass"` where N = total pass across shards, M = total items.

#### Tests to Write FIRST (RED phase)

```go
// distributed_test.go
func TestRun_HappyPath12Requests4Shards(t *testing.T) {
    // 12 requests, 4 shards. Fake coordinator echoes submissions as all pass.
    // Asserts stdout contains each of the observable lines and the returned
    // summary has Total=12, Passed=12, Failed=0.
}
func TestRun_WorkerJoinTimeout(t *testing.T) {
    // Fake never advances worker_count — Run returns ErrWorkerJoinTimeout.
}
func TestRun_ShardReassignment(t *testing.T) {
    // Fake reports shard shd_2 "reassigned" on first poll then "completed"
    // on later poll. Stdout contains the reassignment notice and the final
    // aggregated counts still include shd_2's outcomes.
}
func TestRun_AggregatedFailurePropagates(t *testing.T) {
    // One shard returns 1 fail, 2 pass. Summary.Failed == 1, Passed == 11.
}
func TestRun_AggregatedRequestResultOrderMatchesInput(t *testing.T) {
    // Build 6 requests, verify RequestResult slice order == input order
    // even though shard 2 finishes before shard 1.
}
func TestRun_ContextCancellationStopsPolling(t *testing.T) {
    // Cancel ctx mid-poll; Run returns ctx.Err().
}
```

#### Impact on Existing Tests
- None — new package.

---

### Step 4: CLI wiring — `--workers` / `--coordinator-url` in `cmd/curlew/main.go`
**Rationale:** Ties Steps 1–3 into the user-facing binary. This is the largest blast radius; it comes last.

#### Files to Modify

| File                                    | Action | Description                                                              |
|-----------------------------------------|--------|--------------------------------------------------------------------------|
| `cmd/curlew/main.go`                   | modify | Extend `runFlags`, `parseRunArgs`, dispatch in `runCmdInner`, help text. |
| `cmd/curlew/run_test.go`               | modify | Add parse + dispatch tests (gate, missing URL, --workers 1 warning, happy path using fake). |
| `cmd/curlew/testdata/distributed/*.yaml` | create | 12-request collection for the smoke/integration test. |

#### Current Code (relevant extract)
```go
// runFlags at line 120
type runFlags struct {
    file, envName, format, report                                   string
    vars, envVarVars                                                map[string]string
    seed                                                            *int64
    noColor                                                         bool
    verbosity                                                       output.Verbosity
    allowSensitive, showDeps, dryRun, parallel, confirmLargeDataset bool

    reportUpload bool
    org          string
    pr           int
    repo         string
    triggeredBy  string
    gitSha       string
}
```

#### New Code (additions)
```go
type runFlags struct {
    // ...existing fields...

    // M5-010: distributed execution
    workers        int    // 0 = single-process (default); 1 = warn+fallback; ≥2 = distributed
    coordinatorURL string // --coordinator-url; falls back to CURLEW_COORDINATOR_URL
}

// parseRunArgs additions:
case "--workers":
    i++
    if i >= len(args) {
        return errorf("--workers requires an integer value (e.g. --workers 4)")
    }
    n, parseErr := strconv.Atoi(args[i])
    if parseErr != nil {
        return errorf("--workers value must be an integer: %w", parseErr)
    }
    if n < 1 {
        return errorf("--workers must be >= 1")
    }
    f.workers = n
case "--coordinator-url":
    i++
    if i >= len(args) {
        return errorf("--coordinator-url requires a value")
    }
    f.coordinatorURL = args[i]
```

In `runCmdInner`, after `parseRunArgs` succeeds and before `runner.Run` at line 660, insert distributed dispatch:

```go
// M5-010: --workers >1 → distributed path (Enterprise tier).
switch {
case flags.workers == 1:
    errOut.Warning("--workers 1 has no benefit; falling back to local execution")
    // fall through to local pipeline
case flags.workers >= 2:
    // Feature gate: Enterprise.
    if gateErr := auth.CheckFeature(auth.DefaultRegistry(), "distributed_execution", currentTier()); gateErr != nil {
        writeGateForFormat(os.Stdout, os.Stderr, format, report, noColor, gateErr)
        return 6, nil
    }
    coordURL := flags.coordinatorURL
    if coordURL == "" {
        coordURL = os.Getenv("CURLEW_COORDINATOR_URL")
    }
    if coordURL == "" {
        _, _ = fmt.Fprintln(os.Stderr, "error: --workers requires CURLEW_COORDINATOR_URL")
        return 2, nil
    }
    token := os.Getenv("CURLEW_BACKEND_TOKEN")
    if token == "" {
        _, _ = fmt.Fprintln(os.Stderr, "error: --workers requires CURLEW_BACKEND_TOKEN")
        return 2, nil
    }
    if flags.org == "" {
        _, _ = fmt.Fprintln(os.Stderr, "error: --workers requires --org")
        return 1, nil
    }
    results, summary, dErr := distributed.Run(ctx, distributed.Config{
        CoordinatorURL: coordURL,
        Token:          token,
        Org:            flags.org,
        Workers:        flags.workers,
        CollectionSha:  collectionSha(file),
        Collection:     col,
        PreExecVars:    mergedVars(projectCfg, envVars, dotenvVars, envVarVars, cliVars),
        Stdout:         os.Stdout,
    }, distributed.Deps{})
    if dErr != nil {
        errOut.StructuredError(dErr)
        return 2, summary
    }
    // Reuse the existing terminal printer for `results`.
    return renderResults(out, errOut, col, results, summary, format, report, flags, noColor), summary
}
```

Where `collectionSha(path)` is a new helper (sha256 of file bytes, hex-encoded) and `mergedVars(...)` builds a flat `map[string]string` in the same precedence order `runner.buildScope` uses. `renderResults` is a small extract of the existing terminal-rendering block in `runCmdInner` (lines 900-994) that we factor out to avoid duplication; alternative — inline the render only for the distributed path, keeping the existing `runCmdInner` untouched. **Decision: inline-minimal for this PR** (see Risks) — we only emit SummaryWithDuration + per-result lines using the existing `out` printer because that preserves terminal/json/tap/junit/html output with no structural refactor.

#### `printHelp` additions (around line 2640)
```go
fmt.Println("  --workers <N>       Distribute execution across N remote workers (Enterprise tier)")
fmt.Println("  --coordinator-url <url>  Coordinator base URL (or set CURLEW_COORDINATOR_URL)")
fmt.Println()
fmt.Println("Distributed Execution (Enterprise tier):")
fmt.Println("  --workers N          Shards the main phase across N workers and aggregates results")
fmt.Println("  --org <slug>         Required with --workers")
fmt.Println("  Env vars:  CURLEW_COORDINATOR_URL   Coordinator base URL")
fmt.Println("             CURLEW_BACKEND_TOKEN    Bearer token for the coordinator API")
fmt.Println("             (See docs/distributed.md for full setup.)")
```

#### Tests to Write FIRST (RED phase)

```go
// cmd/curlew/run_test.go
func TestParseRunArgs_Workers(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        want    runFlags
        wantErr bool
    }{
        {"no workers flag", []string{"file.yaml"}, runFlags{workers: 0, file: "file.yaml"}, false},
        {"--workers 4", []string{"--workers", "4", "file.yaml"}, runFlags{workers: 4, file: "file.yaml"}, false},
        {"--workers 0 is rejected", []string{"--workers", "0", "file.yaml"}, runFlags{}, true},
        {"--workers negative rejected", []string{"--workers", "-1", "file.yaml"}, runFlags{}, true},
        {"--workers non-int rejected", []string{"--workers", "abc", "file.yaml"}, runFlags{}, true},
        {"--coordinator-url", []string{"--workers", "4", "--coordinator-url", "http://c:9000", "file.yaml"}, runFlags{workers: 4, coordinatorURL: "http://c:9000", file: "file.yaml"}, false},
    }
    // table loop
}

func TestRunCmd_WorkersMissingCoordinatorURL(t *testing.T) {
    // CURLEW_COORDINATOR_URL unset → exit 2 + exact error string.
}

func TestRunCmd_WorkersFeatureGate(t *testing.T) {
    // CURLEW_TIER=free → exit 6, "distributed_execution" in gate message.
}

func TestRunCmd_Workers1WarnsAndFallsBack(t *testing.T) {
    // Exit 0 (assuming local run passes) + stderr contains "falling back to local".
}

func TestRunCmd_HelpMentionsWorkers(t *testing.T) {
    // --help stdout contains "--workers" and "--coordinator-url" and "Enterprise tier".
}

func TestRunCmd_WorkersObservableE2E(t *testing.T) {
    // Boots an httptest fake coordinator + fake API-under-test server,
    // runs against testdata/distributed/12-req.yaml with --workers 4,
    // asserts stdout contains all observable lines.
}
```

#### Impact on Existing Tests
- `TestParseRunArgs_*` family — unaffected; new flags are additive.
- `TestPrintHelp` (if present) — must extend the expected-substring set to include `--workers`.

---

### Step 5: Smoke-test coverage
**Rationale:** Validates the binary surface end-to-end without real backend dependencies. Matches the M5-009 smoke style.

#### Files to Modify

| File            | Action | Description                                                          |
|-----------------|--------|----------------------------------------------------------------------|
| `smoke/run.sh`  | modify | Add a `run --workers` block: help text, missing-URL exit-2, --workers 1 warning. |

#### New Smoke Block (sketch)
```bash
echo "=== Run --workers (M5-010) ==="

# --help mentions --workers
RUN_HELP=$(./curlew --help 2>&1)
echo "$RUN_HELP" | grep -q -- "--workers" \
  || { echo "FAIL: --help missing --workers"; exit 1; }
echo "PASS: run --help documents --workers"

# --workers with no coordinator URL → exit 2
unset CURLEW_COORDINATOR_URL
SMOKE_OUT=$(./curlew run sample/hello.yaml --workers 4 --org acme 2>&1) || SMOKE_RC=$?
if [ "$SMOKE_RC" -eq 2 ]; then
  echo "PASS: --workers without CURLEW_COORDINATOR_URL exits 2"
else
  echo "FAIL: expected exit 2, got $SMOKE_RC"; echo "$SMOKE_OUT"; exit 1
fi
echo "$SMOKE_OUT" | grep -q "CURLEW_COORDINATOR_URL" \
  || { echo "FAIL: error message missing CURLEW_COORDINATOR_URL"; exit 1; }

# --workers 1 warns and falls back
SMOKE_OUT=$(CURLEW_COORDINATOR_URL=http://unused ./curlew run sample/hello.yaml --workers 1 2>&1) || true
echo "$SMOKE_OUT" | grep -q "falling back to local" \
  || { echo "FAIL: --workers 1 missing fallback warning"; exit 1; }
echo "PASS: --workers 1 falls back to local"
```

#### Impact on Existing Tests
- None — new smoke block only.

---

## Test Impact Summary

| Test File                                         | Test Function                        | Impact | Action Required                      |
|---------------------------------------------------|--------------------------------------|--------|--------------------------------------|
| `internal/auth/registry_test.go`                  | `TestDefaultRegistry`                | extend | Add `distributed_execution` row.     |
| `internal/runner/shard/shard_test.go`             | `TestSplit*`                         | new    | Create — 6 table cases + round-trip. |
| `internal/runner/distributed/distributed_test.go` | `TestRun_*` (6 funcs)                | new    | Create — join, aggregate, reassign, order, ctx. |
| `cmd/curlew/run_test.go`                         | `TestParseRunArgs_Workers`           | new    | Parse-level flag coverage.           |
| `cmd/curlew/run_test.go`                         | `TestRunCmd_Workers*`                | new    | Gate, missing URL, --workers 1, help, E2E. |
| `cmd/curlew/main_test.go`                        | help-text assertions (if any)        | review | Ensure help includes --workers lines. |

**Coverage target:** distributed package ≥ 80% statements; shard package ≥ 95% (pure function); overall project coverage must not regress.

## Risks and Edge Cases

- **Risk:** M5-008 backend does not persist per-shard request payloads — workers would receive `[]` in production. → **Mitigation:** scope of this task is `track: go-cli`; the CLI writes a forward-compatible `shards` array that the fake coordinator honours. Document the backend follow-up in the plan's exit comment and in `docs/distributed.md`. A new backlog task (proposed: M5-010-followup) is needed before this is production-viable, but the go-cli deliverable (observable, tests, smoke) is complete.
- **Risk:** Setup/teardown phases are not sharded. → **Mitigation:** Execute locally on the CLI host before distributed dispatch; propagate extracted vars via `CreateJobBody.Variables`. This is called out in the distributed package doc comment.
- **Risk:** Terminal rendering divergence between local and distributed. → **Mitigation:** the distributed path converts per-shard `SubmitItem` into `runner.RequestResult` keyed by the original index map from `shard.Plan.OriginalIndex`, guaranteeing the same printer output as the local path for the same collection.
- **Risk:** Polling never converges (runaway process). → **Mitigation:** Honour `ctx.Done()` and a hard per-job wall-clock ceiling (`Config.JobTimeout`, default 30m) — exit with a descriptive error if exceeded.
- **Risk:** Output ordering across shards is non-deterministic. → **Mitigation:** aggregate into a slice indexed by `OriginalIndex` before printing; never print per-shard items as they arrive.
- **Risk:** `CURLEW_BACKEND_TOKEN` is sensitive and could leak in error messages. → **Mitigation:** `Client.doWithRetry` already only prints the URL and status; we don't echo headers. Add an assertion in a unit test that the token string is absent from captured stderr for the happy path.
- **Edge case:** `--workers 4` but the collection has only 2 requests. → **Handling:** `shard.Split` produces two empty shards; the distributed runner creates them, the coordinator marks empty shards `completed` on first poll (workers simply see empty `requests_json`), and the aggregator reports `N/N pass` with N=2. Test covers this.
- **Edge case:** Collection parse error happens before we know the distributed path is needed. → **Handling:** the existing `parser.ParseFileWithOptions` path runs first, returning exit 3; no change.
- **Edge case:** `--workers` combined with `--parallel`. → **Handling:** distributed path ignores `--parallel` (workers aren't ours to parallelise). We emit a one-line warning: `"--parallel is ignored when --workers is set"` and proceed.
- **Edge case:** `--workers` with `--format json` / `--format tap` / `--format junit` / `--format html`. → **Handling:** aggregated results feed the same printers via `runner.RequestResult`; formats work unchanged. Integration test covers at least `terminal` and `json` output to confirm.
- **Edge case:** Context cancellation (SIGINT). → **Handling:** the `ctx` derived in `runCmdInner` propagates to `distributed.Run`; polling loop selects on `ctx.Done()` and returns `ctx.Err()`.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
./scripts/ci-local.sh --go
```

Observable verification (from the task YAML):
```bash
go build ./cmd/curlew
go test ./internal/runner/... -run Distributed
# Expected: ok  internal/runner/...  (≥6 distributed tests passing)
CURLEW_COORDINATOR_URL=http://127.0.0.1:9000 \
CURLEW_BACKEND_TOKEN=svc_token_dev \
  ./curlew run testdata/team/e2e-collection.yaml --workers 4 --org acme
# Expected stdout (against the repo's fake coordinator fixture):
#   "Sharding 12 requests across 4 workers..."
#   "Waiting for workers... 4/4 joined"
#   "Shard shd_1 done (3/3 pass); shard shd_2 done (3/3 pass); ..."
#   "All shards complete: 12/12 pass"
# exit 0
```

Note: the observable assumes a fake coordinator is already running on `127.0.0.1:9000`. The integration test in `cmd/curlew/run_test.go::TestRunCmd_WorkersObservableE2E` spins up this fake via `httptest.NewServer` and validates the same stdout contract, so the observable is fully reproducible by the `go test` invocation alone. A follow-on `management/plans/M5-010-verified.md` should record the test log excerpt for that specific test as the observable evidence.
