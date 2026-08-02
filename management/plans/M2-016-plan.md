# Implementation Plan: M2-016

## Overview
Implement wave-based parallel execution of HTTP requests using goroutines, building on the dependency analysis from M2-015. Requests within a wave run concurrently; waves execute sequentially. Adds the `--parallel` flag to the `run` command with feature gating at the Professional tier.

## Task Details
- **ID:** M2-016
- **Title:** Parallel request execution with wave-based scheduling
- **Phase:** M2: Parallel Execution
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-015 | Dependency analysis algorithm | done |

## Implementation Steps

### Step 1: Create `internal/parallel/executor.go` — Executor Core Types and WaveResult
**Rationale:** Define the types and interfaces first so tests can be written against them. This is the smallest blast radius — just types, no logic yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | create | Core executor types: Executor, Config, WaveResult, RequestOutcome |
| `internal/parallel/executor_test.go` | create | Tests for executor |

#### New Code
```go
package parallel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/peterlindqvist/apitest/internal/assertion"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/retry"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// RequestOutcome holds the result of executing a single request within a wave.
type RequestOutcome struct {
	Index            int
	Name             string
	Method           string
	URL              string
	RequestHeaders   map[string]string
	RequestBody      any
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	SkipReason       string
	AssertionResults *assertion.Results
	RetryCount       int
	WaveIndex        int
}

// WaveResult holds the outcome of executing a single wave.
type WaveResult struct {
	WaveIndex int
	Outcomes  []RequestOutcome
	Duration  time.Duration
}

// ExecutionResult holds the complete parallel execution result.
type ExecutionResult struct {
	Waves    []WaveResult
	Duration time.Duration
}

// ExecuteFunc is the function signature for executing a single HTTP request.
type ExecuteFunc func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error)

// Config holds parallel execution configuration.
type Config struct {
	Graph       *DependencyGraph
	Items       []parser.RequestItem
	Scope       *variable.Scope
	ExecFunc    ExecuteFunc
	RetryConfig func(item parser.RequestItem) retry.Config
	MaxRequests int
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWaves_AllIndependent(t *testing.T) {
	tests := []struct {
		name          string
		requestCount  int
		wantWaves     int
		wantOutcomes  int
	}{
		{"three independent requests in single wave", 3, 1, 3},
		{"single request in single wave", 1, 1, 1},
	}
}

func TestExecuteWaves_LinearChain(t *testing.T) {
	tests := []struct {
		name         string
		wantWaves    int
	}{
		{"A->B->C produces three sequential waves", 3},
	}
}

func TestExecuteWaves_DiamondDependency(t *testing.T) {
	// Wave 0: Login; Wave 1: Get User, Get Orders (parallel); Wave 2: Get Order Detail
}

func TestExecuteWaves_VariableExtraction_PropagatesBetweenWaves(t *testing.T) {
	// Wave 0 extracts token, wave 1 uses {{token}}
}

func TestExecuteWaves_FailedRequest_SkipsDependents(t *testing.T) {
	// Wave 0 request A fails, wave 1 request B depends on A -> B is skipped
}

func TestExecuteWaves_GuardRail_StopsAtLimit(t *testing.T) {
	// MaxRequests=2, 3 requests -> third is skipped
}

func TestExecuteWaves_ContextCancellation(t *testing.T) {
	// Cancel context mid-execution
}

func TestExecuteWaves_ConcurrentExecution_TimingVerification(t *testing.T) {
	// 3 independent requests each take 50ms, total should be ~50ms not 150ms
}
```

#### Impact on Existing Tests
- No existing tests affected — this is a new file

### Step 2: Implement `ExecuteWaves` — Wave-Based Parallel Executor
**Rationale:** The core execution logic. Each wave's requests run concurrently with goroutines and `sync.WaitGroup`. Variable extraction results are merged between waves using a snapshot-copy pattern for thread safety.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | modify | Add ExecuteWaves function |
| `internal/parallel/executor_test.go` | modify | Add comprehensive tests |

