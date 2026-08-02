# Implementation Plan: M1-020

## Overview
Add `--format json` flag to the `run` command. Output is a single JSON document containing the collection name, overall status, duration, and a requests array with per-request name, method, URL, status code, duration, and assertions. All errors are embedded in the JSON structure rather than written to stderr.

## Task Details
- **ID:** M1-020
- **Title:** JSON output format (--format json)
- **Phase:** M1: Core CLI
- **Priority:** 20
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-019 | Terminal colors and formatting | done |

---

## Implementation Steps

### Step 1: Define JSON output types and `WriteJSON` in `internal/output/json.go`

**Rationale:** Smallest blast radius — a new file with no calls to existing code. Enables TDD to start immediately with pure unit tests.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | create | JSON structs (JSONOutput, JSONRequest, JSONAssertion, JSONError) and WriteJSON |
| `internal/output/json_test.go` | create | Unit tests for struct marshaling, empty arrays, field presence |

#### New Code

```go
// internal/output/json.go
package output

import (
	"encoding/json"
	"io"
)

// JSONOutput is the top-level JSON output structure for --format json.
type JSONOutput struct {
	Name       string        `json:"name"`
	Status     string        `json:"status"`
	DurationMs int64         `json:"duration_ms"`
	Requests   []JSONRequest `json:"requests"`
	Errors     []JSONError   `json:"errors,omitempty"`
}

// JSONRequest represents one request result in JSON output.
type JSONRequest struct {
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	Method     string          `json:"method"`
	URL        string          `json:"url"`
	StatusCode int             `json:"status_code,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	Assertions []JSONAssertion `json:"assertions"` // never omitempty — always []
	Error      *JSONError      `json:"error,omitempty"`
}

// JSONAssertion represents one assertion outcome in JSON output.
type JSONAssertion struct {
	Type     string `json:"type"`
	Operator string `json:"operator"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
}

// JSONError represents an error in JSON output.
type JSONError struct {
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// WriteJSON serializes out as indented JSON to w.
func WriteJSON(w io.Writer, out *JSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/output/json_test.go
func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name   string
		input  *JSONOutput
		check  func(t *testing.T, data []byte)
	}{
		{
			name: "valid JSON output",
			input: &JSONOutput{
				Name:       "My Suite",
				Status:     "passed",
				DurationMs: 123,
				Requests:   []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				if !json.Valid(data) {
					t.Fatalf("not valid JSON: %s", data)
				}
			},
		},
		{
			name: "empty assertions array not null",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "passed",
				Requests: []JSONRequest{
					{
						Name:       "req1",
						Status:     "passed",
						Method:     "GET",
						URL:        "https://example.com",
						Assertions: []JSONAssertion{}, // explicitly empty
					},
				},
			},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"assertions": []`)) {
					t.Fatalf("expected empty assertions array, got: %s", data)
				}
			},
		},
		{
			name: "all top-level fields present",
			input: &JSONOutput{
				Name:       "Suite",
				Status:     "passed",
				DurationMs: 500,
				Requests:   []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				for _, field := range []string{`"name"`, `"status"`, `"duration_ms"`, `"requests"`} {
					if !bytes.Contains(data, []byte(field)) {
						t.Errorf("missing field %s in: %s", field, data)
					}
				}
			},
		},
		{
			name: "assertion failure fields present",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "failed",
				Requests: []JSONRequest{
					{
						Name:   "req",
						Status: "failed",
						Method: "GET",
						URL:    "https://example.com",
						Assertions: []JSONAssertion{
							{
								Type:     "status",
								Operator: "equals",
								Expected: "200",
								Actual:   "404",
								Passed:   false,
							},
						},
					},
				},
			},
			check: func(t *testing.T, data []byte) {
				for _, field := range []string{`"expected"`, `"actual"`, `"operator"`, `"passed"`} {
					if !bytes.Contains(data, []byte(field)) {
						t.Errorf("missing assertion field %s", field)
					}
				}
				if bytes.Contains(data, []byte(`"passed": true`)) {
					t.Error("expected passed:false, got true")
				}
			},
		},
		{
			name: "error with hint",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "error",
				Errors: []JSONError{{Message: "connection refused", Hint: "check server"}},
				Requests: []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"errors"`)) {
					t.Fatal("missing errors field")
				}
				if !bytes.Contains(data, []byte(`"hint"`)) {
					t.Fatal("missing hint field")
				}
			},
		},
		{
			name: "no extraneous text — starts with { ends with newline",
			input: &JSONOutput{
				Name:     "Suite",
				Status:   "passed",
				Requests: []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				trimmed := bytes.TrimSpace(data)
				if !bytes.HasPrefix(trimmed, []byte("{")) {
					t.Fatalf("expected output to start with '{', got: %s", data[:10])
				}
				if !bytes.HasSuffix(trimmed, []byte("}")) {
					t.Fatalf("expected output to end with '}', got tail: %s", data[len(data)-10:])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tc.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			tc.check(t, buf.Bytes())
		})
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — new file only.

---

### Step 2: Add `Method` and `URL` fields to `runner.RequestResult`

**Rationale:** The JSON output needs method and URL per request. Adding fields to `RequestResult` is backward-compatible (zero values for untouched code paths). Must happen before CLI wiring.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add Method, URL string fields to RequestResult; populate in executePhase |
| `internal/runner/runner_test.go` | modify | Add test cases verifying Method/URL populated |

#### Current Code (`runner.go:30-37`)

```go
// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
	Name             string
	Phase            Phase // empty string treated as PhaseMain for backward compat
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	AssertionResults *assertion.Results
}
```

#### New Code

```go
// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
	Name             string
	Phase            Phase // empty string treated as PhaseMain for backward compat
	Method           string           // HTTP method (after interpolation); empty for skipped
	URL              string           // Full URL (after interpolation); empty for skipped
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	AssertionResults *assertion.Results
}
```

Populate in `executePhase` — three sites need updating:

**Site 1 — error result (runner.go:208)**

```go
// Current:
results = append(results, RequestResult{Name: item.Name, Phase: phase, Err: execErr})

