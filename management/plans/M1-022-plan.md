# Implementation Plan: M1-022

## Overview
Add verbosity level flags (`-v`, `-vv`, `-q`) to the CLI. `-v` shows request/response headers, `-vv` adds full body dumps, and `-q` (quiet) suppresses all per-request output showing only the summary line.

## Task Details
- **ID:** M1-022
- **Title:** Verbosity levels (-v, -vv, -q)
- **Phase:** M1: Core CLI
- **Priority:** 22
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-019 | Terminal colors and formatting | done |

## Implementation Steps

### Step 1: Define `Verbosity` Type in `internal/output/verbosity.go`
**Rationale:** Smallest blast radius — pure new file, zero impact on existing code. Defines the vocabulary used by all subsequent steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/verbosity.go` | create | Verbosity type and constants |
| `internal/output/verbosity_test.go` | create | Unit tests for Verbosity |

#### New Code
```go
// internal/output/verbosity.go
package output

// Verbosity controls how much detail is printed per request.
type Verbosity int

const (
    VerbosityQuiet   Verbosity = -1 // -q: summary line only
    VerbosityDefault Verbosity = 0  // no flag: name, status, assertions
    VerbosityVerbose Verbosity = 1  // -v: + request/response headers
    VerbosityDebug   Verbosity = 2  // -vv: + full body dump
)

