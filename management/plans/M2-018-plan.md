# Implementation Plan: M2-018

## Overview
Extend all three output formatters (terminal, JSON, TAP) to support parallel execution metadata: wave grouping in terminal output, wave_index and parallel_execution fields in JSON, and wave annotations in TAP. Add a parallel execution summary showing total waves, maximum parallelism, and speedup factor.

## Task Details
- **ID:** M2-018
- **Title:** Parallel execution output formatting (terminal, JSON, TAP)
- **Phase:** M2: Parallel Execution
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-016 | Parallel request execution with wave-based scheduling | done |
| M1-020 | JSON output format | done |
| M1-021 | TAP output format | done |

## Architecture Decisions

1. **Wave index propagation**: Add `WaveIndex` to `runner.RequestResult` so all formatters can access it. This is the cleanest approach since results are already built from parallel outcomes in `executeParallelMain`.

2. **Parallel metadata in Summary**: Add `WaveCount`, `MaxParallelism`, and wave-level durations to `runner.Summary` so all formatters can compute speedup. The `executeParallelMain` function already has access to `execResult.Waves` which provides all needed data.

3. **Terminal wave headers**: Render wave headers (e.g., "Wave 1 (2 concurrent):") when printing parallel results in terminal mode. Detect transitions by checking `WaveIndex` changes in sequential iteration.

4. **JSON schema additions**: Add `wave_index` to `JSONRequest` and a `parallel_execution` block to `JSONOutput` with wave count, max parallelism, and speedup factor.

5. **TAP wave annotations**: Use TAP comments (`# Wave N`) to annotate wave boundaries. Comments are allowed by TAP 13 spec and do not affect test counts.

6. **Parallel summary in terminal**: Add a new `ParallelSummary` method to `Printer` that displays wave count, max parallelism, and speedup factor after the main summary.

7. **Dry-run output**: The spec shows `--parallel --dry-run --show-dependencies` producing wave execution plan with speedup estimate. The existing `FormatWaves` already handles most of this; we enhance it with max parallelism and expected speedup lines.

## Implementation Steps

### Step 1: Add WaveIndex and Parallel Metadata to Runner Types
**Rationale:** This is pure data plumbing with no behavior change — smallest blast radius. All subsequent output formatting steps depend on this data being available.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `WaveIndex` field to `RequestResult`, add parallel fields to `Summary`, propagate wave data in `executeParallelMain` |
| `internal/runner/runner_test.go` | modify | Add tests for new fields in parallel execution paths |

#### Current Code
```go
// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
	Name             string
	Phase            Phase
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
}

// Summary holds aggregate execution results.
type Summary struct {
	Total                   int
	Passed                  int
	Failed                  int
	Skipped                 int
	AssertionFailures       int
	TeardownErrors          int
	TeardownAssertionErrors int
	Duration                time.Duration
	LimitExceeded           bool
	RequestsExecuted        int
	AuthSensitive           *variable.SensitiveSet
	Impact                  []parallel.ImpactEntry
}
```

#### New Code
```go
// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
	Name             string
	Phase            Phase
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
	WaveIndex        int  // parallel execution wave index (0-based); -1 when not parallel
}

// Summary holds aggregate execution results.
type Summary struct {
	Total                   int
	Passed                  int
	Failed                  int
	Skipped                 int
	AssertionFailures       int
	TeardownErrors          int
	TeardownAssertionErrors int
	Duration                time.Duration
	LimitExceeded           bool
	RequestsExecuted        int
	AuthSensitive           *variable.SensitiveSet
	Impact                  []parallel.ImpactEntry
	// Parallel execution metadata (only populated when --parallel is used)
	WaveCount       int             // total number of execution waves
	MaxParallelism  int             // largest wave size (max concurrent requests)
	WaveDurations   []time.Duration // duration of each wave
	IsParallel      bool            // true when parallel execution was used
}
```