// New:
results = append(results, RequestResult{Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL, Err: execErr})
```

**Site 2 — assertion result (runner.go:228)**

```go
// Current:
rr := RequestResult{Name: item.Name, Phase: phase, Result: result, AssertionResults: ar}

// New:
rr := RequestResult{Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL, Result: result, AssertionResults: ar}
```

**Site 3 — skipped due to stopped/context (runner.go:171, 177)**

These remain as-is (skipped requests have no interpolated URL). Method/URL stay zero values.

Also add Method/URL when setup skips main phase (runner.go:122-123):

```go
// Current (inside setupFailed branch):
all = append(all, RequestResult{Name: item.Name, Phase: PhaseMain, Skipped: true})

// New:
all = append(all, RequestResult{Name: item.Name, Phase: PhaseMain, Method: item.Request.Method, URL: item.Request.URL, Skipped: true})
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunPopulatesMethodAndURL(t *testing.T) {
	col := &parser.Collection{
		Name: "Test",
		Requests: []parser.RequestItem{
			{
				Name:    "Get Test",
				Request: parser.Request{Method: "GET", URL: "https://example.com/path"},
			},
		},
	}
	mockExec := func(_ context.Context, req *parser.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200}, nil
	}
	results, _, err := Run(context.Background(), col, mockExec, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Method != "GET" {
		t.Errorf("expected Method=GET, got %q", results[0].Method)
	}
	if results[0].URL != "https://example.com/path" {
		t.Errorf("expected URL=https://example.com/path, got %q", results[0].URL)
	}
}

func TestRunPopulatesMethodAndURL_onError(t *testing.T) {
	// When exec returns an error, Method/URL should still be populated.
}

