# Implementation Plan: M1-010

## Overview

Add `extract:` support to collection files so that JSONPath expressions can capture values from HTTP responses and make them available as variables for subsequent requests. This enables chained workflows like "login → extract token → use token".

## Task Details
- **ID:** M1-010
- **Title:** Variable extraction from responses
- **Phase:** M1: Core CLI
- **Priority:** 10
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-009 | Collection-level variables with interpolation | done |

## Implementation Steps

### Step 1: Add `Extract` field to parser types

**Rationale:** Smallest change — adds YAML parsing with zero runtime impact. No existing behavior changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Extract` field to `RequestItem` |
| `internal/parser/parser_test.go` | modify | Add tests for `extract:` parsing |
| `internal/parser/testdata/with_extract_single.yaml` | create | Test fixture |
| `internal/parser/testdata/with_extract_multiple.yaml` | create | Test fixture |

#### Current Code
```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string     `yaml:"name"`
	Request    Request    `yaml:"request"`
	Assertions Assertions `yaml:"assertions,omitempty"`
}
```

#### New Code
```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string            `yaml:"name"`
	Request    Request           `yaml:"request"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_extract_single(t *testing.T) {
	// Parse YAML with extract: { token: "$.data.token" }
	// Verify col.Requests[0].Extract["token"] == "$.data.token"
}

func TestParseFile_extract_multiple(t *testing.T) {
	// Parse YAML with multiple extract entries
	// Verify all extraction paths are present
}

func TestParseFile_no_extract_remains_nil(t *testing.T) {
	// Verify existing YAML without extract: parses to nil Extract
}
```

#### Impact on Existing Tests
- No existing tests affected. `Extract` is `omitempty` and `nil` maps are zero-value for `reflect.DeepEqual`.

---

### Step 2: Add `Set` method to `variable.Scope`

**Rationale:** Small, isolated addition. Enables post-resolution variable injection needed by extraction.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Add `Set` method |
| `internal/variable/variable_test.go` | modify | Add tests for `Set` |

#### Current Code
```go
// Scope has no way to inject variables after Resolve()
type Scope struct {
	vars     map[string]string
	resolved map[string]string
}
```

#### New Code
```go
// Set adds or overrides a variable in the resolved scope.
// Used for extracted variables injected after initial resolution.
func (s *Scope) Set(name, value string) {
	if s.resolved == nil {
		s.resolved = make(map[string]string)
	}
	s.resolved[name] = value
	if s.vars == nil {
		s.vars = make(map[string]string)
	}
	s.vars[name] = value
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestScope_Set(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]string
		setName  string
		setValue string
		template string
		want     string
	}{
		{"adds new variable", nil, "token", "abc123", "Bearer {{token}}", "Bearer abc123"},
		{"overrides existing variable", map[string]string{"token": "old"}, "token", "new", "{{token}}", "new"},
		{"on empty scope", map[string]string{}, "x", "1", "{{x}}", "1"},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected. New method, no signature changes.

---

### Step 3: Add extraction logic

**Rationale:** Core extraction function, isolated from runner wiring. Reuses existing `internal/jsonpath` package.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/extract.go` | create | Extraction function and types |
| `internal/variable/extract_test.go` | create | Extraction tests |

