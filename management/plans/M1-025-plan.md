# Implementation Plan: M1-025

## Overview
Add a `validate` command to the CLI that parses and structurally checks collection files without executing HTTP requests, collecting all issues and reporting them in terminal or JSON format.

## Task Details
- **ID:** M1-025
- **Title:** Validate command (curlew validate)
- **Phase:** M1: Core CLI
- **Priority:** 25
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-016 | Setup/Teardown support | done |

## Implementation Steps

### Step 1: Export `FindReferences` from `internal/variable`
**Rationale:** Smallest blast radius — adds one exported wrapper around an unexported function. No behavioral changes to existing code. Required by the validator package.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Add exported `FindReferences` wrapper |
| `internal/variable/variable_test.go` | modify | Add tests for `FindReferences` |

#### Current Code
```go
// findVarRefs returns all variable names referenced in s via {{varName}} syntax.
func findVarRefs(s string) []string {
	matches := varPattern.FindAllStringSubmatch(s, -1)
	refs := make([]string, 0, len(matches))
	for _, m := range matches {
		refs = append(refs, m[1])
	}
	return refs
}
```

#### New Code
```go
// findVarRefs returns all variable names referenced in s via {{varName}} syntax.
func findVarRefs(s string) []string {
	matches := varPattern.FindAllStringSubmatch(s, -1)
	refs := make([]string, 0, len(matches))
	for _, m := range matches {
		refs = append(refs, m[1])
	}
	return refs
}

// FindReferences returns all static variable names referenced via {{varName}} in s.
// Dynamic function references ({{$funcName}}) are not included.
func FindReferences(s string) []string {
	return findVarRefs(s)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestFindReferences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"no_refs", "hello world", nil},
		{"single_ref", "{{foo}}", []string{"foo"}},
		{"multiple_refs", "{{a}} and {{b}}", []string{"a", "b"}},
		{"dynamic_func_excluded", "{{$uuid}}", nil},
		{"mixed_refs_and_funcs", "{{a}} {{$uuid}} {{b}}", []string{"a", "b"}},
		{"empty_string", "", nil},
		{"ref_in_url", "https://example.com/{{id}}/items", []string{"id"}},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — purely additive

---

### Step 2: Create `internal/validator/` Package
**Rationale:** New package isolates all validation logic. `ParseFile` fails fast; the validator collects all issues. Must be created before the output types and CLI wiring that depend on it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/validator/validator.go` | create | Core validation logic, types, `Validate` function |
| `internal/validator/validator_test.go` | create | Table-driven tests for all behaviors |

#### New Code
```go
package validator

import (
	"errors"
	"fmt"
	"strings"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

// Severity classifies a validation finding.
type Severity int

const (
	SeverityError   Severity = iota // blocks execution — exit 3
	SeverityWarning                 // informational — exit 0
)

// Issue represents a single validation finding.
type Issue struct {
	Severity Severity
	FilePath string
	Line     int
	Message  string
	Hint     string
}

// Result holds all validation findings for a single file.
type Result struct {
	FilePath string
	Valid    bool   // true if no SeverityError issues
	Issues   []Issue
}

// Validate performs comprehensive validation on a collection file.
// It collects ALL issues rather than failing on the first one.
// knownVars optionally provides variable names known at validate time.
func Validate(path string, knownVars map[string]string) *Result

// collectAllStringRefs recursively extracts {{varName}} references from any value.
func collectAllStringRefs(v any) []string

// knownCollectionVars returns all variable names statically knowable from the collection.
func knownCollectionVars(col *parser.Collection) map[string]bool
```

#### Tests to Write FIRST (RED phase)