func TestRunPopulatesMethodAndURL_skipped_in_setup_fail(t *testing.T) {
	// When setup fails and main is skipped, Method/URL come from parsed item.
}
```

#### Impact on Existing Tests
- All existing runner tests compile and pass unchanged (new fields are zero-valued in existing test assertions that don't reference Method/URL).

---

### Step 3: Add `--format` flag parsing to `parseRunArgs`

**Rationale:** Enables the CLI integration before wiring the output path. Third because it depends on nothing and can be tested in isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add format return value to parseRunArgs, add --format case |
| `cmd/apitest/main_test.go` | modify | Add --format test cases to existing TestParseRunArgs table |

#### Current Code (`main.go:49`)

```go
func parseRunArgs(args []string) (file, envName string, vars, envVarVars map[string]string, seed *int64, noColor bool, err error) {
```

#### New Code

```go
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, err error) {
```

Add `--format` case in the switch block (after `--seed`):

```go
case "--format":
    i++
    if i >= len(args) {
        return "", "", "", nil, nil, nil, false, fmt.Errorf("--format requires a value (e.g. --format json)")
    }
    format = args[i]
```

Update `runCmd` caller at `main.go:116`:

```go
// Current:
file, envName, cliVars, envVarVars, seed, noColor, parseErr := parseRunArgs(args)

// New:
file, envName, format, cliVars, envVarVars, seed, noColor, parseErr := parseRunArgs(args)
```

#### Tests to Write FIRST (RED phase)

```go
// In TestParseRunArgs table:
{"format json", []string{"f.yaml", "--format", "json"}, "f.yaml", "", "json", nil, false, false},
{"no format defaults empty", []string{"f.yaml"}, "f.yaml", "", "", nil, false, false},
{"format without value returns error", []string{"f.yaml", "--format"}, "", "", "", nil, false, true /* wantErr */},
```

#### Impact on Existing Tests
- All existing `TestParseRunArgs` cases need a new `format` field in the expected struct — mechanical update (add `""` to all passing cases).

---

### Step 4: Wire JSON output into `runCmd` — core path

**Rationale:** This is the main integration step. Steps 1–3 provide all the building blocks.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add buildJSONOutput helper, branch on format in runCmd |

#### New Code (added to `main.go`)

```go
// buildJSONOutput constructs a JSONOutput from runner results and summary.
// preExecErr is non-nil for errors that occurred before runner.Run was called.
func buildJSONOutput(name string, results []runner.RequestResult, summary *runner.Summary, preExecErr error) *output.JSONOutput {
	out := &output.JSONOutput{
		Name:     name,
		Requests: make([]output.JSONRequest, 0),
	}

	if summary != nil {
		out.DurationMs = summary.Duration.Milliseconds()
	}

	if preExecErr != nil {
		out.Status = "error"
		out.Errors = []output.JSONError{{Message: preExecErr.Error()}}
		return out
	}

	for _, r := range results {
		jr := output.JSONRequest{
			Name:       r.Name,
			Method:     r.Method,
			URL:        r.URL,
			Assertions: make([]output.JSONAssertion, 0),
		}

		switch {
		case r.Skipped:
			jr.Status = "skipped"
		case r.Err != nil:
			jr.Status = "error"
			jr.Error = &output.JSONError{Message: r.Err.Error()}
		default:
			if r.Result != nil {
				jr.StatusCode = r.Result.StatusCode
				jr.DurationMs = r.Result.Duration.Milliseconds()
			}
			if r.AssertionResults != nil {
				for _, ar := range r.AssertionResults.Items {
					jr.Assertions = append(jr.Assertions, output.JSONAssertion{
						Type:     ar.Type,
						Operator: extractJSONOperator(ar.Type),
						Expected: ar.Expected,
						Actual:   ar.Actual,
						Passed:   ar.Passed,
					})
				}
				if r.AssertionResults.Passed {
					jr.Status = "passed"
				} else {
					jr.Status = "failed"
				}
			} else {
				jr.Status = "passed"
			}
		}

		out.Requests = append(out.Requests, jr)
	}

	if summary != nil {
		mainFailed := summary.Failed - summary.TeardownErrors
		if mainFailed > 0 {
			out.Status = "failed"
		} else {
			out.Status = "passed"
		}
	}

	return out
}

// extractJSONOperator extracts the operator keyword from an assertion type string.
// The Type field is structured as e.g. "status", "body $.id equals", "header X equals".
// For single-word types (status, timing), the type itself is returned.
// For multi-word types, the last word is the operator.
func extractJSONOperator(assertionType string) string {
	parts := strings.Fields(assertionType)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[len(parts)-1]
}
```

In `runCmd`, replace terminal output path with a format branch:

```go
func runCmd(args []string) int {
	file, envName, format, cliVars, envVarVars, seed, noColor, parseErr := parseRunArgs(args)
	if parseErr != nil {
		if format == "json" {
			out := buildJSONOutput("", nil, &runner.Summary{}, parseErr)
			_ = output.WriteJSON(os.Stdout, out)
			return 1
		}
		_, _ = fmt.Fprintln(os.Stderr, "Usage: apitest run <collection-file> [--env <name>] [--env-var VAR ...] [--var key=value ...] [--seed <number>] [--format <type>] [--no-color]")
		errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
		errOut.StructuredError(parseErr)
		return 1
	}

	// ... (existing parse/load code unchanged) ...

	// At runner.Run call, collect results
	results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{...})

	if format == "json" {
		out := buildJSONOutput(col.Name, results, summary, varErr)
		if err := output.WriteJSON(os.Stdout, out); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
			return 1
		}
		// Exit codes unchanged — same logic as terminal mode
		if summary != nil {
			mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
			mainFailed := summary.Failed - summary.TeardownErrors
			if varErr != nil {
				return 5
			}
			if mainAssertionFailed > 0 {
				return 1
			}
			if mainFailed > 0 {
				return 4
			}
		}
		return 0
	}

	// Existing terminal output code unchanged below ...
}
```

Note: Pre-parse errors (parse file, load env, load project config) must also redirect to JSON in format=="json" mode. Each early `errOut.StructuredError(err); return N` gets a JSON branch:

```go
if format == "json" {
    out := buildJSONOutput(col.Name, nil, &runner.Summary{}, err)
    _ = output.WriteJSON(os.Stdout, out)
    return N
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestBuildJSONOutput(t *testing.T) {
	tests := []struct {
		name      string
		colName   string
		results   []runner.RequestResult
		summary   *runner.Summary
		preErr    error
		wantStatus string
	}{
		{"all passed", "Suite", []runner.RequestResult{{Name: "r", Method: "GET", URL: "u", Result: &httpexec.Result{StatusCode: 200}}}, &runner.Summary{Passed: 1}, nil, "passed"},
		{"assertion failure", "Suite", []runner.RequestResult{{Name: "r", Method: "GET", URL: "u", Result: &httpexec.Result{StatusCode: 404}, AssertionResults: &assertion.Results{Items: []assertion.Result{{Passed: false}}, Passed: false}}}, &runner.Summary{Failed: 1, AssertionFailures: 1}, nil, "failed"},
		{"skipped request", "Suite", []runner.RequestResult{{Name: "r", Skipped: true}}, &runner.Summary{Skipped: 1}, nil, "passed"},
		{"pre-exec error", "Suite", nil, &runner.Summary{}, errors.New("boom"), "error"},
		{"network error in request", "Suite", []runner.RequestResult{{Name: "r", Method: "GET", URL: "u", Err: errors.New("refused")}}, &runner.Summary{Failed: 1}, nil, "failed"},
	}
	// ...
}

func TestExtractJSONOperator(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"status", "status"},
		{"timing", "timing"},
		{"body $.id equals", "equals"},
		{"body $.url contains", "contains"},
		{"header Content-Type matches", "matches"},
		{"", ""},
	}
	// ...
}
```

#### Impact on Existing Tests
- `TestRunCmd` and integration tests: no breakage if they don't pass `--format json`. Format defaults to `""` (terminal).
- `TestParseRunArgs`: all existing cases must add `""` as the format expected value.

---

### Step 5: Handle pre-execution errors in JSON mode

**Rationale:** Behavior 5 — errors must appear in JSON, not stderr. This is a localized change to each early-return path in `runCmd`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Gate each early-return error on format=="json" and emit JSON error instead |

Error paths that need JSON branches (with their exit codes):
1. `parser.ParseFile` fails → exit 3
2. `config.LoadEnvironment` fails → exit 3
3. `config.LoadProjectConfig` fails → exit 3
4. `config.LoadDotenv` fails → exit 3
5. `runner.Run` returns varErr → exit 5

Each becomes:

```go
if err != nil {
    if format == "json" {
        jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, err)
        _ = output.WriteJSON(os.Stdout, jsonOut)
        return 3 // or 5
    }
    errOut.StructuredError(err)
    return 3 // or 5
}
```

For parse errors where `col.Name` is not yet known, pass `""` as name.

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_JSONMode_ParseError(t *testing.T) {
	// Run with --format json and a nonexistent file
	// Expect stdout to be valid JSON with status: "error"
	// Expect stderr to be empty
}

func TestRunCmd_JSONMode_VarError(t *testing.T) {
	// Run with --format json and circular variables
	// Expect stdout JSON with status: "error", exit 5
	// Expect stderr empty
}
```

