# Implementation Plan: M1-021

## Overview
Add a TAP (Test Anything Protocol) version 13 output formatter so `apitest run --format tap` produces TAP-compatible output consumable by standard TAP harnesses.

## Task Details
- **ID:** M1-021
- **Title:** TAP output format (--format tap)
- **Phase:** M1: Core CLI
- **Priority:** 21
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-019 | Terminal colors and formatting | done |

---

## Implementation Steps

### Step 1: TAP formatter unit tests (RED)
**Rationale:** Smallest blast radius — pure new file, no existing code touched. Defines the contract for `WriteTAP` before any implementation exists.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/tap_test.go` | create | Table-driven tests for `WriteTAP` and TAP types |

#### Tests to Write FIRST (RED phase)

```go
func TestWriteTAP(t *testing.T) {
    tests := []struct {
        name    string
        results []TAPResult
        passed  int
        failed  int
        check   func(t *testing.T, out string)
    }{
        {
            name: "version line and plan line emitted",
            results: []TAPResult{{Name: "req", Passed: true, DurationMs: 50}},
            passed: 1, failed: 0,
            check: func(t *testing.T, out string) {
                if !strings.HasPrefix(out, "TAP version 13\n") { ... }
                if !strings.Contains(out, "1..1\n") { ... }
            },
        },
        {
            name: "passing request emits ok line with duration",
            ...
        },
        {
            name: "failing request emits not ok line with YAML diagnostic block",
            results: []TAPResult{{
                Name:     "Get User",
                Passed:   false,
                Failures: []TAPFailure{{Type: "status", Expected: "200", Actual: "404"}},
            }},
            check: func(t *testing.T, out string) {
                // not ok 1 - Get User
                // followed by --- / ... YAML block with failures
            },
        },
        {
            name: "skipped request emits ok line with SKIP directive",
            results: []TAPResult{{Name: "req", Skipped: true}},
            check: func(t *testing.T, out string) {
                // ok 1 - req # SKIP
            },
        },
        {
            name: "error request emits not ok line with error diagnostic",
            results: []TAPResult{{Name: "req", Passed: false, Error: "connection refused"}},
            check: func(t *testing.T, out string) {
                // not ok 1 - req
                // --- / error: 'connection refused' / ...
            },
        },
        {
            name: "no results - plan is 1..0",
            results: nil,
            passed: 0, failed: 0,
            check: func(t *testing.T, out string) {
                // 1..0
            },
        },
        {
            name: "mixed pass and fail - correct numbering",
            // 3 results: pass, fail, pass → ok 1, not ok 2, ok 3
        },
        {
            name: "multiple assertion failures all appear in diagnostic block",
            results: []TAPResult{{
                Name:   "req",
                Passed: false,
                Failures: []TAPFailure{
                    {Type: "status", Expected: "200", Actual: "404"},
                    {Type: "body $.id equals", Expected: "1", Actual: "2"},
                },
            }},
        },
        {
            name: "summary comment at end",
            // # Summary: N passed, M failed
        },
        {
            name: "hash in test name does not break TAP parsing",
            results: []TAPResult{{Name: "req # with hash", Passed: true}},
            check: func(t *testing.T, out string) {
                // name must be sanitized or quoted so # is not interpreted as directive
            },
        },
        {
            name: "request with no assertions is ok",
            results: []TAPResult{{Name: "req", Passed: true, DurationMs: 0}},
            check: func(t *testing.T, out string) {
                // ok 1 - req
            },
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            var buf bytes.Buffer
            if err := WriteTAP(&buf, tc.results, tc.passed, tc.failed); err != nil {
                t.Fatalf("WriteTAP error: %v", err)
            }
            tc.check(t, buf.String())
        })
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new file only)

---

### Step 2: TAP formatter implementation (GREEN)
**Rationale:** Implement only what the tests in Step 1 require. Self-contained new file, no existing code changed.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/tap.go` | create | `TAPResult`, `TAPFailure` types and `WriteTAP` function |

#### New Code

```go
package output

import (
    "fmt"
    "io"
    "strings"
)