In `executeParallelMain`, propagate wave metadata:
```go
// After converting outcomes to results:
// Compute wave metadata
waveCount := len(execResult.Waves)
maxPar := 0
waveDurations := make([]time.Duration, len(execResult.Waves))
for i, wave := range execResult.Waves {
	size := len(wave.Outcomes)
	if size > maxPar {
		maxPar = size
	}
	waveDurations[i] = wave.Duration
}

// Set WaveIndex on results
for _, wave := range execResult.Waves {
	for _, outcome := range wave.Outcomes {
		// Find result by index and set WaveIndex
		// (outcomes already ordered by wave in results slice)
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteParallelMain_PropagatesWaveIndex(t *testing.T) {
	tests := []struct {
		name           string
		// ... collection with dependencies creating multiple waves
		wantWaveIndex  []int // expected wave index for each result
	}{
		{"two waves - wave index set on each result", ...},
		{"single wave - all wave index 0", ...},
	}
}

func TestSummary_ParallelMetadata(t *testing.T) {
	tests := []struct {
		name              string
		wantWaveCount     int
		wantMaxParallelism int
		wantIsParallel    bool
	}{
		{"parallel run populates wave count and max parallelism", ...},
		{"sequential run has zero wave count", ...},
	}
}
```

#### Impact on Existing Tests
- `TestRun` and related tests in `runner_test.go` — will NOT break because `WaveIndex` defaults to 0 (zero value) and `IsParallel` defaults to false. All existing behavior is additive.
- Parallel-specific tests in `runner_test.go` will need to verify `WaveIndex` is set correctly.

---

### Step 2: Terminal Output — Wave Headers and Parallel Summary
**Rationale:** Terminal is the default output format and the most visible. Depends on Step 1 for wave data.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `WaveHeader` and `ParallelSummary` methods to `Printer` |
| `internal/output/terminal_test.go` | modify | Add tests for new methods |
| `cmd/curlew/main.go` | modify | Use wave headers in terminal output loop when parallel mode is active |

#### New Code — terminal.go
```go
// WaveHeader writes a wave section header for parallel execution.
// Shows wave number (1-based) and count of concurrent requests.
func (p *Printer) WaveHeader(waveNum, concurrentCount int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	label := fmt.Sprintf("Wave %d (%d concurrent):", waveNum, concurrentCount)
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(label, ansiBoldCyan, p.color))
}

// ParallelSummary writes parallel execution metadata: wave count,
// max parallelism, and speedup factor.
func (p *Printer) ParallelSummary(waveCount, maxParallelism int, totalRequests int, duration time.Duration, waveDurations []time.Duration) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "  Waves: %d, Max parallelism: %d\n", waveCount, maxParallelism)
	// Speedup: sum of wave durations (sequential estimate) vs actual duration
	var seqEstimate time.Duration
	for _, d := range waveDurations {
		seqEstimate += d
	}
	// Only show speedup when meaningful (more than 1 wave)
	if waveCount > 1 && duration > 0 {
		speedup := float64(seqEstimate) / float64(duration)
		_, _ = fmt.Fprintf(p.w, "  Speedup: %.1fx\n", speedup)
	}
}
```

#### New Code — main.go terminal rendering
```go
// In the terminal output loop, detect wave transitions for parallel mode:
var currentWaveIndex = -1
for _, r := range results {
	// ... existing phase header logic ...
	if r.Phase == runner.PhaseMain && summary.IsParallel && r.WaveIndex != currentWaveIndex {
		currentWaveIndex = r.WaveIndex
		waveSize := countWaveResults(results, currentWaveIndex)
		out.WaveHeader(currentWaveIndex+1, waveSize)
	}
	// ... existing result rendering ...
}

// After summary line, add parallel summary if applicable:
if summary.IsParallel {
	out.ParallelSummary(summary.WaveCount, summary.MaxParallelism, summary.Total, summary.Duration, summary.WaveDurations)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrinter_WaveHeader(t *testing.T) {
	tests := []struct {
		name            string
		waveNum         int
		concurrentCount int
		verbosity       Verbosity
		color           bool
		wantParts       []string
		wantEmpty       bool
	}{
		{"shows wave number and concurrent count", 1, 3, VerbosityDefault, false, []string{"Wave 1", "3 concurrent"}, false},
		{"wave 2 with 1 request", 2, 1, VerbosityDefault, false, []string{"Wave 2", "1 concurrent"}, false},
		{"quiet verbosity suppresses output", 1, 2, VerbosityQuiet, false, nil, true},
		{"color mode emits bold cyan ANSI", 1, 2, VerbosityDefault, true, []string{"\033[1;36m"}, false},
	}
}

func TestPrinter_ParallelSummary(t *testing.T) {
	tests := []struct {
		name      string
		waveCount int
		maxPar    int
		total     int
		duration  time.Duration
		waveDurs  []time.Duration
		wantParts []string
		wantEmpty bool
	}{
		{"shows wave count and max parallelism", 3, 2, 5, 100*time.Millisecond, []time.Duration{50*time.Millisecond, 30*time.Millisecond, 20*time.Millisecond}, []string{"Waves: 3", "Max parallelism: 2"}, false},
		{"shows speedup factor for multi-wave", 2, 2, 4, 50*time.Millisecond, []time.Duration{50*time.Millisecond, 50*time.Millisecond}, []string{"Speedup:", "2.0x"}, false},
		{"no speedup for single wave", 1, 3, 3, 50*time.Millisecond, []time.Duration{50*time.Millisecond}, []string{"Waves: 1"}, false},
		{"quiet verbosity suppresses output", 2, 2, 4, 50*time.Millisecond, nil, nil, true},
	}
}
```