---

### Step 6: Update help text and usage string

**Rationale:** Completeness contract — help text must reflect all supported flags.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add --format to printHelp() and usage string in runCmd |

#### Current Code (`main.go:240-245`)

```go
fmt.Println("Run Options:")
fmt.Println("  --env <name>        Load environment file (from environments/<name>.yaml)")
fmt.Println("  --env-var VAR_NAME  Import OS environment variable (repeatable)")
fmt.Println("  --env-var VAR=$OS   Import and rename OS environment variable")
fmt.Println("  --var key=value     Set a variable (overrides all other sources, repeatable)")
fmt.Println("  --seed <number>     Seed for deterministic random variable functions")
fmt.Println("  --no-color          Disable colored output (also respects NO_COLOR env var)")
```

#### New Code

```go
fmt.Println("Run Options:")
fmt.Println("  --env <name>        Load environment file (from environments/<name>.yaml)")
fmt.Println("  --env-var VAR_NAME  Import OS environment variable (repeatable)")
fmt.Println("  --env-var VAR=$OS   Import and rename OS environment variable")
fmt.Println("  --var key=value     Set a variable (overrides all other sources, repeatable)")
fmt.Println("  --seed <number>     Seed for deterministic random variable functions")
fmt.Println("  --format <type>     Output format: terminal (default), json")
fmt.Println("  --no-color          Disable colored output (also respects NO_COLOR env var)")
```

