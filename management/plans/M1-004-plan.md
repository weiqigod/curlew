# Implementation Plan: M1-004

## Overview

Add status code assertions to the execution pipeline. Requests can declare `assertions.status` (single int or list) and the tool reports pass/fail with `✓`/`✗` indicators and uses exit code 1 for assertion failures (distinct from exit code 4 for network errors).

## Task Details
- **ID:** M1-004
- **Title:** Assert on status code
- **Phase:** M1: Core CLI
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-001 | Run a single GET request from a collection file | done |

## Implementation Steps

### Step 1: Create `internal/assertion/` — Core Types and Status Check

**Rationale:** Pure logic with zero impact on existing code. Smallest blast radius.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | create | Result types and CheckStatus function |
| `internal/assertion/assertion_test.go` | create | Table-driven tests for status assertion |

#### New Code

```go
package assertion

import (
	"fmt"
	"strings"
)

// Result represents the outcome of a single assertion.
type Result struct {
	Type     string // e.g. "status"
	Expected string // human-readable expected value
	Actual   string // human-readable actual value
	Passed   bool
}

// Results is a collection of assertion outcomes for one request.
type Results struct {
	Items  []Result
	Passed bool // true if all items passed (or empty)
}

// CheckStatus evaluates whether actual matches any of the expected status codes.
// Returns nil if expected is empty (no assertion).
func CheckStatus(expected []int, actual int) *Result {
	if len(expected) == 0 {
		return nil
	}
	for _, code := range expected {
		if code == actual {
			return &Result{
				Type:     "status",
				Expected: formatCodes(expected),
				Actual:   fmt.Sprintf("%d", actual),
				Passed:   true,
			}
		}
	}
	return &Result{
		Type:     "status",
		Expected: formatCodes(expected),
		Actual:   fmt.Sprintf("%d", actual),
		Passed:   false,
	}
}

func formatCodes(codes []int) string {
	if len(codes) == 1 {
		return fmt.Sprintf("%d", codes[0])
	}
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = fmt.Sprintf("%d", c)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// Evaluate runs all assertions for a request and returns the aggregated results.
// Returns nil if no assertions are defined.
func Evaluate(statusCodes []int, actualStatus int) *Results {
	r := CheckStatus(statusCodes, actualStatus)
	if r == nil {
		return nil
	}
	return &Results{
		Items:  []Result{*r},
		Passed: r.Passed,
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckStatus(t *testing.T) {
	tests := []struct {
		name     string
		expected []int
		actual   int
		wantNil  bool
		wantPass bool
	}{
		{"single match 200", []int{200}, 200, false, true},
		{"single mismatch 200 vs 404", []int{200}, 404, false, false},
		{"list match first", []int{200, 201}, 200, false, true},
		{"list match second", []int{200, 201}, 201, false, true},
		{"list mismatch", []int{200, 201}, 404, false, false},
		{"exact 404 match", []int{404}, 404, false, true},
		{"empty expected returns nil", []int{}, 200, true, false},
		{"nil expected returns nil", nil, 200, true, false},
	}
	// ...
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name       string
		codes      []int
		actual     int
		wantNil    bool
		wantPassed bool
	}{
		{"no assertions returns nil", nil, 200, true, false},
		{"passing assertion", []int{200}, 200, false, true},
		{"failing assertion", []int{200}, 404, false, false},
	}
	// ...
}

func TestFormatCodes(t *testing.T) {
	// tested indirectly via Result.Expected in CheckStatus tests
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 2: Add `Assertions` to Parser Structs with Custom YAML Unmarshaling

**Rationale:** Extends the data model without changing behavior. Existing collections without `assertions` parse identically.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add Assertions struct and StatusCodes custom type to RequestItem |
| `internal/parser/parser_test.go` | modify | Add test cases for assertion parsing |
| `internal/parser/testdata/with_status_assertion.yaml` | create | Test fixture: single status |
| `internal/parser/testdata/with_status_list_assertion.yaml` | create | Test fixture: status list |

#### Current Code

```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name    string  `yaml:"name"`
	Request Request `yaml:"request"`
}
```

#### New Code

```go
// Assertions holds the assertion definitions for a request.
type Assertions struct {
	Status StatusCodes `yaml:"status,omitempty"`
}

// StatusCodes handles both `status: 200` and `status: [200, 201]` in YAML.
type StatusCodes struct {
	Codes []int
}

// UnmarshalYAML handles scalar int, string-as-int, and sequence forms.
func (s *StatusCodes) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var code int
		if err := value.Decode(&code); err != nil {
			return fmt.Errorf("invalid status code %q: %w", value.Value, err)
		}
		s.Codes = []int{code}
		return nil
	case yaml.SequenceNode:
		var codes []int
		if err := value.Decode(&codes); err != nil {
			return fmt.Errorf("invalid status code list: %w", err)
		}
		s.Codes = codes
		return nil
	default:
		return fmt.Errorf("status must be an integer or list of integers")
	}
}

// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string     `yaml:"name"`
	Request    Request    `yaml:"request"`
	Assertions Assertions `yaml:"assertions,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
// In parser_test.go, add to existing table or new test function:
{"parses single status assertion", "testdata/with_status_assertion.yaml", ...},
{"parses list status assertion", "testdata/with_status_list_assertion.yaml", ...},
{"no assertions field parses to empty", "testdata/basic.yaml", ...},  // existing fixture
```

Test fixture `with_status_assertion.yaml`:
```yaml
name: Status Assert Test
requests:
  - name: Check Status
    request:
      method: GET
      url: "https://example.com"
    assertions:
      status: 200
```

Test fixture `with_status_list_assertion.yaml`:
```yaml
name: Status List Assert Test
requests:
  - name: Check Status
    request:
      method: GET
      url: "https://example.com"
    assertions:
      status: [200, 201]
```

#### Impact on Existing Tests
- No existing tests affected — `Assertions` is `omitempty`, existing fixtures have no `assertions` field, so zero value is used

---

### Step 3: Integrate Assertions into the Runner

**Rationale:** Wires assertion logic into the execution loop. All existing tests remain green because no-assertions = always-pass.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add AssertionResults to RequestResult, AssertionFailures to Summary, evaluate assertions after successful execution |
| `internal/runner/runner_test.go` | modify | Add assertion-related test cases, extend makeCollection helper |

#### Current Code

```go
type RequestResult struct {
	Name    string
	Result  *httpexec.Result
	Err     error
	Skipped bool
}

type Summary struct {
	Total    int
	Passed   int
	Failed   int
	Skipped  int
	Duration time.Duration
}

// In the Run loop, success path:
results = append(results, RequestResult{Name: item.Name, Result: result})
summary.Passed++
```

#### New Code

```go
type RequestResult struct {
	Name             string
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	AssertionResults *assertion.Results // nil if no assertions defined or skipped/error
}

type Summary struct {
	Total             int
	Passed            int
	Failed            int
	Skipped           int
	AssertionFailures int // subset of Failed that are assertion failures (not network errors)
	Duration          time.Duration
}

// In the Run loop, success path:
ar := assertion.Evaluate(item.Assertions.Status.Codes, result.StatusCode)
rr := RequestResult{Name: item.Name, Result: result, AssertionResults: ar}

if ar != nil && !ar.Passed {
	summary.Failed++
	summary.AssertionFailures++
	if col.Options.StopOnFailure {
		stopped = true
	}
} else {
	summary.Passed++
}
results = append(results, rr)
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_assertions(t *testing.T) {
	tests := []struct {
		name                  string
		statusCodes           []int    // assertion status codes
		wantPassed            int
		wantFailed            int
		wantAssertionFailures int
		wantResultPassed      bool     // AssertionResults.Passed on the result
	}{
		{"passing status assertion", []int{200}, 1, 0, 0, true},
		{"failing status assertion", []int{201}, 0, 1, 1, false},
		{"no assertions always passes", nil, 1, 0, 0, false},  // AssertionResults is nil
		{"list assertion match", []int{200, 201}, 1, 0, 0, true},
		{"list assertion mismatch", []int{201, 202}, 0, 1, 1, false},
	}
	// Uses a helper that creates a collection with assertions on the request item
}

func TestRun_assertion_failure_with_stop_on_failure(t *testing.T) {
	// 3 requests, second has failing assertion, stop_on_failure=true
	// Expected: 1 passed, 1 failed (assertion), 1 skipped
}

func TestRun_assertion_failure_increments_only_assertion_failures(t *testing.T) {
	// 2 requests: first has network error, second has assertion failure
	// summary.Failed=2, summary.AssertionFailures=1
}
```

#### Impact on Existing Tests
- `makeCollection` helper produces `RequestItem` with zero-value `Assertions` (empty `StatusCodes.Codes`) → `Evaluate` returns nil → same behavior as before → all existing tests pass unchanged
- `Summary` struct gains `AssertionFailures` field (defaults to 0) → existing assertions on `Summary` fields remain valid

---

### Step 4: Update Output to Show Pass/Fail Indicators

**Rationale:** Changes visual output. Done after runner changes so tests can verify the full pipeline.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `passed` parameter to PrintResult, add PrintAssertionDetail function |
| `internal/output/terminal_test.go` | modify | Update existing tests, add new indicator tests |

#### Current Code