#### Impact on Existing Tests
- No existing terminal tests break — new methods are additive.
- `cmd/curlew/main.go` tests (integration) may need updates for parallel terminal output, but those are typically smoke tests.

---

### Step 3: JSON Output — Wave Index and Parallel Execution Metadata
**Rationale:** JSON output is the machine-readable format. Depends on Step 1 for data availability.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add `WaveIndex` to `JSONRequest`, add `ParallelExecution` struct and field to `JSONOutput` |
| `internal/output/json_test.go` | modify | Add tests for new JSON fields |
| `cmd/curlew/main.go` | modify | Populate new JSON fields in `buildJSONOutput` |

#### Current Code — json.go
```go
type JSONRequest struct {
	Name            string              `json:"name"`
	Status          string              `json:"status"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	StatusCode      int                 `json:"status_code"`
	DurationMs      int64               `json:"duration_ms"`
	RetryCount      int                 `json:"retry_count,omitempty"`
	SkipReason      string              `json:"skip_reason,omitempty"`
	Assertions      []JSONAssertion     `json:"assertions"`
	Error           *JSONError          `json:"error,omitempty"`
	RequestHeaders  map[string]string   `json:"request_headers,omitempty"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
	ResponseBody    string              `json:"response_body,omitempty"`
}

type JSONOutput struct {
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	DurationMs int64          `json:"duration_ms"`
	Requests   []JSONRequest  `json:"requests"`
	Errors     []JSONError    `json:"errors,omitempty"`
	GuardRail  *GuardRailJSON `json:"guard_rail,omitempty"`
	Impact     []JSONImpact   `json:"impact,omitempty"`
}
```

#### New Code — json.go
```go
type JSONRequest struct {
	Name            string              `json:"name"`
	Status          string              `json:"status"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	StatusCode      int                 `json:"status_code"`
	DurationMs      int64               `json:"duration_ms"`
	RetryCount      int                 `json:"retry_count,omitempty"`
	WaveIndex       *int                `json:"wave_index,omitempty"`
	SkipReason      string              `json:"skip_reason,omitempty"`
	Assertions      []JSONAssertion     `json:"assertions"`
	Error           *JSONError          `json:"error,omitempty"`
	RequestHeaders  map[string]string   `json:"request_headers,omitempty"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
	ResponseBody    string              `json:"response_body,omitempty"`
}

// ParallelExecutionJSON holds parallel execution metadata in JSON output.
type ParallelExecutionJSON struct {
	WaveCount      int     `json:"wave_count"`
	MaxParallelism int     `json:"max_parallelism"`
	SpeedupFactor  float64 `json:"speedup_factor,omitempty"`
}

type JSONOutput struct {
	Name              string                 `json:"name"`
	Status            string                 `json:"status"`
	DurationMs        int64                  `json:"duration_ms"`
	Requests          []JSONRequest          `json:"requests"`
	Errors            []JSONError            `json:"errors,omitempty"`
	GuardRail         *GuardRailJSON         `json:"guard_rail,omitempty"`
	Impact            []JSONImpact           `json:"impact,omitempty"`
	ParallelExecution *ParallelExecutionJSON `json:"parallel_execution,omitempty"`
}
```