Also update usage string in `runCmd` to include `[--format <type>]`.

#### Tests to Write FIRST (RED phase)

```go
func TestHelp_contains_format(t *testing.T) {
	// Capture printHelp output, verify it contains "--format"
}
```

---

### Step 7: Add binary integration tests

**Rationale:** End-to-end verification using the real binary (following the existing test pattern in `main_test.go`).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main_test.go` | modify | Add --format json integration test cases |

#### Tests to Write FIRST (RED phase)

```go
// Table-driven integration tests using the real binary:
func TestCLIIntegration_JSONFormat(t *testing.T) {
	tests := []struct {
		name           string
		collectionYAML string
		args           []string
		wantStatus     string
		wantExit       int
		checkRequests  func(t *testing.T, requests []map[string]any)
	}{
		{
			name: "valid JSON output — all fields present",
			collectionYAML: `
name: JSON Suite
requests:
  - name: Get Test
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 200
`,
			args:       []string{"--format", "json"},
			wantStatus: "passed",
			wantExit:   0,
			checkRequests: func(t *testing.T, requests []map[string]any) {
				if len(requests) != 1 {
					t.Fatalf("expected 1 request, got %d", len(requests))
				}
				r := requests[0]
				for _, field := range []string{"name", "method", "url", "status_code", "duration_ms", "assertions"} {
					if _, ok := r[field]; !ok {
						t.Errorf("missing field %q", field)
					}
				}
				if r["method"] != "GET" {
					t.Errorf("expected method GET, got %v", r["method"])
				}
			},
		},
		{
			name: "empty assertions array not omitted",
			// collection with no assertions
			collectionYAML: `name: Suite\nrequests:\n  - name: R\n    request:\n      method: GET\n      url: "https://httpbin.org/get"`,
			args: []string{"--format", "json"},
			checkRequests: func(t *testing.T, requests []map[string]any) {
				assertions, ok := requests[0]["assertions"]
				if !ok {
					t.Fatal("assertions field missing")
				}
				list, ok := assertions.([]any)
				if !ok || list == nil {
					t.Fatal("assertions should be [] not null")
				}
			},
		},
		{
			name: "failing assertion in JSON",
			// status: 404 but returns 200
			wantStatus: "failed",
			wantExit:   1,
			checkRequests: func(t *testing.T, requests []map[string]any) {
				assertions := requests[0]["assertions"].([]any)
				if len(assertions) == 0 {
					t.Fatal("expected assertions")
				}
				a := assertions[0].(map[string]any)
				if a["passed"] != false {
					t.Error("expected passed:false")
				}
				for _, field := range []string{"expected", "actual", "operator"} {
					if _, ok := a[field]; !ok {
						t.Errorf("missing field %q", field)
					}
				}
			},
		},
		{
			name:       "no extraneous text in JSON output",
			wantExit:   0,
			args:       []string{"--format", "json"},
			checkRequests: nil, // checked at outer level
		},
		{
			name:     "parse error appears in JSON, not stderr",
			// nonexistent file with --format json
			wantStatus: "error",
			wantExit:   3,
			// stderr should be empty
		},
		{
			name:     "exit codes unchanged with --format json",
			wantExit: 0,
		},
	}
	// ...
}
```

#### Impact on Existing Tests
- None — new test functions only.

---

### Step 8: Update smoke test

**Rationale:** Completeness contract — smoke test must cover new capability.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add --format json scenarios |

#### New Smoke Sections

```bash
echo "--- Running with --format json (expect valid JSON) ---"
JSON_FILE=$(mktemp /tmp/apitest_json_XXXXXX.yaml)
cat > "$JSON_FILE" << 'YAML'
name: JSON Format Smoke
requests:
  - name: JSON Get
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 200
YAML
JSON_OUTPUT=$(./apitest run "$JSON_FILE" --format json)
echo "$JSON_OUTPUT" | python3 -m json.tool > /dev/null && echo "PASS: --format json produces valid JSON" || { echo "FAIL: Invalid JSON output: $JSON_OUTPUT"; exit 1; }
echo "$JSON_OUTPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'name' in d and 'status' in d and 'requests' in d" && echo "PASS: top-level fields present" || { echo "FAIL: Missing fields"; exit 1; }
rm -f "$JSON_FILE"
echo

