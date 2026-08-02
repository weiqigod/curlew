# Implementation Plan: M2-025

## Overview
Integrate retry logic with parallel execution (retries within wave, not blocking other wave members), ensure per-iteration retry behavior in data-driven testing, and update all output formatters (terminal, JSON, TAP) to include detailed retry metadata including per-attempt details (status, duration, delay).

## Task Details
- **ID:** M2-025
- **Title:** Retry output and integration with parallel/data-driven execution
- **Phase:** M2: Advanced Retries
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-024 | Advanced retry trigger conditions and method restrictions | done |
| M2-016 | Parallel execution | done |
| M2-019 | Data-driven testing | done |

## Current State Analysis

### Parallel Execution + Retry Gap
The `parallel/executor.go` `executeOneRequest` function calls `cfg.ExecFunc` directly **without** retry wrapping. Regular requests in parallel waves do NOT get retried. However, data-driven items in waves DO get retried because the runner's `buildDataDrivenFunc` delegates to `executeDataDriven`, which uses `retry.ExecuteWithRetry`.

**Fix needed:** Integrate `retry.ExecuteWithRetry` into the parallel executor's `executeOneRequest` path, or have the runner wrap `cfg.ExecFunc` with retry logic before passing to the parallel executor.

### Retry Output Gap
The `retry.Outcome` struct only tracks `Attempts` (int) and `Warnings` ([]string). There are no per-attempt details. The task behaviors require:
- Verbose terminal: each attempt shows attempt number, status, and delay
- JSON: `retry_count` and `attempt_details` array
- TAP: diagnostic comment with retry information

**Fix needed:** Add `AttemptDetail` struct and `AttemptDetails []AttemptDetail` to `retry.Outcome`, populated during `ExecuteWithRetry`.

### Data-Driven + Retry
Already working correctly for both sequential and parallel data-driven. `executeDataDriven` and `executeDataDrivenParallel` both use `retry.ExecuteWithRetry`. The only gap is output formatting to show retry details per iteration.

## Implementation Steps

### Step 1: Add AttemptDetail to retry.Outcome
**Rationale:** Smallest blast radius — this is a pure additive change to a data struct. All downstream code depends on this struct.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/retry.go` | modify | Add `AttemptDetail` struct and `AttemptDetails` field to `Outcome` |
| `internal/retry/retry_test.go` | modify | Add tests verifying attempt details are populated |

#### Current Code
```go
// Outcome holds the result of a retried execution.
type Outcome struct {
	Result   *httpexec.Result
	Err      error
	Attempts int      // total attempts (1 = no retries)
	Warnings []string // warnings (e.g., non-idempotent method retry)
}
```

#### New Code
```go
// AttemptDetail holds the outcome of a single retry attempt.
type AttemptDetail struct {
	Number     int           // 1-based attempt number
	StatusCode int           // HTTP status code (0 if network error)
	Duration   time.Duration // how long the attempt took
	Delay      time.Duration // backoff delay waited before this attempt (0 for first)
	Err        error         // error if attempt failed with non-HTTP error
}

// Outcome holds the result of a retried execution.
type Outcome struct {
	Result         *httpexec.Result
	Err            error
	Attempts       int              // total attempts (1 = no retries)
	Warnings       []string         // warnings (e.g., non-idempotent method retry)
	AttemptDetails []AttemptDetail  // per-attempt details (populated when retries > 0)
}
```

Update `ExecuteWithRetry` to populate `AttemptDetails`:
- Record the first attempt's result immediately
- Before each retry, record the delay
- After each retry attempt, record status, duration, error

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWithRetry_AttemptDetails(t *testing.T) {
    tests := []struct {
        name            string
        cfg             Config
        execResults     []execResult // (statusCode, err) pairs
        wantDetails     int
        checkDetails    func(t *testing.T, details []AttemptDetail)
    }{
        {"no retry - single attempt detail", ...},
        {"retry succeeds on second attempt - two details", ...},
        {"all attempts fail - three details", ...},
        {"retry disabled - no details", ...},
        {"attempt details include delay for retries", ...},
        {"attempt details include status codes", ...},
        {"attempt details include error for network failure", ...},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — `AttemptDetails` is a new additive field

---

### Step 2: Integrate retry into parallel executor
**Rationale:** The parallel executor's `executeOneRequest` does not use retry. This is the core integration gap. Must come before output changes since it produces the RetryCount and AttemptDetails that output relies on.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | modify | Add retry config and sleep func to `Config`, use `retry.ExecuteWithRetry` in `executeOneRequest` |
| `internal/parallel/executor_test.go` | modify | Add tests for retry within waves |
| `internal/runner/runner.go` | modify | Pass retry config to parallel executor |

#### Current Code (parallel/executor.go)
```go
// Config holds parallel execution configuration.
type Config struct {
    Graph          *DependencyGraph
    Items          []parser.RequestItem
    Scope          *variable.Scope
    ExecFunc       requtil.ExecuteFunc
    MaxRequests    int
    DataDrivenFunc DataDrivenFunc
}
```

```go
// executeOneRequest - current implementation calls ExecFunc directly
result, execErr := cfg.ExecFunc(ctx, requtil.ToHTTPRequest(&req))
```

#### New Code (parallel/executor.go)
```go
// RetryConfigFunc returns the resolved retry.Config for a given request item.
// The runner builds this from the merged precedence chain.
type RetryConfigFunc func(item parser.RequestItem) retry.Config

