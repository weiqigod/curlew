# Implementation Plan: M2-020

## Overview
Extend the `internal/datadriven/` package with YAML data source support, conditional row filtering with expression evaluation, row limiting/ranges (limit, start_row, end_row), fail-fast vs continue-on-error modes, and CSV type conversion filters (`|int`, `|float`, `|bool`).

## Task Details
- **ID:** M2-020
- **Title:** Data-driven YAML support, filtering, and row control
- **Phase:** M2: Data-Driven Testing
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-019 | Data-driven testing with CSV and JSON data sources | done |

## Architecture Decisions

1. **YAML loader** follows the same pattern as `loadCSV` and `loadJSON`: a package-private `loadYAML(path string) (*DataSet, error)` function, plus a new `"yaml"` case in both `Load()` switch and `detectFormat()`.

2. **Filter expression evaluation** is implemented as a new file `filter.go` with a simple recursive-descent parser/evaluator. Expressions use the row's resolved variable values (after interpolating `{{col}}`). Operators: `==`, `!=`, `<`, `>`, `<=`, `>=`, `contains`, `starts_with`, `ends_with`, `AND`, `OR`, `NOT`. Numeric comparisons attempted first; fall back to string comparison.

3. **Row control** (limit, start_row, end_row) and **fail_fast** are new fields on `Config`. A new function `ApplyControls(ds *DataSet, cfg Config) (*DataSet, error)` slices rows before execution. Filtering is applied first, then range/limit.

4. **CSV type conversion filters** (`|int`, `|float`, `|bool`) are handled at the assertion/interpolation boundary, not during data loading. Since all CSV values are strings in `Row`, the type filters are applied when a `{{var|int}}` pattern appears. However, the current `varPattern` regex does NOT capture the pipe suffix -- it only matches `{{name}}`. The spec's "Complete Data-Driven Syntax Reference" shows `filter: "{{age}} >= 18"` which means the filter evaluator needs to parse `{{col}}` references, look them up in the row, and do numeric/string comparison. The type conversion filters (`|int` etc.) are used in assertion contexts like `status: "{{expected_status|int}}"` and are out of scope for this task's filter evaluator -- they belong to the variable interpolation system. **Decision:** For this task, implement type conversion as helper functions in the datadriven package that the filter evaluator uses internally (e.g., attempting numeric parse before comparison). Document that assertion-level `|int` filters are a separate concern.

5. **Fail-fast** is a `Config` field. The `execute()` function in `execute.go` and the runner's `executeDataDriven()` both need to respect it. Since the runner currently does its own iteration loop (not using `execute()`), the fail_fast logic goes into both places. **Decision:** Add `FailFast` to Config and check it in the runner's iteration loop. Also add it to the package-level `execute()` for consistency.

## Implementation Steps

### Step 1: Add YAML data source loader
**Rationale:** Smallest self-contained unit; extends existing pattern with no signature changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/yaml.go` | create | YAML data source loader |
| `internal/datadriven/yaml_test.go` | create | Tests for YAML loader |
| `internal/datadriven/datadriven.go` | modify | Add "yaml" case to `Load()` and `detectFormat()` |
| `internal/datadriven/datadriven_test.go` | modify | Add YAML test cases to `TestLoad` |

#### Current Code
```go
// datadriven.go - detectFormat
func detectFormat(source string) string {
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".csv":
		return "csv"
	case ".json":
		return "json"
	default:
		return ext
	}
}

// datadriven.go - Load switch
switch strings.ToLower(format) {
case "csv":
    return loadCSV(path)
case "json":
    return loadJSON(path)
default:
    return nil, fmt.Errorf("%w: %q (supported: csv, json)", ErrUnsupportedFormat, format)
}
```

#### New Code
```go
// datadriven.go - detectFormat
func detectFormat(source string) string {
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".csv":
		return "csv"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ext
	}
}

// datadriven.go - Load switch
switch strings.ToLower(format) {
case "csv":
    return loadCSV(path)
case "json":
    return loadJSON(path)
case "yaml", "yml":
    return loadYAML(path)
default:
    return nil, fmt.Errorf("%w: %q (supported: csv, json, yaml)", ErrUnsupportedFormat, format)
}