Note: `WaveIndex` on `JSONRequest` is `*int` (pointer) so it can be `omitempty` when nil (non-parallel requests). When parallel, we set it to `&waveIdx`.

#### Tests to Write FIRST (RED phase)

```go
func TestWriteJSON_WaveIndex(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"wave_index present when set",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						WaveIndex: intPtr(0), Assertions: []JSONAssertion{}},
				},
			},
			[]string{`"wave_index": 0`},
			nil,
		},
		{
			"wave_index omitted when nil",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						Assertions: []JSONAssertion{}},
				},
			},
			nil,
			[]string{`"wave_index"`},
		},
	}
}

func TestWriteJSON_ParallelExecution(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"parallel_execution present when set",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
				ParallelExecution: &ParallelExecutionJSON{
					WaveCount: 3, MaxParallelism: 2, SpeedupFactor: 1.5,
				},
			},
			[]string{`"parallel_execution"`, `"wave_count": 3`, `"max_parallelism": 2`, `"speedup_factor": 1.5`},
			nil,
		},
		{
			"parallel_execution omitted when nil",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
			},
			nil,
			[]string{`"parallel_execution"`},
		},
		{
			"speedup_factor omitted when zero",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
				ParallelExecution: &ParallelExecutionJSON{
					WaveCount: 1, MaxParallelism: 3,
				},
			},
			[]string{`"wave_count": 1`},
			[]string{`"speedup_factor"`},
		},
	}
}
```

#### Impact on Existing Tests
- Existing JSON tests pass unchanged because `WaveIndex` defaults to nil (omitted) and `ParallelExecution` defaults to nil (omitted).
- No schema-breaking changes — all new fields are `omitempty`.

---

### Step 4: TAP Output — Wave Annotations
**Rationale:** TAP is the simplest formatter to update. Wave boundaries are expressed as TAP comments.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/tap.go` | modify | Add `WaveIndex` field to `TAPResult`, modify `WriteTAP` to emit wave comments |
| `internal/output/tap_test.go` | modify | Add tests for wave annotations |
| `cmd/curlew/main.go` | modify | Populate `WaveIndex` in `buildTAPOutput` |

#### Current Code — tap.go
```go
type TAPResult struct {
	Name       string
	Passed     bool
	Skipped    bool
	Error      string
	Failures   []TAPFailure
	DurationMs int64
	RetryCount int
}
```

#### New Code — tap.go
```go
type TAPResult struct {
	Name       string
	Passed     bool
	Skipped    bool
	Error      string
	Failures   []TAPFailure
	DurationMs int64
	RetryCount int
	WaveIndex  int  // -1 = no wave (non-parallel)
}

