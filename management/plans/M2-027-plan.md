# Implementation Plan: M2-027

## Overview
Add HTML report generation as a self-contained output format. When users run `apitest run tests.yaml --format html --report report.html`, a single HTML file is generated with embedded CSS/JS showing test summary, pass/fail status, timing, and assertion details. Registered as Professional-tier feature gate.

## Task Details
- **ID:** M2-027
- **Title:** HTML report generation
- **Phase:** M2: Reporting
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-026 | JUnit XML output | done |
| M1-028 | Feature gate framework | done |

## Implementation Steps

### Step 1: Register HTML report feature gate
**Rationale:** Smallest blast radius first. Add the feature gate definition so the system knows about the `html_report` feature before any output code exists.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add `html_report` feature definition to `DefaultRegistry()` |
| `internal/auth/gate_test.go` | modify | Add test for `html_report` gate at different tiers |

#### Current Code
```go
r.Register(FeatureDefinition{
    Name:         "custom_backoff",
    RequiredTier: TierProfessional,
    Description:  "Custom backoff strategies (linear, constant) require Professional tier ($19/month)",
    Workaround:   "Use exponential backoff (the default) which is available at Solo tier",
})
return r
```

#### New Code
```go
r.Register(FeatureDefinition{
    Name:         "custom_backoff",
    RequiredTier: TierProfessional,
    Description:  "Custom backoff strategies (linear, constant) require Professional tier ($19/month)",
    Workaround:   "Use exponential backoff (the default) which is available at Solo tier",
})
r.Register(FeatureDefinition{
    Name:         "html_report",
    RequiredTier: TierProfessional,
    Description:  "HTML report generation requires Professional tier ($19/month)",
    Workaround:   "Use --format json for machine-readable output, or --format junit for CI integration",
})
return r
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckFeature_HTMLReport(t *testing.T) {
    tests := []struct {
        name    string
        tier    Tier
        wantErr bool
    }{
        {"free tier gated", TierFree, true},
        {"solo tier gated", TierSolo, true},
        {"professional tier allowed", TierProfessional, false},
        {"team tier allowed", TierTeam, false},
    }
    registry := DefaultRegistry()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := CheckFeature(registry, "html_report", tt.tier)
            if (err != nil) != tt.wantErr {
                t.Errorf("CheckFeature(html_report, %q) error = %v, wantErr %v", tt.tier, err, tt.wantErr)
            }
        })
    }
}
```

#### Impact on Existing Tests
- No existing tests affected

### Step 2: Create HTML report types and WriteHTML function
**Rationale:** Build the core output package code before wiring it into main.go. This is the main deliverable — a self-contained HTML file with embedded CSS/JS using Go templates.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/html.go` | create | HTML report types, Go template, and `WriteHTML` function |
| `internal/output/html_test.go` | create | Tests for HTML report generation |

#### New Code — `internal/output/html.go`

The file will contain:

```go
package output

import (
    "html/template"
    "io"
    "time"
)

// HTMLReport is the top-level data structure passed to the HTML template.
type HTMLReport struct {
    Name       string
    Status     string // "passed", "failed"
    Total      int
    Passed     int
    Failed     int
    Skipped    int
    DurationMs int64
    Requests   []HTMLRequest
    GeneratedAt string // RFC3339 timestamp
}

// HTMLRequest represents one request in the HTML report.
type HTMLRequest struct {
    Name       string
    Status     string // "passed", "failed", "skipped", "error"
    Method     string
    URL        string
    StatusCode int
    DurationMs int64
    Assertions []HTMLAssertion
    Error      string // non-empty when Status == "error"
    SkipReason string // non-empty when Status == "skipped"
}

// HTMLAssertion represents one assertion outcome in the HTML report.
type HTMLAssertion struct {
    Type     string
    Expected string
    Actual   string
    Passed   bool
}

