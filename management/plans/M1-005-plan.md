# Implementation Plan: M1-005

## Overview

Add JSONPath body assertions to the assertion engine, allowing users to assert on response body values using operators like `equals`, `exists`, `not_exists`, and `type`. This requires capturing the response body (currently discarded), building a minimal JSONPath evaluator, extending the parser and assertion packages, and wiring everything through the runner.

## Task Details
- **ID:** M1-005
- **Title:** Assert on response body with JSONPath
- **Phase:** M1: Core CLI
- **Priority:** 5
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-004 | Assert on status code | done |

## Implementation Steps

### Step 1: Capture response body in httpexec.Result

**Rationale:** Body assertions require the response body. Currently `executor.go` discards it with `io.Copy(io.Discard, resp.Body)`. This is the smallest, most self-contained change and a prerequisite for everything else.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/httpexec/executor.go` | modify | Add `Body []byte` field to `Result`, read body with `io.ReadAll` |
| `internal/httpexec/executor_test.go` | modify | Add tests verifying body capture |

#### Current Code
```go
// internal/httpexec/executor.go:17-21
type Result struct {
	StatusCode int
	Duration   time.Duration
}

// lines 54-57
defer func() {
    _, _ = io.Copy(io.Discard, resp.Body)
    _ = resp.Body.Close()
}()

return &Result{
    StatusCode: resp.StatusCode,
    Duration:   duration,
}, nil
```

#### New Code
```go
type Result struct {
	StatusCode int
	Duration   time.Duration
	Body       []byte
}

// Replace discard with ReadAll
body, err := io.ReadAll(resp.Body)
_ = resp.Body.Close()
if err != nil {
    return nil, fmt.Errorf("%w: reading response body: %w", ErrNetwork, err)
}

return &Result{
    StatusCode: resp.StatusCode,
    Duration:   duration,
    Body:       body,
}, nil
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecute_body_is_captured(t *testing.T) {
    // Server returns JSON body, verify result.Body contains it
}

func TestExecute_empty_body_is_empty_slice(t *testing.T) {
    // Server returns empty body, verify result.Body is []byte{}
}
```

#### Impact on Existing Tests
- No existing tests assert on `Body` — adding the field is additive and backward-compatible
- The `successExecutor` and `failAtN` helpers in `runner_test.go` return `Result` without `Body` — they work fine since `Body` defaults to `nil`

---

### Step 2: Create internal/jsonpath package

**Rationale:** The JSONPath evaluator is a pure function with no external dependencies. Building it next provides the foundation for body assertion evaluation without touching existing packages.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/jsonpath/jsonpath.go` | create | Minimal JSONPath evaluator (dot notation + array indexing) |
| `internal/jsonpath/jsonpath_test.go` | create | Comprehensive table-driven tests |

#### New Code
```go
package jsonpath

import "errors"

// ErrNotFound indicates the path matched nothing in the document.
var ErrNotFound = errors.New("no match at path")

// ErrInvalidPath indicates the path expression could not be parsed.
var ErrInvalidPath = errors.New("invalid JSONPath expression")

// Evaluate resolves a JSONPath expression against a parsed JSON document.
// The document should be the result of json.Unmarshal into any.
// Returns (value, nil) on success, (nil, nil) when path points to JSON null,
// or (nil, ErrNotFound) when path matches nothing.
func Evaluate(path string, doc any) (any, error)
```

Supported path features:
- Root reference: `$`
- Dot notation: `.field`
- Array indexing: `[0]`, `[1]`
- Chaining: `$.data.items[0].name`

**Design decision:** Minimal internal implementation instead of a third-party library. The behaviors defined for M1-005 only need simple dot-notation and array indexing. This aligns with the project's "minimise external dependencies" philosophy. Full JSONPath features (wildcards, recursive descent, filters) can be added later if needed.

#### Tests to Write FIRST (RED phase)