echo "--- --format json with failing assertion (expect JSON with failed status) ---"
JSON_FAIL_FILE=$(mktemp /tmp/apitest_json_fail_XXXXXX.yaml)
cat > "$JSON_FAIL_FILE" << 'YAML'
name: JSON Fail Smoke
requests:
  - name: Expect 404
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 404
YAML
JSON_FAIL_OUTPUT=$(./apitest run "$JSON_FAIL_FILE" --format json || true)
echo "$JSON_FAIL_OUTPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['status']=='failed'" && echo "PASS: --format json failed status" || { echo "FAIL: Wrong status in: $JSON_FAIL_OUTPUT"; exit 1; }
rm -f "$JSON_FAIL_FILE"
echo

echo "--- --format json with no assertions (assertions array not null) ---"
JSON_NO_ASSERT_FILE=$(mktemp /tmp/apitest_json_no_assert_XXXXXX.yaml)
cat > "$JSON_NO_ASSERT_FILE" << 'YAML'
name: No Assert Smoke
requests:
  - name: No Assert
    request:
      method: GET
      url: "https://httpbin.org/get"
YAML
JSON_NO_ASSERT_OUTPUT=$(./apitest run "$JSON_NO_ASSERT_FILE" --format json)
echo "$JSON_NO_ASSERT_OUTPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); r=d['requests'][0]; assert isinstance(r['assertions'], list)" && echo "PASS: assertions is array not null" || { echo "FAIL: assertions not array"; exit 1; }
rm -f "$JSON_NO_ASSERT_FILE"
echo