// Config holds parallel execution configuration.
type Config struct {
    Graph            *DependencyGraph
    Items            []parser.RequestItem
    Scope            *variable.Scope
    ExecFunc         requtil.ExecuteFunc
    MaxRequests      int
    DataDrivenFunc   DataDrivenFunc
    RetryConfigFunc  RetryConfigFunc  // nil = no retries for regular requests
    SleepFunc        retry.SleepFunc  // nil = retry.DefaultSleep
}
```

```go
// In executeOneRequest, wrap the execution call with retry:
retryCfg := retry.Config{}
if cfg.RetryConfigFunc != nil {
    retryCfg = cfg.RetryConfigFunc(item)
}
sleepFn := cfg.SleepFunc
if sleepFn == nil {
    sleepFn = retry.DefaultSleep
}

outcome := retry.ExecuteWithRetry(ctx, retryCfg, req.Method, func(rCtx context.Context) (*httpexec.Result, error) {
    return cfg.ExecFunc(rCtx, requtil.ToHTTPRequest(&req))
}, sleepFn)

return RequestOutcome{
    Index:            idx,
    Name:             item.Name,
    Method:           req.Method,
    URL:              req.URL,
    RequestHeaders:   req.Headers,
    RequestBody:      req.Body,
    Result:           outcome.Result,
    Err:              outcome.Err,
    AssertionResults: ar,  // evaluated on final result
    RetryCount:       outcome.Attempts - 1,
    RetryWarnings:    outcome.Warnings,
    WaveIndex:        waveIdx,
}
```

Also add `AttemptDetails` to `RequestOutcome`:
```go
type RequestOutcome struct {
    // ... existing fields ...
    AttemptDetails []retry.AttemptDetail
}
```

#### Runner integration (runner.go `executeParallelMain`):
Pass a `RetryConfigFunc` that calls `resolveRetryConfig`:
```go
retryConfigFunc := func(item parser.RequestItem) retry.Config {
    return resolveRetryConfig(vars.GlobalRetry, col.Retry, col.Requests.Retry, item.Retry)
}

execResult, err := parallel.ExecuteWaves(ctx, parallel.Config{
    // ... existing fields ...
    RetryConfigFunc: retryConfigFunc,
})
```

Also propagate `AttemptDetails` when converting parallel outcomes to runner results.

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWaves_RetryWithinWave(t *testing.T) {
    tests := []struct {
        name string
        // ...
    }{
        {"retry in wave does not block other wave members", ...},
        {"retried request succeeds - dependent wave 2 proceeds", ...},
        {"retry exhausted - dependent requests skipped", ...},
        {"retry count propagated to outcome", ...},
        {"nil RetryConfigFunc means no retry", ...},
    }
}
```

#### Impact on Existing Tests
- `parallel/executor_test.go` — existing tests set no `RetryConfigFunc` (nil), so they continue working unchanged (no retries)
- `runner/runner_test.go` — parallel execution tests will get retry integration automatically; no existing tests should break since they don't configure retry

---