// WriteTAP writes TAP version 13 formatted output to w.
// When wave annotations are present (WaveIndex >= 0), wave boundary comments
// are inserted between test points.
func WriteTAP(w io.Writer, results []TAPResult, passed, failed int) error {
	// ... existing header ...
	currentWave := -1
	for i, r := range results {
		// Insert wave comment at wave boundaries
		if r.WaveIndex >= 0 && r.WaveIndex != currentWave {
			currentWave = r.WaveIndex
			if _, err := fmt.Fprintf(w, "# Wave %d\n", currentWave+1); err != nil {
				return err
			}
		}
		// ... existing test point logic ...
	}
	// ... existing summary ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteTAP_WaveAnnotations(t *testing.T) {
	tests := []struct {
		name    string
		results []TAPResult
		passed  int
		failed  int
		check   func(t *testing.T, out string)
	}{
		{
			"wave comments inserted at wave boundaries",
			[]TAPResult{
				{Name: "A", Passed: true, DurationMs: 10, WaveIndex: 0},
				{Name: "B", Passed: true, DurationMs: 20, WaveIndex: 0},
				{Name: "C", Passed: true, DurationMs: 15, WaveIndex: 1},
			},
			3, 0,
			func(t *testing.T, out string) {
				if !strings.Contains(out, "# Wave 1\n") {
					t.Errorf("expected '# Wave 1' comment, got: %q", out)
				}
				if !strings.Contains(out, "# Wave 2\n") {
					t.Errorf("expected '# Wave 2' comment, got: %q", out)
				}
			},
		},
		{
			"no wave comments when WaveIndex is -1",
			[]TAPResult{
				{Name: "A", Passed: true, DurationMs: 10, WaveIndex: -1},
			},
			1, 0,
			func(t *testing.T, out string) {
				if strings.Contains(out, "# Wave") {
					t.Errorf("unexpected wave comment for non-parallel results, got: %q", out)
				}
			},
		},
		{
			"single wave still shows wave comment",
			[]TAPResult{
				{Name: "A", Passed: true, DurationMs: 10, WaveIndex: 0},
				{Name: "B", Passed: true, DurationMs: 20, WaveIndex: 0},
			},
			2, 0,
			func(t *testing.T, out string) {
				if !strings.Contains(out, "# Wave 1\n") {
					t.Errorf("expected '# Wave 1' comment, got: %q", out)
				}
				if strings.Count(out, "# Wave") != 1 {
					t.Errorf("expected exactly 1 wave comment, got: %q", out)
				}
			},
		},
	}
}
```

#### Impact on Existing Tests
- **All existing `TestWriteTAP` tests will continue to pass** because `WaveIndex` defaults to 0. However, this means existing tests would now see "# Wave 1" comments unexpectedly.
- **Fix:** Use `-1` as default for non-parallel mode. Since the zero value of `int` is `0`, we need to ensure existing test code sets `WaveIndex: -1` OR we change the logic: only emit wave comments when the result set contains any result with `WaveIndex >= 0`.
- **Decision:** Use a simpler approach — check if ANY result has `WaveIndex >= 0` at the start; if none do, skip all wave annotations. This preserves backward compatibility perfectly since existing tests never set `WaveIndex`, so all results have `WaveIndex: 0`, but we only emit wave comments if the results were explicitly marked as parallel. We'll add a `IsParallel` bool field to the `WriteTAP` signature or use a separate `WriteTAPParallel` helper.
- **Better approach:** Add an optional `TAPOptions` parameter to keep the API clean, or simply add a boolean parameter `parallel` to `WriteTAP`. Since Go doesn't have optional params, we use a separate function `WriteTAPWithWaves` or add a `Parallel bool` to `TAPResult` struct that acts as a "this set is from parallel execution" flag.
- **Final decision:** The simplest approach is to have the `buildTAPOutput` in main.go set `WaveIndex` to `-1` for non-parallel results. The wave comment logic only triggers when `WaveIndex >= 0`. This keeps the TAP writer simple and existing tests unaffected (0 is a valid wave index, but we gate on explicit parallel mode in the caller).

Actually, re-evaluating: the zero value `WaveIndex: 0` would cause false positives. The cleanest solution: use `*int` (pointer) for `WaveIndex` in `TAPResult`, nil = not parallel. This matches the JSON approach.

**Revised approach:**
```go
type TAPResult struct {
	// ... existing fields ...
	WaveIndex *int // nil = non-parallel; 0+ = wave index
}
```

This way, existing tests that create `TAPResult{}` get `WaveIndex: nil` and no wave comments are emitted.

---

### Step 5: Wire Everything in cmd/curlew/main.go
**Rationale:** This is the integration layer. Depends on Steps 1-4 for the data types and formatter methods to exist.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Update terminal rendering loop for wave headers, populate JSON parallel fields, populate TAP wave index |

#### Current Code — terminal output loop (lines 494-531)
```go
var currentPhase runner.Phase
for _, r := range results {
	if r.Phase != currentPhase {
		currentPhase = r.Phase
		switch currentPhase {
		case runner.PhaseSetup:
			out.SectionHeader("Setup")
		case runner.PhaseTeardown:
			out.SectionHeader("Teardown")
		}
	}
	// ... render result ...
}
```

#### New Code — terminal output loop
```go
var currentPhase runner.Phase
currentWaveIndex := -1
for _, r := range results {
	if r.Phase != currentPhase {
		currentPhase = r.Phase
		currentWaveIndex = -1 // reset wave tracking on phase change
		switch currentPhase {
		case runner.PhaseSetup:
			out.SectionHeader("Setup")
		case runner.PhaseTeardown:
			out.SectionHeader("Teardown")
		}
	}
	// Wave header for parallel execution
	if summary.IsParallel && r.Phase == runner.PhaseMain && r.WaveIndex != currentWaveIndex {
		currentWaveIndex = r.WaveIndex
		waveSize := countWaveResults(results, currentWaveIndex)
		out.WaveHeader(currentWaveIndex+1, waveSize)
	}
	// ... existing result rendering ...
}
```

Add helper:
```go
// countWaveResults counts results belonging to the given wave index in the main phase.
func countWaveResults(results []runner.RequestResult, waveIndex int) int {
	count := 0
	for _, r := range results {
		if r.Phase == runner.PhaseMain && r.WaveIndex == waveIndex {
			count++
		}
	}
	return count
}
```

#### Current Code — buildJSONOutput (lines 619-712)
Populate `WaveIndex` and `ParallelExecution` from summary:
```go
// Inside the result loop:
if summary != nil && summary.IsParallel && r.Phase == runner.PhaseMain {
	idx := r.WaveIndex
	jr.WaveIndex = &idx
}