```go
func PrintResult(w io.Writer, name string, result *httpexec.Result) {
	_, _ = fmt.Fprintf(w, "  %s  %d  %dms\n", name, result.StatusCode, result.Duration.Milliseconds())
}
```

#### New Code

```go
// PrintResult writes a single request result line with pass/fail indicator.
func PrintResult(w io.Writer, name string, result *httpexec.Result, passed bool) {
	indicator := "✓"
	if !passed {
		indicator = "✗"
	}
	_, _ = fmt.Fprintf(w, "  %s %s  %d  %dms\n", indicator, name, result.StatusCode, result.Duration.Milliseconds())
}

// PrintAssertionDetail writes an assertion failure detail line.
func PrintAssertionDetail(w io.Writer, assertType, expected, actual string) {
	_, _ = fmt.Fprintf(w, "    ✗ %s: expected %s, got %s\n", assertType, expected, actual)
}
```

#### Tests to Write FIRST (RED phase)

```go
// Update existing TestPrintResult to include passed=true (they should still contain name, status, duration)
// Add new tests:
{"passed indicator shown", "Get Users", result200, true, "✓"},
{"failed indicator shown", "Get Users", result200, false, "✗"},

func TestPrintAssertionDetail(t *testing.T) {
	tests := []struct {
		name       string
		assertType string
		expected   string
		actual     string
		wantParts  []string
	}{
		{"status mismatch", "status", "200", "404", []string{"✗", "status", "expected 200", "got 404"}},
	}
}
```

#### Impact on Existing Tests
- `TestPrintResult` — call sites must add `passed` parameter. All existing test cases use successful requests → pass `true`. The test assertions use `strings.Contains` for name/status/duration, which still match. The output now has a `✓` prefix, but existing checks don't look for the exact prefix position.

---

### Step 5: Wire Everything in `cmd/curlew/main.go`

**Rationale:** Final wiring — depends on all previous steps being complete.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Pass assertion results to output, print assertion details, distinguish exit codes 1 vs 4 |
| `cmd/curlew/run_test.go` | modify | Add assertion pass/fail test cases, update PrintResult calls |
| `cmd/curlew/main_test.go` | modify | Add integration tests for assertions |

#### Current Code

```go
// In runCmd, the result output loop:
default:
	output.PrintResult(os.Stdout, r.Name, r.Result)

// Exit code logic:
if summary.Failed > 0 {
	return 4
}
return 0
```

#### New Code

```go
// In runCmd, the result output loop:
default:
	passed := r.AssertionResults == nil || r.AssertionResults.Passed
	output.PrintResult(os.Stdout, r.Name, r.Result, passed)
	if r.AssertionResults != nil {
		for _, ar := range r.AssertionResults.Items {
			if !ar.Passed {
				output.PrintAssertionDetail(os.Stdout, ar.Type, ar.Expected, ar.Actual)
			}
		}
	}

// Exit code logic:
if summary.AssertionFailures > 0 {
	return 1
}
if summary.Failed > 0 {
	return 4
}
return 0
```

#### Tests to Write FIRST (RED phase)

```go
// run_test.go additions:
func TestRunCmd_assertion_pass(t *testing.T) {
	// Server returns 200, collection asserts status: 200 → exit code 0
}

func TestRunCmd_assertion_fail(t *testing.T) {
	// Server returns 200, collection asserts status: 201 → exit code 1
}

func TestRunCmd_assertion_fail_vs_network_error(t *testing.T) {
	// Assertion failure → exit 1, network error → exit 4
}

// main_test.go integration additions:
func TestCLIIntegration_assertion_pass(t *testing.T) {
	// Binary test: server returns 200, assert status: 200
	// Check: exit 0, output contains ✓
}

func TestCLIIntegration_assertion_fail(t *testing.T) {
	// Binary test: server returns 200, assert status: 404
	// Check: exit 1, output contains ✗, output contains assertion detail
}

func TestCLIIntegration_assertion_list(t *testing.T) {
	// Binary test: server returns 201, assert status: [200, 201]
	// Check: exit 0, output contains ✓
}

func TestCLIIntegration_no_assertions_still_passes(t *testing.T) {
	// Binary test: no assertions → exit 0, output contains ✓
}
```

#### Impact on Existing Tests
- `TestRunCmd_successful_request` — exit code 0 unchanged (no assertions = pass)
- `TestRunCmd_network_error` — exit code 4 unchanged (network error, no assertions)
- `TestCLIIntegration_successful_run` — checks for `"200"`, `"Get Test Server"`, `"1 passed"`. Output now includes `✓` prefix. All `strings.Contains` checks still match. Exit code 0 unchanged.
- `TestCLIIntegration_stop_on_failure` — network error + stop_on_failure. Exit code 4 unchanged (no assertion failures, only network error).
- `TestCLIIntegration_network_error` — exit code 4 unchanged.