### Step 3: Add AttemptDetails to runner.RequestResult and propagation
**Rationale:** The runner needs to propagate attempt details from retry.Outcome through to output formatters. This is a plumbing step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `AttemptDetails` field to `RequestResult`, populate from retry outcome |

#### Current Code
```go
type RequestResult struct {
    // ... existing fields ...
    RetryCount    int
    RetryWarnings []string
}
```

#### New Code
```go
type RequestResult struct {
    // ... existing fields ...
    RetryCount     int
    RetryWarnings  []string
    AttemptDetails []retry.AttemptDetail // per-attempt details (populated when retries > 0)
}
```

Propagation points (all in runner.go):
1. `executePhase` (line ~731): after `retry.ExecuteWithRetry`, set `retryDetails := outcome.AttemptDetails`
2. `executeDataDriven` (line ~970): after `retry.ExecuteWithRetry`, propagate details
3. `executeDataDrivenParallel` (line ~1131): propagate from `IterationResult`
4. `executeParallelMain` (line ~519): propagate from `parallel.RequestOutcome`

Also add to `datadriven.IterationResult`:
```go
type IterationResult struct {
    // ... existing fields ...
    AttemptDetails []retry.AttemptDetail
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_RetryAttemptDetailsPropagated(t *testing.T) {
    // Verify that when retries happen, AttemptDetails are populated in results
}
```

#### Impact on Existing Tests
- No existing tests break — additive field only

---

### Step 4: Terminal verbose output for retry attempts
**Rationale:** After data is flowing, add verbose display. Terminal is the primary interactive output.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `RetryAttemptDetail` method |
| `internal/output/terminal_test.go` | modify | Add tests for retry detail output |
| `cmd/apitest/main.go` | modify | Call `RetryAttemptDetail` when verbose and retryCount > 0 |

#### New Code (terminal.go)
```go
// RetryAttemptDetails writes per-attempt retry information (shown at -v and -vv).
// Each line shows attempt number, status/error, duration, and delay.
func (p *Printer) RetryAttemptDetails(details []retry.AttemptDetail) {
    if p.verbosity < VerbosityVerbose || len(details) <= 1 {
        return
    }
    for _, d := range details {
        if d.Err != nil {
            _, _ = fmt.Fprintf(p.w, "    %s\n",
                colorize(fmt.Sprintf("Attempt %d: error (%dms, delay %dms)",
                    d.Number, d.Duration.Milliseconds(), d.Delay.Milliseconds()), ansiGray, p.color))
        } else {
            _, _ = fmt.Fprintf(p.w, "    %s\n",
                colorize(fmt.Sprintf("Attempt %d: %d (%dms, delay %dms)",
                    d.Number, d.StatusCode, d.Duration.Milliseconds(), d.Delay.Milliseconds()), ansiGray, p.color))
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrinterRetryAttemptDetails(t *testing.T) {
    tests := []struct {
        name      string
        verbosity Verbosity
        details   []retry.AttemptDetail
        wantParts []string
        wantNot   []string
    }{
        {"verbose shows attempt details", VerbosityVerbose, twoAttempts, ...},
        {"default verbosity hides attempt details", VerbosityDefault, twoAttempts, ...},
        {"single attempt not shown", VerbosityVerbose, oneAttempt, ...},
        {"error attempt shows error", VerbosityVerbose, errorAttempts, ...},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 5: JSON output with attempt_details
**Rationale:** JSON output is the machine-readable format used in CI. Must include structured retry data.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add `AttemptDetails` field to `JSONRequest` and `JSONAttemptDetail` struct |
| `internal/output/json_test.go` | modify | Add tests for attempt_details in JSON |
| `cmd/apitest/main.go` | modify | Populate `AttemptDetails` in `buildJSONOutput` |

#### New Code (json.go)
```go
// JSONAttemptDetail represents a single retry attempt in JSON output.
type JSONAttemptDetail struct {
    Attempt    int    `json:"attempt"`
    StatusCode int    `json:"status_code,omitempty"`
    DurationMs int64  `json:"duration_ms"`
    DelayMs    int64  `json:"delay_ms"`
    Error      string `json:"error,omitempty"`
}