// WriteHTML generates a self-contained HTML report and writes it to w.
func WriteHTML(w io.Writer, report *HTMLReport) error {
    tmpl, err := template.New("report").Parse(htmlTemplate)
    if err != nil {
        return fmt.Errorf("parse html template: %w", err)
    }
    return tmpl.Execute(w, report)
}

const htmlTemplate = `...` // Full HTML template with embedded CSS/JS
```

The HTML template will be a Go `const` string containing:
- A complete HTML5 document
- Embedded CSS for styling (no external dependencies)
- Summary dashboard: total, passed, failed, skipped, duration
- Per-request rows with pass/fail/skip status, method, URL, duration
- Collapsible assertion details using minimal JavaScript (toggle visibility)
- Failed assertions showing expected vs actual values
- Color coding: green for pass, red for fail, gray for skip

#### Tests to Write FIRST (RED phase)

```go
func TestWriteHTML(t *testing.T) {
    tests := []struct {
        name       string
        input      *HTMLReport
        wantSubstr []string
        wantAbsent []string
    }{
        {
            "self-contained HTML with no external references",
            &HTMLReport{
                Name: "My Collection", Status: "passed",
                Total: 2, Passed: 2, DurationMs: 500,
                Requests: []HTMLRequest{
                    {Name: "Get Users", Status: "passed", Method: "GET", URL: "http://api/users", StatusCode: 200, DurationMs: 250},
                    {Name: "Get Items", Status: "passed", Method: "GET", URL: "http://api/items", StatusCode: 200, DurationMs: 250},
                },
            },
            []string{"<!DOCTYPE html>", "<html", "</html>", "<style>", "My Collection", "Get Users", "Get Items", "2 passed"},
            []string{"<link rel=\"stylesheet\"", "<script src="},
        },
        {
            "summary shows total passed failed skipped duration",
            &HTMLReport{
                Name: "Test", Status: "failed",
                Total: 4, Passed: 2, Failed: 1, Skipped: 1, DurationMs: 1234,
                Requests: []HTMLRequest{
                    {Name: "A", Status: "passed", StatusCode: 200, DurationMs: 100},
                    {Name: "B", Status: "failed", StatusCode: 500, DurationMs: 200},
                    {Name: "C", Status: "skipped"},
                    {Name: "D", Status: "passed", StatusCode: 200, DurationMs: 300},
                },
            },
            []string{"4", "2 passed", "1 failed", "1 skipped", "1234"},
            nil,
        },
        {
            "request shows name status method URL duration",
            &HTMLReport{
                Name: "Detail", Status: "passed",
                Total: 1, Passed: 1, DurationMs: 456,
                Requests: []HTMLRequest{
                    {Name: "Create User", Status: "passed", Method: "POST", URL: "http://api/users", StatusCode: 201, DurationMs: 456},
                },
            },
            []string{"Create User", "POST", "http://api/users", "201", "456"},
            nil,
        },
        {
            "failed assertion details shown",
            &HTMLReport{
                Name: "Assert", Status: "failed",
                Total: 1, Failed: 1, DurationMs: 100,
                Requests: []HTMLRequest{
                    {
                        Name: "Check Status", Status: "failed", Method: "GET",
                        URL: "http://api/health", StatusCode: 500, DurationMs: 100,
                        Assertions: []HTMLAssertion{
                            {Type: "status", Expected: "200", Actual: "500", Passed: false},
                        },
                    },
                },
            },
            []string{"status", "200", "500"},
            nil,
        },
        {
            "no external CSS or JS dependencies",
            &HTMLReport{
                Name: "Standalone", Status: "passed",
                Total: 0, DurationMs: 0,
            },
            []string{"<style>"},
            []string{"<link rel=\"stylesheet\"", "<script src=\"http", "<script src=\"/"},
        },
        {
            "skipped request shows skip reason",
            &HTMLReport{
                Name: "Skip", Status: "passed",
                Total: 1, Skipped: 1, DurationMs: 0,
                Requests: []HTMLRequest{
                    {Name: "Dep Failed", Status: "skipped", SkipReason: "dependency failed"},
                },
            },
            []string{"skipped", "dependency failed"},
            nil,
        },
        {
            "error request shows error message",
            &HTMLReport{
                Name: "ErrTest", Status: "failed",
                Total: 1, Failed: 1, DurationMs: 50,
                Requests: []HTMLRequest{
                    {Name: "Bad Call", Status: "error", Error: "connection refused"},
                },
            },
            []string{"error", "connection refused"},
            nil,
        },
        {
            "special HTML characters escaped",
            &HTMLReport{
                Name: "Test <script>", Status: "passed",
                Total: 1, Passed: 1, DurationMs: 10,
                Requests: []HTMLRequest{
                    {Name: "XSS & \"Test\"", Status: "passed", Method: "GET", URL: "http://api/<endpoint>", StatusCode: 200, DurationMs: 10},
                },
            },
            []string{"&lt;script&gt;", "XSS &amp;", "&lt;endpoint&gt;"},
            []string{"<script>alert"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var buf bytes.Buffer
            if err := WriteHTML(&buf, tt.input); err != nil {
                t.Fatalf("WriteHTML() error = %v", err)
            }
            got := buf.String()
            for _, sub := range tt.wantSubstr {
                if !strings.Contains(got, sub) {
                    t.Errorf("output missing expected substring %q", sub)
                }
            }
            for _, absent := range tt.wantAbsent {
                if strings.Contains(got, absent) {
                    t.Errorf("output contains unexpected substring %q", absent)
                }
            }
        })
    }
}

