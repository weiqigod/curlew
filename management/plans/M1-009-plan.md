# Implementation Plan: M1-009

## Overview
Add collection-level variable interpolation (`variables:` block) with `{{var}}` syntax, circular reference detection, depth limiting, and undefined variable errors.

## Task Details
- **ID:** M1-009
- **Title:** Collection-level variables with interpolation
- **Phase:** M1: Core CLI
- **Priority:** 9
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-003 | HTTP GET execution | done |

## Implementation Steps

### Step 1: Create `internal/variable/` package — Core interpolation engine
**Rationale:** Standalone package with no project dependencies. Smallest blast radius; can be built and tested in complete isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | create | Core types: Scope, Resolve, Interpolate, cycle detection, depth limiting |
| `internal/variable/variable_test.go` | create | Comprehensive tests for all 7 behaviors |

#### New Code

```go
package variable

import (
    "errors"
    "fmt"
    "regexp"
    "sort"
    "strings"
)

// MaxDepth is the maximum variable reference chain depth allowed.
const MaxDepth = 10

// Sentinel errors for variable resolution failures.
var (
    ErrCircularReference = errors.New("circular variable reference")
    ErrDepthExceeded     = errors.New("variable interpolation depth limit exceeded")
    ErrUndefinedVariable = errors.New("undefined variable")
)

var varPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

// Scope holds a set of named variables that can be resolved and interpolated.
type Scope struct {
    vars     map[string]string
    resolved map[string]string
}

// NewScope creates a Scope from a variables map.
func NewScope(vars map[string]string) *Scope

// Resolve resolves all internal variable references (variables referencing other variables),
// detecting circular references and enforcing depth limits.
// Must be called before Interpolate.
func (s *Scope) Resolve() error

// Interpolate replaces all {{var}} placeholders in the input string
// with their resolved values. Returns error if any variable is undefined.
func (s *Scope) Interpolate(input string) (string, error)

// InterpolateMap replaces all {{var}} placeholders in map values.
// Returns a new map with interpolated values.
func (s *Scope) InterpolateMap(m map[string]string) (map[string]string, error)

// InterpolateBody handles body interpolation for string, map[string]interface{}, and []interface{} bodies.
func (s *Scope) InterpolateBody(body any) (any, error)

// AvailableVars returns a sorted list of variable names in this scope.
func (s *Scope) AvailableVars() []string

// resolveVar resolves a single variable, tracking the resolution stack for
// cycle detection and depth limiting.
func (s *Scope) resolveVar(name string, stack []string, depth int) (string, error)

// findVarRefs extracts all variable names referenced in a string value.
func findVarRefs(s string) []string
```

#### Tests to Write FIRST (RED phase)

```go
func TestScope_Interpolate(t *testing.T) {
    tests := []struct {
        name    string
        vars    map[string]string
        input   string
        want    string
        wantErr error
    }{
        {"simple variable in URL", map[string]string{"base_url": "https://example.com"}, "{{base_url}}/get", "https://example.com/get", nil},
        {"multiple variables in same string", map[string]string{"host": "example.com", "path": "api"}, "https://{{host}}/{{path}}", "https://example.com/api", nil},
        {"variable in middle of string", map[string]string{"host": "example.com", "ver": "v1"}, "https://{{host}}/{{ver}}/resource", "https://example.com/v1/resource", nil},
        {"no variables passes through unchanged", map[string]string{"x": "y"}, "no vars here", "no vars here", nil},
        {"undefined variable", map[string]string{"a": "1"}, "{{unknown}}", "", ErrUndefinedVariable},
        {"special characters in value", map[string]string{"q": "a&b=c?d{e}f"}, "{{q}}", "a&b=c?d{e}f", nil},
        {"empty variable value", map[string]string{"empty": ""}, "pre{{empty}}post", "prepost", nil},
    }
    // ...
}

func TestScope_Resolve(t *testing.T) {
    tests := []struct {
        name    string
        vars    map[string]string
        wantErr error
    }{
        {"no references - all literal values", map[string]string{"a": "1", "b": "2"}, nil},
        {"simple chain a->b", map[string]string{"a": "{{b}}", "b": "value"}, nil},
        {"two-level chain a->b->c", map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "done"}, nil},
        {"circular reference a->b->a", map[string]string{"a": "{{b}}", "b": "{{a}}"}, ErrCircularReference},
        {"self-reference", map[string]string{"a": "{{a}}"}, ErrCircularReference},
        {"three-way cycle", map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "{{a}}"}, ErrCircularReference},
        {"depth exactly 10 succeeds", /* 10-level chain */, nil},
        {"depth 11 exceeds limit", /* 11-level chain */, ErrDepthExceeded},
    }
    // ...
}

func TestScope_Resolve_error_messages(t *testing.T) {
    // "circular path in error message includes cycle"
    // "depth path in error message includes resolution chain"
    // "undefined variable error lists available variables"
}

func TestScope_InterpolateMap(t *testing.T) {
    // "headers map with {{var}} in values"
    // "query params with {{var}} in values"
}

func TestScope_InterpolateBody(t *testing.T) {
    // "string body with {{var}}"
    // "map body with {{var}} in values"
    // "nested map body"
    // "nil body returns nil"
}

func TestScope_empty_scope(t *testing.T) {
    // "empty variables map, resolve succeeds, interpolate on plain string succeeds"
}
```

