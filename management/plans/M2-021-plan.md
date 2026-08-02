# Implementation Plan: M2-021

## Overview
Extend the output system to provide data-driven-specific formatting: compact terminal with progress bar for >= 10 iterations, verbose terminal for < 10 iterations, JSON output with data-driven aggregate fields (`type`, `total_iterations`, `passed_iterations`, `failed_iterations`, `average_duration_ms`), and TAP output with per-iteration test lines. Failed iteration details are always shown regardless of mode.

## Task Details
- **ID:** M2-021
- **Title:** Data-driven output formatting (terminal compact/verbose, JSON, TAP)
- **Phase:** M2: Data-Driven Testing
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-019 | Data-driven testing with CSV and JSON data sources | done |
| M1-020 | JSON output format | done |
| M1-021 | TAP output format | done |

## Architecture Decisions

1. **Data-driven metadata on RequestResult:** The `runner.RequestResult` struct currently has no field to distinguish data-driven iteration results from regular results. We need to add fields: `IsDataDriven bool`, `DataDrivenName string` (the base request name, without `[X/Y]`), `IterationIndex int`, `IterationTotal int`. This lets the output layer group and aggregate iterations.

2. **Compact vs verbose threshold at output layer:** The spec says >= 10 iterations use compact mode, < 10 use verbose. This logic belongs in the output/terminal layer, not the runner. The terminal Printer will inspect the data-driven metadata to decide.

3. **Progress bar is post-execution:** Since iterations execute sequentially in the runner and results are returned as a batch, the "progress bar" is rendered post-execution as a summary line (not a live updating bar). The compact output shows a condensed summary line for the data-driven group rather than individual lines per iteration.

4. **JSON data-driven envelope:** The JSON output adds a `data_driven` field to `JSONOutput` when data-driven results are present. This contains `type`, `total_iterations`, `passed_iterations`, `failed_iterations`, `total_duration_ms`, `average_duration_ms`. Individual iteration results remain in the `requests` array.

5. **TAP per-iteration lines:** TAP already gets one line per iteration because each iteration is a separate `RequestResult`. No structural change needed -- just ensure data-driven iterations produce correctly formatted TAP output (which they already do since each iteration becomes a TAPResult).

## Implementation Steps

### Step 1: Add Data-Driven Metadata to RequestResult
**Rationale:** All output formatters need this metadata. Smallest blast radius -- only adds fields, doesn't change behavior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `IsDataDriven`, `DataDrivenName`, `IterationIndex`, `IterationTotal` fields to `RequestResult`; populate them in `executeDataDriven` |

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
	WaveIndex        int
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
	WaveIndex        int
	// Data-driven iteration metadata (populated only for data-driven results)
	IsDataDriven   bool   // true when this result is from a data-driven iteration
	DataDrivenName string // base request name (without [X/Y] suffix)
	IterationIndex int    // 0-based iteration index
	IterationTotal int    // total number of iterations in this data-driven group
}
```

In `executeDataDriven`, when building each `RequestResult`, set:
```go
rr := RequestResult{
	Name:           iterName,
	Phase:          phase,
	// ...existing fields...
	IsDataDriven:   true,
	DataDrivenName: item.Name,
	IterationIndex: idx,
	IterationTotal: total,
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_DataDriven_ResultMetadata(t *testing.T) {
	tests := []struct {
		name           string
		rows           int
		wantDataDriven bool
		wantBaseName   string
		wantTotal      int
	}{
		{"3 iterations have metadata", 3, true, "Create User", 3},
	}
	// Verify each result has IsDataDriven=true, DataDrivenName, IterationIndex, IterationTotal
}
```

#### Impact on Existing Tests
- No existing tests affected -- only adding new fields with zero-value defaults

### Step 2: Add Data-Driven Terminal Output Methods
**Rationale:** Terminal output is the primary user-facing format. Needs new Printer methods for compact and verbose data-driven display.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `DataDrivenHeader`, `DataDrivenCompactSummary`, `DataDrivenVerboseResult`, `DataDrivenSummary` methods |
| `internal/output/terminal_test.go` | modify | Add tests for all new methods |

#### New Code
```go
// DataDrivenHeader writes the data-driven section header.
// Format: "Data-Driven: <name> (<total> iterations)"
func (p *Printer) DataDrivenHeader(name string, total int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	label := fmt.Sprintf("Data-Driven: %s (%d iterations)", name, total)
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(label, ansiBoldCyan, p.color))
}

