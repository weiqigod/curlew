# Implementation Plan: M1-003

## Overview

Add sequential multi-request execution with per-request output, pass/fail/skipped tracking, `stop_on_failure` option, total duration in summary, and empty-collection warning. Extracts the inline execution loop from `main.go` into a testable `internal/runner/` package.

## Task Details
- **ID:** M1-003
- **Title:** Run multiple requests sequentially
- **Phase:** M1: Core CLI
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-001 | Run a single GET request from a collection file | done |

## Implementation Steps

### Step 1: Add `Options` to parser collection model

**Rationale:** Smallest change — extends the data model with no behavior change to existing code. The zero value of `Options` preserves all existing behavior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Options` struct and field to `Collection` |
| `internal/parser/parser_test.go` | modify | Add test cases for options parsing |
| `internal/parser/testdata/with_options.yaml` | create | Test fixture with `stop_on_failure: true` |
| `internal/parser/testdata/empty_requests.yaml` | create | Test fixture with name but no requests |

#### Current Code

```go
// internal/parser/collection.go
type Collection struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Requests    []RequestItem `yaml:"requests"`
}
```

#### New Code

```go
// internal/parser/collection.go

// Options holds collection-level execution options.
type Options struct {
	StopOnFailure bool `yaml:"stop_on_failure"`
}

type Collection struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Requests    []RequestItem `yaml:"requests"`
	Options     Options       `yaml:"options,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
// Add to existing TestParseFile table in parser_test.go:
{
    name: "collection with stop_on_failure true",
    file: "testdata/with_options.yaml",
    wantCol: &Collection{
        Name: "Options Test",
        Requests: []RequestItem{
            {
                Name:    "Get Example",
                Request: Request{Method: "GET", URL: "https://example.com"},
            },
        },
        Options: Options{StopOnFailure: true},
    },
},
{
    name: "collection without options defaults stop_on_failure to false",
    file: "testdata/minimal.yaml",
    // existing test — verify Options field is zero value
    // (already covered by existing "valid minimal collection" case
    //  once Collection gains the Options field with zero value)
},
```

Also add a separate test for empty requests parsing:
```go
func TestParseFile_empty_requests(t *testing.T) {
    col, err := ParseFile("testdata/empty_requests.yaml")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(col.Requests) != 0 {
        t.Errorf("got %d requests, want 0", len(col.Requests))
    }
}
```

#### Impact on Existing Tests
- `"valid minimal collection"` test uses `reflect.DeepEqual` — the `Options` field will be zero value (`Options{}`) in both actual and expected, so **no change needed** as long as the expected struct also has the zero-value `Options` field. However, since the test explicitly constructs `&Collection{...}` without `Options`, Go sets it to `Options{}` automatically. **No breakage.**

---

### Step 2: Add output functions for warning, skipped, and duration summary

**Rationale:** Extends output capabilities needed by the runner. No behavior change to existing callers — all new functions.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `PrintWarning`, `PrintSkipped`, `PrintSummaryWithDuration` |
| `internal/output/terminal_test.go` | modify | Add test cases for new functions |

#### Current Code

```go
// internal/output/terminal.go — existing functions only
func PrintResult(w io.Writer, name string, result *httpexec.Result)
func PrintCollectionHeader(w io.Writer, name string)
func PrintSummary(w io.Writer, total, passed, failed int)
func PrintError(w io.Writer, msg string)
```

#### New Code

