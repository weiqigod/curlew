# Implementation Plan: M2-022

## Overview
Extend data-driven testing with parallel iteration execution (worker pool, rate limiting), chunked processing for large datasets (>10,000 rows), and result storage options (all/summary/failed_only). Wire data-driven requests as atomic units in the parallel dependency graph.

## Task Details
- **ID:** M2-022
- **Title:** Data-driven integration with parallel execution and large datasets
- **Phase:** M2: Data-Driven Testing
- **Priority:** 4
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-019 | Data-driven testing with CSV and JSON data sources | done |
| M2-016 | Parallel execution | done |

## Implementation Steps

### Step 1: Extend Config with Parallel, RateLimit, and StoreResults Fields
**Rationale:** Must update the configuration struct first since all other steps depend on parsing these new fields.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/datadriven.go` | modify | Add `Parallel`, `RateLimitRPS`, `StoreResults` fields to `Config` |
| `internal/datadriven/datadriven_test.go` | modify | Add test verifying Config YAML unmarshalling for new fields |

#### Current Code
```go
type Config struct {
	Source   string `yaml:"source"`
	Format   string `yaml:"format,omitempty"`
	Filter   string `yaml:"filter,omitempty"`
	Limit    *int   `yaml:"limit,omitempty"`
	StartRow *int   `yaml:"start_row,omitempty"`
	EndRow   *int   `yaml:"end_row,omitempty"`
	FailFast bool   `yaml:"fail_fast,omitempty"`
}
```

#### New Code
```go
type Config struct {
	Source       string `yaml:"source"`
	Format       string `yaml:"format,omitempty"`
	Filter       string `yaml:"filter,omitempty"`
	Limit        *int   `yaml:"limit,omitempty"`
	StartRow     *int   `yaml:"start_row,omitempty"`
	EndRow       *int   `yaml:"end_row,omitempty"`
	FailFast     bool   `yaml:"fail_fast,omitempty"`
	Parallel     bool   `yaml:"parallel,omitempty"`       // run iterations concurrently
	RateLimitRPS *int   `yaml:"rate_limit_rps,omitempty"` // max requests per second (nil = unlimited)
	StoreResults string `yaml:"store_results,omitempty"`  // all | summary | failed_only (default: all)
}

// DefaultMaxWorkers is the maximum number of concurrent workers for parallel data-driven execution.
const DefaultMaxWorkers = 20

// DefaultChunkSize is the number of rows processed per chunk for large datasets.
const DefaultChunkSize = 1000

// LargeDatasetThreshold is the row count above which a confirmation/warning is triggered.
const LargeDatasetThreshold = 10000

// StoreResults constants.
const (
	StoreAll        = "all"
	StoreSummary    = "summary"
	StoreFailedOnly = "failed_only"
)