// DataDrivenCompactSummary writes a one-line compact summary for >= 10 iterations.
// Format: "  <passed> passed, <failed> failed (<total> total, avg <avg>ms)"
func (p *Printer) DataDrivenCompactSummary(passed, failed, total int, avgDurationMs int64, failedIndices []int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	passedStr := fmt.Sprintf("%d passed", passed)
	failedStr := fmt.Sprintf("%d failed", failed)
	if p.color {
		passedStr = colorize(passedStr, ansiGreen, true)
		if failed > 0 {
			failedStr = colorize(failedStr, ansiRed, true)
		}
	}
	_, _ = fmt.Fprintf(p.w, "  %s, %s (%d total, avg %dms)\n", passedStr, failedStr, total, avgDurationMs)
	if len(failedIndices) > 0 {
		idxStrs := make([]string, len(failedIndices))
		for i, idx := range failedIndices {
			idxStrs[i] = fmt.Sprintf("%d", idx)
		}
		_, _ = fmt.Fprintf(p.w, "  %s\n", colorize("Failed iterations: "+strings.Join(idxStrs, ", "), ansiRed, p.color))
	}
}

// DataDrivenVerboseResult writes a single verbose iteration result line.
// Format: "  ✓ Iteration <n>: <label> (<duration>ms)" or "  ✗ Iteration <n>: <label> (<duration>ms)"
func (p *Printer) DataDrivenVerboseResult(iterationNum int, label string, durationMs int64, passed bool) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	indicator := "✓"
	code := ansiGreen
	if !passed {
		indicator = "✗"
		code = ansiRed
	}
	_, _ = fmt.Fprintf(p.w, "  %s Iteration %d: %s (%dms)\n",
		colorize(indicator, code, p.color), iterationNum, label, durationMs)
}

// DataDrivenSummary writes the data-driven summary line.
// Format: "  Summary: <passed> passed, <failed> failed (<total> total)"
//         "  Average duration: <avg>ms"
func (p *Printer) DataDrivenSummary(passed, failed, total int, avgDurationMs int64) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\n  Summary: %d passed, %d failed (%d total)\n", passed, failed, total)
	_, _ = fmt.Fprintf(p.w, "  Average duration: %dms\n", avgDurationMs)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrinter_DataDrivenHeader(t *testing.T) {
	tests := []struct {
		name      string
		ddName    string
		total     int
		verbosity Verbosity
		color     bool
		wantParts []string
		wantEmpty bool
	}{
		{"shows name and total", "Create Users", 100, VerbosityDefault, false, []string{"Data-Driven:", "Create Users", "100 iterations"}, false},
		{"quiet suppresses output", "Create Users", 100, VerbosityQuiet, false, nil, true},
		{"color emits bold cyan ANSI", "Create Users", 100, VerbosityDefault, true, []string{"\033[1;36m"}, false},
	}
}

func TestPrinter_DataDrivenCompactSummary(t *testing.T) {
	tests := []struct {
		name          string
		passed        int
		failed        int
		total         int
		avgDurationMs int64
		failedIndices []int
		color         bool
		wantParts     []string
		wantEmpty     bool
	}{
		{"all passed compact", 100, 0, 100, 135, nil, false, []string{"100 passed", "0 failed", "100 total", "avg 135ms"}, false},
		{"some failed compact with indices", 97, 3, 100, 135, []int{15, 48, 72}, false, []string{"97 passed", "3 failed", "Failed iterations:", "15", "48", "72"}, false},
		{"color green passed red failed", 90, 10, 100, 50, []int{1}, true, []string{"\033[32m", "\033[31m"}, false},
	}
}

