# Implementation Plan: M1-006

## Overview
Add header assertion (equals, exists, matches) and timing assertion (max_duration_ms) to the assertion pipeline, including capturing response headers from HTTP execution.

## Task Details
- **ID:** M1-006
- **Title:** Assert on response headers and timing
- **Phase:** M1: Core CLI
- **Priority:** 6
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-004 | Assert on status code | done |

## Implementation Steps

### Step 1: Capture Response Headers in `httpexec.Result`
**Rationale:** Smallest blast radius — additive field, no signature changes, no existing test breakage.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/httpexec/executor.go` | modify | Add `Headers http.Header` field to `Result`, populate from `resp.Header` |
| `internal/httpexec/executor_test.go` | modify | Add tests verifying headers are captured |

#### Current Code
```go
type Result struct {
	StatusCode int
	Duration   time.Duration
	Body       []byte
}
```

#### New Code
```go
type Result struct {
	StatusCode int
	Duration   time.Duration
	Body       []byte
	Headers    http.Header
}
```

In `Execute()`, add `Headers: resp.Header` to the return struct.

#### Tests to Write FIRST (RED phase)

```go
func TestExecute_response_headers_captured(t *testing.T) {
	// Verify Content-Type and X-Custom headers are captured
}

func TestExecute_multiple_response_headers_captured(t *testing.T) {
	// Verify multiple headers including multi-value headers
}
```

#### Impact on Existing Tests
- No existing tests affected — additive field only

---

### Step 2: Parse Header and Timing Assertions from YAML
**Rationale:** Parser changes are isolated — no runtime behavior changes until wired in later steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `HeaderAssertion`, `HeaderAssertions`, `TimingAssertion` types; add fields to `Assertions` |
| `internal/parser/parser_test.go` | modify | Add parsing tests for header and timing assertion YAML |

#### Files to Create

| File | Description |
|------|-------------|
| `internal/parser/testdata/with_header_assertion_equals.yaml` | Test fixture |
| `internal/parser/testdata/with_header_assertion_exists.yaml` | Test fixture |
| `internal/parser/testdata/with_header_assertion_matches.yaml` | Test fixture |
| `internal/parser/testdata/with_timing_assertion.yaml` | Test fixture |
| `internal/parser/testdata/with_header_and_timing_assertion.yaml` | Test fixture |

#### New Types

```go
// HeaderAssertion represents a single assertion on a response header.
type HeaderAssertion struct {
	Name     string
	Operator string
	Value    any
}

// HeaderAssertions handles the YAML map-of-maps format for header assertions.
type HeaderAssertions struct {
	Items []HeaderAssertion
}

func (h *HeaderAssertions) UnmarshalYAML(value *yaml.Node) error

// TimingAssertion holds timing-related assertion configuration.
type TimingAssertion struct {
	MaxDurationMs int `yaml:"max_duration_ms"`
}
```

Updated `Assertions` struct:
```go
type Assertions struct {
	Status  StatusCodes      `yaml:"status,omitempty"`
	Headers HeaderAssertions `yaml:"headers,omitempty"`
	Body    BodyAssertions   `yaml:"body,omitempty"`
	Timing  TimingAssertion  `yaml:"timing,omitempty"`
}
```

`HeaderAssertions.UnmarshalYAML` follows the same map-of-maps pattern as `BodyAssertions.UnmarshalYAML` — key is header name, value is operator→expected map.

#### Tests to Write FIRST (RED phase)

```go
// Table-driven tests:
// - "TestParseFile_header_assertion_equals"
// - "TestParseFile_header_assertion_exists"
// - "TestParseFile_header_assertion_matches"
// - "TestParseFile_header_assertion_multiple_operators"
// - "TestParseFile_timing_assertion"
// - "TestParseFile_header_and_timing_combined"
// - "TestParseFile_no_header_assertions_parses_to_empty"
```

#### Impact on Existing Tests
- No existing tests affected — new fields have `omitempty`; existing YAML has no `headers` or `timing`

---

### Step 3: Implement Header Assertion Logic
**Rationale:** Core assertion logic before wiring — unit testable in isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `HeaderInput` type and `CheckHeaders()` function |
| `internal/assertion/assertion_test.go` | modify | Add table-driven tests for `CheckHeaders` |

#### New Code

```go
// HeaderInput describes a single header assertion to evaluate.
type HeaderInput struct {
	Name     string
	Operator string
	Value    string
}