// TAPResult represents one test point in TAP version 13 output.
type TAPResult struct {
    Name       string
    Passed     bool
    Skipped    bool
    Error      string       // non-empty for execution errors (not assertion failures)
    Failures   []TAPFailure // individual assertion failures (only when !Passed && Error == "")
    DurationMs int64        // 0 for skipped/error requests
}

// TAPFailure represents a single assertion failure for TAP YAML diagnostics.
type TAPFailure struct {
    Type     string
    Expected string
    Actual   string
}

// WriteTAP writes TAP version 13 formatted output to w.
// The plan line uses len(results) as the total test count.
// A trailing comment line summarises pass/fail counts.
func WriteTAP(w io.Writer, results []TAPResult, passed, failed int) error {
    if _, err := fmt.Fprintln(w, "TAP version 13"); err != nil {
        return err
    }
    if _, err := fmt.Fprintf(w, "1..%d\n", len(results)); err != nil {
        return err
    }

    for i, r := range results {
        n := i + 1
        name := sanitizeTAPName(r.Name)

        switch {
        case r.Skipped:
            if _, err := fmt.Fprintf(w, "ok %d - %s # SKIP\n", n, name); err != nil {
                return err
            }
        case !r.Passed:
            if _, err := fmt.Fprintf(w, "not ok %d - %s\n", n, name); err != nil {
                return err
            }
            if err := writeTAPDiagnostics(w, r); err != nil {
                return err
            }
        default:
            if r.DurationMs > 0 {
                if _, err := fmt.Fprintf(w, "ok %d - %s (%dms)\n", n, name, r.DurationMs); err != nil {
                    return err
                }
            } else {
                if _, err := fmt.Fprintf(w, "ok %d - %s\n", n, name); err != nil {
                    return err
                }
            }
        }
    }

    _, err := fmt.Fprintf(w, "# Summary: %d passed, %d failed\n", passed, failed)
    return err
}

// writeTAPDiagnostics writes a TAP 13 YAML diagnostic block for a failing result.
func writeTAPDiagnostics(w io.Writer, r TAPResult) error {
    if _, err := fmt.Fprintln(w, "  ---"); err != nil {
        return err
    }
    if r.Error != "" {
        if _, err := fmt.Fprintf(w, "  error: %q\n", r.Error); err != nil {
            return err
        }
    } else if len(r.Failures) > 0 {
        if _, err := fmt.Fprintln(w, "  failures:"); err != nil {
            return err
        }
        for _, f := range r.Failures {
            if _, err := fmt.Fprintf(w, "    - type: %q\n      expected: %q\n      actual: %q\n",
                f.Type, f.Expected, f.Actual); err != nil {
                return err
            }
        }
    }
    _, err := fmt.Fprintln(w, "  ...")
    return err
}