```go
func TestValidate(t *testing.T) {
	// behavior 1: valid file
	{"valid_minimal_collection_exit_0", ...},
	{"valid_collection_with_all_sections", ...},

	// behavior 2: YAML syntax errors
	{"invalid_yaml_returns_parse_error", ...},
	{"invalid_yaml_includes_line_number", ...},

	// behavior 3: missing required fields
	{"missing_request_name_returns_error", ...},
	{"missing_url_returns_error", ...},
	{"unsupported_http_method_returns_error", ...},

	// behavior 4: undefined variable references
	{"undefined_variable_in_url_returns_warning", ...},
	{"defined_collection_var_no_warning", ...},
	{"defined_request_var_no_warning", ...},
	{"extracted_var_no_warning_downstream", ...},
	{"dynamic_func_ref_no_warning", ...},

	// behavior 5: external file references
	{"missing_external_ref_returns_error", ...},
	{"existing_external_ref_no_error", ...},

	// edge cases
	{"nonexistent_file_returns_error", ...},
	{"empty_requests_list_is_valid", ...},
}
```

---

### Step 3: Add Validation JSON Output Types to `internal/output`
**Rationale:** Output types depend on `validator.Result` shape. Created before CLI wiring so the command can use them cleanly.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add `ValidationJSONOutput` types and `WriteValidationJSON` |
| `internal/output/json_test.go` | modify | Add tests for validation JSON serialization |

#### New Code
```go
// ValidationJSONOutput is the top-level JSON structure for validate --format json.
type ValidationJSONOutput struct {
	Files []ValidationFileJSON `json:"files"`
	Valid bool                 `json:"valid"`
}

// ValidationFileJSON represents validation results for one file.
type ValidationFileJSON struct {
	File   string                `json:"file"`
	Valid  bool                  `json:"valid"`
	Issues []ValidationIssueJSON `json:"issues"`
}

// ValidationIssueJSON represents a single validation issue.
type ValidationIssueJSON struct {
	Severity string `json:"severity"` // "error" or "warning"
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

// WriteValidationJSON serializes validation output to JSON.
func WriteValidationJSON(w io.Writer, out *ValidationJSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteValidationJSON(t *testing.T) {
	{"empty_files_produces_valid_json", ...},
	{"single_valid_file_no_issues", ...},
	{"single_file_with_errors", ...},
	{"single_file_with_warnings", ...},
	{"multiple_files_mixed_validity", ...},
	{"line_omitted_when_zero", ...},
}
```

#### Impact on Existing Tests
- No existing tests affected — purely additive

---

### Step 4: Wire Up `validateCmd` in `cmd/curlew/main.go`
**Rationale:** CLI wiring comes last so all dependencies are in place. Tests here exercise the full command end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `case "validate"` dispatch, `validateCmd`, `parseValidateArgs`, `expandGlobs`, update `printHelp` |
| `cmd/curlew/main_test.go` | modify | Add `TestValidateCmd` table-driven tests |

#### Current Code (`main.go` switch)
```go
switch args[0] {
case "run":
    return runCmd(args[1:])
case "init":
    return initCmd(args[1:])
default:
    fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
    printHelp()
    return 1
}
```

#### New Code
```go
switch args[0] {
case "run":
    return runCmd(args[1:])
case "validate":
    return validateCmd(args[1:])
case "init":
    return initCmd(args[1:])
default:
    fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
    printHelp()
    return 1
}
```

New functions:
```go
func validateCmd(args []string) int

func parseValidateArgs(args []string) (files []string, format string, noColor bool, err error)

func expandGlobs(patterns []string) ([]string, error)

func containsGlobMeta(s string) bool {
    return strings.ContainsAny(s, "*?[")
}
```

Updated `printHelp()` to include:
```
  validate <file>   Validate collection files without executing requests
```

#### Tests to Write FIRST (RED phase)