```go
func TestEvaluate(t *testing.T) {
    tests := []struct {
        name    string
        path    string
        doc     any
        want    any
        wantErr error
    }{
        {"root object returns document", "$", map[string]any{"a": 1}, map[string]any{"a": 1}, nil},
        {"simple field access", "$.name", map[string]any{"name": "Alice"}, "Alice", nil},
        {"nested field access", "$.data.id", map[string]any{"data": map[string]any{"id": float64(1)}}, float64(1), nil},
        {"deeply nested", "$.a.b.c.d", nested4, "value", nil},
        {"array index zero", "$.items[0]", map[string]any{"items": []any{"a", "b"}}, "a", nil},
        {"array index nested field", "$.items[0].name", arrayOfObjects, "first", nil},
        {"null value is found", "$.value", map[string]any{"value": nil}, nil, nil},
        {"missing field returns ErrNotFound", "$.missing", map[string]any{"name": "x"}, nil, ErrNotFound},
        {"missing nested field returns ErrNotFound", "$.a.b.c", shallow, nil, ErrNotFound},
        {"array index out of bounds returns ErrNotFound", "$.items[5]", threeItems, nil, ErrNotFound},
        {"field on non-object returns ErrNotFound", "$.name.x", map[string]any{"name": "str"}, nil, ErrNotFound},
        {"index on non-array returns ErrNotFound", "$.name[0]", map[string]any{"name": "str"}, nil, ErrNotFound},
        {"invalid path - no dollar", "no dollar", anyDoc, nil, ErrInvalidPath},
        {"invalid path - empty string", "", anyDoc, nil, ErrInvalidPath},
        {"invalid path - unclosed bracket", "$[", anyDoc, nil, ErrInvalidPath},
    }
}
```

#### Impact on Existing Tests
- None — new package, no existing code affected

---

### Step 3: Add BodyAssertion types and YAML parsing to parser

**Rationale:** The parser must understand the `assertions.body` YAML format before the assertion engine can evaluate it. This follows the data-flow order: parse → evaluate.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `BodyAssertion`, `BodyAssertions` types with custom YAML unmarshalling |
| `internal/parser/parser_test.go` | modify | Add tests for body assertion parsing |
| `internal/parser/testdata/with_body_assertion_equals.yaml` | create | Test fixture |
| `internal/parser/testdata/with_body_assertion_exists.yaml` | create | Test fixture |
| `internal/parser/testdata/with_body_assertion_not_exists.yaml` | create | Test fixture |
| `internal/parser/testdata/with_body_assertion_type.yaml` | create | Test fixture |
| `internal/parser/testdata/with_body_assertion_multiple.yaml` | create | Test fixture |
| `internal/parser/testdata/with_body_and_status_assertion.yaml` | create | Test fixture |

#### Current Code
```go
// internal/parser/collection.go:30-32
type Assertions struct {
	Status StatusCodes `yaml:"status,omitempty"`
}
```

#### New Code
```go
// BodyAssertion represents a single body assertion on a JSONPath.
type BodyAssertion struct {
	Path     string // JSONPath expression, e.g. "$.data.id"
	Operator string // "equals", "exists", "not_exists", "type"
	Value    any    // expected value (meaning depends on operator)
}

// BodyAssertions handles the YAML map-of-maps format for body assertions.
type BodyAssertions struct {
	Items []BodyAssertion
}

// UnmarshalYAML parses the body assertion map format:
//   $.data.id:
//     equals: 1
//   $.token:
//     exists: true
func (b *BodyAssertions) UnmarshalYAML(value *yaml.Node) error

// Assertions updated to include Body field:
type Assertions struct {
	Status StatusCodes    `yaml:"status,omitempty"`
	Body   BodyAssertions `yaml:"body,omitempty"`
}
```

The YAML format from the spec uses JSONPath keys mapped to operator objects:
```yaml
assertions:
  body:
    $.data.id:
      equals: 1
    $.token:
      exists: true
    $.name:
      type: string
```

Each JSONPath key maps to exactly one operator. The `UnmarshalYAML` method iterates over the mapping node, extracting path (key) and operator/value (from the nested map).

#### Tests to Write FIRST (RED phase)

```go
// In parser_test.go, add test cases:
{"body assertion with equals operator", "with_body_assertion_equals.yaml", ...},
{"body assertion with exists operator", "with_body_assertion_exists.yaml", ...},
{"body assertion with not_exists operator", "with_body_assertion_not_exists.yaml", ...},
{"body assertion with type operator", "with_body_assertion_type.yaml", ...},
{"multiple body assertions", "with_body_assertion_multiple.yaml", ...},
{"body assertions alongside status", "with_body_and_status_assertion.yaml", ...},
{"no body assertions parses to empty items", "minimal.yaml", ...},
```

#### Impact on Existing Tests
- Existing parser tests pass — the `Body` field defaults to zero value (`BodyAssertions{}` with nil `Items`)
- Existing test fixtures without `body:` continue to parse correctly

---

### Step 4: Add body assertion evaluation to internal/assertion