// After building all requests:
if summary != nil && summary.IsParallel {
	speedup := 0.0
	if summary.WaveCount > 1 && summary.Duration > 0 {
		var seqEstimate time.Duration
		for _, d := range summary.WaveDurations {
			seqEstimate += d
		}
		speedup = float64(seqEstimate) / float64(summary.Duration)
		// Round to 1 decimal
		speedup = math.Round(speedup*10) / 10
	}
	out.ParallelExecution = &output.ParallelExecutionJSON{
		WaveCount:      summary.WaveCount,
		MaxParallelism: summary.MaxParallelism,
		SpeedupFactor:  speedup,
	}
}
```

#### Current Code — buildTAPOutput (lines 715-750)
Populate `WaveIndex`:
```go
// In the buildTAPOutput loop:
if r.Phase == runner.PhaseMain && summary != nil && summary.IsParallel {
	idx := r.WaveIndex
	tr.WaveIndex = &idx
}
```

Wait — `buildTAPOutput` doesn't have access to `summary`. We need to pass it or add a `parallel` bool parameter.

**Decision:** Change `buildTAPOutput` signature to accept a `parallel bool` parameter:
```go
func buildTAPOutput(results []runner.RequestResult, parallel bool) []output.TAPResult
```

#### Tests to Write FIRST (RED phase)

Integration tests are mainly covered by the smoke test. The unit-level wiring is tested through Steps 1-4.

```go
func TestBuildJSONOutput_ParallelExecution(t *testing.T) {
	// Test that buildJSONOutput populates ParallelExecution and WaveIndex
}

func TestBuildTAPOutput_WaveIndex(t *testing.T) {
	// Test that buildTAPOutput sets WaveIndex when parallel=true
}

func TestCountWaveResults(t *testing.T) {
	tests := []struct {
		name       string
		results    []runner.RequestResult
		waveIndex  int
		wantCount  int
	}{
		{"counts results in wave 0", ..., 0, 2},
		{"counts results in wave 1", ..., 1, 1},
		{"zero for nonexistent wave", ..., 5, 0},
	}
}
```

#### Impact on Existing Tests
- `buildTAPOutput` signature change from `buildTAPOutput(results)` to `buildTAPOutput(results, parallel)` requires updating call sites. Only 2 call sites: one in `runCmdInner` and any test helpers.
- The `buildJSONOutput` function gains new logic but existing calls pass `summary.IsParallel == false`, so no behavior change.

---

### Step 6: Enhanced Dry-Run Output
**Rationale:** The spec shows `--parallel --dry-run --show-dependencies` with enriched output. Depends on Step 1 data types.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/dot.go` | modify | Enhance `FormatWaves` to include max parallelism and expected speedup |
| `internal/parallel/dot_test.go` | modify | Update tests for enhanced format |

#### Current Code
```go
func FormatWaves(graph *DependencyGraph) string {
	// ... lists waves and total count ...
}
```