```go
// PrintWarning writes a warning message to w.
func PrintWarning(w io.Writer, msg string) {
	_, _ = fmt.Fprintf(w, "Warning: %s\n", msg)
}

// PrintSkipped writes a skipped request line to w.
func PrintSkipped(w io.Writer, name string) {
	_, _ = fmt.Fprintf(w, "  %s  SKIPPED\n", name)
}

// PrintSummaryWithDuration writes the execution summary including total duration.
// When skipped is 0, the skipped segment is omitted.
func PrintSummaryWithDuration(w io.Writer, total, passed, failed, skipped int, duration time.Duration) {
	if skipped > 0 {
		_, _ = fmt.Fprintf(w, "\n%d request(s): %d passed, %d failed, %d skipped (%dms)\n",
			total, passed, failed, skipped, duration.Milliseconds())
	} else {
		_, _ = fmt.Fprintf(w, "\n%d request(s): %d passed, %d failed (%dms)\n",
			total, passed, failed, duration.Milliseconds())
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrintWarning(t *testing.T) {
	var buf bytes.Buffer
	PrintWarning(&buf, "collection has no requests")
	got := buf.String()
	if !strings.Contains(got, "Warning:") {
		t.Errorf("output %q does not contain 'Warning:'", got)
	}
	if !strings.Contains(got, "collection has no requests") {
		t.Errorf("output %q does not contain message", got)
	}
}

func TestPrintSkipped(t *testing.T) {
	var buf bytes.Buffer
	PrintSkipped(&buf, "Skipped Request")
	got := buf.String()
	if !strings.Contains(got, "Skipped Request") {
		t.Errorf("output %q does not contain request name", got)
	}
	if !strings.Contains(got, "SKIPPED") {
		t.Errorf("output %q does not contain SKIPPED marker", got)
	}
}

func TestPrintSummaryWithDuration(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		passed    int
		failed    int
		skipped   int
		duration  time.Duration
		wantParts []string
		wantNot   []string
	}{
		{
			name:      "all passed no skipped",
			total:     3,
			passed:    3,
			failed:    0,
			skipped:   0,
			duration:  150 * time.Millisecond,
			wantParts: []string{"3 request(s)", "3 passed", "0 failed", "150ms"},
			wantNot:   []string{"skipped"},
		},
		{
			name:      "some failed no skipped",
			total:     3,
			passed:    2,
			failed:    1,
			skipped:   0,
			duration:  200 * time.Millisecond,
			wantParts: []string{"3 request(s)", "2 passed", "1 failed", "200ms"},
			wantNot:   []string{"skipped"},
		},
		{
			name:      "with skipped requests",
			total:     3,
			passed:    1,
			failed:    1,
			skipped:   1,
			duration:  100 * time.Millisecond,
			wantParts: []string{"3 request(s)", "1 passed", "1 failed", "1 skipped", "100ms"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			PrintSummaryWithDuration(&buf, tt.total, tt.passed, tt.failed, tt.skipped, tt.duration)
			got := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output %q does not contain %q", got, part)
				}
			}
			for _, part := range tt.wantNot {
				if strings.Contains(got, part) {
					t.Errorf("output %q should not contain %q", got, part)
				}
			}
		})
	}
}
```

#### Impact on Existing Tests
- None. All new functions; `PrintSummary` left intact.

---

### Step 3: Create `internal/runner/` package

**Rationale:** Core of the task. A testable orchestration layer with no I/O — returns results for the caller to display.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | create | `Run` function, `RequestResult`, `Summary` types, `ExecuteFunc` type |
| `internal/runner/runner_test.go` | create | Table-driven tests with mock executor |

#### New Code