#### New Code
```go
package variable

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	"github.com/peterlindqvist/apitest/internal/jsonpath"
)

var (
	ErrExtractionFailed = errors.New("variable extraction failed")
	ErrNotJSON          = errors.New("response body is not JSON")
)

// ExtractionInput defines what to extract from a response.
type ExtractionInput struct {
	Extractions map[string]string // variable name -> JSONPath expression
	Body        []byte            // response body
}

// ExtractionResult holds the outcome of variable extraction.
type ExtractionResult struct {
	Variables map[string]string
}

// Extract evaluates JSONPath expressions against a response body and returns
// extracted variables as strings.
func Extract(input ExtractionInput) (*ExtractionResult, error) {
	if len(input.Extractions) == 0 {
		return &ExtractionResult{Variables: map[string]string{}}, nil
	}

	var doc any
	if err := json.Unmarshal(input.Body, &doc); err != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryConfig,
			Message:  "cannot extract variables: response body is not valid JSON",
			Hint:     "Extraction requires a JSON response body",
			Inner:    ErrNotJSON,
		}
	}

	result := &ExtractionResult{Variables: make(map[string]string, len(input.Extractions))}

	// Process in sorted order for deterministic error messages
	names := make([]string, 0, len(input.Extractions))
	for name := range input.Extractions {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		path := input.Extractions[name]
		val, err := jsonpath.Evaluate(path, doc)
		if err != nil {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryConfig,
				Message:  fmt.Sprintf("extraction failed for variable %q at path %s: no match", name, path),
				Hint:     "Check that the JSONPath expression matches the response structure",
				Inner:    ErrExtractionFailed,
			}
		}
		result.Variables[name] = stringify(val)
	}

	return result, nil
}

// stringify converts an extracted value to a string for variable storage.
func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		// Arrays and objects: marshal to JSON
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(data)
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExtract(t *testing.T) {
	tests := []struct {
		name        string
		extractions map[string]string
		body        string
		want        map[string]string
		wantErr     error
	}{
		{"single string value", map[string]string{"token": "$.data.token"}, `{"data":{"token":"abc"}}`, map[string]string{"token": "abc"}, nil},
		{"single number value", map[string]string{"id": "$.id"}, `{"id":42}`, map[string]string{"id": "42"}, nil},
		{"multiple extractions", map[string]string{"a": "$.a", "b": "$.b"}, `{"a":"x","b":"y"}`, map[string]string{"a": "x", "b": "y"}, nil},
		{"nested path", map[string]string{"t": "$.data.auth.token"}, `{"data":{"auth":{"token":"deep"}}}`, map[string]string{"t": "deep"}, nil},
		{"boolean value", map[string]string{"ok": "$.ok"}, `{"ok":true}`, map[string]string{"ok": "true"}, nil},
		{"null value", map[string]string{"v": "$.v"}, `{"v":null}`, map[string]string{"v": ""}, nil},
		{"array value", map[string]string{"arr": "$.items"}, `{"items":[1,2]}`, map[string]string{"arr": "[1,2]"}, nil},
		{"object value", map[string]string{"obj": "$.data"}, `{"data":{"k":"v"}}`, map[string]string{"obj": `{"k":"v"}`}, nil},
		{"no extractions", map[string]string{}, `{}`, map[string]string{}, nil},
		{"path not found", map[string]string{"x": "$.missing"}, `{"a":1}`, nil, ErrExtractionFailed},
		{"non-JSON body", map[string]string{"x": "$.a"}, `<html>not json</html>`, nil, ErrNotJSON},
		{"empty body", map[string]string{"x": "$.a"}, ``, nil, ErrNotJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Extract(ExtractionInput{
				Extractions: tt.extractions,
				Body:        []byte(tt.body),
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// compare result.Variables to tt.want
		})
	}
}
```

#### Impact on Existing Tests
- No existing tests affected. New files only.

---

### Step 4: Wire extraction into runner

**Rationale:** Largest blast radius — connects parsing, extraction, and variable scope in the execution loop.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add extraction step after assertions |
| `internal/runner/runner_test.go` | modify | Add extraction integration tests |

#### Current Code
```go
// Variable resolution (before any HTTP execution)
var scope *variable.Scope
if len(col.Variables) > 0 {
	scope = variable.NewScope(col.Variables)
	if err := scope.Resolve(); err != nil {
		return nil, summary, err
	}
}
```

#### New Code
```go
// Variable resolution (before any HTTP execution)
// Always create scope — extraction may add variables even without collection-level vars
scope := variable.NewScope(col.Variables)
if err := scope.Resolve(); err != nil {
	return nil, summary, err
}
```