func (v Verbosity) String() string {
    switch v {
    case VerbosityQuiet:
        return "quiet"
    case VerbosityDefault:
        return "default"
    case VerbosityVerbose:
        return "verbose"
    case VerbosityDebug:
        return "debug"
    default:
        return "unknown"
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestVerbosity(t *testing.T) {
    tests := []struct {
        name     string
        v        Verbosity
        wantStr  string
    }{
        {"quiet string", VerbosityQuiet, "quiet"},
        {"default string", VerbosityDefault, "default"},
        {"verbose string", VerbosityVerbose, "verbose"},
        {"debug string", VerbosityDebug, "debug"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := tt.v.String(); got != tt.wantStr {
                t.Errorf("got %q, want %q", got, tt.wantStr)
            }
        })
    }
}

func TestVerbosityOrdering(t *testing.T) {
    if !(VerbosityQuiet < VerbosityDefault) {
        t.Error("quiet must be less than default")
    }
    if !(VerbosityDefault < VerbosityVerbose) {
        t.Error("default must be less than verbose")
    }
    if !(VerbosityVerbose < VerbosityDebug) {
        t.Error("verbose must be less than debug")
    }
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 2: Add Verbosity to `runner.RequestResult`
**Rationale:** The runner must capture request headers and body during execution so verbose output has the data to display. Additive struct change — no existing code breaks.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `RequestHeaders` and `RequestBody` to `RequestResult`; populate in `executePhase` |
| `internal/runner/runner_test.go` | modify | Add tests verifying new fields are populated |

#### Current Code
```go
// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
	Name             string
	Phase            Phase
	Method           string
	URL              string
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	AssertionResults *assertion.Results
}
```

```go
// line ~210 (error path in executePhase)
results = append(results, RequestResult{Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL, Err: execErr})

// line ~230 (success path)
rr := RequestResult{Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL, Result: result, AssertionResults: ar}
```

#### New Code
```go
type RequestResult struct {
	Name             string
	Phase            Phase
	Method           string
	URL              string
	RequestHeaders   map[string]string // interpolated request headers (for -v/-vv output)
	RequestBody      any               // interpolated request body (for -vv output)
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	AssertionResults *assertion.Results
}
```

```go
// error path
results = append(results, RequestResult{
    Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
    RequestHeaders: req.Headers, RequestBody: req.Body,
    Err: execErr,
})

// success path
rr := RequestResult{
    Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
    RequestHeaders: req.Headers, RequestBody: req.Body,
    Result: result, AssertionResults: ar,
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunPopulatesRequestHeadersAndBody(t *testing.T) {
    // Use existing test fixture with headers set on a request
    // Verify RequestResult.RequestHeaders is populated after Run
    // Verify RequestResult.RequestBody is populated after Run
}
```

#### Impact on Existing Tests
- `internal/runner/runner_test.go` — no tests check for absence of these fields; purely additive. All existing tests pass.

---

### Step 3: Extend `Printer` with Verbosity Support
**Rationale:** Core output change. Uses variadic parameter to preserve backward compatibility; existing callers (`NewPrinter(w, color)`) continue to compile and behave identically.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `verbosity` field, update `NewPrinter`, gate existing methods, add verbose methods |
| `internal/output/terminal_test.go` | modify | Add verbosity-specific test cases |

#### Current Code
```go
type Printer struct {
	w     io.Writer
	color bool
}

func NewPrinter(w io.Writer, color bool) *Printer {
	return &Printer{w: w, color: color}
}
```

#### New Code
```go
type Printer struct {
	w         io.Writer
	color     bool
	verbosity Verbosity
}

// NewPrinter creates a Printer. Pass VerbosityVerbose or VerbosityDebug for
// verbose output, VerbosityQuiet for quiet mode.
func NewPrinter(w io.Writer, color bool, verbosity ...Verbosity) *Printer {
	v := VerbosityDefault
	if len(verbosity) > 0 {
		v = verbosity[0]
	}
	return &Printer{w: w, color: color, verbosity: v}
}
```

Gate existing methods to suppress at quiet:
```go
// CollectionHeader — suppress at quiet
func (p *Printer) CollectionHeader(name string) {
    if p.verbosity <= VerbosityQuiet {
        return
    }
    // ... existing code
}

// SectionHeader — suppress at quiet
func (p *Printer) SectionHeader(label string) {
    if p.verbosity <= VerbosityQuiet {
        return
    }
    // ... existing code
}

// Result — suppress at quiet
func (p *Printer) Result(name string, result *httpexec.Result, passed bool) {
    if p.verbosity <= VerbosityQuiet {
        return
    }
    // ... existing code
}

// AssertionDetail — suppress at quiet
func (p *Printer) AssertionDetail(kind, expected, actual string) {
    if p.verbosity <= VerbosityQuiet {
        return
    }
    // ... existing code
}

// Skipped — suppress at quiet
func (p *Printer) Skipped(name string) {
    if p.verbosity <= VerbosityQuiet {
        return
    }
    // ... existing code
}

// SummaryWithDuration — always shown (even at quiet)
// RequestError — always shown (even at quiet); errors must be visible
```

New verbose output methods:
```go
// RequestDetail writes request method, URL, and headers (shown at -v and -vv).
func (p *Printer) RequestDetail(method, url string, headers map[string]string) {
    if p.verbosity < VerbosityVerbose {
        return
    }
    fmt.Fprintf(p.w, "  > %s %s\n", method, url)
    for k, v := range headers {
        fmt.Fprintf(p.w, "  > %s: %s\n", k, v)
    }
}

// ResponseDetail writes response status and headers (shown at -v and -vv).
func (p *Printer) ResponseDetail(statusCode int, headers http.Header) {
    if p.verbosity < VerbosityVerbose {
        return
    }
    fmt.Fprintf(p.w, "  < %d\n", statusCode)
    for k, vs := range headers {
        for _, v := range vs {
            fmt.Fprintf(p.w, "  < %s: %s\n", k, v)
        }
    }
}

// RequestBodyDump writes full request body (shown at -vv only).
func (p *Printer) RequestBodyDump(body any) {
    if p.verbosity < VerbosityDebug || body == nil {
        return
    }
    fmt.Fprintf(p.w, "  > (body): %v\n", body)
}

// ResponseBodyDump writes full response body (shown at -vv only).
// Truncates at 10KB with indicator.
func (p *Printer) ResponseBodyDump(body []byte) {
    if p.verbosity < VerbosityDebug || len(body) == 0 {
        return
    }
    const maxBytes = 10 * 1024
    if len(body) > maxBytes {
        fmt.Fprintf(p.w, "  < (body, truncated): %s\n  < [%d bytes total]\n", body[:maxBytes], len(body))
    } else {
        fmt.Fprintf(p.w, "  < (body): %s\n", body)
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrinterVerbosityQuiet(t *testing.T) {
    tests := []struct {
        name    string
        action  func(p *Printer)
        wantOut string
    }{
        {"quiet_suppresses_collection_header", func(p *Printer) { p.CollectionHeader("suite") }, ""},
        {"quiet_suppresses_section_header", func(p *Printer) { p.SectionHeader("setup") }, ""},
        {"quiet_suppresses_result", func(p *Printer) {
            p.Result("req", &httpexec.Result{StatusCode: 200}, true)
        }, ""},
        {"quiet_suppresses_assertion_detail", func(p *Printer) {
            p.AssertionDetail("status", "200", "404")
        }, ""},
        {"quiet_suppresses_skipped", func(p *Printer) { p.Skipped("req") }, ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var buf bytes.Buffer
            p := NewPrinter(&buf, false, VerbosityQuiet)
            tt.action(p)
            if got := buf.String(); got != tt.wantOut {
                t.Errorf("got %q, want %q", got, tt.wantOut)
            }
        })
    }
}

func TestPrinterVerbosityQuiet_SummaryStillShown(t *testing.T) {
    var buf bytes.Buffer
    p := NewPrinter(&buf, false, VerbosityQuiet)
    p.SummaryWithDuration(1, 1, 0, 0, 50*time.Millisecond)
    if buf.Len() == 0 {
        t.Error("SummaryWithDuration should output even at quiet verbosity")
    }
}

func TestPrinterVerbosityVerbose_ShowsRequestDetail(t *testing.T) {
    var buf bytes.Buffer
    p := NewPrinter(&buf, false, VerbosityVerbose)
    p.RequestDetail("GET", "https://api.example.com/users", map[string]string{"Accept": "application/json"})
    out := buf.String()
    if !strings.Contains(out, "> GET https://api.example.com/users") {
        t.Errorf("expected request line, got: %q", out)
    }
    if !strings.Contains(out, "Accept: application/json") {
        t.Errorf("expected header line, got: %q", out)
    }
}

func TestPrinterVerbosityDefault_NoRequestDetail(t *testing.T) {
    var buf bytes.Buffer
    p := NewPrinter(&buf, false)
    p.RequestDetail("GET", "https://api.example.com", nil)
    if buf.Len() != 0 {
        t.Error("RequestDetail should be silent at default verbosity")
    }
}

func TestPrinterVerbosityDebug_ShowsResponseBody(t *testing.T) {
    var buf bytes.Buffer
    p := NewPrinter(&buf, false, VerbosityDebug)
    p.ResponseBodyDump([]byte(`{"id":1}`))
    if !strings.Contains(buf.String(), `{"id":1}`) {
        t.Errorf("expected response body, got: %q", buf.String())
    }
}

func TestPrinterVerbosityVerbose_NoResponseBody(t *testing.T) {
    var buf bytes.Buffer
    p := NewPrinter(&buf, false, VerbosityVerbose)
    p.ResponseBodyDump([]byte(`{"id":1}`))
    if buf.Len() != 0 {
        t.Error("ResponseBodyDump should be silent at verbose (not debug) verbosity")
    }
}
```

#### Impact on Existing Tests
- `internal/output/terminal_test.go` — existing `NewPrinter(&buf, tt.color)` calls compile unchanged (variadic). Existing test assertions pass because default verbosity == `VerbosityDefault`, identical to current behavior.

---

### Step 4: Parse `-v`, `-vv`, `-q` Flags in `parseRunArgs`
**Rationale:** Add verbosity to the CLI flag parser. This changes the return signature of `parseRunArgs`, which will require updating all test call sites mechanically.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add verbosity cases to `parseRunArgs` switch; update signature; update `runCmd` callers; update usage string |
| `cmd/apitest/main_test.go` | modify | Update all `parseRunArgs` destructuring to accept 9th return value; add verbosity test cases |

#### Current Code
```go
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, err error) {
```

```go
// In main_test.go (four call sites):
file, envName, _, vars, envVarVars, _, _, err := parseRunArgs(tt.args)
_, _, _, _, _, seed, _, err := parseRunArgs(tt.args)
_, _, _, _, _, _, noColor, err := parseRunArgs(tt.args)
_, _, format, _, _, _, _, err := parseRunArgs(tt.args)
```

#### New Code
```go
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, verbosity output.Verbosity, err error) {
    vars = make(map[string]string)
    envVarVars = make(map[string]string)
    var positional []string
    for i := 0; i < len(args); i++ {
        switch args[i] {
        // ... existing cases ...
        case "-vv":
            verbosity = output.VerbosityDebug
        case "-v":
            verbosity = output.VerbosityVerbose
        case "-q", "--quiet":
            verbosity = output.VerbosityQuiet
        // ... rest of existing cases ...
        }
    }
    // ...
}
```

Note: `-vv` must come before `-v` in the switch to avoid fall-through issues (though Go switch doesn't fall through by default, ordering is still clearer).

Updated `runCmd`:
```go
file, envName, format, cliVars, envVarVars, seed, noColor, verbosity, parseErr := parseRunArgs(args)
// ...
out = output.NewPrinter(os.Stdout, useColor, verbosity)
```

Updated usage string in `runCmd`:
```go
_, _ = fmt.Fprintln(os.Stderr, "Usage: apitest run <collection-file> [--env <name>] [--env-var VAR ...] [--var key=value ...] [--seed <number>] [--format <type>] [--no-color] [-v] [-vv] [-q]")
```

Updated test call sites (mechanical):
```go
file, envName, _, vars, envVarVars, _, _, _, err := parseRunArgs(tt.args)
_, _, _, _, _, seed, _, _, err := parseRunArgs(tt.args)
_, _, _, _, _, _, noColor, _, err := parseRunArgs(tt.args)
_, _, format, _, _, _, _, _, err := parseRunArgs(tt.args)
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseRunArgs_Verbosity(t *testing.T) {
    tests := []struct {
        name        string
        args        []string
        wantVerbosity output.Verbosity
    }{
        {"no flag defaults to default", []string{"file.yaml"}, output.VerbosityDefault},
        {"v flag sets verbose", []string{"file.yaml", "-v"}, output.VerbosityVerbose},
        {"vv flag sets debug", []string{"file.yaml", "-vv"}, output.VerbosityDebug},
        {"q flag sets quiet", []string{"file.yaml", "-q"}, output.VerbosityQuiet},
        {"quiet flag sets quiet", []string{"file.yaml", "--quiet"}, output.VerbosityQuiet},
        {"last flag wins when both v and q", []string{"file.yaml", "-v", "-q"}, output.VerbosityQuiet},
        {"last flag wins when both q and v", []string{"file.yaml", "-q", "-v"}, output.VerbosityVerbose},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, _, _, _, _, _, _, verbosity, err := parseRunArgs(tt.args)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if verbosity != tt.wantVerbosity {
                t.Errorf("got %v, want %v", verbosity, tt.wantVerbosity)
            }
        })
    }
}
```

#### Impact on Existing Tests
- **`cmd/apitest/main_test.go`** — **Compilation break.** All four `parseRunArgs` call sites destructure 8 return values; adding a 9th causes a compile error. Fix by adding `_` for verbosity at each call site (mechanical). No test logic changes needed.

---

### Step 5: Wire Verbose Output Into `runCmd` Rendering Loop
**Rationale:** Connect the data (from Step 2) to the output methods (from Step 3) in the main execution loop.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Call `RequestDetail`, `ResponseDetail`, `RequestBodyDump`, `ResponseBodyDump` in the results rendering loop |

#### Current Code
```go
default:
    passed := r.AssertionResults == nil || r.AssertionResults.Passed
    out.Result(r.Name, r.Result, passed)
    if r.AssertionResults != nil {
        for _, ar := range r.AssertionResults.Items {
            if !ar.Passed {
                out.AssertionDetail(ar.Type, ar.Expected, ar.Actual)
            }
        }
    }
```

#### New Code
```go
default:
    passed := r.AssertionResults == nil || r.AssertionResults.Passed
    out.RequestDetail(r.Method, r.URL, r.RequestHeaders)
    out.RequestBodyDump(r.RequestBody)
    out.Result(r.Name, r.Result, passed)
    if r.Result != nil {
        out.ResponseDetail(r.Result.StatusCode, r.Result.Headers)
        out.ResponseBodyDump(r.Result.Body)
    }
    if r.AssertionResults != nil {
        for _, ar := range r.AssertionResults.Items {
            if !ar.Passed {
                out.AssertionDetail(ar.Type, ar.Expected, ar.Actual)
            }
        }
    }
```

The verbose methods gate on `p.verbosity` internally — no extra conditionals in `runCmd`.

#### Tests to Write FIRST (RED phase)

Integration-style tests in `main_test.go`:
```go
func TestRunCmdDirect_QuietMode_OnlySummary(t *testing.T) {
    // Use a mock executor that returns successful results
    // Run with -q flag
    // Verify stdout contains summary but NOT individual request lines
    // Verify stdout is a single line
}

func TestRunCmdDirect_VerboseMode_ShowsHeaders(t *testing.T) {
    // Run with -v flag
    // Verify stdout contains ">" header lines
}

func TestRunCmdDirect_DebugMode_ShowsBody(t *testing.T) {
    // Run with -vv flag
    // Verify stdout contains body content
}
```

#### Impact on Existing Tests
- Existing rendering loop tests in `main_test.go` — unaffected. The new methods are silent at `VerbosityDefault`.

---

### Step 6: JSON Verbosity Support
**Rationale:** Behavior 6 requires `-v` to affect JSON output detail level. Extend `JSONRequest` with optional fields and thread verbosity through `buildJSONOutput`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add `RequestHeaders`, `ResponseHeaders`, `ResponseBody` optional fields to `JSONRequest` |
| `cmd/apitest/main.go` | modify | Add `verbosity` parameter to `buildJSONOutput`; populate optional fields when verbose |
| `cmd/apitest/main_test.go` | modify | Update `buildJSONOutput` call sites to pass verbosity; add JSON verbosity tests |

#### Current Code
```go
// internal/output/json.go
type JSONRequest struct {
    Name       string           `json:"name"`
    Phase      string           `json:"phase,omitempty"`
    Method     string           `json:"method"`
    URL        string           `json:"url"`
    StatusCode int              `json:"status_code,omitempty"`
    Duration   float64          `json:"duration_ms,omitempty"`
    Passed     bool             `json:"passed"`
    Error      string           `json:"error,omitempty"`
    Assertions []JSONAssertion  `json:"assertions,omitempty"`
}

func buildJSONOutput(name string, results []runner.RequestResult, summary *runner.Summary, preExecErr error) *output.JSONOutput {
```

#### New Code
```go
// internal/output/json.go
type JSONRequest struct {
    Name            string              `json:"name"`
    Phase           string              `json:"phase,omitempty"`
    Method          string              `json:"method"`
    URL             string              `json:"url"`
    StatusCode      int                 `json:"status_code,omitempty"`
    Duration        float64             `json:"duration_ms,omitempty"`
    Passed          bool                `json:"passed"`
    Error           string              `json:"error,omitempty"`
    Assertions      []JSONAssertion     `json:"assertions,omitempty"`
    RequestHeaders  map[string]string   `json:"request_headers,omitempty"`
    ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
    ResponseBody    string              `json:"response_body,omitempty"`
}

func buildJSONOutput(name string, results []runner.RequestResult, summary *runner.Summary, preExecErr error, verbosity output.Verbosity) *output.JSONOutput {
```

In `buildJSONOutput`, after building the base `JSONRequest`:
```go
if verbosity >= output.VerbosityVerbose && r.RequestHeaders != nil {
    req.RequestHeaders = r.RequestHeaders
}
if verbosity >= output.VerbosityVerbose && r.Result != nil {
    req.ResponseHeaders = map[string][]string(r.Result.Headers)
}
if verbosity >= output.VerbosityDebug && r.Result != nil {
    req.ResponseBody = string(r.Result.Body)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestBuildJSONOutput_Verbosity(t *testing.T) {
    tests := []struct {
        name            string
        verbosity       output.Verbosity
        wantReqHeaders  bool
        wantRespHeaders bool
        wantBody        bool
    }{
        {"default_no_extra_fields", output.VerbosityDefault, false, false, false},
        {"verbose_includes_headers", output.VerbosityVerbose, true, true, false},
        {"debug_includes_body", output.VerbosityDebug, true, true, true},
        {"quiet_no_extra_fields", output.VerbosityQuiet, false, false, false},
    }
    // ...
}
```

#### Impact on Existing Tests
- `cmd/apitest/main_test.go` — all `buildJSONOutput(...)` calls become compilation errors. Fix by appending `output.VerbosityDefault` as the last argument. Three call sites: lines 2460, 2489, 2521.

---

### Step 7: Update Help Text and Usage String
**Rationale:** Complete the "completeness contract" — user-facing flags must appear in help output.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `-v`, `-vv`, `-q` to `printHelp()` and usage string |

#### New Code
```go
// In printHelp():
fmt.Println("  --format <type>     Output format: terminal (default), json, tap")
fmt.Println("  --no-color          Disable colored output (also respects NO_COLOR env var)")
fmt.Println("  -v                  Verbose: show request/response headers")
fmt.Println("  -vv                 Very verbose: full HTTP request/response dump")
fmt.Println("  -q, --quiet         Quiet: summary line only")
```

#### Tests to Write FIRST (RED phase)
```go
func TestHelp_ContainsVerbosityFlags(t *testing.T) {
    // Capture printHelp() output (or run binary with --help)
    // Verify contains "-v", "-vv", "-q"
}
```

#### Impact on Existing Tests
- Existing help text tests must be updated to include the new flags.

---

### Step 8: Update Smoke Test
**Rationale:** Observable verification per the completeness contract.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add smoke scenarios for `-q`, `-v`, `-vv` flags |

#### New Code (additions to `smoke/run.sh`)
```bash
# Test quiet mode: output should be a single summary line
OUTPUT=$(./apitest run smoke/collection.yaml -q 2>&1)
LINE_COUNT=$(echo "$OUTPUT" | wc -l | tr -d ' ')
if [ "$LINE_COUNT" -gt 3 ]; then
  echo "FAIL: quiet mode produced $LINE_COUNT lines, expected <= 3"
  exit 1
fi
echo "PASS: quiet mode output is minimal"

# Test verbose mode: should contain header-like lines
OUTPUT=$(./apitest run smoke/collection.yaml -v 2>&1)
if ! echo "$OUTPUT" | grep -q "^  >"; then
  echo "FAIL: verbose mode missing request detail lines"
  exit 1
fi
echo "PASS: verbose mode shows request detail"

# Test help contains verbosity flags
OUTPUT=$(./apitest --help 2>&1)
for flag in "-v" "-vv" "-q"; do
  if ! echo "$OUTPUT" | grep -q "$flag"; then
    echo "FAIL: --help missing $flag"
    exit 1
  fi
done
echo "PASS: help text contains verbosity flags"
```

#### Impact on Existing Tests
- None. Smoke test additions only.

---

## Test Impact Summary

| File | Test Function | Impact | Action Required |
|------|--------------|--------|----------------|
| `cmd/apitest/main_test.go` | All `parseRunArgs` calls | **Compilation break** | Add `_` for new 9th return value |
| `cmd/apitest/main_test.go` | All `buildJSONOutput` calls | **Compilation break** | Append `output.VerbosityDefault` argument |
| `internal/output/terminal_test.go` | All existing tests | None | No changes needed |
| `internal/output/json_test.go` | All existing tests | None | No changes needed |
| `internal/runner/runner_test.go` | All existing tests | None | No changes needed |

## Risks and Edge Cases

- **`-vv` flag parsing:** Must be handled before `-v` in the switch (or use exact match). Go's switch uses exact matching by default, so both cases are safe regardless of order — but put `-vv` first for clarity.
- **Conflicting flags (`-v` and `-q`):** Last flag wins. Simple, predictable CLI behavior.
- **Large response bodies at `-vv`:** Truncate at 10KB with `[N bytes total]` indicator to avoid terminal flooding.
- **Binary response bodies:** Detect non-UTF-8 content; show `[binary content, N bytes]` placeholder.
- **`nil` `RequestHeaders`/`RequestBody`:** Guard all verbose output methods against nil (skipped requests have no request data).
- **JSON quiet mode:** `-q` with `--format json` — quiet only gates terminal output. JSON verbosity affects detail fields (headers/body), not whether the JSON document is produced.
- **Sensitive headers at `-v`:** `Authorization` and `Cookie` headers will be visible. This is expected for a developer debugging tool; redaction is a separate future task (M1-023).

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Default: request name, status, assertion results
./apitest run smoke/collection.yaml

# Verbose: + request/response headers
./apitest run smoke/collection.yaml -v

# Debug: + full body dump
./apitest run smoke/collection.yaml -vv

# Quiet: summary line only
./apitest run smoke/collection.yaml -q

# Quiet with all passing: single line
./apitest run smoke/collection.yaml -q && echo "exit 0"

# Verbose + JSON: headers appear in JSON output
./apitest run smoke/collection.yaml -v --format json | jq .
```