```go
// internal/runner/runner.go
package runner

import (
	"context"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
)

// ExecuteFunc is the function signature for executing a single request.
type ExecuteFunc func(ctx context.Context, req *parser.Request) (*httpexec.Result, error)

// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
	Name    string
	Result  *httpexec.Result // nil if skipped or error
	Err     error            // non-nil on execution error
	Skipped bool
}

// Summary holds aggregate execution results.
type Summary struct {
	Total    int
	Passed   int
	Failed   int
	Skipped  int
	Duration time.Duration
}

// Run executes all requests in the collection sequentially and returns
// per-request results and a summary.
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc) ([]RequestResult, *Summary) {
	results := make([]RequestResult, 0, len(col.Requests))
	summary := &Summary{Total: len(col.Requests)}

	start := time.Now()
	stopped := false

	for _, item := range col.Requests {
		if stopped {
			results = append(results, RequestResult{Name: item.Name, Skipped: true})
			summary.Skipped++
			continue
		}

		if err := ctx.Err(); err != nil {
			results = append(results, RequestResult{Name: item.Name, Skipped: true})
			summary.Skipped++
			stopped = true
			continue
		}

		result, execErr := exec(ctx, &item.Request)
		if execErr != nil {
			results = append(results, RequestResult{Name: item.Name, Err: execErr})
			summary.Failed++
			if col.Options.StopOnFailure {
				stopped = true
			}
			continue
		}

		results = append(results, RequestResult{Name: item.Name, Result: result})
		summary.Passed++
	}

	summary.Duration = time.Since(start)
	return results, summary
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun(t *testing.T) {
	tests := []struct {
		name          string
		collection    *parser.Collection
		execFunc      ExecuteFunc
		wantPassed    int
		wantFailed    int
		wantSkipped   int
		wantOrder     []string // expected order of result names
	}{
		{
			name: "all requests succeed",
			// 3 requests, all return 200
			wantPassed: 3, wantFailed: 0, wantSkipped: 0,
		},
		{
			name: "execution order is preserved",
			// 3 requests named A, B, C — verify order
			wantOrder: []string{"A", "B", "C"},
		},
		{
			name: "one network error in middle counts correctly",
			// 3 requests, middle returns error
			wantPassed: 2, wantFailed: 1, wantSkipped: 0,
		},
		{
			name: "stop_on_failure true skips remaining after first failure",
			// 3 requests, first fails, StopOnFailure: true
			wantPassed: 0, wantFailed: 1, wantSkipped: 2,
		},
		{
			name: "stop_on_failure false continues after failure",
			// 3 requests, first fails, StopOnFailure: false
			wantPassed: 2, wantFailed: 1, wantSkipped: 0,
		},
		{
			name: "stop_on_failure true second fails third skipped",
			// 3 requests, second fails
			wantPassed: 1, wantFailed: 1, wantSkipped: 1,
		},
		{
			name: "empty requests returns zero summary",
			// 0 requests
			wantPassed: 0, wantFailed: 0, wantSkipped: 0,
		},
		{
			name: "total duration is positive",
			// verify summary.Duration > 0
		},
		{
			name: "skipped results have Skipped true and nil Result",
			// verify RequestResult fields for skipped entries
		},
		{
			name: "context cancellation stops execution",
			// cancel context after first request
		},
	}
	// ...
}
```

#### Impact on Existing Tests
- None. New package entirely.

---

### Step 4: Wire runner into `cmd/curlew/main.go`

**Rationale:** This is where behavior changes become visible. Depends on Steps 1-3.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Replace inline loop with `runner.Run()`, add empty-requests warning |
| `cmd/curlew/main_test.go` | modify | Add integration tests for multi-request, empty, stop_on_failure |
| `cmd/curlew/testdata/empty_requests.yaml` | create | Test fixture for empty requests integration test |

#### Current Code

```go
// cmd/curlew/main.go:41-74
func runCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "Usage: curlew run <collection-file>")
		return 1
	}

	col, err := parser.ParseFile(args[0])
	if err != nil {
		output.PrintError(os.Stderr, err.Error())
		return 3
	}

	ctx := context.Background()
	output.PrintCollectionHeader(os.Stdout, col.Name)

	passed, failed := 0, 0
	for _, item := range col.Requests {
		result, execErr := httpexec.Execute(ctx, &item.Request)
		if execErr != nil {
			output.PrintError(os.Stderr, fmt.Sprintf("%s: %v", item.Name, execErr))
			failed++
			continue
		}
		output.PrintResult(os.Stdout, item.Name, result)
		passed++
	}

	output.PrintSummary(os.Stdout, passed+failed, passed, failed)

	if failed > 0 {
		return 4
	}
	return 0
}
```

#### New Code

```go
func runCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "Usage: curlew run <collection-file>")
		return 1
	}

	col, err := parser.ParseFile(args[0])
	if err != nil {
		output.PrintError(os.Stderr, err.Error())
		return 3
	}

	if len(col.Requests) == 0 {
		output.PrintWarning(os.Stderr, "collection has no requests")
		return 0
	}

	ctx := context.Background()
	output.PrintCollectionHeader(os.Stdout, col.Name)

	results, summary := runner.Run(ctx, col, httpexec.Execute)

	for _, r := range results {
		switch {
		case r.Skipped:
			output.PrintSkipped(os.Stdout, r.Name)
		case r.Err != nil:
			output.PrintError(os.Stderr, fmt.Sprintf("%s: %v", r.Name, r.Err))
		default:
			output.PrintResult(os.Stdout, r.Name, r.Result)
		}
	}

	output.PrintSummaryWithDuration(os.Stdout, summary.Total, summary.Passed,
		summary.Failed, summary.Skipped, summary.Duration)

	if summary.Failed > 0 {
		return 4
	}
	return 0
}
```