#### New Code
```go
func FormatWaves(graph *DependencyGraph) string {
	if len(graph.Waves) == 0 {
		return "Total waves: 0\nNo requests to execute.\n"
	}

	var sb strings.Builder
	maxPar := 0
	totalRequests := 0
	for i, wave := range graph.Waves {
		names := make([]string, len(wave))
		for j, idx := range wave {
			names[j] = graph.Nodes[idx].Name
		}
		concurrency := "concurrent"
		if len(wave) == 1 {
			concurrency = "sequential"
		}
		fmt.Fprintf(&sb, "  Wave %d: [%s] → %d %s\n", i+1, strings.Join(names, ", "), len(wave), concurrency)
		if len(wave) > maxPar {
			maxPar = len(wave)
		}
		totalRequests += len(wave)
	}
	fmt.Fprintf(&sb, "\nTotal waves: %d\n", len(graph.Waves))
	fmt.Fprintf(&sb, "Maximum parallelism: %d requests per wave\n", maxPar)
	if len(graph.Waves) > 1 && maxPar > 0 {
		speedup := float64(totalRequests) / float64(len(graph.Waves))
		fmt.Fprintf(&sb, "Expected speedup: %.1fx\n", speedup)
	}
	return sb.String()
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestFormatWaves_Enhanced(t *testing.T) {
	tests := []struct {
		name      string
		graph     *DependencyGraph
		wantParts []string
	}{
		{
			"includes max parallelism and speedup",
			&DependencyGraph{
				Nodes: []RequestNode{
					{Index: 0, Name: "A"},
					{Index: 1, Name: "B"},
					{Index: 2, Name: "C"},
				},
				Waves:   [][]int{{0, 1}, {2}},
				IsValid: true,
			},
			[]string{"Wave 1: [A, B]", "2 concurrent", "Wave 2: [C]", "1 sequential", "Maximum parallelism: 2", "Expected speedup:"},
		},
		{
			"single wave shows no speedup",
			&DependencyGraph{
				Nodes: []RequestNode{{Index: 0, Name: "A"}, {Index: 1, Name: "B"}},
				Waves: [][]int{{0, 1}},
				IsValid: true,
			},
			[]string{"Wave 1: [A, B]", "Maximum parallelism: 2"},
		},
	}
}
```

#### Impact on Existing Tests
- `TestFormatWaves` in `dot_test.go` — **will break** because the output format changes (adding max parallelism line, changing wave line format). Fix by updating expected output assertions.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | All parallel tests | none | New fields have zero defaults |
| `internal/output/terminal_test.go` | All existing | none | New methods are additive |
| `internal/output/json_test.go` | All existing | none | New fields are omitempty |
| `internal/output/tap_test.go` | `TestWriteTAP` | none | WaveIndex nil by default |
| `internal/parallel/dot_test.go` | `TestFormatWaves` | breaks | Update expected output format |
| `cmd/curlew/main.go` tests | buildTAPOutput callers | breaks | Add parallel parameter |

## Risks and Edge Cases

- **Risk:** Speedup calculation could be misleading if wave durations include waiting time rather than just HTTP time. → **Mitigation:** Use actual wave duration (wall-clock from wave start to wave end) which is what users care about. Document that speedup reflects observed wall-clock improvement.

- **Edge case:** Single request in parallel mode — should show "1 wave, 1 concurrent" without speedup factor. → **Handling:** Only show speedup when `WaveCount > 1`.

- **Edge case:** All requests skipped due to dependency failures — waves still counted. → **Handling:** Count waves from the graph, not from executed results. The graph has the canonical wave structure.

- **Edge case:** Guard rail stops execution mid-wave — partial wave shown. → **Handling:** WaveIndex is set per-result, so partial waves render correctly. Summary shows the total waves from the graph.

- **Risk:** `WaveIndex` zero-value conflict — Go int zero is 0, which is a valid wave index. → **Mitigation:** Use `*int` in JSON and TAP types (nil = non-parallel), and `-1` or a boolean `IsParallel` in runner types. In `runner.RequestResult`, we use `WaveIndex int` with `-1` for non-parallel (sequential) mode.

- **Edge case:** Context cancellation mid-execution — remaining waves show as skipped with wave index. → **Handling:** Already handled in executor — cancelled wave outcomes have `WaveIndex` set.

- **Edge case:** Empty collection with --parallel — no waves to display. → **Handling:** `IsParallel` is only true when there were actual parallel results. Zero requests = no parallel summary.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Terminal output with wave grouping
curlew run --parallel tests.yaml

# JSON output with parallel_execution metadata
curlew run --parallel tests.yaml --format json

# TAP output with wave annotations
curlew run --parallel tests.yaml --format tap

# Dry-run output with enhanced wave plan
curlew run --parallel tests.yaml --dry-run --show-dependencies

# Unit tests
go test ./internal/output/...
```
