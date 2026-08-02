# Implementation Plan: M2-026

## Overview
Add JUnit XML output format (`--format junit`) with `--report <file>` flag for CI/CD integration, following the standard JUnit XML schema with testsuites/testsuite/testcase elements. Register as Professional-tier feature gate.

## Task Details
- **ID:** M2-026
- **Title:** JUnit XML output format
- **Phase:** M2: Reporting
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-020 | JSON output format | done |
| M1-028 | Feature gate framework | done |

## Implementation Steps

### Step 1: Register `junit_xml` Feature Gate
**Rationale:** Smallest blast radius — adds a single entry to the existing registry. No behavior changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add `junit_xml` feature definition to `DefaultRegistry()` |
| `internal/auth/gate_test.go` | modify | Add tests for `junit_xml` gate at Free/Solo/Professional tiers |

#### Current Code
```go
// In DefaultRegistry():
r.Register(FeatureDefinition{
    Name:         "data_driven",
    RequiredTier: TierProfessional,
    Description:  "Data-driven testing requires Professional tier ($19/month)",
    Workaround:   "Duplicate requests manually for different data values",
})
return r
```

#### New Code
```go
r.Register(FeatureDefinition{
    Name:         "data_driven",
    RequiredTier: TierProfessional,
    Description:  "Data-driven testing requires Professional tier ($19/month)",
    Workaround:   "Duplicate requests manually for different data values",
})
r.Register(FeatureDefinition{
    Name:         "junit_xml",
    RequiredTier: TierProfessional,
    Description:  "JUnit XML output requires Professional tier ($19/month)",
    Workaround:   "Use --format json for machine-readable output, or --format tap for CI integration",
})
return r
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckFeature_junitXML(t *testing.T) {
    registry := DefaultRegistry()
    tests := []struct {
        name    string
        tier    Tier
        wantErr bool
    }{
        {"free tier blocked", TierFree, true},
        {"solo tier blocked", TierSolo, true},
        {"professional tier allowed", TierProfessional, false},
        {"team tier allowed", TierTeam, false},
        {"enterprise tier allowed", TierEnterprise, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := CheckFeature(registry, "junit_xml", tt.tier)
            if (err != nil) != tt.wantErr {
                t.Errorf("CheckFeature(junit_xml, %q) error = %v, wantErr %v", tt.tier, err, tt.wantErr)
            }
            if tt.wantErr {
                var gateErr *GateError
                if !errors.As(err, &gateErr) {
                    t.Errorf("expected *GateError, got %T", err)
                }
            }
        })
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — adding new feature definition does not alter existing gate behavior.

---

### Step 2: Create JUnit XML Types and Writer
**Rationale:** Pure data types and serialization with no wiring to CLI — fully testable in isolation. This is the core of the feature.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/junit.go` | create | JUnit XML types and `WriteJUnitXML(w, *JUnitOutput)` |
| `internal/output/junit_test.go` | create | Comprehensive table-driven tests |