#### Tests to Write FIRST (RED phase)

```go
// Add to cmd/curlew/main_test.go:

func TestCLIIntegration_multiple_requests_all_succeed(t *testing.T) {
	// 3-endpoint test server, collection with 3 requests
	// Assert: exit 0, stdout contains "3 passed, 0 failed", duration in ms
}

func TestCLIIntegration_empty_requests_warning(t *testing.T) {
	// Collection with name but no requests
	// Assert: exit 0, stderr contains "Warning:", "no requests"
}

func TestCLIIntegration_stop_on_failure(t *testing.T) {
	// 3 requests: valid server, unreachable, valid server
	// options.stop_on_failure: true
	// Assert: exit 4, stdout contains "SKIPPED", "1 passed, 1 failed, 1 skipped"
}

func TestCLIIntegration_summary_shows_duration(t *testing.T) {
	// Any successful run
	// Assert: stdout summary line contains "ms"
}
```

#### Impact on Existing Tests
- `TestCLIIntegration_successful_run` — checks `Contains(stdout, "1 passed")`. The new summary format `"1 passed, 0 failed (Xms)"` still contains `"1 passed"`. **No breakage.**
- `TestCLIIntegration_network_error` — checks exit code 4. Still returns 4 on failure. **No breakage.**
- `TestCLIIntegration` table — checks substrings and exit codes. **No breakage.**

---

### Step 5: Update smoke test

**Rationale:** Ensures the new capability is exercised end-to-end with the real binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add test for multi-request collection and empty collection |

#### New Code

Add after the existing `"Running sample collection"` block:

```bash
echo "--- Running with empty requests collection (expect warning) ---"
cat > /tmp/curlew_empty.yaml << 'YAML'
name: Empty Collection
requests: []
YAML
./curlew run /tmp/curlew_empty.yaml 2>&1 || true
echo
```

#### Impact on Existing Tests
- None. Additive only.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | `TestParseFile` (existing cases) | none | Options zero value matches |
| `internal/output/terminal_test.go` | All existing | none | New functions only |
| `cmd/curlew/main_test.go` | `TestCLIIntegration_successful_run` | none | `"1 passed"` substring still present |
| `cmd/curlew/main_test.go` | `TestCLIIntegration_network_error` | none | Exit code 4 unchanged |
| `cmd/curlew/main_test.go` | `TestCLIIntegration` table | none | All substring checks still pass |

## Risks and Edge Cases

- **Risk:** Exit code semantics — currently exit 4 for any request failure. Spec distinguishes exit 1 (assertion) vs exit 4 (network). Since assertions don't exist yet (M1-004+), keeping exit 4 for all failures is acceptable. → **Mitigation:** Document as known simplification; revisit when assertions land.
- **Risk:** `reflect.DeepEqual` in parser tests with new `Options` field → **Mitigation:** Go zero-values match; no test breakage.
- **Risk:** Duration timing is wall-clock (includes overhead between requests) → **Mitigation:** This is correct for "total execution time"; not sum of request durations.
- **Edge case:** Empty requests array → **Handling:** Print warning to stderr, return exit 0 before entering runner.
- **Edge case:** `stop_on_failure: true` + first request fails → **Handling:** 1 failed + (N-1) skipped, correctly counted.
- **Edge case:** Context cancellation mid-run → **Handling:** Check `ctx.Err()` before each request; remaining are skipped.
- **Edge case:** All requests fail with `stop_on_failure: false` → **Handling:** All counted as failed, 0 passed, 0 skipped.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create a 3-request collection and run it:
cat > /tmp/multi.yaml << 'YAML'
name: Multi Request Test
requests:
  - name: Request One
    request:
      method: GET
      url: "https://httpbin.org/get"
  - name: Request Two
    request:
      method: GET
      url: "https://httpbin.org/status/200"
  - name: Request Three
    request:
      method: GET
      url: "https://httpbin.org/ip"
YAML

./curlew run /tmp/multi.yaml
# Expected: per-request output + "3 request(s): 3 passed, 0 failed (Xms)"
```