**Rationale:** With the JSONPath evaluator and parser types in place, the assertion engine can now evaluate body assertions. This is the core logic step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | Add `BodyAssertionInput` type, `CheckBody` function |
| `internal/assertion/assertion_test.go` | modify | Add comprehensive body assertion tests |

#### Current Code
```go
// internal/assertion/assertion.go:48-57
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

#### New Code
```go
// BodyAssertionInput describes a single body assertion to evaluate.
type BodyAssertionInput struct {
	Path     string // JSONPath, e.g. "$.data.id"
	Operator string // "equals", "exists", "not_exists", "type"
	Value    any    // expected value (operator-dependent)
}

// CheckBody evaluates body assertions against the response body bytes.
// Returns nil if no assertions are defined.
// Returns results with an error item if body is not valid JSON.
func CheckBody(assertions []BodyAssertionInput, body []byte) []Result

// Evaluate runs all assertions and returns aggregated results.
// Returns nil if no assertions are defined (both statusCodes and bodyAssertions empty).
func Evaluate(statusCodes []int, actualStatus int, bodyAssertions []BodyAssertionInput, body []byte) *Results
```

Operator semantics:
- `equals`: Deep equality between JSONPath result and expected value. Numeric coercion: JSON numbers are `float64`; integer YAML values need comparison as `float64`.
- `exists`: Path resolves to any value (including `null`). No `ErrNotFound`.
- `not_exists`: Path does NOT resolve (`ErrNotFound` from JSONPath). Passes when path is missing.
- `type`: Compare Go type of resolved value to expected type string (`string`, `number`, `boolean`, `array`, `object`, `null`).

#### Tests to Write FIRST (RED phase)

```go
func TestCheckBody(t *testing.T) {
    tests := []struct {
        name       string
        assertions []BodyAssertionInput
        body       []byte
        wantCount  int
        wantPassed []bool
    }{
        {"equals passes when values match", equalsMatch, jsonBody, 1, []bool{true}},
        {"equals fails when values differ", equalsMismatch, jsonBody, 1, []bool{false}},
        {"equals with string value", equalsString, stringBody, 1, []bool{true}},
        {"exists passes when path has value", existsPresent, jsonBody, 1, []bool{true}},
        {"exists passes for null value", existsNull, nullBody, 1, []bool{true}},
        {"exists fails when path missing", existsMissing, jsonBody, 1, []bool{false}},
        {"not_exists passes when path missing", notExistsMissing, jsonBody, 1, []bool{true}},
        {"not_exists fails when path has value", notExistsPresent, jsonBody, 1, []bool{false}},
        {"type string passes for string", typeString, stringBody, 1, []bool{true}},
        {"type number passes for number", typeNumber, numberBody, 1, []bool{true}},
        {"type boolean passes for bool", typeBool, boolBody, 1, []bool{true}},
        {"type array passes for array", typeArray, arrayBody, 1, []bool{true}},
        {"type object passes for object", typeObject, objectBody, 1, []bool{true}},
        {"type null passes for null", typeNull, nullBody, 1, []bool{true}},
        {"type fails when type mismatches", typeMismatch, numberBody, 1, []bool{false}},
        {"no match at path gives clear message", noMatch, jsonBody, 1, []bool{false}},
        {"non-JSON body returns error result", nonJSON, htmlBody, 1, []bool{false}},
        {"empty body with assertions returns error result", emptyBody, []byte{}, 1, []bool{false}},
        {"nil assertions returns nil", nil, jsonBody, 0, nil},
        {"multiple assertions all evaluated", multiAssert, jsonBody, 3, []bool{true, true, true}},
    }
}
```

#### Impact on Existing Tests
- **`TestEvaluate`** — signature changes from `Evaluate(codes, actual)` to `Evaluate(codes, actual, bodyAssertions, body)`. Fix: add `nil, nil` to existing calls.

---

### Step 5: Update runner to pass body data

**Rationale:** The runner is the integration point that connects parsed assertions with the assertion engine and HTTP results. This is the final wiring step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Map parser body assertions to assertion inputs, pass body to `Evaluate` |
| `internal/runner/runner_test.go` | modify | Add body assertion tests, update existing helpers |

#### Current Code
```go
// internal/runner/runner.go:67
ar := assertion.Evaluate(item.Assertions.Status.Codes, result.StatusCode)
```

#### New Code
```go
bodyInputs := toBodyInputs(item.Assertions.Body.Items)
ar := assertion.Evaluate(item.Assertions.Status.Codes, result.StatusCode, bodyInputs, result.Body)