// EffectiveStoreResults returns the store_results value, defaulting to "all".
func (c Config) EffectiveStoreResults() string {
	if c.StoreResults == "" {
		return StoreAll
	}
	return c.StoreResults
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestConfig_EffectiveStoreResults(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"default is all", Config{}, "all"},
		{"explicit all", Config{StoreResults: "all"}, "all"},
		{"summary", Config{StoreResults: "summary"}, "summary"},
		{"failed_only", Config{StoreResults: "failed_only"}, "failed_only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EffectiveStoreResults(); got != tt.want {
				t.Errorf("EffectiveStoreResults() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

#### Impact on Existing Tests
- No existing tests affected; new fields have zero-value defaults matching prior behavior

### Step 2: Implement Parallel Execution with Worker Pool
**Rationale:** This is the core feature. The worker pool is self-contained within the `datadriven` package and can be tested independently before integration.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/parallel.go` | create | Worker pool, rate limiter, parallel execute function |
| `internal/datadriven/parallel_test.go` | create | Tests for parallel execution, rate limiting, concurrency bounds |

#### New Code
```go
// parallel.go

package datadriven

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/weiqigod/curlew/internal/variable"
)

// parallelConfig extends executeConfig with parallel-specific settings.
type parallelConfig struct {
	DataSet      *DataSet
	Scope        *variable.Scope
	ExecFn       func(ctx context.Context, iterScope *variable.Scope, index int) (*iterationResult, error)
	FailFast     bool
	MaxWorkers   int // max concurrent workers (default 20)
	RateLimitRPS int // max requests per second (0 = unlimited)
}

// rateLimiter implements a simple token-bucket rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

func newRateLimiter(rps int) *rateLimiter {
	if rps <= 0 {
		return nil
	}
	return &rateLimiter{interval: time.Second / time.Duration(rps)}
}

func (rl *rateLimiter) wait(ctx context.Context) error {
	if rl == nil {
		return nil
	}
	rl.mu.Lock()
	now := time.Now()
	next := rl.last.Add(rl.interval)
	if next.After(now) {
		delay := next.Sub(now)
		rl.last = next
		rl.mu.Unlock()
		select {
		case <-time.After(delay):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rl.last = now
	rl.mu.Unlock()
	return nil
}

// executeParallel runs data-driven iterations concurrently using a worker pool.
// Results are returned in original index order. Accumulated extractions are
// collected after all workers complete to avoid races.
func executeParallel(ctx context.Context, cfg parallelConfig) ([]iterationResult, map[string][]string, error) {
	total := len(cfg.DataSet.Rows)
	results := make([]iterationResult, total)
	var resultErrors []error
	var mu sync.Mutex

	workers := cfg.MaxWorkers
	if workers <= 0 {
		workers = DefaultMaxWorkers
	}
	if workers > total {
		workers = total
	}

	rl := newRateLimiter(cfg.RateLimitRPS)

	// Cancellation support for fail-fast
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var failFastTriggered bool

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for idx, row := range cfg.DataSet.Rows {
		if runCtx.Err() != nil {
			break
		}

		mu.Lock()
		if failFastTriggered {
			mu.Unlock()
			break
		}
		mu.Unlock()

		// Rate limiting before acquiring worker slot
		if err := rl.wait(runCtx); err != nil {
			break
		}

		sem <- struct{}{} // acquire worker slot
		wg.Add(1)
		go func(idx int, row Row) {
			defer wg.Done()
			defer func() { <-sem }() // release worker slot

			iterScope := InjectIterationVars(cfg.Scope, row, idx, total)
			result, err := cfg.ExecFn(runCtx, iterScope, idx)
			if err != nil {
				mu.Lock()
				resultErrors = append(resultErrors, fmt.Errorf("iteration %d: %w", idx, err))
				mu.Unlock()
				return
			}

			result.Row = row
			mu.Lock()
			results[idx] = *result

			if cfg.FailFast && result.Err != nil {
				failFastTriggered = true
				cancel()
			}
			mu.Unlock()
		}(idx, row)
	}

	wg.Wait()

	if len(resultErrors) > 0 {
		return nil, nil, resultErrors[0]
	}

	// Filter out zero-value results (from cancelled iterations)
	var filtered []iterationResult
	accumulated := make(map[string][]string)
	for i := 0; i < total; i++ {
		r := results[i]
		if r.Row == nil && r.Err == nil && r.Name == "" {
			continue // not executed (cancelled before dispatch)
		}
		filtered = append(filtered, r)
		for k, v := range r.Extracted {
			accumulated[k] = append(accumulated[k], v)
		}
	}

	return filtered, accumulated, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteParallel(t *testing.T) {
	tests := []struct {
		name           string
		rows           []Row
		maxWorkers     int
		wantIterations int
	}{
		{"three iterations with 2 workers", []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}}, 2, 3},
		{"single iteration", []Row{{"a": "1"}}, 5, 1},
		{"more workers than rows", []Row{{"a": "1"}, {"a": "2"}}, 10, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ...test body
		})
	}
}

func TestExecuteParallel_ConcurrencyBound(t *testing.T) {
	// Verify that no more than MaxWorkers goroutines run concurrently
}

func TestExecuteParallel_RateLimit(t *testing.T) {
	// Verify rate limiting bounds total RPS
}

func TestExecuteParallel_FailFast(t *testing.T) {
	// Verify fail-fast cancels remaining iterations
}

func TestExecuteParallel_ContextCancellation(t *testing.T) {
	// Verify external context cancellation stops workers
}

func TestExecuteParallel_ExtractionAccumulates(t *testing.T) {
	// Verify accumulated variables in index order
}

func TestExecuteParallel_ScopeIsolation(t *testing.T) {
	// Each iteration should get its own scope snapshot
}

func TestRateLimiter_NilWhenDisabled(t *testing.T) {
	// RPS=0 returns nil limiter
}

func TestRateLimiter_ThrottlesRequests(t *testing.T) {
	// Verify limiter respects RPS ceiling
}
```

#### Impact on Existing Tests
- No existing tests affected; this is new code in new files

### Step 3: Implement Chunked Processing for Large Datasets
**Rationale:** Large dataset handling is needed before runner integration to implement the confirmation/warning prompt and chunked execution.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/chunked.go` | create | Chunked iterator and large dataset warning logic |
| `internal/datadriven/chunked_test.go` | create | Tests for chunking and threshold detection |

#### New Code
```go
// chunked.go

package datadriven

// LargeDatasetInfo provides performance estimates for large datasets.
type LargeDatasetInfo struct {
	TotalRows         int
	ChunkSize         int
	ChunkCount        int
	EstimatedDuration string // human-readable estimate
	StorageEstimate   string // human-readable storage estimate
}

// CheckLargeDataset returns info if the dataset exceeds LargeDatasetThreshold, or nil if not.
func CheckLargeDataset(ds *DataSet) *LargeDatasetInfo {
	if len(ds.Rows) <= LargeDatasetThreshold {
		return nil
	}
	chunks := (len(ds.Rows) + DefaultChunkSize - 1) / DefaultChunkSize
	return &LargeDatasetInfo{
		TotalRows:         len(ds.Rows),
		ChunkSize:         DefaultChunkSize,
		ChunkCount:        chunks,
		EstimatedDuration: estimateDuration(len(ds.Rows)),
		StorageEstimate:   estimateStorage(len(ds.Rows)),
	}
}

// ChunkRows splits rows into chunks of the given size.
func ChunkRows(rows []Row, chunkSize int) [][]Row {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	var chunks [][]Row
	for i := 0; i < len(rows); i += chunkSize {
		end := i + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunks = append(chunks, rows[i:end])
	}
	return chunks
}

func estimateDuration(rows int) string { /* ... */ }
func estimateStorage(rows int) string  { /* ... */ }
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckLargeDataset_BelowThreshold(t *testing.T) {
	// 1000 rows -> nil (no warning)
}

func TestCheckLargeDataset_AboveThreshold(t *testing.T) {
	// 10001 rows -> non-nil info with correct chunk count
}

func TestCheckLargeDataset_ExactThreshold(t *testing.T) {
	// 10000 rows -> nil (not above, at threshold)
}

func TestChunkRows(t *testing.T) {
	tests := []struct {
		name       string
		rowCount   int
		chunkSize  int
		wantChunks int
		wantLast   int // rows in last chunk
	}{
		{"exact multiple", 3000, 1000, 3, 1000},
		{"remainder", 2500, 1000, 3, 500},
		{"smaller than chunk", 500, 1000, 1, 500},
		{"single row", 1, 1000, 1, 1},
		{"zero chunk size defaults", 100, 0, 1, 100},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected; new code in new files

### Step 4: Implement Result Storage Filtering
**Rationale:** Result storage options affect what the runner stores and returns. This is independent of parallel execution and can be tested in isolation before runner wiring.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/results.go` | create | Result storage filtering functions |
| `internal/datadriven/results_test.go` | create | Tests for result filtering |

#### New Code
```go
// results.go

package datadriven

// FilterResults applies store_results policy to iteration results.
// - "all": returns results unchanged
// - "summary": strips detail fields, keeps only index/pass/fail/timing
// - "failed_only": returns full details only for failed iterations, summary for passed
func FilterResults(results []iterationResult, policy string) []iterationResult {
	switch policy {
	case StoreSummary:
		return summarizeResults(results)
	case StoreFailedOnly:
		return failedOnlyResults(results)
	default: // StoreAll or unrecognized
		return results
	}
}

// IterationSummary holds a compact summary for one iteration.
type IterationSummary struct {
	Index  int
	Passed bool
	// DurationMs will be available from the iteration result's timing
}

func summarizeResults(results []iterationResult) []iterationResult { /* strip extracted/detailed fields */ }
func failedOnlyResults(results []iterationResult) []iterationResult { /* full for failures, minimal for passes */ }
```

#### Tests to Write FIRST (RED phase)

```go
func TestFilterResults_All(t *testing.T) {
	// Returns results unchanged
}

func TestFilterResults_Summary(t *testing.T) {
	// Returns results with stripped detail fields
}

func TestFilterResults_FailedOnly(t *testing.T) {
	// Full details for failed, minimal for passed
}

func TestFilterResults_DefaultIsAll(t *testing.T) {
	// Empty/unknown policy returns all
}
```

#### Impact on Existing Tests
- No existing tests affected; new code in new files

### Step 5: Wire Parallel Data-Driven into Runner
**Rationale:** Now that the building blocks (parallel executor, chunking, result filtering) exist, wire them into the runner's `executeDataDriven` function. This step has the largest blast radius since it modifies the main execution path.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Update `executeDataDriven` to dispatch to parallel path when `Parallel: true`; add chunked processing; add result storage filtering |
| `internal/runner/runner_test.go` | modify | Add integration tests for parallel data-driven, rate limiting, large dataset warning, store_results |

#### Current Code (executeDataDriven, relevant section)
```go
// Execute iterations sequentially
var results []RequestResult
accumulated := make(map[string][]string)
total := len(ds.Rows)
requiredFailed := false

for idx, row := range ds.Rows {
    // ...sequential iteration loop...
}
```

#### New Code (conceptual changes to executeDataDriven)
```go
// After loading ds, check for large dataset warning
if info := datadriven.CheckLargeDataset(ds); info != nil && !vars.ConfirmLargeDataset {
    return nil, false, fmt.Errorf(
        "data file %q has %d rows (>%d). Performance estimate: %s, storage: %s. "+
            "Use --confirm-large-dataset to proceed or add store_results: summary|failed_only",
        item.DataDriven.Source, info.TotalRows, datadriven.LargeDatasetThreshold,
        info.EstimatedDuration, info.StorageEstimate)
}

// Feature gate: parallel data-driven requires professional tier
if item.DataDriven.Parallel {
    if gateErr := auth.CheckFeature(reg, "data_driven", tier); gateErr != nil {
        return nil, false, gateErr
    }
    return executeDataDrivenParallel(ctx, item, scope, exec, vars, phase, ...)
}

// else: existing sequential path
```

The new `executeDataDrivenParallel` function will:
1. Create a `parallelConfig` with MaxWorkers=20, RateLimitRPS from config
2. Build an `ExecFn` closure that wraps the interpolation + auth + retry + HTTP execution
3. Call `executeParallel()` 
4. Convert `iterationResult` slice into `[]RequestResult`
5. Apply `FilterResults` for store_results policy
6. Accumulate extracted variables into the outer scope

#### Tests to Write FIRST (RED phase)

```go
func TestRun_DataDriven_ParallelExecution(t *testing.T) {
	// Given data-driven with parallel:true, iterations run concurrently
}

func TestRun_DataDriven_ParallelRateLimit(t *testing.T) {
	// Given rate_limit_rps:100, verify RPS ceiling
}

func TestRun_DataDriven_ParallelFailFast(t *testing.T) {
	// Given parallel+fail_fast, verify early termination
}

func TestRun_DataDriven_ParallelExtractionAccumulates(t *testing.T) {
	// Given parallel+extraction, accumulated arrays available after completion
}

func TestRun_DataDriven_LargeDatasetWarning(t *testing.T) {
	// Given >10000 rows without confirmation, error with performance estimates
}

func TestRun_DataDriven_LargeDatasetConfirmed(t *testing.T) {
	// Given >10000 rows with confirmation, proceeds normally
}

func TestRun_DataDriven_StoreResultsSummary(t *testing.T) {
	// Given store_results:summary, only pass/fail/timing returned
}

func TestRun_DataDriven_StoreResultsFailedOnly(t *testing.T) {
	// Given store_results:failed_only, only failed details returned
}

func TestRun_DataDriven_AtomicInDependencyGraph(t *testing.T) {
	// Data-driven request treated as atomic unit; dependents wait for all iterations
}
```

#### Impact on Existing Tests
- `TestRun_DataDriven_CSVSource` — no change (sequential path unchanged)
- `TestRun_DataDriven_JSONSource` — no change
- `TestRun_DataDriven_FailFast` — no change (sequential path)
- All existing data-driven tests remain on sequential path since `Parallel: false` is default

### Step 6: Add VarSources.ConfirmLargeDataset and CLI Flag
**Rationale:** The large dataset confirmation needs a way to be passed from CLI through to the runner. This is a small, targeted change.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `ConfirmLargeDataset` field to `VarSources` |
| `cmd/curlew/main.go` | modify | Add `--confirm-large-dataset` flag |

#### Current Code (VarSources)
```go
type VarSources struct {
    // ...existing fields
}
```

#### New Code
```go
type VarSources struct {
    // ...existing fields
    ConfirmLargeDataset bool // true when user confirmed large dataset execution
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_DataDriven_LargeDataset_ConfirmFlag(t *testing.T) {
    // VarSources.ConfirmLargeDataset=true bypasses the warning
}
```

#### Impact on Existing Tests
- No existing tests affected; new zero-value field defaults to `false` which preserves current behavior (no large dataset check was done before)

### Step 7: Ensure Data-Driven is Atomic in Parallel Dependency Graph
**Rationale:** Per specification, data-driven requests should be treated as atomic units in the collection-level parallel dependency graph. The existing parallel executor already processes requests as whole items. We need to verify this behavior explicitly and ensure the runner's parallel execution path handles data-driven items correctly.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Ensure parallel execution path delegates data-driven items to `executeDataDriven` (or parallel variant) and treats them as atomic |
| `internal/runner/runner_test.go` | modify | Add test for data-driven + collection-level parallel |

#### Tests to Write FIRST (RED phase)

```go
func TestRun_DataDriven_AtomicInParallelGraph(t *testing.T) {
    // A data-driven request in a parallel collection completes all iterations
    // before dependent requests execute. Verify accumulated variables are
    // available to the dependent request.
}
```

#### Impact on Existing Tests
- No existing parallel tests affected; the parallel executor already treats each request as a unit

### Step 8: Parser Tests for New Config Fields
**Rationale:** Final step to verify end-to-end YAML parsing of the new data_driven fields.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/testdata/data_driven_parallel.yaml` | create | Test fixture with parallel, rate_limit_rps, store_results |
| `internal/parser/parser_test.go` | modify | Add test for parsing new data_driven fields |

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_dataDrivenParallelConfig(t *testing.T) {
    col, err := ParseFile("testdata/data_driven_parallel.yaml")
    // Verify Parallel=true, RateLimitRPS=100, StoreResults="failed_only"
}
```

#### Impact on Existing Tests
- No existing parser tests affected; new test fixture and test function

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/datadriven/datadriven_test.go` | all existing | none | no changes needed |
| `internal/datadriven/execute_test.go` | all existing | none | no changes needed |
| `internal/runner/runner_test.go` | all existing data-driven tests | none | sequential path unchanged |
| `internal/parser/parser_test.go` | all existing | none | new test added, old tests unaffected |

## Risks and Edge Cases

- **Risk:** Race conditions in parallel worker pool when accumulating extracted variables -> **Mitigation:** Collect all results into pre-allocated indexed slice; accumulate extractions sequentially after `wg.Wait()` completes. All accumulation is single-threaded.

- **Risk:** Rate limiter inaccuracy under high concurrency -> **Mitigation:** Use mutex-protected token bucket; accept ~5% variance in rate limiting (token bucket is standard approach). Tests verify ceiling, not exact timing.

- **Risk:** Memory pressure with 10,000+ rows and `store_results: all` -> **Mitigation:** `CheckLargeDataset` warns users and suggests `summary` or `failed_only`. The chunked processing ensures rows are not all loaded into memory simultaneously during execution (though they are loaded from file upfront; true streaming would require a larger refactor not warranted at this stage).

- **Edge case:** `parallel: true` with `fail_fast: true` -- some in-flight workers may complete after cancellation -> **Handling:** Cancel context on first failure; in-flight workers check `ctx.Err()` before starting HTTP call. Results from cancelled workers are filtered out.

- **Edge case:** `parallel: true` with extraction -- accumulated variables must be in original index order -> **Handling:** Pre-allocate results array by index; iterate 0..N after workers complete to build accumulated map in order.

- **Edge case:** `rate_limit_rps: 0` or negative -> **Handling:** Treat as unlimited (nil rate limiter).

- **Edge case:** `store_results` with invalid value -> **Handling:** Default to `"all"` (no error, graceful fallback).

- **Edge case:** Data-driven with `parallel: true` inside a collection running with `--parallel` -> **Handling:** Data-driven request is atomic in collection graph; internal iteration parallelism is orthogonal to collection-level wave parallelism.

- **Design Decision (large dataset confirmation):** The specification says ">10,000 rows trigger confirmation prompt with performance estimates." Since we are a CLI tool and prompts in non-interactive mode (CI) would break pipelines, we implement this as an error with a bypass flag (`--confirm-large-dataset`) rather than an interactive prompt. This follows the "always-runnable" philosophy where CI pipelines can add the flag to opt in.

- **Design Decision (chunked processing):** The spec says "Files >10,000 rows processed in 1,000-row chunks to avoid memory overload." We implement chunking at the execution level (process rows in chunks through the worker pool) rather than at the file loading level, since all supported formats (CSV, JSON, YAML) require reading the full file to parse it. The chunking reduces peak memory from concurrent execution (only N chunk rows have in-flight HTTP results at once).

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Run data-driven with parallel: true and confirm concurrent iteration execution
# (requires a test collection YAML with parallel data-driven config)
./curlew run test-parallel-datadriven.yaml

# Run data-driven with 10,000+ rows and confirm warning/chunked processing
./curlew run test-large-dataset.yaml

# Unit tests for parallel and large dataset
go test ./internal/datadriven/... -v -run TestExecuteParallel
go test ./internal/datadriven/... -v -run TestChunkRows
go test ./internal/datadriven/... -v -run TestCheckLargeDataset
go test ./internal/runner/... -v -run TestRun_DataDriven_Parallel
```