#### New Code
```go
package output

import (
    "encoding/xml"
    "fmt"
    "io"
)

// JUnitTestSuites is the top-level container in JUnit XML output.
type JUnitTestSuites struct {
    XMLName    xml.Name         `xml:"testsuites"`
    TestSuites []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents one collection run.
type JUnitTestSuite struct {
    XMLName   xml.Name        `xml:"testsuite"`
    Name      string          `xml:"name,attr"`
    Tests     int             `xml:"tests,attr"`
    Failures  int             `xml:"failures,attr"`
    Errors    int             `xml:"errors,attr"`
    Skipped   int             `xml:"skipped,attr"`
    Time      string          `xml:"time,attr"`
    TestCases []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents one request result.
type JUnitTestCase struct {
    XMLName   xml.Name      `xml:"testcase"`
    Name      string        `xml:"name,attr"`
    ClassName string        `xml:"classname,attr"`
    Time      string        `xml:"time,attr"`
    Failure   *JUnitFailure `xml:"failure,omitempty"`
    Error     *JUnitError   `xml:"error,omitempty"`
    Skipped   *JUnitSkipped `xml:"skipped,omitempty"`
}

// JUnitFailure represents an assertion failure.
type JUnitFailure struct {
    Message string `xml:"message,attr"`
    Type    string `xml:"type,attr,omitempty"`
    Body    string `xml:",chardata"`
}

// JUnitError represents a network/execution error.
type JUnitError struct {
    Message string `xml:"message,attr"`
    Type    string `xml:"type,attr,omitempty"`
    Body    string `xml:",chardata"`
}

// JUnitSkipped represents a skipped test case.
type JUnitSkipped struct {
    Message string `xml:"message,attr,omitempty"`
}

// WriteJUnitXML serializes JUnit XML output to the writer.
// Writes the XML declaration followed by the testsuites element.
func WriteJUnitXML(w io.Writer, suites *JUnitTestSuites) error {
    if _, err := fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`); err != nil {
        return err
    }
    enc := xml.NewEncoder(w)
    enc.Indent("", "  ")
    if err := enc.Encode(suites); err != nil {
        return err
    }
    _, err := fmt.Fprintln(w)
    return err
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteJUnitXML(t *testing.T) {
    tests := []struct {
        name       string
        input      *JUnitTestSuites
        wantSubstr []string
        wantAbsent []string
    }{
        {
            "valid XML with passing tests",
            // ... passing testcases
            []string{`<?xml version="1.0"`, `<testsuites>`, `<testsuite`, `<testcase`, `time="`},
            []string{`<failure`, `<error`, `<skipped`},
        },
        {
            "failure element for assertion failures",
            // ... testcase with failure
            []string{`<failure message="`},
            nil,
        },
        {
            "error element for network errors",
            // ... testcase with error
            []string{`<error message="`},
            nil,
        },
        {
            "skipped element for skipped requests",
            // ... testcase with skipped
            []string{`<skipped`},
            nil,
        },
        {
            "time attribute reflects duration in seconds",
            // ... testcase with known duration
            []string{`time="0.123"`},
            nil,
        },
        {
            "empty collection produces valid XML",
            // ... empty testsuites
            []string{`<?xml version="1.0"`, `<testsuites`},
            nil,
        },
        {
            "suite-level counts correct",
            // ... suite with known pass/fail/error/skip counts
            []string{`tests="4"`, `failures="1"`, `errors="1"`, `skipped="1"`},
            nil,
        },
        {
            "classname attribute set to collection name",
            // ... testcase with classname
            []string{`classname="`},
            nil,
        },
        {
            "multiple assertion failures concatenated in body",
            // ... testcase with multiple failures
            []string{`Expected status 200`},
            nil,
        },
        {
            "XML is well-formed — parseable by xml.Unmarshal",
            // ... round-trip test
            nil,
            nil,
        },
    }
    // Table-driven: write to buffer, check substrings, and parse XML to verify well-formedness.
}
```

#### Impact on Existing Tests
- No existing tests affected — new file.

---

### Step 3: Add `--report` Flag to `parseRunArgs` and JUnit Format Recognition
**Rationale:** Adds flag parsing before wiring output, so the CLI layer is ready. Also adds `"junit"` to the format validation whitelist.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `--report` flag to `parseRunArgs`; add `"junit"` to format validation; add `report` return value |
| `cmd/curlew/run_test.go` | modify | Add tests for `--report` flag parsing and `--format junit` recognition |

#### Current Code
```go
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, verbosity output.Verbosity, allowSensitive, showDeps, dryRun, parallel bool, err error) {
```

#### New Code
```go
func parseRunArgs(args []string) (file, envName, format, report string, vars, envVarVars map[string]string, seed *int64, noColor bool, verbosity output.Verbosity, allowSensitive, showDeps, dryRun, parallel bool, err error) {
```

The `--report` flag will be parsed in the switch block:
```go
case "--report":
    i++
    if i >= len(args) {
        return ..., fmt.Errorf("--report requires a file path (e.g. --report results.xml)")
    }
    report = args[i]
```

Format validation will add `"junit"`:
```go
if format != "" && format != "json" && format != "terminal" && format != "tap" && format != "junit" {
    errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json, tap, junit)", format))
    return 1, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseRunArgs_report(t *testing.T) {
    tests := []struct {
        name       string
        args       []string
        wantReport string
        wantErr    bool
    }{
        {"report flag parsed", []string{"col.yaml", "--report", "results.xml"}, "results.xml", false},
        {"report flag missing value", []string{"col.yaml", "--report"}, "", true},
        {"no report flag", []string{"col.yaml"}, "", false},
    }
    // ...
}

func TestRunCmd_format_junit_recognized(t *testing.T) {
    // --format junit does not produce "unknown output format" error
}
```

#### Impact on Existing Tests
- Every call site of `parseRunArgs` needs to accept the new `report` return value. The callers are:
  - `runCmdInner()`
  - `watchCmd()` (uses `parseRunArgs` for flag extraction)
- Tests calling `parseRunArgs` directly will need updating for the new return signature.

---

### Step 4: Wire JUnit Output in `runCmdInner` with Feature Gate and `--report` Support
**Rationale:** This is the integration step — connects the JUnit writer to the run pipeline, including feature gate check, file output via `--report`, and exit code logic.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add JUnit output path in `runCmdInner`; build JUnit output from runner results; feature gate check; `--report` file write; early bailout handling |
| `cmd/curlew/main_test.go` | modify | Add integration tests for `--format junit` end-to-end |

#### New Code — `buildJUnitOutput` helper function
```go
// buildJUnitOutput constructs JUnit XML output from runner results and summary.
func buildJUnitOutput(name string, results []runner.RequestResult, summary *runner.Summary) *output.JUnitTestSuites {
    suite := output.JUnitTestSuite{
        Name: name,
    }
    var failures, errCount, skipped int
    for _, r := range results {
        tc := output.JUnitTestCase{
            Name:      r.Name,
            ClassName: name,
        }
        switch {
        case r.Skipped:
            msg := r.SkipReason
            tc.Skipped = &output.JUnitSkipped{Message: msg}
            skipped++
        case r.Err != nil:
            tc.Error = &output.JUnitError{
                Message: r.Err.Error(),
                Type:    "ExecutionError",
            }
            errCount++
        default:
            if r.Result != nil {
                tc.Time = fmt.Sprintf("%.3f", r.Result.Duration.Seconds())
            }
            if r.AssertionResults != nil && !r.AssertionResults.Passed {
                var msgs []string
                for _, ar := range r.AssertionResults.Items {
                    if !ar.Passed {
                        msgs = append(msgs, fmt.Sprintf("Expected %s %s, got %s", ar.Type, ar.Expected, ar.Actual))
                    }
                }
                tc.Failure = &output.JUnitFailure{
                    Message: msgs[0],
                    Type:    "AssertionFailure",
                    Body:    strings.Join(msgs, "\n"),
                }
                failures++
            }
        }
        suite.TestCases = append(suite.TestCases, tc)
    }
    suite.Tests = len(results)
    suite.Failures = failures
    suite.Errors = errCount
    suite.Skipped = skipped
    if summary != nil {
        suite.Time = fmt.Sprintf("%.3f", summary.Duration.Seconds())
    }
    return &output.JUnitTestSuites{TestSuites: []output.JUnitTestSuite{suite}}
}
```

#### JUnit Output Path in `runCmdInner`
After runner.Run completes and the format is "junit":

1. Feature gate check for `junit_xml` at current tier — exit 6 if gated (before writing any output)
2. Build JUnit output from results
3. If `--report <file>` specified: write to file, NOT stdout
4. If no `--report`: write to stdout
5. Exit code logic mirrors JSON/TAP branches

#### Feature Gate Check (early, before runner.Run)
```go
if format == "junit" {
    reg := auth.DefaultRegistry()
    if gateErr := auth.CheckFeature(reg, "junit_xml", currentTier()); gateErr != nil {
        // Similar pattern to parallel_execution gate check
        // Exit 6
    }
}
```

**Decision:** The feature gate check for `--format junit` will happen early (before runner.Run), similar to how `--parallel` is gate-checked. This means at Free tier, the user gets immediate feedback without running any requests.

#### `--report` File Output
```go
var junitWriter io.Writer = os.Stdout
if report != "" {
    f, err := os.Create(report)
    if err != nil {
        errOut.StructuredError(fmt.Errorf("cannot create report file: %w", err))
        return 1, nil
    }
    defer f.Close()
    junitWriter = f
}
if writeErr := output.WriteJUnitXML(junitWriter, junitOut); writeErr != nil {
    _, _ = fmt.Fprintf(os.Stderr, "junit xml encode error: %v\n", writeErr)
    return 1, summary
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_format_junit_passed(t *testing.T) {
    // httptest server returning 200
    // --format junit produces valid XML with <testcase> and no <failure>
}

func TestRunCmd_format_junit_assertion_failure(t *testing.T) {
    // httptest server returning 404, collection asserts status 200
    // --format junit produces <failure> element with assertion message
}

func TestRunCmd_format_junit_network_error(t *testing.T) {
    // collection pointing to unreachable server
    // --format junit produces <error> element
}

func TestRunCmd_format_junit_skipped(t *testing.T) {
    // collection with skipped request (e.g., dependency failure in parallel)
    // --format junit produces <skipped> element
}

func TestRunCmd_format_junit_report_file(t *testing.T) {
    // --format junit --report /tmp/results.xml
    // stdout is empty, file contains valid JUnit XML
}

func TestRunCmd_format_junit_report_not_to_stdout(t *testing.T) {
    // --format junit --report file
    // verify stdout is empty
}

func TestRunCmd_format_junit_timing(t *testing.T) {
    // verify time attribute is in seconds (e.g., "0.123")
}

func TestRunCmd_format_junit_feature_gate_free_tier(t *testing.T) {
    // override currentTier to TierFree
    // --format junit → exit 6
}

func TestRunCmd_format_junit_feature_gate_professional_tier(t *testing.T) {
    // override currentTier to TierProfessional
    // --format junit → runs normally
}
```

#### Impact on Existing Tests
- Tests that call `parseRunArgs` directly need updating for the new `report` return value.
- `captureRunCmd` helper can be reused for stdout capture verification.
- `runCmdInner` callers in `watchCmd` need the `report` variable handled (passed through but not used — watch mode typically does not use `--report`).

---

### Step 5: Update Help Text and Smoke Test
**Rationale:** Last step because it's purely cosmetic/verification and depends on everything else working.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Update `printHelp()` to show `junit` in format options and `--report` flag |
| `smoke/run.sh` | modify | Add JUnit XML smoke test section |

#### Current Help Text
```go
fmt.Println("  --format <type>     Output format: terminal (default), json, tap")
```

#### New Help Text
```go
fmt.Println("  --format <type>     Output format: terminal (default), json, tap, junit")
fmt.Println("  --report <file>     Write report to file instead of stdout (for --format junit)")
```

#### Smoke Test Addition
```bash
echo "--- Running with --format junit (expect valid XML, gated at free tier) ---"
JUNIT_EXIT=$(./curlew run "$SOME_FILE" --format junit 2>&1; echo "EXIT:$?")
echo "$JUNIT_EXIT" | grep -q "EXIT:6" && echo "PASS: --format junit gated at free tier" || { echo "FAIL: Expected exit 6"; exit 1; }

echo "--- Help text shows junit in --format ---"
./curlew --help | grep -q "junit" && echo "PASS: junit in help" || { echo "FAIL: Missing junit in help"; exit 1; }

echo "--- Help text shows --report ---"
./curlew --help | grep -q "\-\-report" && echo "PASS: --report in help" || { echo "FAIL: Missing --report in help"; exit 1; }
```

#### Impact on Existing Tests
- No existing tests affected — only adding new lines.

---

### Step 6: Handle Early Bailout and Error Paths for JUnit Format
**Rationale:** Ensure JUnit format handles the same error paths as JSON/TAP: parse errors, empty collections, env loading errors, etc.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `format == "junit"` branches alongside existing `format == "json"` and `format == "tap"` error handling branches |

#### Current Pattern (repeated ~5 times in `runCmdInner`)
```go
if format == "json" {
    jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, err, output.VerbosityDefault)
    _ = output.WriteJSON(os.Stdout, jsonOut)
    return 3, nil
}
if format == "tap" {
    _ = writeTAPBailout(os.Stdout, err)
    return 3, nil
}
```

#### New Pattern
```go
if format == "json" {
    jsonOut := buildJSONOutput(col.Name, nil, &runner.Summary{}, err, output.VerbosityDefault)
    _ = output.WriteJSON(os.Stdout, jsonOut)
    return 3, nil
}
if format == "tap" {
    _ = writeTAPBailout(os.Stdout, err)
    return 3, nil
}
if format == "junit" {
    _ = writeJUnitError(os.Stdout, err)
    return 3, nil
}
```

Where `writeJUnitError` creates a minimal JUnit XML with a single error testcase:
```go
func writeJUnitError(w io.Writer, err error) error {
    suites := &output.JUnitTestSuites{
        TestSuites: []output.JUnitTestSuite{{
            Name:  "curlew",
            Tests: 1,
            Errors: 1,
            TestCases: []output.JUnitTestCase{{
                Name:      "initialization",
                ClassName: "curlew",
                Error: &output.JUnitError{
                    Message: err.Error(),
                    Type:    "InitializationError",
                },
            }},
        }},
    }
    return output.WriteJUnitXML(w, suites)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_format_junit_parse_error(t *testing.T) {
    // Invalid YAML file with --format junit → valid XML with error element, exit 3
}

func TestRunCmd_format_junit_empty_collection(t *testing.T) {
    // Empty collection with --format junit → valid XML with 0 tests, exit 0
}

func TestRunCmd_format_junit_missing_file(t *testing.T) {
    // Missing file with --format junit → valid XML with error element, exit 3
}
```

#### Impact on Existing Tests
- No existing tests affected — adding new branches in existing code paths.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/gate_test.go` | (new) `TestCheckFeature_junitXML` | new test | write |
| `internal/output/junit_test.go` | (new) all | new file | write |
| `cmd/curlew/run_test.go` | `TestParseRunArgs_*` (if any exist) | breaks | update for new `report` return value |
| `cmd/curlew/main_test.go` | (new) `TestRunCmd_format_junit_*` | new tests | write |
| `cmd/curlew/main.go` | `watchCmd` | may break | update `parseRunArgs` call site |

## Risks and Edge Cases
- **Risk:** `parseRunArgs` signature change breaks multiple call sites → **Mitigation:** Update all call sites (`runCmdInner`, `watchCmd`) in the same step; search for all references before committing.
- **Risk:** `--report` with invalid path (permissions, directory doesn't exist) → **Mitigation:** Use `os.Create` which returns descriptive errors; wrap with structured error.
- **Edge case:** `--report` with `--format` other than `junit` → **Handling:** `--report` is only meaningful for `--format junit`; for other formats, silently ignore (or warn). Decision: silently ignore, matching typical CI tool behavior where `--report` only applies to XML formats.
- **Edge case:** `--format junit --report -` (stdout explicitly) → **Handling:** Treat `-` as stdout (standard Unix convention). Or simply: do not special-case; if someone passes `-` as filename, write to a file named `-`. Decision: write to file named `-` for simplicity; not worth the complexity of special-casing.
- **Edge case:** Empty results (no requests) with JUnit → **Handling:** Produce valid XML with `tests="0"` and no testcase elements.
- **Edge case:** Very long assertion failure messages → **Handling:** Include full messages; XML encoding handles special characters via `encoding/xml`.
- **Edge case:** XML special characters in request names (`<`, `>`, `&`, `"`) → **Handling:** `encoding/xml` automatically escapes these in attribute values and text content.
- **Risk:** `--report` file creation fails mid-write → **Mitigation:** Write to buffer first, then flush to file atomically. Decision: For simplicity, write directly; partial files are acceptable since CI tools typically check exit code.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Feature gate at free tier:
./curlew run tests.yaml --format junit
# Expected: exit code 6, feature gate message

# At professional tier (requires test override):
# Run go test with currentTier override, verify valid JUnit XML output.

# File output:
./curlew run tests.yaml --format junit --report results.xml
# Expected: results.xml contains valid JUnit XML, stdout is empty

# XML validation:
xmllint --noout results.xml
# Expected: validates successfully

# Unit tests:
go test ./internal/output/... -run TestWriteJUnitXML
go test ./internal/auth/... -run TestCheckFeature_junitXML
go test ./cmd/curlew/... -run TestRunCmd_format_junit
```