#### New Code
```go
// ExecuteWaves runs requests in waves. Within each wave, requests execute
// concurrently. Between waves, extracted variables are propagated to the scope.
// Setup and teardown are handled externally by the caller (runner package).
func ExecuteWaves(ctx context.Context, cfg Config) (*ExecutionResult, error) {
	if cfg.Graph == nil || !cfg.Graph.IsValid {
		return nil, fmt.Errorf("invalid dependency graph")
	}

	result := &ExecutionResult{}
	start := time.Now()
	counter := 0
	failedIndices := make(map[int]bool)

	for waveIdx, wave := range cfg.Graph.Waves {
		if ctx.Err() != nil {
			// Skip remaining waves on cancellation
			for _, remainingWave := range cfg.Graph.Waves[waveIdx:] {
				wr := WaveResult{WaveIndex: waveIdx}
				for _, idx := range remainingWave {
					wr.Outcomes = append(wr.Outcomes, RequestOutcome{
						Index: idx, Name: cfg.Items[idx].Name,
						Skipped: true, SkipReason: "context cancelled",
						WaveIndex: waveIdx,
					})
				}
				result.Waves = append(result.Waves, wr)
			}
			break
		}

		waveStart := time.Now()
		wr := WaveResult{WaveIndex: waveIdx}

		// Check which requests in this wave should be skipped due to failed deps
		toRun := make([]int, 0, len(wave))
		for _, idx := range wave {
			if shouldSkip, reason := checkDependencyFailure(cfg.Graph, idx, failedIndices); shouldSkip {
				wr.Outcomes = append(wr.Outcomes, RequestOutcome{
					Index: idx, Name: cfg.Items[idx].Name,
					Skipped: true, SkipReason: reason,
					WaveIndex: waveIdx,
				})
				failedIndices[idx] = true
				continue
			}
			if counter >= cfg.MaxRequests {
				wr.Outcomes = append(wr.Outcomes, RequestOutcome{
					Index: idx, Name: cfg.Items[idx].Name,
					Skipped: true, SkipReason: "request limit exceeded",
					WaveIndex: waveIdx,
				})
				continue
			}
			toRun = append(toRun, idx)
		}

		// Execute requests in this wave concurrently
		outcomes := make([]RequestOutcome, len(toRun))
		var wg sync.WaitGroup
		var mu sync.Mutex  // protects counter
		for i, idx := range toRun {
			wg.Add(1)
			go func(i, idx int) {
				defer wg.Done()
				outcome := executeOneRequest(ctx, cfg, idx, waveIdx)
				outcomes[i] = outcome
				mu.Lock()
				counter++
				mu.Unlock()
			}(i, idx)
		}
		wg.Wait()

		// Process outcomes: extract variables and record failures
		for _, outcome := range outcomes {
			wr.Outcomes = append(wr.Outcomes, outcome)
			if outcome.Err != nil || (outcome.AssertionResults != nil && !outcome.AssertionResults.Passed) {
				failedIndices[outcome.Index] = true
			}
			// Propagate extracted variables to scope for next wave
			if outcome.Err == nil && len(cfg.Items[outcome.Index].Extract) > 0 {
				// Variable extraction from response
				// (done sequentially after wave completes to avoid races)
				extractVarsToScope(cfg.Scope, cfg.Items[outcome.Index], outcome.Result)
			}
		}

		wr.Duration = time.Since(waveStart)
		result.Waves = append(result.Waves, wr)
	}

	result.Duration = time.Since(start)
	return result, nil
}
```

The key design decisions:
1. **Thread safety**: Each goroutine receives its own snapshot of the scope for interpolation. Variables are extracted *after* the wave completes (sequentially), so `scope.Set()` is never called concurrently.
2. **Failure propagation**: A `failedIndices` map tracks which requests failed. Before executing a request, we check if any of its dependencies are in the failed set.
3. **Guard rail**: A shared counter (protected by mutex) enforces the request limit.