echo "--- Help text shows --format ---"
HELP_OUTPUT=$(./apitest --help)
echo "$HELP_OUTPUT" | grep -q "\-\-format" && echo "PASS: --format in help" || { echo "FAIL: Missing --format in help output"; exit 1; }
echo
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/apitest/main_test.go` | All `TestParseRunArgs` cases | breaks | Add `""` format field to each expected result |
| `internal/runner/runner_test.go` | All `TestRun*` cases | none — fields are additive | No action needed |
| `internal/output/*_test.go` | All existing | none — new file | No action needed |

---

## Risks and Edge Cases

- **Risk: `assertions` serializes as `null` instead of `[]`** → **Mitigation:** Always initialize with `make([]output.JSONAssertion, 0)`, never `nil`. Unit test explicitly verifies `"assertions": []`.

- **Risk: Extraneous text in JSON output (ANSI codes, header lines)** → **Mitigation:** In JSON mode, the terminal `Printer` is never created for stdout. `out.CollectionHeader` call is gated on `format != "json"`. All writes to stdout go through `WriteJSON` only.

- **Risk: `parseRunArgs` signature change breaks callers** → **Mitigation:** Only `runCmd` calls `parseRunArgs`. Update both `runCmd` and all `TestParseRunArgs` test structs.

- **Risk: `operator` extraction breaks if `assertion.Result.Type` format changes** → **Mitigation:** `extractJSONOperator` is unit-tested against all known Type formats: `"status"`, `"timing"`, `"body $.path equals"`, `"header Name matches"`. If Type format changes upstream, the unit test will catch it.

- **Risk: Exit codes change in JSON mode** → **Mitigation:** The exit code computation (mainFailed/mainAssertionFailed) runs identically in JSON mode. Integration tests verify `--format json` exit codes match non-JSON mode.

- **Edge case: Empty collection with `--format json`** → JSON output: `{"name":"...","status":"passed","duration_ms":0,"requests":[]}`. The current early-return-with-warning path must output JSON in format=="json" mode.

- **Edge case: Method/URL for skipped requests** → Skipped requests that were stopped mid-phase have no interpolated URL. Method/URL come from the parsed `item.Request` for setup-induced skips; they are empty strings for context-cancelled/stopped skips. This is acceptable — `"method": ""` in JSON for skipped items.

- **Edge case: `--format unknown`** → `parseRunArgs` stores the value; `runCmd` should validate and return an error if unknown format is specified. Add a validation check after parse.

---

## Verification

```bash
# Build
go build ./cmd/apitest

# All tests
go test ./...

# Targeted package tests
go test -v ./internal/output/...
go test -v ./internal/runner/...
go test -v ./cmd/apitest/...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Lint
~/go/bin/golangci-lint run

# Smoke test
./smoke/run.sh
```

Observable verification:
```bash
./apitest run collection.yaml --format json | jq .
# Verify valid JSON with all fields: name, status, duration_ms, requests[].{name,method,url,status_code,duration_ms,assertions}
```