```go
func TestValidateCmd(t *testing.T) {
	// behavior 1: valid file
	{"valid_file_exit_0", ...},
	{"valid_file_prints_valid_message", ...},

	// behavior 2: invalid YAML
	{"invalid_yaml_exit_3", ...},
	{"invalid_yaml_shows_line_number", ...},

	// behavior 3: missing required fields
	{"missing_required_field_exit_3", ...},

	// behavior 4: undefined variable warns, exit 0
	{"undefined_var_warns_exit_0", ...},

	// behavior 5: missing external file ref
	{"missing_external_file_exit_3", ...},

	// behavior 6: --format json
	{"format_json_valid_file_exit_0", ...},
	{"format_json_invalid_file_exit_3", ...},
	{"format_json_output_is_valid_json", ...},

	// behavior 7: multiple files / glob
	{"multiple_files_all_valid_exit_0", ...},
	{"multiple_files_one_invalid_exit_3", ...},
	{"glob_pattern_matches_files", ...},
	{"glob_pattern_no_matches_exit_3", ...},

	// edge cases
	{"no_args_prints_usage_exit_1", ...},
	{"nonexistent_file_exit_3", ...},
}
```

#### Impact on Existing Tests
- `TestRun_unknown_command` — unaffected (`"bogus"` still unknown)
- Help text tests — check for substrings only, adding a new command won't break them
- No existing tests affected

---

### Step 5: Update Smoke Test
**Rationale:** Final — adds observable verification to `smoke/run.sh` after all code is complete.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add validate smoke test cases |

#### New Code (additions to smoke/run.sh)
```bash
echo "=== Validate command ==="

echo "--- Validate: valid collection ---"
./curlew validate requests/hello.yaml
check_exit 0 "validate valid collection"

echo "--- Validate: missing file ---"
./curlew validate nonexistent.yaml
check_exit 3 "validate nonexistent file"

echo "--- Validate: help text shows validate ---"
./curlew help | grep -q "validate"
check_exit 0 "validate appears in help"
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/variable/variable_test.go` | `TestFindReferences` | new | write |
| `internal/validator/validator_test.go` | `TestValidate` | new | write |
| `internal/output/json_test.go` | `TestWriteValidationJSON` | new | write |
| `cmd/curlew/main_test.go` | `TestValidateCmd` | new | write |
| All existing tests | — | none | no change |

## Risks and Edge Cases

- **Risk:** `ParseFile` fails fast, can't do further structural checks on broken YAML → **Mitigation:** Accept this — a broken YAML file yields a single parse error; further validation only runs on parseable files.
- **Risk:** Variable references may be defined at runtime (`--env`, `--var`, `.env`) causing false-positive warnings → **Mitigation:** Report as warnings (exit 0), not errors. Message says "potentially unresolved" and hints at runtime sources.
- **Risk:** `Request.Body` is `any` — could be string, map, or slice → **Mitigation:** Write a recursive string scanner `collectAllStringRefs` that walks any value and extracts `{{varName}}` refs from all string leaves.
- **Risk:** `extract:` blocks create implicit downstream variable bindings → **Mitigation:** Build known-variables incrementally in request order: add each request's `extract:` keys to the known set before scanning the next request.
- **Edge case:** Empty `requests:` list → **Handling:** A collection with `name:` but empty requests is valid.
- **Edge case:** Glob pattern matches no files → **Handling:** Error with "no files matched pattern" message, exit 3.
- **Edge case:** Multiple files where some don't exist → **Handling:** Report each non-existent file as its own error; continue validating rest; exit 3 if any had errors.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Valid collection
./curlew validate requests/hello.yaml
# Expected: exit 0, "✓ requests/hello.yaml is valid" (or similar)

# Invalid YAML
echo "invalid: yaml: :" > /tmp/bad.yaml
./curlew validate /tmp/bad.yaml
# Expected: exit 3, error with line number

# Multiple files with glob
./curlew validate "requests/*.yaml"
# Expected: exit 0 if all valid

# JSON format
./curlew validate --format json requests/hello.yaml
# Expected: exit 0, JSON with {"files":[{"file":"...","valid":true,"issues":[]}],"valid":true}
```