#### Impact on Existing Tests
- No existing tests affected — new package

---

### Step 2: Add `Variables` field to `parser.Collection`
**Rationale:** Additive struct change with `omitempty` — zero risk to existing parsing. Must come before runner integration.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Variables map[string]string` field |
| `internal/parser/parser_test.go` | modify | Add test for parsing variables block |
| `internal/parser/testdata/with_variables.yaml` | create | Test fixture with variables |

#### Current Code
```go
type Collection struct {
    Name        string        `yaml:"name"`
    Description string        `yaml:"description,omitempty"`
    Requests    []RequestItem `yaml:"requests"`
    Options     Options       `yaml:"options,omitempty"`
}
```

#### New Code
```go
type Collection struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description,omitempty"`
    Variables   map[string]string `yaml:"variables,omitempty"`
    Requests    []RequestItem     `yaml:"requests"`
    Options     Options           `yaml:"options,omitempty"`
}
```

#### Testdata fixture: `internal/parser/testdata/with_variables.yaml`
```yaml
name: Variables Test
variables:
  base_url: "https://example.com"
  api_version: "v1"
requests:
  - name: Get Example
    request:
      method: GET
      url: "{{base_url}}/{{api_version}}/resource"
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_with_variables(t *testing.T) {
    // Parse testdata/with_variables.yaml
    // Assert col.Variables["base_url"] == "https://example.com"
    // Assert col.Variables["api_version"] == "v1"
    // Assert len(col.Variables) == 2
}

func TestParseFile_no_variables(t *testing.T) {
    // Parse existing testdata/minimal.yaml
    // Assert col.Variables is nil or empty
}
```

#### Impact on Existing Tests
- None. Adding `Variables map[string]string` with `omitempty` to the struct is backward-compatible. Existing test fixtures without `variables:` produce nil/empty map. Existing `reflect.DeepEqual` comparisons pass because expected structs also have zero-value Variables.

---

### Step 3: Wire interpolation into `runner.Run()`
**Rationale:** Integration point. Must come after Steps 1–2 are complete. Changes `Run()` signature to return error for pre-execution failures.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add variable resolution before execution, change signature |
| `internal/runner/runner_test.go` | modify | Update all ~20 call sites, add variable-specific tests |

#### Current Code
```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc) ([]RequestResult, *Summary) {
    // ...
    for _, item := range col.Requests {
        // ...
        result, execErr := exec(ctx, &item.Request)
        // ...
    }
    return results, summary
}
```

#### New Code
```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc) ([]RequestResult, *Summary, error) {
    results := make([]RequestResult, 0, len(col.Requests))
    summary := &Summary{Total: len(col.Requests)}

    // Variable resolution (before any HTTP execution)
    var scope *variable.Scope
    if len(col.Variables) > 0 {
        scope = variable.NewScope(col.Variables)
        if err := scope.Resolve(); err != nil {
            return nil, summary, err
        }
    }

    start := time.Now()
    stopped := false

    for _, item := range col.Requests {
        if stopped { /* ... same skip logic ... */ }
        if err := ctx.Err(); err != nil { /* ... same cancel logic ... */ }

        // Interpolate request fields (on a copy, not mutating parsed collection)
        req := item.Request
        if scope != nil {
            interpolated, err := interpolateRequest(scope, &req)
            if err != nil {
                return nil, summary, err
            }
            req = *interpolated
        }

        result, execErr := exec(ctx, &req)
        // ... rest unchanged ...
    }
    // ...
    return results, summary, nil
}