// sanitizeTAPName removes characters that could break TAP parsing.
// TAP uses '#' as a directive delimiter, so we strip it from names.
func sanitizeTAPName(name string) string {
    name = strings.ReplaceAll(name, "#", "")
    name = strings.ReplaceAll(name, "\n", " ")
    return strings.TrimSpace(name)
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 3: Integration tests in main_test.go (RED)
**Rationale:** Tests for the wiring in `main.go` must be written before the wiring code. Follows the existing `TestRunCmdDirect_JSON*` pattern using `captureRunCmd`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main_test.go` | modify | Add TAP integration tests and `TestBuildTAPOutput` |

#### Current Code
```go
// Existing tests follow the pattern:
func TestRunCmdDirect_JSONSuccess(t *testing.T) { ... }
func TestRunCmdDirect_JSONAssertionFailure(t *testing.T) { ... }
```

#### New Code (tests to add)

```go
func TestRunCmdDirect_TAPSuccess(t *testing.T) {
    // Use an existing fixture collection that passes
    // Verify: stdout starts with "TAP version 13", has "1..", has "ok 1"
    // Verify: exit code 0
}

func TestRunCmdDirect_TAPAssertionFailure(t *testing.T) {
    // Fixture collection with a failing assertion
    // Verify: "not ok" line present, YAML diagnostic block present
    // Verify: exit code 1
}

func TestRunCmdDirect_TAPParseError(t *testing.T) {
    // Non-existent file with --format tap
    // Verify: "TAP version 13\nBail out!" in stdout
    // Verify: exit code 3
}

func TestRunCmdDirect_TAPEmptyCollection(t *testing.T) {
    // Empty collection (no requests) with --format tap
    // Verify: "1..0" in stdout
    // Verify: exit code 0
}

func TestBuildTAPOutput(t *testing.T) {
    // Table-driven: test conversion of runner.RequestResult to []output.TAPResult
    // Cases: passing, failing with assertions, skipped, error
}
```

#### Impact on Existing Tests
- `TestRunCmdDirect_UnknownFormat` may check the error message string. After wiring TAP, the supported formats string in the error message changes from `"supported: terminal, json"` to `"supported: terminal, json, tap"`. Must update this test.

---

### Step 4: Wire TAP format into main.go (GREEN)
**Rationale:** Now implement the wiring that makes the Step 3 tests pass. Touch the most central file last to minimise risk.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Format validation, pre-exec error handling, post-exec TAP output, help text |

#### Current Code (format validation, line 131)
```go
if format != "" && format != "json" && format != "terminal" {
    errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
    errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json)", format))
    return 1
}
```

#### New Code (format validation)
```go
if format != "" && format != "json" && format != "terminal" && format != "tap" {
    errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
    errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json, tap)", format))
    return 1
}
```

#### Current Code (parse error handling, line 141)
```go
col, err := parser.ParseFile(file)
if err != nil {
    if format == "json" {
        jsonOut := buildJSONOutput("", nil, &runner.Summary{}, err)
        _ = output.WriteJSON(os.Stdout, jsonOut)
        return 3
    }
    errOut.StructuredError(err)
    return 3
}
```

#### New Code (parse error handling)
```go
col, err := parser.ParseFile(file)
if err != nil {
    if format == "json" {
        jsonOut := buildJSONOutput("", nil, &runner.Summary{}, err)
        _ = output.WriteJSON(os.Stdout, jsonOut)
        return 3
    }
    if format == "tap" {
        _ = writeTAPBailout(os.Stdout, err)
        return 3
    }
    errOut.StructuredError(err)
    return 3
}
```

Apply the same `format == "tap"` / `writeTAPBailout` branches to the three other pre-exec error sites (empty collection, env load error, project config error, dotenv error). For empty collection with TAP, emit `1..0` instead of bail-out.

#### Current Code (post-exec JSON output block, line 225)
```go
if format == "json" {
    jsonOut := buildJSONOutput(col.Name, results, summary, varErr)
    ...
    return 0
}
```

#### New Code (post-exec TAP block — add immediately after JSON block)
```go
if format == "tap" {
    tapResults := buildTAPOutput(results)
    passed := summary.Passed
    failed := summary.Failed
    if writeErr := output.WriteTAP(os.Stdout, tapResults, passed, failed); writeErr != nil {
        _, _ = fmt.Fprintf(os.Stderr, "tap encode error: %v\n", writeErr)
        return 1
    }
    if varErr != nil {
        return 5
    }
    if summary != nil {
        mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
        mainFailed := summary.Failed - summary.TeardownErrors
        if mainAssertionFailed > 0 {
            return 1
        }
        if mainFailed > 0 {
            return 4
        }
    }
    return 0
}
```

#### New helper functions to add at end of main.go

```go
// buildTAPOutput converts runner results to TAP result structs.
func buildTAPOutput(results []runner.RequestResult) []output.TAPResult {
    out := make([]output.TAPResult, 0, len(results))
    for _, r := range results {
        tr := output.TAPResult{Name: r.Name}
        switch {
        case r.Skipped:
            tr.Skipped = true
            tr.Passed = true
        case r.Err != nil:
            tr.Passed = false
            tr.Error = r.Err.Error()
        default:
            if r.Result != nil {
                tr.DurationMs = r.Result.Duration.Milliseconds()
            }
            if r.AssertionResults != nil {
                tr.Passed = r.AssertionResults.Passed
                if !r.AssertionResults.Passed {
                    for _, ar := range r.AssertionResults.Items {
                        if !ar.Passed {
                            tr.Failures = append(tr.Failures, output.TAPFailure{
                                Type:     ar.Type,
                                Expected: ar.Expected,
                                Actual:   ar.Actual,
                            })
                        }
                    }
                }
            } else {
                tr.Passed = true
            }
        }
        out = append(out, tr)
    }
    return out
}