---

### Step 6: Update Smoke Test and Sample Collection

**Rationale:** Final polish — validates the full binary with assertions.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add assertion smoke test cases |
| `sample/hello.yaml` | modify | Add status assertions to existing requests |

#### New Code (smoke/run.sh additions)

```bash
echo "--- Running collection with status assertion (expect pass) ---"
ASSERT_FILE=$(mktemp /tmp/curlew_assert_XXXXXX.yaml)
cat > "$ASSERT_FILE" << YAML
name: Assert Pass
requests:
  - name: Check Status
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 200
YAML
./curlew run "$ASSERT_FILE"
echo "Exit code: $?"
rm -f "$ASSERT_FILE"
echo

echo "--- Running collection with failing assertion (expect exit 1) ---"
ASSERT_FAIL_FILE=$(mktemp /tmp/curlew_assert_fail_XXXXXX.yaml)
cat > "$ASSERT_FAIL_FILE" << YAML
name: Assert Fail
requests:
  - name: Check Status
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 404
YAML
./curlew run "$ASSERT_FAIL_FILE" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$ASSERT_FAIL_FILE"
echo
```

#### sample/hello.yaml update

```yaml
requests:
  - name: Get httpbin
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 200

  - name: Post with JSON body
    request:
      method: POST
      url: "https://httpbin.org/post"
      headers:
        Accept: application/json
      query:
        source: curlew
      body:
        message: "Hello from Curlew"
        timestamp: "2026-01-01T00:00:00Z"
    assertions:
      status: 200
```

#### Impact on Existing Tests
- Smoke test: adds new sections, does not modify existing ones
- Sample: adds assertions to existing requests (backward compatible — was passing without them)

---

### Step 7: Update CHANGELOG.md

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add M1-004 entries under [Unreleased] |

```markdown
### Added
- Status code assertions: `assertions.status` supports single int or list of ints (M1-004)
- Pass/fail indicators (✓/✗) in request output (M1-004)
- Exit code 1 for assertion failures, distinct from exit code 4 for network errors (M1-004)
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/output/terminal_test.go` | `TestPrintResult` | signature change | add `true` as 4th arg to `PrintResult` calls |
| `cmd/curlew/run_test.go` | all `TestRunCmd_*` | none | exit codes unchanged |
| `cmd/curlew/main_test.go` | `TestCLIIntegration_successful_run` | output format | still passes — uses `strings.Contains` |
| `cmd/curlew/main_test.go` | `TestCLIIntegration_stop_on_failure` | none | network error, exit 4 unchanged |
| `cmd/curlew/main_test.go` | `TestCLIIntegration_network_error` | none | exit 4 unchanged |
| `internal/runner/runner_test.go` | all `TestRun*` | none | no assertions = always pass, Summary gains field with zero default |

## Risks and Edge Cases

- **Risk:** `PrintResult` signature change breaks callers → **Mitigation:** Only 2 call sites (output_test.go, main.go). Both updated in the same step as the signature change.
- **Risk:** YAML type ambiguity (`status: "200"` string vs int) → **Mitigation:** Custom `UnmarshalYAML` with `yaml.Node` handles scalar → int decode. YAML `"200"` is quoted string; Go yaml library decodes it as int when target is int. If truly non-numeric, returns clear error.
- **Edge case:** Empty status list `status: []` → `Codes` is `[]int{}` (length 0) → `CheckStatus` returns nil → no assertion → passes. Correct behavior.
- **Edge case:** No response (network error) → assertions never evaluated → counts as network error (exit 4). Correct.
- **Edge case:** `status: 0` → valid YAML, assertion will always fail since HTTP status codes start at 100. No special handling needed — user wrote a bad assertion.
- **Risk:** Unicode `✓`/`✗` on Windows terminals → **Mitigation:** Out of scope. Go outputs UTF-8; modern Windows Terminal handles it. Could add ASCII fallback later (M1-019 output formats).

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create test collection with passing assertion
cat > /tmp/assert_test.yaml << 'EOF'
name: Assertion Test
requests:
  - name: Check httpbin
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 200
EOF
./curlew run /tmp/assert_test.yaml
# Expected: ✓ Check httpbin  200  Xms, exit code 0

# Modify to expect wrong status
cat > /tmp/assert_test.yaml << 'EOF'
name: Assertion Test
requests:
  - name: Check httpbin
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 404
EOF
./curlew run /tmp/assert_test.yaml
# Expected: ✗ Check httpbin  200  Xms, assertion detail line, exit code 1
```