func TestWriteHTML_validHTML(t *testing.T) {
    // Verify the output starts with DOCTYPE and contains closing html tag
    report := &HTMLReport{
        Name: "Valid", Status: "passed",
        Total: 1, Passed: 1, DurationMs: 100,
        Requests: []HTMLRequest{
            {Name: "Test", Status: "passed", Method: "GET", URL: "http://test", StatusCode: 200, DurationMs: 100},
        },
    }
    var buf bytes.Buffer
    if err := WriteHTML(&buf, report); err != nil {
        t.Fatalf("WriteHTML() error = %v", err)
    }
    got := buf.String()
    if !strings.HasPrefix(got, "<!DOCTYPE html>") {
        t.Error("output should start with <!DOCTYPE html>")
    }
    if !strings.Contains(got, "</html>") {
        t.Error("output should contain closing </html> tag")
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new files)

### Step 3: Wire HTML format into CLI (main.go)
**Rationale:** Connect the new format to the CLI, including: format validation, feature gate check, `--format html` requires `--report`, building the HTMLReport from runner results, and writing to the report file.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `html` to format validation, feature gate check, `--format html` requires `--report`, HTML report generation and writing |

#### Current Code — format validation (line ~200)
```go
if format != "" && format != "json" && format != "terminal" && format != "tap" && format != "junit" {
    errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json, tap, junit)", format))
    return 1, nil
}
```

#### New Code — format validation
```go
if format != "" && format != "json" && format != "terminal" && format != "tap" && format != "junit" && format != "html" {
    errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json, tap, junit, html)", format))
    return 1, nil
}
```

#### Current Code — feature gate check for junit (line ~207)
```go
// Feature gate check for --format junit (early, before any parsing/loading)
if format == "junit" {
    reg := auth.DefaultRegistry()
    if gateErr := auth.CheckFeature(reg, "junit_xml", currentTier()); gateErr != nil {
        _ = writeJUnitError(os.Stdout, gateErr)
        return 6, nil
    }
}
```

#### New Code — feature gate check for html (added after junit gate)
```go
// Feature gate check for --format html (early, before any parsing/loading)
if format == "html" {
    reg := auth.DefaultRegistry()
    if gateErr := auth.CheckFeature(reg, "html_report", currentTier()); gateErr != nil {
        errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
        errOut.StructuredError(gateErr)
        return 6, nil
    }
    // --format html requires --report flag
    if report == "" {
        errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
        errOut.StructuredError(fmt.Errorf("--format html requires --report <file> (e.g. --format html --report report.html)"))
        return 1, nil
    }
}
```

#### New Code — HTML output block (after existing junit block, before terminal output)
A new `if format == "html"` block that:
1. Builds an `output.HTMLReport` from `results` and `summary` (similar to `buildJUnitOutput`)
2. Creates the report file via `os.Create(report)`
3. Calls `output.WriteHTML(f, htmlReport)`
4. Returns appropriate exit code

A new helper function `buildHTMLReport`:
```go
func buildHTMLReport(name string, results []runner.RequestResult, summary *runner.Summary) *output.HTMLReport {
    report := &output.HTMLReport{
        Name:        name,
        GeneratedAt: time.Now().Format(time.RFC3339),
    }
    if summary != nil {
        report.Total = summary.Total
        report.Passed = summary.Passed
        report.Failed = summary.Failed
        report.Skipped = summary.Skipped
        report.DurationMs = summary.Duration.Milliseconds()
        if summary.Failed > 0 {
            report.Status = "failed"
        } else {
            report.Status = "passed"
        }
    }
    for _, r := range results {
        hr := output.HTMLRequest{
            Name: r.Name,
            Method: r.Method,
            URL: r.URL,
        }
        switch {
        case r.Skipped:
            hr.Status = "skipped"
            hr.SkipReason = r.SkipReason
        case r.Err != nil:
            hr.Status = "error"
            hr.Error = r.Err.Error()
        default:
            if r.Result != nil {
                hr.StatusCode = r.Result.StatusCode
                hr.DurationMs = r.Result.Duration.Milliseconds()
            }
            if r.AssertionResults != nil && !r.AssertionResults.Passed {
                hr.Status = "failed"
                for _, ar := range r.AssertionResults.Items {
                    hr.Assertions = append(hr.Assertions, output.HTMLAssertion{
                        Type: ar.Type, Expected: ar.Expected,
                        Actual: ar.Actual, Passed: ar.Passed,
                    })
                }
            } else {
                hr.Status = "passed"
            }
        }
        report.Requests = append(report.Requests, hr)
    }
    return report
}
```

Additionally, throughout `runCmdInner`, the early-exit error paths (parse error, empty collection, env load error, etc.) need a `format == "html"` branch that writes to the report file. Following the pattern of `junit`, these will write an error HTML report.

#### New Code — error handling function
```go
func writeHTMLError(path string, err error) error {
    f, createErr := os.Create(path)
    if createErr != nil {
        return createErr
    }
    defer f.Close()
    report := &output.HTMLReport{
        Name:        "apitest",
        Status:      "error",
        GeneratedAt: time.Now().Format(time.RFC3339),
        Requests: []output.HTMLRequest{{
            Name:   "initialization",
            Status: "error",
            Error:  err.Error(),
        }},
        Total: 1, Failed: 1,
    }
    return output.WriteHTML(f, report)
}
```

#### Help text updates
- Line 2036: Add `html` to format list: `"terminal (default), json, tap, junit, html"`
- Line 2037: Update report description: `"--report <file>     Write report to file (required for --format html, optional for --format junit)"`
- Line 202: Error message: Add `html` to supported formats

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_format_html_gated_at_free_tier(t *testing.T) {
    // --format html with free tier should exit 6 (feature gated)
}

func TestRunCmd_format_html_requires_report_flag(t *testing.T) {
    // --format html without --report should error with helpful message
}

func TestRunCmd_format_html_recognized(t *testing.T) {
    // --format html should not be treated as unknown format (exit 1)
}

func TestRunCmd_format_html_generates_report_file(t *testing.T) {
    // With professional tier, --format html --report report.html generates valid HTML
}

func TestRunCmd_format_html_report_contains_results(t *testing.T) {
    // Generated report contains request names, status codes, pass/fail
}

func TestRunCmd_format_html_report_self_contained(t *testing.T) {
    // Report has no external CSS/JS links
}

func TestRunCmd_format_html_failed_assertions_shown(t *testing.T) {
    // Report shows assertion failure details (expected vs actual)
}

func TestRunCmd_format_html_stdout_empty_when_report_used(t *testing.T) {
    // stdout should be empty when --report is specified (all output goes to file)
}
```

#### Impact on Existing Tests
- `TestParseRunArgs_Report` — no impact, already supports --report flag
- Format validation tests — the error message changes to include "html" in supported formats; any test asserting the exact error message string may need updating

### Step 4: Update help text and smoke test
**Rationale:** Final polishing step to ensure the feature is discoverable and verifiable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Update help text for --format and --report |
| `smoke/run.sh` | modify | Add HTML report smoke test |

#### Help Text Changes
```go
// Line 2036 - Run Options:
fmt.Println("  --format <type>     Output format: terminal (default), json, tap, junit, html")
// Line 2037 - Report:
fmt.Println("  --report <file>     Write report to file (required for --format html, optional for --format junit)")
```

#### Smoke Test Addition
```bash
echo "--- Running with --format html (expect gate at free tier, exit 6) ---"
./apitest run sample/hello.yaml --format html --report /tmp/test.html && echo "ERROR: should have been gated" || echo "Exit code: $?"
echo
```

#### Tests to Write FIRST (RED phase)
No additional tests needed for this step — covered by existing help and smoke test infrastructure.

#### Impact on Existing Tests
- Any test checking the exact help text output may need updating for the new format option

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/gate_test.go` | (new) `TestCheckFeature_HTMLReport` | new test | write |
| `internal/output/html_test.go` | (new) all | new file | write |
| `cmd/apitest/main_test.go` | (new) multiple HTML tests | new tests | write |
| `cmd/apitest/main_test.go` | tests checking "unknown format" error message | may break | update message to include "html" |
| `smoke/run.sh` | n/a | update | add HTML smoke test |

## Risks and Edge Cases

- **Risk:** HTML template injection via request names/URLs containing `<script>` tags. **Mitigation:** Use Go's `html/template` package which auto-escapes HTML entities by default. Test with XSS-like input.
- **Risk:** Large report files for collections with many requests. **Mitigation:** Keep the template simple with collapsible sections. No need for pagination in this phase — the file is generated once and opened in a browser.
- **Risk:** `--format html` without `--report` has no sensible behavior (HTML to stdout is useless). **Mitigation:** Require `--report` when format is `html`, with a clear error message.
- **Edge case:** Empty collection with `--format html --report`. **Handling:** Generate a valid HTML file showing 0 requests.
- **Edge case:** Pre-execution errors (parse failures, env load errors) with `--format html`. **Handling:** Use `writeHTMLError` to generate an error report file (similar to `writeJUnitError`).
- **Edge case:** Report file path is not writable. **Handling:** Return error with structured message.
- **Edge case:** Data-driven results in HTML report. **Handling:** Each iteration appears as a separate row in the request table. The `Name` field already contains the iteration suffix (e.g., `"Create User [1/3]"`).
- **Edge case:** Parallel execution wave information. **Handling:** Not explicitly visualized in this phase; requests are listed in execution order. Future M2-028 task may add wave visualization.

## Design Decisions

1. **Go `html/template` over raw string concatenation:** Auto-escaping prevents XSS, and the template engine is part of the standard library.
2. **Single const string for template:** Keeps the template co-located with the Go code, no need for `embed` or external files.
3. **Minimal JavaScript:** Only for collapsible assertion details (toggle visibility). No chart libraries in this phase to keep the file small.
4. **Professional tier:** Aligned with JUnit XML and other advanced output formats.
5. **`--report` required for HTML:** Unlike JUnit which can write to stdout, HTML to stdout is not useful. Enforced at CLI level.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Build and run with professional tier override (for testing)
go build -o apitest ./cmd/apitest
# Test gate at free tier
./apitest run sample/hello.yaml --format html --report /tmp/test.html
# Expected: exit code 6 (feature gated)

# Run tests directly
go test ./internal/output/... -run TestWriteHTML -v
go test ./cmd/apitest/... -run TestRunCmd_format_html -v
```