#### Impact on Existing Tests
- No existing tests affected

### Step 3: Add `--parallel` Flag to CLI and Feature Gate Check
**Rationale:** The flag parsing and feature gate check are needed before the executor can be wired in. This is a small, self-contained change.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add --parallel flag to parseRunArgs; add feature gate check in runCmdInner |
| `cmd/apitest/run_test.go` | modify | Add tests for --parallel flag |

#### Current Code
```go
// parseRunArgs signature
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, verbosity output.Verbosity, allowSensitive, showDeps, dryRun bool, err error) {
```

#### New Code
```go
// parseRunArgs signature — add parallel bool return value
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, verbosity output.Verbosity, allowSensitive, showDeps, dryRun, parallel bool, err error) {
    // ... existing code ...
    case "--parallel":
        parallel = true
    // ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_parallel_flag_free_tier_gated(t *testing.T) {
	// Create a simple collection, run with --parallel at Free tier
	// Expect exit code 6 (feature gated)
}

func TestParseRunArgs_parallel_flag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantParallel bool
	}{
		{"with --parallel", []string{"test.yaml", "--parallel"}, true},
		{"without --parallel", []string{"test.yaml"}, false},
	}
}
```

#### Impact on Existing Tests
- All callers of `parseRunArgs` will need to be updated for the new return value. This includes:
  - `runCmdInner` in main.go
  - `watchCmd` in main.go
  - Any tests that call `parseRunArgs` directly (none found — tests use `runCmd` or `run`)

### Step 4: Wire Parallel Executor into Runner / Main
**Rationale:** Connect the parallel executor into the existing run command flow. When `--parallel` is specified and tier allows it, use wave-based execution for the main phase instead of sequential execution. Setup and teardown remain sequential.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | In runCmdInner, when --parallel is set: analyze deps, execute waves for main phase |
| `internal/parallel/executor.go` | modify | Add helper to convert RequestOutcome to runner.RequestResult |
| `cmd/apitest/run_test.go` | modify | Integration tests for parallel execution |

#### Current Code (runCmdInner main execution path)
```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{...})
```

#### New Code (conceptual flow in runCmdInner)
```go
if parallel {
    // Feature gate
    reg := auth.DefaultRegistry()
    if gateErr := auth.CheckFeature(reg, "parallel_execution", currentTier()); gateErr != nil {
        // handle gate error (exit code 6)
    }

    // Build pre-exec var set and analyze dependencies
    preExecVars := buildPreExecVarSet(...)
    graph := parallel.Analyze(col.Requests.Items, preExecVars)
    if !graph.IsValid {
        // report errors
        return 3, nil
    }

    // Execute: setup (sequential) -> main (parallel waves) -> teardown (sequential)
    // Use runner.Run for setup/teardown phases, parallel.ExecuteWaves for main
    results, summary, varErr = runParallel(ctx, col, graph, vars)
} else {
    results, summary, varErr = runner.Run(ctx, col, httpexec.Execute, runner.VarSources{...})
}
```

The `runParallel` function will:
1. Build the variable scope using `runner.BuildScope` (needs to be exported or use a shared helper)
2. Execute auth profiles (same as Run)
3. Execute setup phase sequentially (reuse `runner.executePhase` pattern)
4. Execute main phase using `parallel.ExecuteWaves`
5. Execute teardown phase sequentially
6. Convert results and build summary

**Design Decision — Approach**: Rather than modifying `runner.Run` (which would risk breaking sequential mode), the parallel executor will be a separate code path in `main.go` that reuses the scope building and phase execution helpers. This means either:
  - (A) Export `buildScope` and `executePhase` from the runner package, or
  - (B) Create a `parallel.RunParallel` function in the parallel package that takes the same VarSources, or
  - (C) Add a `Parallel bool` field to `runner.VarSources` and branch inside `runner.Run`.