// writeTAPBailout writes a TAP bail-out message for pre-execution errors.
func writeTAPBailout(w io.Writer, err error) error {
    if _, wErr := fmt.Fprintln(w, "TAP version 13"); wErr != nil {
        return wErr
    }
    _, wErr := fmt.Fprintf(w, "Bail out! %s\n", err.Error())
    return wErr
}
```

#### Help text update (line 395)
```go
// Before:
fmt.Println("  --format <type>     Output format: terminal (default), json")
// After:
fmt.Println("  --format <type>     Output format: terminal (default), json, tap")
```

#### Impact on Existing Tests
- `TestRunCmdDirect_UnknownFormat`: update expected error message from `"supported: terminal, json"` to `"supported: terminal, json, tap"`

---

### Step 5: Smoke test (GREEN)
**Rationale:** Exercises the real binary end-to-end; validates TAP output is parseable and complete.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add `--format tap` smoke test sections |

#### New Code (add after existing `--format json` smoke tests)

```bash
# --- TAP format ---
echo "--- TAP: passing collection ---"
TAP_OUT=$(./apitest run "$SMOKE_COLLECTION" --format tap 2>&1)
echo "$TAP_OUT" | grep -q "^TAP version 13" || fail "--format tap: missing version line"
echo "$TAP_OUT" | grep -q "^1\.\." || fail "--format tap: missing plan line"
echo "$TAP_OUT" | grep -q "^ok 1" || fail "--format tap: missing ok line"
echo "$TAP_OUT" | grep -q "^# Summary:" || fail "--format tap: missing summary comment"

echo "--- TAP: help text lists tap ---"
./apitest --help | grep -q "tap" || fail "--help missing tap in --format description"
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/apitest/main_test.go` | `TestRunCmdDirect_UnknownFormat` | breaks | Update expected message to include `tap` |
| `internal/output/tap_test.go` | all new tests | new | Write in RED phase |
| `cmd/apitest/main_test.go` | `TestRunCmdDirect_TAP*` (new) | new | Write in RED phase |
| All other existing tests | — | none | No action required |

---

## Risks and Edge Cases

| Risk / Edge Case | Mitigation |
|---|---|
| TAP consumers ignoring TAP 13 YAML diagnostics | Core `ok`/`not ok` lines are TAP 12 compatible; YAML blocks are indented so older parsers skip them |
| `#` in request names parsed as TAP directive | `sanitizeTAPName` strips `#` characters from names |
| Newlines in request names breaking line-oriented parser | `sanitizeTAPName` replaces newlines with spaces |
| Zero assertions on a request — pass or fail? | Follows existing runner convention: no assertions + no error = passed (`buildTAPOutput` sets `Passed: true` when `AssertionResults == nil`) |
| Skipped requests count in plan but show as `ok # SKIP` | TAP convention: skipped tests are `ok` with `# SKIP` directive, counted in plan |
| Pre-execution errors (parse, env, config) | `writeTAPBailout` emits `Bail out!` which is the TAP standard for fatal pre-test errors |
| Empty collection (no requests) | Emit `1..0` rather than bail-out; the collection loaded successfully, there's just nothing to run |
| `varErr` for TAP | Same exit-code logic as JSON: return 5 for variable resolution errors |
| Duration display | Show `(Nms)` suffix only for passing/non-skipped requests with `DurationMs > 0` |

---

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
./apitest run collection.yaml --format tap
# Expected output:
# TAP version 13
# 1..N
# ok 1 - Request Name (123ms)
# not ok 2 - Failing Request
#   ---
#   failures:
#     - type: "status"
#       expected: "200"
#       actual: "404"
#   ...
# # Summary: 1 passed, 1 failed
```