// JSONRequest — add field:
type JSONRequest struct {
    // ... existing fields ...
    AttemptDetails []JSONAttemptDetail `json:"attempt_details,omitempty"`
}
```

#### cmd/apitest/main.go buildJSONOutput:
```go
// After setting RetryCount:
if r.RetryCount > 0 && len(r.AttemptDetails) > 0 {
    jr.AttemptDetails = make([]output.JSONAttemptDetail, len(r.AttemptDetails))
    for i, d := range r.AttemptDetails {
        jr.AttemptDetails[i] = output.JSONAttemptDetail{
            Attempt:    d.Number,
            StatusCode: d.StatusCode,
            DurationMs: d.Duration.Milliseconds(),
            DelayMs:    d.Delay.Milliseconds(),
        }
        if d.Err != nil {
            jr.AttemptDetails[i].Error = d.Err.Error()
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestJSONOutputAttemptDetails(t *testing.T) {
    tests := []struct {
        name string
        // ...
    }{
        {"attempt_details present when retried", ...},
        {"attempt_details omitted when no retry", ...},
        {"attempt_details include status codes and durations", ...},
        {"attempt_details include error for network failures", ...},
    }
}
```

#### Impact on Existing Tests
- No existing tests break — `omitempty` on `AttemptDetails` means existing JSON output unchanged when no retries

---

### Step 6: TAP output with retry diagnostics
**Rationale:** TAP is the CI-friendly format. Retry info should appear as diagnostic comments.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/tap.go` | modify | Add retry diagnostic comment support |
| `internal/output/tap_test.go` | modify | Add tests for retry diagnostics in TAP |
| `cmd/apitest/main.go` | modify | Pass retry details to TAP builder |

#### Current Code (tap.go TAPResult)
```go
type TAPResult struct {
    Name            string
    Passed          bool
    Skipped         bool
    Error           string
    Failures        []TAPFailure
    DurationMs      int64
    RetryCount      int
    WaveIndex       *int
    DataDrivenGroup *string
}
```

#### New Code
```go
// TAPRetryDetail holds per-attempt info for TAP diagnostics.
type TAPRetryDetail struct {
    Attempt    int
    StatusCode int
    DurationMs int64
    DelayMs    int64
    Error      string
}

type TAPResult struct {
    // ... existing fields ...
    RetryDetails []TAPRetryDetail // shown as diagnostic comment when non-empty
}
```

In `WriteTAP`, after writing the result line for a retried request, add a diagnostic comment:
```go
if r.RetryCount > 0 && len(r.RetryDetails) > 0 {
    fmt.Fprintf(w, "  # retry: %d attempt(s)\n", r.RetryCount+1)
    for _, d := range r.RetryDetails {
        if d.Error != "" {
            fmt.Fprintf(w, "  # attempt %d: error (%dms)\n", d.Attempt, d.DurationMs)
        } else {
            fmt.Fprintf(w, "  # attempt %d: %d (%dms, delay %dms)\n", d.Attempt, d.StatusCode, d.DurationMs, d.DelayMs)
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteTAP_RetryDiagnostics(t *testing.T) {
    tests := []struct {
        name  string
        // ...
    }{
        {"retry diagnostic comment shows attempt count", ...},
        {"no retry diagnostic when retry_count is 0", ...},
        {"retry diagnostic includes per-attempt details", ...},
    }
}
```

#### Impact on Existing Tests
- No existing tests break — `RetryDetails` is nil by default, which means no diagnostic output

---

### Step 7: Integration tests
**Rationale:** End-to-end validation that retry + parallel + data-driven + output all work together.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/retry_test.go` | modify | Add integration test combining all behaviors |
| `internal/runner/runner_test.go` | modify | Add integration tests for parallel+retry and data-driven+retry |

#### Tests to Write FIRST (RED phase)

```go
// retry/retry_test.go
func TestExecuteWithRetry_AttemptDetailsIntegration(t *testing.T) {
    // Full integration: retry with backoff, verify AttemptDetails has correct
    // attempt numbers, status codes, delays, and durations
}

// runner/runner_test.go
func TestRun_ParallelWithRetry(t *testing.T) {
    // Parallel execution where one request in wave 1 needs retry
    // Verify: other wave 1 requests execute concurrently
    // Verify: dependent wave 2 requests proceed after retry succeeds
}

func TestRun_ParallelRetryExhausted_DependentsSkipped(t *testing.T) {
    // Parallel execution where a request exhausts all retries
    // Verify: dependent requests are skipped with appropriate reason
}

func TestRun_DataDrivenWithRetry_PerIterationRetry(t *testing.T) {
    // Data-driven execution where some iterations need retry
    // Verify: per-iteration retry behavior
    // Verify: failed iteration (after retries) marked failed, next continues
}

func TestRun_DataDrivenWithRetry_FailFast(t *testing.T) {
    // Data-driven with fail_fast and retries
    // Verify: after retries exhausted on one iteration, remaining skipped
}
```

#### Impact on Existing Tests
- No existing tests affected

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/retry/retry_test.go` | All existing | none | — |
| `internal/parallel/executor_test.go` | All existing | none | nil RetryConfigFunc preserves existing behavior |
| `internal/runner/runner_test.go` | All existing | none | AttemptDetails is additive |
| `internal/output/terminal_test.go` | All existing | none | — |
| `internal/output/json_test.go` | All existing | none | omitempty means no change |
| `internal/output/tap_test.go` | All existing | none | RetryDetails nil means no change |

## Risks and Edge Cases

- **Risk:** Parallel executor performance regression from retry wrapping → **Mitigation:** When `RetryConfigFunc` returns `Config{Enabled: false}`, `ExecuteWithRetry` calls exec exactly once with no overhead (existing fast path)
- **Risk:** Data race on AttemptDetails in parallel execution → **Mitigation:** Each goroutine gets its own `Outcome` from `ExecuteWithRetry`; `AttemptDetails` are built per-goroutine, no sharing
- **Edge case:** Retry-After header in parallel wave could cause one request to delay significantly → **Handling:** This is correct behavior per spec; other wave members continue concurrently
- **Edge case:** Context cancellation during retry within a wave → **Handling:** `ExecuteWithRetry` already respects context cancellation; returns partial `AttemptDetails` for completed attempts
- **Edge case:** Data-driven fail_fast with retries → **Handling:** Already works correctly — `executeDataDriven` checks `FailFast` after retry outcome
- **Edge case:** Guard rail counter in parallel with retries → **Handling:** Each retry attempt should count toward the guard rail. The runner's exec wrapper already increments counter per call. For parallel, the retry happens inside `executeOneRequest` which calls `cfg.ExecFunc` which is the raw HTTP exec. We need to ensure guard rail counting works correctly.
- **Risk:** Guard rail counting with parallel retries → **Mitigation:** The parallel executor's `MaxRequests` config limits requests at the wave level. Individual retries within `executeOneRequest` are additional HTTP calls. This is acceptable because retry is an implementation detail of request execution. The guard rail counts logical requests, not HTTP calls. Document this behavior clearly.
- **Edge case:** `AttemptDetails` for disabled retry → **Handling:** When retry is disabled, `ExecuteWithRetry` returns `Attempts: 1` with no `AttemptDetails` (empty slice). Output formatters skip rendering when `AttemptDetails` is empty.
- **Decision:** For disabled retry, we populate a single `AttemptDetail` only when retry is enabled. When disabled, `AttemptDetails` remains nil/empty to avoid noise.

## Design Decisions

1. **AttemptDetails populated only when retry is enabled and attempts > 1**: This avoids noise in output for non-retried requests. Even if retry is enabled but succeeds first try, we populate details so the user can see the single successful attempt in verbose mode.

2. **Parallel executor gets retry via RetryConfigFunc callback**: Rather than having the parallel executor import and understand retry config merging, we pass a function that the runner builds. This keeps the parallel package focused on wave execution.

3. **TAP retry info as comments, not YAML diagnostics**: TAP comments (`#`) are the appropriate place for metadata. YAML diagnostics are for test failure details. Retry attempts are metadata about execution, not test failures.

4. **Guard rail counts logical requests in parallel, not retry attempts**: Retry attempts within parallel execution are transparent to the guard rail. This matches the spec's Edge Case 8 which says "Retry attempts are transparent to the dependency graph."

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Run a collection with retries and parallel execution
apitest run tests.yaml --parallel -v

# Run data-driven with retries
apitest run tests.yaml -v

# Verify JSON output includes attempt_details
apitest run tests.yaml --format json | jq '.requests[].attempt_details'

# Run integration tests
go test ./internal/retry/... -v
go test ./internal/parallel/... -v
go test ./internal/runner/... -v
go test ./internal/output/... -v
```