// yaml.go - loadYAML
func loadYAML(path string) (*DataSet, error) {
    // Read file, yaml.Unmarshal into []map[string]interface{}
    // Convert each map entry to Row (string values)
    // Collect column names, sort for deterministic ordering
    // Return DataSet
}
```

#### Tests to Write FIRST (RED phase)

```go
// yaml_test.go
func TestLoadYAML(t *testing.T) {
    tests := []struct {
        name     string
        content  string
        wantRows int
        wantCols []string
        wantErr  error
    }{
        {"valid list of maps", "- name: john\n  age: 30\n- name: jane\n  age: 25", 2, []string{"age", "name"}, nil},
        {"single item", "- name: alice", 1, []string{"name"}, nil},
        {"empty list", "[]", 0, nil, ErrEmptyDataFile},
        {"empty file", "", 0, nil, ErrEmptyDataFile},
        {"not a list", "name: john", 0, nil, ErrMalformedData},
        {"list of non-maps", "- hello\n- world", 0, nil, ErrMalformedData},
        {"mixed keys across items", "- name: john\n  email: j@x.com\n- name: jane\n  phone: 555", 2, []string{"email", "name", "phone"}, nil},
        {"nested values flattened to string", "- name: john\n  meta:\n    age: 30", 1, nil, nil},
        {"boolean and null values", "- active: true\n  count: null", 1, nil, nil},
        {"yml extension via Load", "", 0, nil, nil}, // tested in datadriven_test.go
    }
}

func TestLoadYAML_RowValues(t *testing.T) { /* verify actual values */ }
func TestLoadYAML_TypeConversion(t *testing.T) { /* numbers, booleans, nulls -> strings */ }
```

#### Impact on Existing Tests
- `TestLoad` in `datadriven_test.go` -- add test cases for `.yaml` and `.yml` auto-detection; no existing cases break
- No other test files affected

---

### Step 2: Extend Config with control fields
**Rationale:** Must define the Config fields before filter/control logic can use them. Pure struct change, no behavior yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/datadriven.go` | modify | Add fields to Config struct |

#### Current Code
```go
type Config struct {
	Source string `yaml:"source"`
	Format string `yaml:"format,omitempty"`
}
```

#### New Code
```go
type Config struct {
	Source   string `yaml:"source"`
	Format  string `yaml:"format,omitempty"`
	Filter  string `yaml:"filter,omitempty"`
	Limit   *int   `yaml:"limit,omitempty"`
	StartRow *int  `yaml:"start_row,omitempty"`
	EndRow   *int  `yaml:"end_row,omitempty"`
	FailFast bool  `yaml:"fail_fast,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)
No separate tests needed -- the Config fields are tested through the filter and control functions in subsequent steps.

#### Impact on Existing Tests
- No existing tests break; all new fields have zero values that preserve current behavior.

---

### Step 3: Implement filter expression evaluator
**Rationale:** Filter logic is the most complex new feature; isolating it in its own file keeps the blast radius small and makes it independently testable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/filter.go` | create | Expression parser and evaluator |
| `internal/datadriven/filter_test.go` | create | Comprehensive filter tests |

#### New Code
```go
// filter.go

// ErrInvalidFilter is returned when a filter expression cannot be parsed.
var ErrInvalidFilter = errors.New("invalid filter expression")

// EvalFilter evaluates a filter expression against a data row.
// Variable references like {{col}} are resolved from the row.
// Returns true if the row matches the filter, false otherwise.
func EvalFilter(expr string, row Row) (bool, error)