// Helper function:
func toBodyInputs(items []parser.BodyAssertion) []assertion.BodyAssertionInput {
    if len(items) == 0 {
        return nil
    }
    inputs := make([]assertion.BodyAssertionInput, len(items))
    for i, item := range items {
        inputs[i] = assertion.BodyAssertionInput{
            Path:     item.Path,
            Operator: item.Operator,
            Value:    item.Value,
        }
    }
    return inputs
}
```

#### Tests to Write FIRST (RED phase)

```go
// New tests:
{"body assertion pass in runner", ...},
{"body assertion fail in runner", ...},
{"body assertion failure counts as assertion failure", ...},
{"non-JSON body in runner shows error", ...},
{"body assertion with stop_on_failure stops remaining", ...},
{"body and status assertions combined", ...},
```

#### Impact on Existing Tests
- `successExecutor` returns `httpexec.Result{StatusCode: 200, Duration: ...}` — `Body` defaults to `nil`, which is fine for tests without body assertions
- Existing assertion tests in runner use `assertion.Evaluate(codes, status)` — must update to new 4-arg signature: `assertion.Evaluate(codes, status, nil, nil)`

---

### Step 6: Integration tests and smoke test

**Rationale:** End-to-end tests prove the feature works through the real binary. This is the final verification step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/run_test.go` | modify | Add CLI integration tests for body assertions |
| `smoke/run.sh` | modify | Add body assertion smoke test case |
| `smoke/collections/body_assertions.yaml` | create | Smoke test collection |

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_body_assertion_pass(t *testing.T) {
    // Server returns {"data":{"id":1}}, collection asserts $.data.id equals 1
    // Expect exit code 0
}

func TestRunCmd_body_assertion_fail(t *testing.T) {
    // Server returns {"data":{"id":2}}, collection asserts $.data.id equals 1
    // Expect exit code 1
}

func TestRunCmd_body_assertion_exists(t *testing.T) {
    // Server returns {"token":"abc"}, asserts $.token exists
    // Expect exit code 0
}

func TestRunCmd_body_assertion_non_json(t *testing.T) {
    // Server returns plain text, body assertion defined
    // Expect exit code 1
}
```

#### Impact on Existing Tests
- None — new tests only

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/assertion/assertion_test.go` | `TestEvaluate` | breaks | add `nil, nil` args for body assertions |
| `internal/runner/runner_test.go` | all assertion tests | breaks | update `assertion.Evaluate` calls to 4-arg signature |
| `internal/httpexec/executor_test.go` | all | none | — |
| `internal/parser/parser_test.go` | all | none | — |
| `cmd/curlew/run_test.go` | all | none | — |

## Risks and Edge Cases

- **Risk:** JSON number comparison — JSON numbers unmarshal as `float64`, YAML integers unmarshal as `int`. The `equals` operator must handle cross-type numeric comparison (convert both to `float64`).
  → **Mitigation:** In `equals` evaluation, check if both values are numeric and compare as `float64`.

- **Risk:** `exists` vs `not_exists` semantics for `null` — `null` is a valid JSON value that exists at a path.
  → **Mitigation:** The JSONPath evaluator returns `(nil, nil)` for null values and `(nil, ErrNotFound)` for missing paths. `exists` checks for absence of `ErrNotFound`. `not_exists` checks for presence of `ErrNotFound`.

- **Risk:** Response body memory for large responses — `io.ReadAll` loads entire body into memory.
  → **Mitigation:** Acceptable for M1. Future task can add configurable body size limits.

- **Risk:** YAML map ordering — Go maps don't preserve order, but YAML mapping nodes do. Using `yaml.Node` for unmarshalling preserves insertion order of body assertions.
  → **Mitigation:** Use `yaml.Node` iteration in `UnmarshalYAML` (same pattern as `StatusCodes`).

- **Edge case:** Empty `body:` key in YAML — should parse as zero assertions, not error.
  → **Handling:** `UnmarshalYAML` returns empty `Items` for empty mapping.

- **Edge case:** Non-string operator value for `type` — e.g., `type: 123` instead of `type: string`.
  → **Handling:** Assertion evaluation converts to string; test covers this.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create a test collection with body assertions:
cat > /tmp/body_test.yaml << 'EOF'
name: Body Assertion Test
requests:
  - name: Check user
    request:
      method: GET
      url: "https://jsonplaceholder.typicode.com/users/1"
    assertions:
      status: 200
      body:
        $.id:
          equals: 1
        $.name:
          exists: true
        $.name:
          type: string
EOF

./curlew run /tmp/body_test.yaml
# Should see pass/fail per assertion
```