// interpolateRequest applies variable interpolation to all string fields of a request.
func interpolateRequest(scope *variable.Scope, req *parser.Request) (*parser.Request, error) {
    out := *req // shallow copy

    var err error
    if out.URL, err = scope.Interpolate(out.URL); err != nil {
        return nil, fmt.Errorf("URL: %w", err)
    }
    if out.Headers, err = scope.InterpolateMap(out.Headers); err != nil {
        return nil, fmt.Errorf("headers: %w", err)
    }
    if out.QueryParams, err = scope.InterpolateMap(out.QueryParams); err != nil {
        return nil, fmt.Errorf("query params: %w", err)
    }
    if out.Body, err = scope.InterpolateBody(out.Body); err != nil {
        return nil, fmt.Errorf("body: %w", err)
    }
    return &out, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_with_variables_interpolates_url(t *testing.T) {
    // Collection with Variables: {"base": "https://example.com"}
    // Request URL: "{{base}}/path"
    // Capture executor receives "https://example.com/path"
}

func TestRun_with_variables_interpolates_headers(t *testing.T) {
    // Collection with Variables: {"token": "abc123"}
    // Request Headers: {"Authorization": "Bearer {{token}}"}
    // Verify executor receives interpolated header
}

func TestRun_with_variables_interpolates_query_params(t *testing.T) {
    // Collection with Variables: {"ver": "v2"}
    // Request QueryParams: {"version": "{{ver}}"}
    // Verify executor receives interpolated query param
}

func TestRun_with_circular_variables_returns_error(t *testing.T) {
    // Variables: {"a": "{{b}}", "b": "{{a}}"}
    // Expect error (not nil), no requests executed
}

func TestRun_with_undefined_variable_returns_error(t *testing.T) {
    // Variables: {"a": "1"}, URL: "{{unknown}}"
    // Expect error, no requests executed
}

func TestRun_no_variables_unchanged(t *testing.T) {
    // Collection without Variables (nil)
    // Backward compatibility: same behavior as before
}
```

#### Impact on Existing Tests
- **All ~20 call sites** in `runner_test.go` that call `Run()` need mechanical update: `results, summary := Run(...)` becomes `results, summary, err := Run(...)` with `if err != nil { t.Fatal(err) }`.
- No behavioral changes — existing collections have nil Variables, so `err` will always be `nil`.

---

### Step 4: Update `cmd/curlew/main.go` to handle variable errors
**Rationale:** Final wiring — depends on Step 3's signature change. Small change in `runCmd()`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Handle error return from `runner.Run()`, exit code 5 |
| `cmd/curlew/main_test.go` | modify | Add CLI integration tests for variables |

#### Current Code
```go
results, summary := runner.Run(ctx, col, httpexec.Execute)
```

#### New Code
```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute)
if varErr != nil {
    output.PrintStructuredError(os.Stderr, varErr)
    return 5
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestCLIIntegration_variable_interpolation(t *testing.T) {
    // Start httptest server, create collection with variables: { base_url: <server> }
    // URL: "{{base_url}}/path"
    // Expect exit code 0, stdout contains pass indicator
}

func TestCLIIntegration_variable_circular_error(t *testing.T) {
    // Collection with variables: { a: "{{b}}", b: "{{a}}" }
    // Expect exit code 5
    // Expect stderr contains [ERROR] and "circular"
}

func TestCLIIntegration_variable_undefined_error(t *testing.T) {
    // Collection with variables: { a: "1" }, URL: "{{unknown}}"
    // Expect exit code 5
    // Expect stderr contains [ERROR] and "undefined"
}
```

#### Impact on Existing Tests
- None. Existing tests don't use variables, so `varErr` is always `nil`.

---

### Step 5: Error formatting for variable errors
**Rationale:** Variable errors must produce spec-compliant `*errors.Structured` messages with `CategoryConfig`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Return `*apierrors.Structured` with CategoryConfig for all error types |

#### Design

The `Resolve()` and `Interpolate()` methods wrap errors as `*errors.Structured`:

```go
// Circular reference error
&apierrors.Structured{
    Category: apierrors.CategoryConfig,
    Message:  fmt.Sprintf("circular variable reference: %s", strings.Join(cycle, " -> ")),
    Hint:     "Remove the circular dependency between these variables",
}

// Depth exceeded error
&apierrors.Structured{
    Category: apierrors.CategoryConfig,
    Message:  fmt.Sprintf("variable interpolation depth limit exceeded (max %d): %s", MaxDepth, strings.Join(path, " -> ")),
    Hint:     "Simplify the variable chain to fewer than 10 levels",
}

// Undefined variable error
&apierrors.Structured{
    Category: apierrors.CategoryConfig,
    Message:  fmt.Sprintf("undefined variable %q", name),
    Hint:     fmt.Sprintf("Available variables: %s", strings.Join(available, ", ")),
}
```

This is implemented within Step 1 rather than as a separate step — the error types are part of the core package design.

---

### Step 6: Update smoke test
**Rationale:** Validates the full end-to-end capability with the real binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add variable interpolation test case |

#### New Code (appended before "=== Smoke Test Complete ===")

```bash
echo "--- Running collection with variables (expect pass) ---"
VARS_FILE=$(mktemp /tmp/curlew_vars_XXXXXX.yaml)
cat > "$VARS_FILE" << 'YAML'
name: Variable Test
variables:
  base_url: "https://httpbin.org"
requests:
  - name: Check Variables
    request:
      method: GET
      url: "{{base_url}}/get"
    assertions:
      status: 200
YAML
./curlew run "$VARS_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$VARS_FILE"
echo

echo "--- Running collection with circular variables (expect exit 5) ---"
CIRC_FILE=$(mktemp /tmp/curlew_circ_XXXXXX.yaml)
cat > "$CIRC_FILE" << 'YAML'
name: Circular Variable Test
variables:
  a: "{{b}}"
  b: "{{a}}"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "https://httpbin.org/get"
YAML
./curlew run "$CIRC_FILE" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$CIRC_FILE"
echo
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | All `TestRun*` (~20 call sites) | signature change | Add `_, err :=` and `if err != nil { t.Fatal(err) }` |
| `cmd/curlew/main_test.go` | None | none | — |
| `internal/parser/parser_test.go` | None | none | — |

## Risks and Edge Cases

- **Risk:** Body interpolation with nested maps/arrays → **Mitigation:** `InterpolateBody` recursively walks `map[string]interface{}`, `[]interface{}`, and `string` types. Only strings get interpolated; other types pass through unchanged.
- **Risk:** Variable values containing `{{` that aren't references → **Mitigation:** Per spec, all `{{}}` patterns are resolved. No escape mechanism defined for M1-009. Document as known limitation.
- **Risk:** Mutating parsed collection during interpolation → **Mitigation:** Always shallow-copy `parser.Request` before interpolation, then interpolate the copy.
- **Edge case:** Empty variable value (`key: ""`) → **Handling:** Resolves to empty string, valid.
- **Edge case:** Variable in URL that makes it malformed → **Handling:** Existing httpexec error path handles this, no special code needed.
- **Edge case:** Default value syntax `{{var|default:value}}` → **Handling:** Out of scope for M1-009. The simpler regex without `|` suffices.
- **Edge case:** Variables block with non-string values (int, bool) → **Handling:** YAML `map[string]string` type tag will coerce scalars to strings. Complex values will fail YAML deserialization with a parse error.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
cat > /tmp/vartest.yaml << 'YAML'
name: Variable Demo
variables:
  base_url: "https://httpbin.org"
requests:
  - name: Interpolated Request
    request:
      method: GET
      url: "{{base_url}}/get"
    assertions:
      status: 200
YAML
./curlew run /tmp/vartest.yaml
```