**Decision**: Option (C) — Add `Parallel bool` to `VarSources`. This is the cleanest approach because:
- `runner.Run` already orchestrates auth, setup, main, teardown
- We only need to swap the main-phase execution strategy
- All existing tests continue to pass (Parallel defaults to false)
- The runner already has access to the scope, exec func, and all the infrastructure

The change inside `runner.runPhases`:
```go
// Phase 2: Main
if vars.Parallel {
    // Analyze and execute in parallel waves
    preExecVars := buildPreExecVarSetFromScope(scope)
    graph := parallel.Analyze(col.Requests.Items, preExecVars)
    if !graph.IsValid {
        return nil, summary, fmt.Errorf("parallel analysis: %s", strings.Join(graph.Errors, "; "))
    }
    mainResults := executeParallelPhase(ctx, col.Requests.Items, scope, exec, vars, graph, &counter, MaxRequests)
    all = append(all, mainResults...)
} else {
    // existing sequential path
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_parallel_independent_requests(t *testing.T) {
	// 3 independent requests to httptest.NewServer
	// All should pass, exit code 0
}

func TestRunCmd_parallel_dependent_requests(t *testing.T) {
	// A extracts token, B uses token
	// Both should pass in correct order
}

func TestRunCmd_parallel_setup_sequential_main_parallel(t *testing.T) {
	// Setup runs sequentially, main runs in parallel
}

func TestRunCmd_parallel_teardown_always_runs(t *testing.T) {
	// Teardown runs even if main has failures
}

func TestRunCmd_parallel_speedup(t *testing.T) {
	// 3 independent slow requests (50ms each)
	// Total time should be ~50ms, not ~150ms
}
```

#### Impact on Existing Tests
- `runner.VarSources` gets a new `Parallel bool` field — zero value is false, so no existing tests break
- `runPhases` adds a new code path — existing sequential path unchanged

### Step 5: Variable Extraction Propagation Between Waves
**Rationale:** This is the most complex part — ensuring variables extracted in wave N are available to wave N+1. The key constraint is that `variable.Scope` is NOT thread-safe, so we must handle concurrency carefully.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | modify | Implement per-request scope snapshots and post-wave extraction |
| `internal/parallel/executor_test.go` | modify | Tests for variable propagation |

#### Design

For each request within a wave:
1. **Before execution**: Take a snapshot of the current scope using `scope.Resolved()` and create a per-request child scope via `scope.WithOverrides(item.Variables.Values)`. This is done sequentially before the goroutines start.
2. **During execution**: Each goroutine uses its own scope snapshot for interpolation. No shared mutable state.
3. **After wave completes**: Sequentially iterate outcomes, extract variables from responses, call `scope.Set()` for each. This is safe because all goroutines have finished.

```go
// Before wave execution:
reqScopes := make([]*variable.Scope, len(toRun))
for i, idx := range toRun {
    item := cfg.Items[idx]
    if len(item.Variables.Values) > 0 {
        s, err := cfg.Scope.WithOverrides(item.Variables.Values)
        if err != nil { /* handle */ }
        reqScopes[i] = s
    } else {
        // Use shared scope — safe because we only read during wave execution
        reqScopes[i] = cfg.Scope
    }
}

// After wave:
for _, outcome := range outcomes {
    if outcome.Err == nil && outcome.Result != nil {
        item := cfg.Items[outcome.Index]
        if len(item.Extract) > 0 {
            extResult, _ := variable.Extract(variable.ExtractionInput{
                Extractions: item.Extract,
                Body:        outcome.Result.Body,
            })
            for k, v := range extResult.Variables {
                cfg.Scope.Set(k, v)
            }
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWaves_VariableExtraction_AcrossWaves(t *testing.T) {
	tests := []struct {
		name     string
		// Wave 0: request extracts user_id from response
		// Wave 1: request uses {{user_id}} in URL
	}{
		{"extracted var available in next wave"},
		{"multiple vars extracted and used"},
	}
}

func TestExecuteWaves_VariableExtraction_WithinWave_NoRace(t *testing.T) {
	// Two concurrent requests extract different variables
	// Both should succeed without data races
	// Run with -race flag
}
```