// CheckHeaders evaluates header assertions against response headers.
// Header names are matched case-insensitively per HTTP spec.
func CheckHeaders(assertions []HeaderInput, headers http.Header) []Result
```

Key behaviors:
- **Case-insensitive names:** Use `http.Header.Get()` / `http.Header.Values()` which canonicalize names
- **`equals`:** Exact string comparison of header value
- **`exists`:** Check `len(headers.Values(name)) > 0`
- **`matches`:** `regexp.Compile()` then `MatchString()`; invalid regex returns `Result{Passed: false}` with error detail
- **Nil headers:** All assertions fail

#### Tests to Write FIRST (RED phase)

```go
func TestCheckHeaders(t *testing.T) {
	tests := []struct {
		name       string
		assertions []HeaderInput
		headers    http.Header
		wantCount  int
		wantPassed bool
	}{
		{"equals passes when header value matches", ...},
		{"equals fails when header value differs", ...},
		{"equals case-insensitive header name", ...},
		{"equals empty header value", ...},
		{"exists passes when header present", ...},
		{"exists fails when header absent", ...},
		{"exists case-insensitive header name", ...},
		{"matches passes when regex matches", ...},
		{"matches fails when regex does not match", ...},
		{"matches case-insensitive header name", ...},
		{"matches invalid regex returns failure", ...},
		{"nil assertions returns nil", ...},
		{"empty assertions returns nil", ...},
		{"nil headers fails all assertions", ...},
		{"multiple assertions all evaluated", ...},
		{"unsupported operator returns failure", ...},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 4: Implement Timing Assertion Logic
**Rationale:** Independent from headers — small, testable addition.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `CheckTiming()` function |
| `internal/assertion/assertion_test.go` | modify | Add table-driven tests for `CheckTiming` |

#### New Code

```go
// CheckTiming evaluates whether the response duration is within the allowed threshold.
// Returns nil if maxDurationMs is 0 or negative (no assertion).
func CheckTiming(maxDurationMs int, actual time.Duration) *Result
```

Uses `<=` comparison: `actual.Milliseconds() <= int64(maxDurationMs)` — "at threshold" passes.

#### Tests to Write FIRST (RED phase)

```go
func TestCheckTiming(t *testing.T) {
	tests := []struct {
		name          string
		maxDurationMs int
		actual        time.Duration
		wantNil       bool
		wantPassed    bool
	}{
		{"passes when faster than limit", 500, 200 * time.Millisecond, false, true},
		{"fails when slower than limit", 100, 200 * time.Millisecond, false, false},
		{"passes when exactly at limit", 200, 200 * time.Millisecond, false, true},
		{"zero max returns nil", 0, 200 * time.Millisecond, true, false},
		{"negative max returns nil", -1, 200 * time.Millisecond, true, false},
		{"result shows expected and actual ms", 500, 200 * time.Millisecond, false, true},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 5: Refactor Evaluate to Use Input Struct and Wire Into Runner
**Rationale:** This is the integration step — must come after all individual pieces exist. Refactoring Evaluate signature prevents parameter explosion.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Introduce `EvalInput` struct, refactor `Evaluate` signature |
| `internal/assertion/assertion_test.go` | modify | Update all `Evaluate` callers to use `EvalInput`, add header/timing integration tests |
| `internal/runner/runner.go` | modify | Add `toHeaderInputs()`, pass headers/duration/timing to `Evaluate` |
| `internal/runner/runner_test.go` | modify | Add runner tests for header and timing assertion integration |

#### Current `Evaluate` Signature
```go
func Evaluate(statusCodes []int, actualStatus int, bodyAssertions []BodyInput, body []byte) *Results
```

#### New `Evaluate` Signature
```go
type EvalInput struct {
	StatusCodes      []int
	ActualStatus     int
	BodyAssertions   []BodyInput
	Body             []byte
	HeaderAssertions []HeaderInput
	Headers          http.Header
	MaxDurationMs    int
	ActualDuration   time.Duration
}

func Evaluate(in EvalInput) *Results
```

#### Runner Changes

```go
func toHeaderInputs(items []parser.HeaderAssertion) []assertion.HeaderInput {
	inputs := make([]assertion.HeaderInput, len(items))
	for i, item := range items {
		inputs[i] = assertion.HeaderInput{
			Name:     item.Name,
			Operator: item.Operator,
			Value:    fmt.Sprint(item.Value),
		}
	}
	return inputs
}
```

Pass `result.Headers`, `result.Duration`, and `item.Assertions.Timing.MaxDurationMs` into `EvalInput`.

#### Tests to Write FIRST (RED phase)

Evaluate tests (updated + new):
```go
// Existing tests updated to use EvalInput struct (same behavior)
// New tests:
// - "header assertion only"
// - "timing assertion only"
// - "all assertion types pass"
// - "header fails others pass"
// - "timing fails others pass"
```

Runner tests:
```go
// - "TestRun_header_assertion_pass"
// - "TestRun_header_assertion_fail"
// - "TestRun_timing_assertion_pass"
// - "TestRun_timing_assertion_fail"
// - "TestRun_header_and_timing_combined"
```

#### Impact on Existing Tests

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/assertion/assertion_test.go` | `TestEvaluate/*` | breaks | Update all calls to use `EvalInput` struct |
| `internal/runner/runner_test.go` | existing runner tests | may need update | Verify `exec` mock still works with `Headers` field on `Result` |

---

### Step 6: Smoke Test and Sample Update
**Rationale:** End-to-end verification — must come last after all pieces are wired.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add header and timing assertion test cases |
| `sample/hello.yaml` | modify | Add header assertion example |

#### Smoke Test Cases
1. Header assertion that passes (Content-Type header exists)
2. Timing assertion that passes (generous max_duration_ms)
3. Header assertion that fails (expect non-existent header)

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/httpexec/executor_test.go` | existing | none | — |
| `internal/parser/parser_test.go` | existing | none | — |
| `internal/assertion/assertion_test.go` | `TestEvaluate/*` | breaks in Step 5 | Update to use `EvalInput` struct |
| `internal/runner/runner_test.go` | existing | minor update in Step 5 | Add `Headers` to mock `Result` |

## Risks and Edge Cases

- **Risk:** Multiple headers with same name → `http.Header.Get()` returns first value only → **Mitigation:** Document this. `equals`/`matches` operate on first value. `exists` checks any value present.
- **Risk:** Invalid regex in `matches` → **Mitigation:** `regexp.Compile()` error caught, returned as failed `Result` with clear message, no panic.
- **Risk:** Timing flakiness in tests → **Mitigation:** Use large margins in test timing (e.g., 200ms vs 500ms limit). Timing tests use known durations, not live HTTP.
- **Risk:** `Evaluate` signature change breaks callers → **Mitigation:** Refactor to `EvalInput` struct in one step, update all callers simultaneously.
- **Edge case:** Case-insensitive header names → **Handling:** `http.Header` methods canonicalize names automatically per RFC 7230.
- **Edge case:** At-threshold timing → **Handling:** `<=` comparison — exactly at limit passes.
- **Edge case:** Nil headers with assertions → **Handling:** All assertions fail gracefully.
- **Edge case:** Negative `max_duration_ms` → **Handling:** Treated as "no assertion" (return nil).
- **Edge case:** Empty header value → **Handling:** `exists` passes, `equals: ""` passes.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create collection with header and timing assertions, run and verify
./apitest run sample/hello.yaml
```