// Internal: tokenize, parse, evaluate
// Supports: ==, !=, <, >, <=, >=, contains, starts_with, ends_with, AND, OR, NOT
// Numeric comparison: if both sides parse as float64, compare numerically
// String comparison: lexicographic for <, >, <=, >=
```

#### Tests to Write FIRST (RED phase)

```go
func TestEvalFilter(t *testing.T) {
    tests := []struct {
        name   string
        expr   string
        row    Row
        want   bool
        wantErr error
    }{
        // Comparison operators
        {"equals string", "{{name}} == 'john'", Row{"name": "john"}, true, nil},
        {"equals string false", "{{name}} == 'jane'", Row{"name": "john"}, false, nil},
        {"not equals", "{{name}} != 'john'", Row{"name": "jane"}, true, nil},
        {"greater than numeric", "{{age}} > 18", Row{"age": "25"}, true, nil},
        {"greater than numeric false", "{{age}} > 18", Row{"age": "15"}, false, nil},
        {"greater than or equal", "{{age}} >= 18", Row{"age": "18"}, true, nil},
        {"less than", "{{age}} < 18", Row{"age": "15"}, true, nil},
        {"less than or equal", "{{age}} <= 18", Row{"age": "18"}, true, nil},

        // String operators
        {"contains true", "{{email}} contains 'example'", Row{"email": "a@example.com"}, true, nil},
        {"contains false", "{{email}} contains 'test'", Row{"email": "a@example.com"}, false, nil},
        {"starts_with true", "{{name}} starts_with 'jo'", Row{"name": "john"}, true, nil},
        {"starts_with false", "{{name}} starts_with 'ja'", Row{"name": "john"}, false, nil},
        {"ends_with true", "{{email}} ends_with '.com'", Row{"email": "a@example.com"}, true, nil},
        {"ends_with false", "{{email}} ends_with '.org'", Row{"email": "a@example.com"}, false, nil},

        // Boolean logic
        {"AND both true", "{{age}} >= 18 AND {{country}} == 'US'", Row{"age": "25", "country": "US"}, true, nil},
        {"AND one false", "{{age}} >= 18 AND {{country}} == 'US'", Row{"age": "25", "country": "UK"}, false, nil},
        {"OR one true", "{{age}} >= 18 OR {{country}} == 'US'", Row{"age": "15", "country": "US"}, true, nil},
        {"OR both false", "{{age}} >= 18 OR {{country}} == 'US'", Row{"age": "15", "country": "UK"}, false, nil},
        {"NOT true", "NOT {{active}} == 'false'", Row{"active": "true"}, true, nil},
        {"NOT false", "NOT {{active}} == 'true'", Row{"active": "true"}, false, nil},

        // Edge cases
        {"empty expression", "", Row{"a": "1"}, true, nil},
        {"missing variable", "{{missing}} == 'x'", Row{}, false, nil},
        {"numeric string comparison", "{{val}} > '5'", Row{"val": "10"}, true, nil},
        {"float comparison", "{{price}} >= 9.99", Row{"price": "10.50"}, true, nil},

        // Error cases
        {"invalid operator", "{{age}} LIKE 'john'", Row{"age": "25"}, false, ErrInvalidFilter},
        {"unclosed variable", "{{age >= 18", Row{"age": "25"}, false, ErrInvalidFilter},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new file)

---

### Step 4: Implement row control (ApplyControls)
**Rationale:** Depends on filter evaluator from Step 3. Combines filter, start_row, end_row, and limit into a single DataSet transformation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/control.go` | create | Row control logic (filter, limit, range) |
| `internal/datadriven/control_test.go` | create | Tests for row control |

#### New Code
```go
// control.go

// ApplyControls filters and slices a DataSet according to the Config settings.
// Order of operations: filter -> start_row/end_row range -> limit.
// Returns a new DataSet (original is not modified).
func ApplyControls(ds *DataSet, cfg Config) (*DataSet, error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestApplyControls(t *testing.T) {
    tests := []struct {
        name     string
        rows     []Row
        cfg      Config
        wantRows int
        wantErr  error
    }{
        {"no controls", makeRows(10), Config{}, 10, nil},
        {"limit 5", makeRows(10), Config{Limit: intPtr(5)}, 5, nil},
        {"limit exceeds rows", makeRows(3), Config{Limit: intPtr(10)}, 3, nil},
        {"limit zero", makeRows(10), Config{Limit: intPtr(0)}, 0, nil},
        {"start_row 3", makeRows(10), Config{StartRow: intPtr(3)}, 7, nil},
        {"end_row 5", makeRows(10), Config{EndRow: intPtr(5)}, 6, nil},
        {"start_row and end_row", makeRows(10), Config{StartRow: intPtr(2), EndRow: intPtr(5)}, 4, nil},
        {"start_row beyond length", makeRows(5), Config{StartRow: intPtr(10)}, 0, nil},
        {"end_row before start_row", makeRows(10), Config{StartRow: intPtr(5), EndRow: intPtr(3)}, 0, nil},
        {"filter age >= 18", makeAgeRows(), Config{Filter: "{{age}} >= 18"}, /* depends on data */, nil},
        {"filter then limit", makeAgeRows(), Config{Filter: "{{age}} >= 18", Limit: intPtr(2)}, 2, nil},
        {"filter with invalid expression", makeRows(5), Config{Filter: "{{x}} LIKE 'a'"}, 0, ErrInvalidFilter},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new file)

---

### Step 5: Implement CSV type conversion filters
**Rationale:** Needed for the filter evaluator to properly handle numeric comparisons from CSV string data; also needed per spec for assertion contexts like `{{expected_status|int}}`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/typeconv.go` | create | Type conversion filter functions |
| `internal/datadriven/typeconv_test.go` | create | Tests for type conversion |

#### New Code
```go
// typeconv.go

// ConvertValue applies a type conversion filter to a string value.
// Supported filters: "int", "float", "bool".
// Returns the converted value as an interface{} and any error.
func ConvertValue(value, filter string) (interface{}, error)

// TryNumeric attempts to parse a string as a number (int64 then float64).
// Returns the numeric value and true, or 0 and false.
func TryNumeric(s string) (float64, bool)
```

#### Tests to Write FIRST (RED phase)

```go
func TestConvertValue(t *testing.T) {
    tests := []struct {
        name    string
        value   string
        filter  string
        want    interface{}
        wantErr bool
    }{
        {"int valid", "42", "int", int64(42), false},
        {"int invalid", "abc", "int", nil, true},
        {"int float string", "3.14", "int", nil, true},
        {"float valid", "3.14", "float", 3.14, false},
        {"float integer", "42", "float", 42.0, false},
        {"float invalid", "abc", "float", nil, true},
        {"bool true", "true", "bool", true, false},
        {"bool false", "false", "bool", false, false},
        {"bool yes", "yes", "bool", true, false},
        {"bool 1", "1", "bool", true, false},
        {"bool 0", "0", "bool", false, false},
        {"bool invalid", "maybe", "bool", nil, true},
        {"unknown filter", "42", "date", nil, true},
    }
}

func TestTryNumeric(t *testing.T) {
    tests := []struct {
        name string
        s    string
        want float64
        ok   bool
    }{
        {"integer", "42", 42.0, true},
        {"negative", "-5", -5.0, true},
        {"float", "3.14", 3.14, true},
        {"not numeric", "abc", 0, false},
        {"empty", "", 0, false},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new file)

---

### Step 6: Integrate controls into Load and execute flow
**Rationale:** Wire the new ApplyControls into the Load path or provide guidance for the runner to call it. Also add fail_fast support to the `execute()` function.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/datadriven.go` | modify | Add `LoadWithControls` function |
| `internal/datadriven/execute.go` | modify | Add fail_fast support to `execute()` |
| `internal/datadriven/datadriven_test.go` | modify | Add integration tests |
| `internal/datadriven/execute_test.go` | modify | Add fail_fast tests |

#### New Code
```go
// datadriven.go

// LoadWithControls loads a data source and applies row controls (filter, range, limit).
// This is the primary entry point for data-driven execution.
func LoadWithControls(cfg Config, baseDir string) (*DataSet, error) {
    ds, err := Load(cfg, baseDir)
    if err != nil {
        return nil, err
    }
    return ApplyControls(ds, cfg)
}
```

```go
// execute.go - modify executeConfig
type executeConfig struct {
    DataSet  *DataSet
    Scope    *variable.Scope
    ExecFn   func(ctx context.Context, iterScope *variable.Scope, index int) (*iterationResult, error)
    FailFast bool // stop on first failure
}

// execute() - add fail_fast check after each iteration
if cfg.FailFast && result.Err != nil {
    results = append(results, *result)
    break
}
```

#### Tests to Write FIRST (RED phase)

```go
// execute_test.go
func TestExecute_FailFast(t *testing.T) {
    // 5 rows, 3rd fails, fail_fast=true -> only 3 results
}

func TestExecute_ContinueOnError(t *testing.T) {
    // 5 rows, 3rd fails, fail_fast=false (default) -> 5 results
}

// datadriven_test.go
func TestLoadWithControls_FilterAndLimit(t *testing.T) {
    // Create CSV with age column, filter age >= 18, limit 2
}

func TestLoadWithControls_Range(t *testing.T) {
    // 10 rows, start_row=2, end_row=5 -> 4 rows
}
```

#### Impact on Existing Tests
- `TestExecute` -- no break, executeConfig gains a FailFast field with zero value (false), preserving existing behavior
- All existing execute tests continue to pass without modification

---

### Step 7: Wire into runner (fail_fast + LoadWithControls)
**Rationale:** Last step, highest blast radius. Updates the runner to use the new controls and fail_fast from Config.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Use `LoadWithControls` instead of `Load`; add fail_fast iteration break |
| `internal/runner/runner_test.go` | modify | Add tests for fail_fast behavior and filtered data-driven execution |

#### Current Code
```go
// runner.go line 807
ds, loadErr := datadriven.Load(*item.DataDriven, baseDir)
```

#### New Code
```go
// runner.go
ds, loadErr := datadriven.LoadWithControls(*item.DataDriven, baseDir)
```

And in the iteration loop (around line 827):
```go
// After processing each iteration result, check fail_fast
if item.DataDriven.FailFast {
    iterFailed := rr.Err != nil || (rr.AssertionResults != nil && !rr.AssertionResults.Passed)
    if iterFailed {
        break
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// runner_test.go
func TestExecuteDataDriven_FailFast(t *testing.T) {
    // Collection with data_driven.fail_fast: true, server returns 500 on 2nd request
    // Verify only 2 iterations executed
}

func TestExecuteDataDriven_FilteredRows(t *testing.T) {
    // Collection with data_driven.filter, verify only matching rows execute
}

func TestExecuteDataDriven_LimitedRows(t *testing.T) {
    // Collection with data_driven.limit: 3, data file has 10 rows
    // Verify only 3 iterations
}

func TestExecuteDataDriven_YAMLSource(t *testing.T) {
    // Collection with .yaml data source, verify iterations work
}
```

#### Impact on Existing Tests
- `TestExecuteDataDriven_*` in runner_test.go -- the signature of `datadriven.Load` is not changed (LoadWithControls is additive), but the runner's call site changes from `Load` to `LoadWithControls`. Existing tests pass because Config has zero-value controls (no filter, no limit, no range).
- If any runner test constructs a `datadriven.Config` directly, the new fields default to nil/false, preserving behavior.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/datadriven/datadriven_test.go` | `TestLoad` | add cases | add yaml/yml auto-detect cases |
| `internal/datadriven/execute_test.go` | `TestExecute` | struct field added | none (zero value preserves behavior) |
| `internal/runner/runner_test.go` | `TestExecuteDataDriven_*` | call site change | verify pass with LoadWithControls |
| All other tests | -- | none | -- |

## Risks and Edge Cases

- **Risk:** Filter expression parser complexity -- a full recursive-descent parser adds significant code.
  **Mitigation:** Keep grammar minimal (no parentheses grouping in v1; AND/OR are left-associative). Document limitations. NOT binds tighter than AND/OR.

- **Risk:** YAML type coercion -- `gopkg.in/yaml.v3` automatically converts `true`, `yes`, `on` to booleans and numbers to int/float. The loader must normalize everything to strings.
  **Mitigation:** Unmarshal into `[]map[string]interface{}` and use a `yamlValueToString()` helper similar to `jsonValueToString()`.

- **Edge case:** Filter references a column that doesn't exist in a row -> treat as empty string (match fails), not error.
  **Handling:** `EvalFilter` returns `false` for missing variables without error.

- **Edge case:** `start_row` > dataset length -> return empty DataSet (not error).
  **Handling:** `ApplyControls` clamps to bounds.

- **Edge case:** `end_row` < `start_row` -> return empty DataSet.
  **Handling:** `ApplyControls` returns empty slice.

- **Edge case:** `limit: 0` -> return empty DataSet (intentional "run nothing").
  **Handling:** `ApplyControls` respects 0 as a valid limit.

- **Edge case:** Numeric comparison where one side is not a number -> fall back to string comparison.
  **Handling:** `TryNumeric` returns false, comparison proceeds lexicographically.

- **Edge case:** YAML file with `.yml` extension -> must be recognized by `detectFormat`.
  **Handling:** Add `.yml` case to switch.

- **Risk:** The runner's iteration loop duplicates some logic from `execute()`.
  **Mitigation:** Only add fail_fast to the runner loop; keep changes minimal. Future refactoring can unify the loops.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create a YAML data file and confirm data-driven execution
cat > /tmp/test-users.yaml << 'EOF'
- username: john_doe
  email: john@example.com
  age: 32
  role: admin

- username: jane_smith
  email: jane@example.com
  age: 28
  role: user

- username: minor_user
  email: minor@example.com
  age: 15
  role: user
EOF

# Run tests
go test ./internal/datadriven/... -v -count=1

# Verify filter tests pass
go test ./internal/datadriven/... -run TestEvalFilter -v

# Verify control tests pass
go test ./internal/datadriven/... -run TestApplyControls -v

# Verify YAML tests pass
go test ./internal/datadriven/... -run TestLoadYAML -v

# Verify fail_fast tests pass
go test ./internal/datadriven/... -run TestExecute_FailFast -v
```