#### Impact on Existing Tests
- No existing tests affected

### Step 6: Integration Tests and Smoke Test
**Rationale:** End-to-end verification that the complete parallel execution pipeline works from the CLI.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/run_test.go` | modify | Add integration tests for parallel execution |
| `smoke/run.sh` | modify | Add parallel execution smoke test |

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_parallel_feature_gated_free_tier(t *testing.T) {
	// --parallel at Free tier -> exit code 6
}

func TestRunCmd_parallel_independent_timing(t *testing.T) {
	// 3 requests each sleeping 50ms -> total ~50ms with --parallel
}

func TestRunCmd_parallel_wave_ordering(t *testing.T) {
	// A extracts token, B uses token -> A runs first
}

func TestRunCmd_parallel_setup_teardown_sequential(t *testing.T) {
	// Setup and teardown run sequentially even with --parallel
}

func TestRunCmd_parallel_failed_dep_skips(t *testing.T) {
	// Wave 0 request fails, wave 1 dependent is skipped
}

func TestRunCmd_parallel_guard_rail(t *testing.T) {
	// MaxRequests limit applies to parallel execution
}
```

#### Impact on Existing Tests
- No existing tests affected

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/apitest/main_test.go` | All existing | minor | Update parseRunArgs call sites for new `parallel` return value |
| `cmd/apitest/run_test.go` | All existing | none | No changes needed |
| `internal/runner/runner_test.go` | All existing | none | VarSources.Parallel defaults to false |
| `internal/parallel/*_test.go` | All existing | none | No changes needed |

## Risks and Edge Cases

- **Risk:** `variable.Scope` is not thread-safe — concurrent reads of `resolved` map during interpolation could race with writes.
  **Mitigation:** Take scope snapshots before spawning goroutines. Scope reads happen on per-request copies. All `Set()` calls happen sequentially after `wg.Wait()`.

- **Risk:** `http.DefaultClient` is shared and could have connection pool contention under high concurrency.
  **Mitigation:** `http.DefaultClient` is explicitly designed for concurrent use. The Go HTTP client handles connection pooling internally. No action needed.

- **Risk:** Per-request function cache (`BeginRequest`/`EndRequest`) uses shared state on the Scope.
  **Mitigation:** Each goroutine must call `BeginRequest()` on its own scope copy, not the shared scope. The scope snapshot approach handles this.

- **Edge case:** Empty request list with `--parallel` flag.
  **Handling:** Treat the same as sequential — return immediately with empty results.

- **Edge case:** All requests in a single wave (all independent).
  **Handling:** All run concurrently in one wave. This is the optimal case.

- **Edge case:** Linear chain (all dependent) with `--parallel` flag.
  **Handling:** Each wave has exactly one request — degenerates to sequential execution. Performance is equivalent to non-parallel mode.

- **Edge case:** Request counter (guard rail) exceeding limit mid-wave.
  **Handling:** Check counter before launching each goroutine. Once limit reached, skip remaining requests in the wave and all subsequent waves.

- **Edge case:** Context cancellation mid-wave.
  **Handling:** Context is passed to each goroutine's HTTP execution. Cancelled requests return errors. Remaining waves are skipped.

- **Edge case:** Setup failure should skip all main requests even in parallel mode.
  **Handling:** Setup runs sequentially before parallel main phase. If setup fails with a required item, skip the entire parallel main phase.

- **Edge case:** Auth profile variables need to be available to all parallel requests.
  **Handling:** Auth profiles execute before any phase (same as sequential). Their variables are in the scope before parallel execution begins.

## Verification

```bash
go build ./cmd/apitest
go test ./...
go test -race ./internal/parallel/...
go test -race ./internal/runner/...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Feature gate check (Free tier)
./apitest run --parallel sample/hello.yaml
# Should exit with code 6 and feature gate message

# Run tests
go test ./internal/parallel/... -v
go test ./cmd/apitest/... -v -run TestRunCmd_parallel
```