And after the assertion evaluation block:

```go
// Extract variables from response (if configured)
if len(item.Extract) > 0 && execErr == nil {
	extResult, extErr := variable.Extract(variable.ExtractionInput{
		Extractions: item.Extract,
		Body:        result.Body,
	})
	if extErr != nil {
		rr.Err = extErr
		summary.Failed++
		summary.Passed-- // was counted as passed in the assertion block
		if col.Options.StopOnFailure {
			stopped = true
		}
		results = append(results, rr)
		continue
	}
	for k, v := range extResult.Variables {
		scope.Set(k, v)
	}
}
```

Note: The scope `nil` check in the interpolation block (`if scope != nil`) can be removed since scope is always created now.

#### Tests to Write FIRST (RED phase)

```go
func TestRun_extract(t *testing.T) {
	tests := []struct {
		name string
		// test fields...
	}{
		{"sets variable for next request"},
		{"sets variable used in headers"},
		{"path not found fails request"},
		{"non-JSON body fails request"},
		{"multiple variables extracted"},
		{"overrides collection variable"},
		{"in last request no error"},
		{"respects stop on failure"},
		{"no extract unchanged behavior"},
	}
}
```

#### Impact on Existing Tests
- **Scope creation change:** Scope is now always created (not only when `len(col.Variables) > 0`). This is safe because `NewScope(nil)` + `Resolve()` produces an empty scope, and subsequent interpolation with an empty scope will correctly report `ErrUndefinedVariable` if any `{{var}}` references exist.
- **Existing runner tests** that don't use variables: no change — scope exists but does nothing.
- **Existing runner tests** that use variables: no change — scope is still created from `col.Variables`.

---

### Step 5: Add smoke test

**Rationale:** End-to-end verification as required by definition of done.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add extraction test case |
| `smoke/` directory | create fixture | Collection YAML with extraction |

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | all existing | none | — |
| `internal/variable/variable_test.go` | all existing | none | — |
| `internal/runner/runner_test.go` | all existing | none | scope creation is compatible |

## Risks and Edge Cases

- **Risk:** `Scope` becomes mutable during execution via `Set`. → **Mitigation:** Execution is sequential and single-threaded in M1. Document `Set` as intended for extraction use.
- **Risk:** Value stringification loses type info (numbers become strings). → **Mitigation:** Acceptable for M1 where all variable values are strings. Integer formatting uses `%d` (no `.0` suffix).
- **Edge case:** JSONPath returns `null` → **Handling:** Store as empty string `""` — most useful for interpolation contexts like headers.
- **Edge case:** JSONPath returns array/object → **Handling:** Serialize to JSON string via `json.Marshal`.
- **Edge case:** Same variable extracted in different requests → **Handling:** Later extraction overwrites earlier via `Set` — correct behavior.
- **Edge case:** Extraction failure when assertions also failed → **Handling:** Assertions are evaluated first. If assertions fail AND request has extractions, extraction is still attempted (no early exit on assertion failure for the same request). The request is already marked failed from assertions. Extraction failure adds to the error. **Design decision:** If assertions fail, skip extraction — extraction is only useful if the response is as expected. This simplifies the logic: extraction only runs when the request passed assertions.
- **Edge case:** `extract:` on a request with no assertions → **Handling:** Works fine — extraction doesn't depend on assertions.

## Design Decisions

1. **Extraction runs only when `execErr == nil` AND assertions pass** — if the response is wrong, extracting from it is misleading.
2. **Null → empty string** — `{{token}}` interpolates to `""` rather than `"null"`.
3. **Integers → no decimal** — `42` becomes `"42"` not `"42.0"`.
4. **Sorted extraction order** — process extractions alphabetically for deterministic error messages.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create a collection with extraction:
# First request POSTs to httpbin.org/post (which echoes the body),
# extracts a value, and second request uses it.
./apitest run smoke/extract_collection.yaml
```