func TestPrinter_DataDrivenVerboseResult(t *testing.T) {
	tests := []struct {
		name         string
		iterationNum int
		label        string
		durationMs   int64
		passed       bool
		color        bool
		wantParts    []string
	}{
		{"passing iteration", 1, "valid@example.com", 145, true, false, []string{"✓", "Iteration 1", "valid@example.com", "145ms"}},
		{"failing iteration", 3, "invalid-email", 98, false, false, []string{"✗", "Iteration 3", "invalid-email", "98ms"}},
		{"passing with color", 1, "test", 50, true, true, []string{"\033[32m"}},
		{"failing with color", 2, "test", 50, false, true, []string{"\033[31m"}},
	}
}

func TestPrinter_DataDrivenSummary(t *testing.T) {
	tests := []struct {
		name          string
		passed        int
		failed        int
		total         int
		avgDurationMs int64
		verbosity     Verbosity
		wantParts     []string
		wantEmpty     bool
	}{
		{"shows summary line", 4, 1, 5, 128, VerbosityDefault, []string{"4 passed", "1 failed", "5 total", "Average duration:", "128ms"}, false},
		{"quiet suppresses", 4, 1, 5, 128, VerbosityQuiet, nil, true},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected -- only adding new methods

### Step 3: Add Data-Driven JSON Output Structure
**Rationale:** JSON output needs data-driven-specific fields per the specification. Extends existing `JSONOutput` struct.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add `DataDrivenJSON` struct and field to `JSONOutput` |
| `internal/output/json_test.go` | modify | Add tests for data-driven JSON serialization |

#### New Code
```go
// DataDrivenJSON holds data-driven aggregate metadata in JSON output.
type DataDrivenJSON struct {
	Type             string `json:"type"`               // always "data_driven"
	Name             string `json:"name"`               // base request name
	TotalIterations  int    `json:"total_iterations"`
	PassedIterations int    `json:"passed_iterations"`
	FailedIterations int    `json:"failed_iterations"`
	TotalDurationMs  int64  `json:"total_duration_ms"`
	AvgDurationMs    int64  `json:"average_duration_ms"`
}
```

Add to `JSONOutput`:
```go
type JSONOutput struct {
	// ... existing fields ...
	DataDriven []DataDrivenJSON `json:"data_driven,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteJSON_DataDriven(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"data_driven present when set",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{
					{Type: "data_driven", Name: "Create Users", TotalIterations: 100, PassedIterations: 97, FailedIterations: 3, TotalDurationMs: 13500, AvgDurationMs: 135},
				},
			},
			[]string{`"data_driven"`, `"total_iterations": 100`, `"passed_iterations": 97`, `"failed_iterations": 3`, `"average_duration_ms": 135`},
			nil,
		},
		{
			"data_driven omitted when nil",
			&JSONOutput{Name: "Suite", Status: "passed", Requests: []JSONRequest{}},
			nil,
			[]string{`"data_driven"`},
		},
		{
			"data_driven type is data_driven",
			&JSONOutput{
				Name: "Suite", Status: "passed", Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{{Type: "data_driven", Name: "Test", TotalIterations: 5, PassedIterations: 5}},
			},
			[]string{`"type": "data_driven"`},
			nil,
		},
	}
}
```

#### Impact on Existing Tests
- No existing tests break -- `DataDriven` is `omitempty` and defaults to nil

### Step 4: Wire Data-Driven Output in main.go (Terminal)
**Rationale:** Connect the new terminal output methods to the run command's terminal output path. This is where compact vs verbose switching happens.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add data-driven grouping/rendering logic in terminal output section; add `buildDataDrivenJSON` helper; update `buildJSONOutput` |

#### Current Code (terminal output loop in runCmdInner, around line 576)
```go
for _, r := range results {
	// ... phase/wave header logic ...
	switch {
	case r.Skipped:
		// ...
	case r.Err != nil:
		// ...
	default:
		// ... individual result output ...
	}
}
```

#### New Code
The terminal output loop needs to detect data-driven groups. When we encounter the first result of a data-driven group, we:
1. Output `DataDrivenHeader`
2. If total >= 10: collect all iterations, compute stats, output `DataDrivenCompactSummary` + failed iteration details
3. If total < 10: output each iteration with `DataDrivenVerboseResult` + failed details, then `DataDrivenSummary`
4. Skip subsequent iterations in the main loop (they were already rendered)

Implementation approach: build a helper function `renderDataDrivenGroup` that takes a slice of contiguous data-driven results for the same base name and renders them.

```go
// groupDataDrivenResults extracts contiguous data-driven results for the same
// base name starting at index i. Returns the group and the next index to process.
func groupDataDrivenResults(results []runner.RequestResult, startIdx int) ([]runner.RequestResult, int) {
	baseName := results[startIdx].DataDrivenName
	group := []runner.RequestResult{results[startIdx]}
	next := startIdx + 1
	for next < len(results) && results[next].IsDataDriven && results[next].DataDrivenName == baseName {
		group = append(group, results[next])
		next++
	}
	return group, next
}
```

For JSON output, add a helper that computes the `DataDrivenJSON` aggregates from the grouped results and adds them to `JSONOutput`.

#### Tests to Write FIRST (RED phase)

```go
func TestGroupDataDrivenResults(t *testing.T) {
	tests := []struct {
		name      string
		results   []runner.RequestResult
		startIdx  int
		wantLen   int
		wantNext  int
	}{
		{"groups contiguous iterations", makeDataDrivenResults("Create User", 5), 0, 5, 5},
		{"stops at non-data-driven", mixedResults(), 0, 3, 3},
		{"stops at different base name", differentNameResults(), 0, 2, 2},
	}
}
```

#### Impact on Existing Tests
- No existing tests break -- the terminal output path still renders non-data-driven results identically

### Step 5: Wire Data-Driven Output in main.go (JSON)
**Rationale:** Update JSON output building to include data-driven aggregates alongside per-iteration results.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Update `buildJSONOutput` to compute and attach `DataDrivenJSON` entries |

#### New Code
After building all `JSONRequest` entries, scan for data-driven groups and compute aggregates:

```go
// In buildJSONOutput, after populating out.Requests:
ddGroups := groupDataDrivenRequests(results)
if len(ddGroups) > 0 {
	out.DataDriven = make([]output.DataDrivenJSON, 0, len(ddGroups))
	for _, group := range ddGroups {
		dd := computeDataDrivenAggregate(group)
		out.DataDriven = append(out.DataDriven, dd)
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestBuildJSONOutput_DataDriven(t *testing.T) {
	tests := []struct {
		name       string
		results    []runner.RequestResult
		wantDDLen  int
		wantFields []string
	}{
		{"data-driven results produce aggregate", makeRunnerDDResults(100, 97), 1, []string{"total_iterations", "passed_iterations"}},
		{"no data-driven results omit field", makeRunnerNormalResults(3), 0, nil},
		{"mixed results produce correct aggregate", makeMixedRunnerResults(), 1, []string{"total_iterations"}},
	}
}
```

#### Impact on Existing Tests
- No existing tests break -- `buildJSONOutput` returns same structure for non-data-driven inputs

### Step 6: Wire Data-Driven Output in main.go (TAP)
**Rationale:** TAP output already works per-iteration because each iteration is a separate `RequestResult` that becomes a `TAPResult`. However, we should add a TAP comment line to mark data-driven groups for readability.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Update `buildTAPOutput` to insert data-driven group annotations |
| `internal/output/tap.go` | modify | Add support for data-driven comment annotations in `WriteTAP` |

#### New Code
Add a `DataDrivenGroup` field to `TAPResult`:
```go
type TAPResult struct {
	// ... existing fields ...
	DataDrivenGroup *string // non-nil = first result in a data-driven group; value = "Name (N iterations)"
}
```

In `WriteTAP`, before the test line for a result with `DataDrivenGroup != nil`, emit:
```
# Data-Driven: Create Users (100 iterations)
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteTAP_DataDrivenAnnotations(t *testing.T) {
	tests := []struct {
		name    string
		results []TAPResult
		check   func(t *testing.T, out string)
	}{
		{
			"data-driven comment at group start",
			dataDrivenTAPResults(3),
			func(t *testing.T, out string) {
				if !strings.Contains(out, "# Data-Driven: Create User (3 iterations)") {
					t.Errorf("expected data-driven comment, got: %q", out)
				}
			},
		},
		{
			"no comment when not data-driven",
			normalTAPResults(2),
			func(t *testing.T, out string) {
				if strings.Contains(out, "# Data-Driven") {
					t.Errorf("unexpected data-driven comment, got: %q", out)
				}
			},
		},
	}
}
```

#### Impact on Existing Tests
- `DataDrivenGroup` defaults to nil, so existing TAP tests are unaffected

### Step 7: Integration Wiring and Smoke Test
**Rationale:** Verify end-to-end behavior with a real data-driven collection file.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Final integration adjustments if needed |
| `cmd/apitest/main_test.go` | modify | Add integration tests for data-driven output across formats |

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmdInner_DataDriven_TerminalCompact(t *testing.T) {
	// 10+ iterations, verify compact output, no per-iteration lines
}

func TestRunCmdInner_DataDriven_TerminalVerbose(t *testing.T) {
	// < 10 iterations, verify per-iteration lines
}

func TestRunCmdInner_DataDriven_JSONFormat(t *testing.T) {
	// Verify data_driven field in JSON output
}

func TestRunCmdInner_DataDriven_TAPFormat(t *testing.T) {
	// Verify per-iteration TAP lines with data-driven comment
}

func TestRunCmdInner_DataDriven_FailedIterationDetails(t *testing.T) {
	// Verify failed iteration details always shown in compact mode
}
```

#### Impact on Existing Tests
- No existing tests break

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | `TestRun_DataDriven_*` | unaffected | new fields have zero defaults |
| `internal/output/terminal_test.go` | all existing | unaffected | new methods only |
| `internal/output/json_test.go` | all existing | unaffected | new field is omitempty |
| `internal/output/tap_test.go` | all existing | unaffected | new field defaults to nil |
| `cmd/apitest/main_test.go` | all existing | unaffected | no behavioral change for non-data-driven |

## Risks and Edge Cases

- **Risk:** Data-driven iterations interleaved with non-data-driven requests in the results slice could break grouping logic. **Mitigation:** The `executeDataDriven` function in runner.go always returns contiguous results for one data-driven request. The main `executePhase` loop processes items sequentially, so data-driven results are always contiguous in the results slice.

- **Edge case:** Zero iterations after filtering (empty dataset). **Handling:** The runner already handles this by returning a skipped result. The output layer will see `Skipped: true` and render accordingly.

- **Edge case:** All iterations fail in compact mode. **Handling:** Show the compact summary with `0 passed, N failed` and always show failed iteration details. Limit the number of failed indices displayed (e.g., first 20) with a "and N more" suffix.

- **Edge case:** Single data-driven iteration (total=1). **Handling:** Falls into verbose mode (< 10). One iteration line plus summary.

- **Edge case:** Data-driven with fail_fast stops early, so `IterationTotal` may differ from actual results length. **Handling:** `IterationTotal` reflects the total rows in the dataset (what was intended), while actual results may be fewer. The summary should use actual counts for passed/failed.

- **Risk:** Compact mode for terminal still needs to show failed iteration details (request, response, assertion failures). **Mitigation:** After the compact summary line, iterate over the failed results in the group and render their details using existing `AssertionDetail`/`RequestError` methods.

- **Edge case:** JSON `average_duration_ms` calculation when some iterations are errors with 0 duration. **Handling:** Only include iterations that have a non-nil `Result` in the average calculation.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Run data-driven with 100 iterations and confirm compact terminal output
# (requires a test collection with data-driven and a mock/test server)
go test ./internal/output/... -run DataDriven -v

# Run data-driven with 3 iterations and confirm verbose terminal output
go test ./internal/output/... -run DataDriven -v

# Run data-driven with --format json and confirm data-driven JSON schema
go test ./cmd/apitest/... -run DataDriven.*JSON -v

# Full test suite
go test ./...
```
