# Implementation Plan: M2-019

## Overview
Add data-driven testing with CSV and JSON data sources, enabling a single request definition to execute multiple times with different data values. This is the foundational task — it covers data loading, sequential iteration execution, special variables, extraction accumulation, feature gating, and error handling for missing/empty files.

## Task Details
- **ID:** M2-019
- **Title:** Data-driven testing with CSV and JSON data sources
- **Phase:** M2: Data-Driven Testing
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-009 | Collection variables | done |
| M1-010 | Variable extraction | done |
| M1-028 | Feature gate framework | done |

## Implementation Steps

### Step 1: Create `internal/datadriven/` package — data source types and loading

**Rationale:** The data model is the foundation. All other code (parsing, execution, runner integration) depends on these types and loaders. Starting here has zero blast radius on existing code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/datadriven.go` | create | Config type, sentinel errors, Row/DataSet types |
| `internal/datadriven/datadriven_test.go` | create | Tests for Config validation and edge cases |
| `internal/datadriven/csv.go` | create | CSV file loader |
| `internal/datadriven/csv_test.go` | create | CSV loading tests (valid, empty, malformed) |
| `internal/datadriven/json.go` | create | JSON file loader |
| `internal/datadriven/json_test.go` | create | JSON loading tests (array, nested, empty) |

#### New Code — Types

```go
package datadriven

import (
    "errors"
    "fmt"
    "path/filepath"
)

// Sentinel errors for data-driven testing failures.
var (
    ErrFileNotFound     = errors.New("data file not found")
    ErrEmptyDataFile    = errors.New("data file is empty")
    ErrUnsupportedFormat = errors.New("unsupported data format")
    ErrMalformedData    = errors.New("malformed data file")
)

// Config holds the data-driven configuration parsed from a collection YAML.
type Config struct {
    Source   string `yaml:"source"`
    Format   string `yaml:"format,omitempty"`   // csv, json; auto-detected from extension if empty
}

// Row represents a single iteration's data — column name to string value.
type Row map[string]string

// DataSet holds all rows loaded from a data source.
type DataSet struct {
    Rows    []Row
    Columns []string // ordered column names (from CSV header or JSON keys)
}

// Load reads a data file relative to baseDir and returns the parsed dataset.
func Load(cfg Config, baseDir string) (*DataSet, error) {
    // Resolve path, detect format, delegate to CSV/JSON loader
}
```

#### Tests to Write FIRST (RED phase)

```go
// csv_test.go
func TestLoadCSV(t *testing.T) {
    tests := []struct {
        name     string
        content  string
        wantRows int
        wantCols []string
        wantErr  error
    }{
        {"valid 3 rows", "name,email\njohn,john@x.com\njane,jane@x.com\nbob,bob@x.com", 3, []string{"name", "email"}, nil},
        {"single row", "name\nalice", 1, []string{"name"}, nil},
        {"empty file no header", "", 0, nil, ErrEmptyDataFile},
        {"header only no data", "name,email\n", 0, nil, ErrEmptyDataFile},
        {"inconsistent columns", "name,email\njohn", 0, nil, ErrMalformedData},
    }
}

// json_test.go
func TestLoadJSON(t *testing.T) {
    tests := []struct {
        name     string
        content  string
        wantRows int
        wantErr  error
    }{
        {"valid array of objects", `[{"name":"john"},{"name":"jane"}]`, 2, nil},
        {"empty array", `[]`, 0, ErrEmptyDataFile},
        {"nested values flattened to string", `[{"name":"john","meta":{"age":30}}]`, 1, nil},
        {"not an array", `{"name":"john"}`, 0, ErrMalformedData},
        {"invalid JSON", `[{broken`, 0, ErrMalformedData},
    }
}

// datadriven_test.go
func TestLoad(t *testing.T) {
    tests := []struct {
        name    string
        cfg     Config
        file    string // relative path within temp dir
        content string
        wantErr error
    }{
        {"auto-detect CSV from extension", Config{Source: "data.csv"}, "data.csv", "a\n1", nil},
        {"auto-detect JSON from extension", Config{Source: "data.json"}, "data.json", `[{"a":"1"}]`, nil},
        {"explicit format overrides extension", Config{Source: "data.txt", Format: "csv"}, "data.txt", "a\n1", nil},
        {"file not found", Config{Source: "missing.csv"}, "", "", ErrFileNotFound},
        {"unsupported format", Config{Source: "data.xml"}, "data.xml", "<x/>", ErrUnsupportedFormat},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — this is a new package.

---

### Step 2: Add special iteration variables and iteration execution logic

**Rationale:** This step adds the core iteration loop that executes a request once per row, injecting row data and special variables (`_index`, `_iteration`, `_total`, `_row_number`) into the scope. Still isolated in `internal/datadriven/`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/execute.go` | create | Iteration execution loop with special variable injection |
| `internal/datadriven/execute_test.go` | create | Tests for iteration execution, special vars, extraction accumulation |

#### New Code

```go
package datadriven

import (
    "context"
    "fmt"

    "github.com/peterlindqvist/apitest/internal/variable"
)

// IterationResult holds the outcome of a single data-driven iteration.
type IterationResult struct {
    Index      int               // 0-based
    Row        Row               // data for this iteration
    Err        error             // execution error (nil = success)
    Extracted  map[string]string // variables extracted from this iteration
    // Fields populated by the caller (runner integration)
    Name       string            // formatted name (e.g., "Create User [1/3]")
}

// ExecuteConfig holds the configuration for executing data-driven iterations.
type ExecuteConfig struct {
    DataSet     *DataSet
    Scope       *variable.Scope
    ExecFn      func(ctx context.Context, iterScope *variable.Scope, index int) (*IterationResult, error)
}

// InjectIterationVars adds _index, _iteration, _total, _row_number, and row
// column values to a scope snapshot. Row data has highest precedence (overrides
// collection defaults per spec).
func InjectIterationVars(scope *variable.Scope, row Row, index, total int) *variable.Scope {
    snap := scope.Snapshot()
    // Special iteration variables
    snap.Set("_index", fmt.Sprintf("%d", index))
    snap.Set("_iteration", fmt.Sprintf("%d", index+1))
    snap.Set("_total", fmt.Sprintf("%d", total))
    snap.Set("_row_number", fmt.Sprintf("%d", index+1))
    // Row data (highest precedence — overrides all)
    for k, v := range row {
        snap.Set(k, v)
    }
    return snap
}

// Execute runs the data-driven iterations sequentially.
// Returns results for each iteration. Extraction values accumulate as arrays.
func Execute(ctx context.Context, cfg ExecuteConfig) ([]IterationResult, map[string][]string, error) {
    // Sequential loop over DataSet.Rows
    // For each row: InjectIterationVars, call ExecFn, collect extraction
    // Return accumulated extractions (variable -> []value for array accumulation)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestInjectIterationVars(t *testing.T) {
    tests := []struct {
        name       string
        row        Row
        index      int
        total      int
        wantIndex  string
        wantIter   string
        wantTotal  string
        wantRowNum string
        wantData   map[string]string
    }{
        {"first of three", Row{"email": "a@b.com"}, 0, 3, "0", "1", "3", "1", map[string]string{"email": "a@b.com"}},
        {"last of three", Row{"email": "c@d.com"}, 2, 3, "2", "3", "3", "3", map[string]string{"email": "c@d.com"}},
        {"single iteration", Row{"x": "1"}, 0, 1, "0", "1", "1", "1", map[string]string{"x": "1"}},
    }
}

func TestExecute(t *testing.T) {
    tests := []struct {
        name             string
        rows             []Row
        wantIterations   int
        wantAccumulated  map[string]int // variable name -> expected array length
    }{
        {"three iterations", []Row{{"a": "1"}, {"a": "2"}, {"a": "3"}}, 3, nil},
        {"extraction accumulates as array", []Row{{"a": "1"}, {"a": "2"}}, 2, map[string]int{"user_id": 2}},
    }
}

func TestExecute_ContextCancellation(t *testing.T) {
    // Cancel context mid-iteration
}
```

#### Impact on Existing Tests
- No existing tests affected — new files only.

---

### Step 3: Parse `data_driven:` block in collection YAML

**Rationale:** This modifies the parser to recognize the `data_driven` field on `RequestItem`. It's a small, focused change to `internal/parser/collection.go` that unlocks the runner integration step. Existing parser tests continue to pass since `data_driven` is optional.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `DataDriven *datadriven.Config` field to `RequestItem` |
| `internal/parser/parser_test.go` | modify | Add test cases for data_driven parsing |

#### Current Code
```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
    Name       string            `yaml:"name"`
    Path       string            `yaml:"path,omitempty"`
    Auth       string            `yaml:"auth,omitempty"`
    Required   *bool             `yaml:"required,omitempty"`
    Retry      *retry.FullConfig `yaml:"retry,omitempty"`
    Request    Request           `yaml:"request"`
    Variables  SensitiveVars     `yaml:"variables,omitempty"`
    Assertions Assertions        `yaml:"assertions,omitempty"`
    Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### New Code
```go
import "github.com/peterlindqvist/apitest/internal/datadriven"

// RequestItem is a single test entry in a collection.
type RequestItem struct {
    Name       string             `yaml:"name"`
    Path       string             `yaml:"path,omitempty"`
    Auth       string             `yaml:"auth,omitempty"`
    Required   *bool              `yaml:"required,omitempty"`
    Retry      *retry.FullConfig  `yaml:"retry,omitempty"`
    DataDriven *datadriven.Config `yaml:"data_driven,omitempty"`
    Request    Request            `yaml:"request"`
    Variables  SensitiveVars      `yaml:"variables,omitempty"`
    Assertions Assertions         `yaml:"assertions,omitempty"`
    Extract    map[string]string  `yaml:"extract,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
// In parser_test.go - add test cases to TestParseFile
{
    name: "collection with data_driven CSV source",
    file: "testdata/data_driven_csv.yaml",
    wantCol: &Collection{
        Name: "Data Driven Test",
        Requests: Section{Items: []RequestItem{
            {
                Name: "Test Users",
                DataDriven: &datadriven.Config{Source: "./users.csv"},
                Request: Request{Method: "POST", URL: "https://example.com/users"},
            },
        }},
    },
},
{
    name: "collection with data_driven explicit JSON format",
    file: "testdata/data_driven_json.yaml",
    wantCol: &Collection{
        Name: "Data Driven JSON",
        Requests: Section{Items: []RequestItem{
            {
                Name: "Test Users",
                DataDriven: &datadriven.Config{Source: "./users.json", Format: "json"},
                Request: Request{Method: "GET", URL: "https://example.com"},
            },
        }},
    },
},
{
    name: "collection without data_driven parses as before",
    file: "testdata/minimal.yaml",
    // existing test — DataDriven should be nil
},
```

Also add testdata YAML files:
```yaml
# testdata/data_driven_csv.yaml
name: Data Driven Test
requests:
  - name: Test Users
    data_driven:
      source: ./users.csv
    request:
      method: POST
      url: https://example.com/users
```

#### Impact on Existing Tests
- No existing tests break — `DataDriven` is a new optional field with `omitempty`.
- All existing parser tests continue to pass (DataDriven will be nil).

---

### Step 4: Register feature gate for data-driven testing (Professional tier)

**Rationale:** Small, isolated change to `internal/auth/registry.go`. Must be done before runner integration so the feature gate check works.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Register `data_driven` feature as Professional tier |
| `internal/auth/gate_test.go` | modify | Add test for data_driven feature gate |

#### Current Code
```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    // ... existing registrations ...
    r.Register(FeatureDefinition{
        Name:         "parallel_execution",
        RequiredTier: TierProfessional,
        Description:  "Parallel execution requires Professional tier ($19/month)",
        Workaround:   "Requests execute sequentially in Free and Solo tiers",
    })
    return r
}
```

#### New Code
```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    // ... existing registrations ...
    r.Register(FeatureDefinition{
        Name:         "parallel_execution",
        RequiredTier: TierProfessional,
        Description:  "Parallel execution requires Professional tier ($19/month)",
        Workaround:   "Requests execute sequentially in Free and Solo tiers",
    })
    r.Register(FeatureDefinition{
        Name:         "data_driven",
        RequiredTier: TierProfessional,
        Description:  "Data-driven testing requires Professional tier ($19/month)",
        Workaround:   "Duplicate requests manually for different data values",
    })
    return r
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckFeature_DataDriven(t *testing.T) {
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
}
```

#### Impact on Existing Tests
- No existing tests break — adding a new feature definition does not affect existing lookups.

---

### Step 5: Integrate data-driven execution into the runner

**Rationale:** This is the core integration step. When `executePhase` encounters a `RequestItem` with `DataDriven != nil`, it loads the data source, checks the feature gate, and runs the iteration loop. Each iteration counts toward the guard rail. Extracted variables accumulate as arrays. This is the largest step but depends on all previous steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add data-driven detection and execution in `executePhase` |
| `internal/runner/runner_test.go` | modify | Add data-driven integration tests |

#### Current Code (executePhase loop)
```go
for _, item := range items {
    // ... guard rail, context check, variable scoping ...
    // ... interpolate, execute, assert, extract ...
}
```

#### New Code (data-driven branch in executePhase)
```go
for _, item := range items {
    // ... existing guard rail, context, skip checks ...

    // Data-driven execution: run once per row from data source
    if item.DataDriven != nil {
        // Feature gate check
        reg := vars.Registry
        if reg == nil { reg = auth.DefaultRegistry() }
        tier := vars.Tier
        if tier == "" { tier = auth.TierFree }
        if gateErr := auth.CheckFeature(reg, "data_driven", tier); gateErr != nil {
            return results, requiredFailed, gateErr
        }

        // Load data
        ds, loadErr := datadriven.Load(*item.DataDriven, vars.ProjectRoot)
        if loadErr != nil {
            if errors.Is(loadErr, datadriven.ErrEmptyDataFile) {
                // Empty file: skip with warning (exit code 0)
                results = append(results, RequestResult{
                    Name: item.Name, Phase: phase, Skipped: true,
                    SkipReason: "data file is empty", WaveIndex: -1,
                })
                continue
            }
            return results, requiredFailed, fmt.Errorf("request %q data_driven: %w", item.Name, loadErr)
        }

        // Execute iterations sequentially
        accumulated := make(map[string][]string) // for array extraction
        for idx, row := range ds.Rows {
            if *counter >= maxRequests { break }
            iterScope := datadriven.InjectIterationVars(scope, row, idx, len(ds.Rows))
            // ... interpolate with iterScope, execute, assert, collect extraction ...
            // Accumulate extracted values as arrays
        }
        // Set accumulated arrays into scope
        for k, vals := range accumulated {
            scope.Set(k, formatArray(vals))
        }
        continue
    }

    // ... existing non-data-driven execution ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_DataDriven_CSVSource(t *testing.T) {
    // Create temp CSV file with 3 rows
    // Create collection with data_driven pointing to CSV
    // Execute with Professional tier
    // Verify 3 results (one per row)
    // Verify each result has correct interpolated data
}

func TestRun_DataDriven_JSONSource(t *testing.T) {
    // Same as CSV but with JSON array data source
}

func TestRun_DataDriven_SpecialVars(t *testing.T) {
    // Verify _index, _iteration, _total, _row_number are available
    // Use them in URL or body interpolation
}

func TestRun_DataDriven_ExtractionAccumulates(t *testing.T) {
    // Configure extract on data-driven request
    // Verify extracted values accumulate as JSON array string
    // Verify subsequent requests can access accumulated array
}

func TestRun_DataDriven_FeatureGate_FreeTier(t *testing.T) {
    // Use Free tier -> expect GateError with exit code 6
}

func TestRun_DataDriven_MissingFile(t *testing.T) {
    // Reference missing CSV -> expect error with clear message and suggestions
}

func TestRun_DataDriven_EmptyFile(t *testing.T) {
    // Empty data file -> skip with warning, exit code 0
}

func TestRun_DataDriven_GuardRailCountsIterations(t *testing.T) {
    // Set MaxRequests = 5, run data-driven with 10 rows
    // Verify only 5 iterations execute
}

func TestRun_DataDriven_RowDataOverridesCollectionVars(t *testing.T) {
    // Collection var "email" = "default@x.com"
    // CSV row has "email" = "row@x.com"
    // Verify row value wins (iteration data is highest precedence)
}
```

#### Impact on Existing Tests
- No existing runner tests break — the new code path only triggers when `item.DataDriven != nil`, which no existing test data uses.
- `executePhase` signature does not change.

---

### Step 6: Wire data-driven into `cmd/apitest/main.go` (if needed) and add integration test

**Rationale:** The `main.go` already calls `runner.Run` which calls `executePhase`. Since data-driven is parsed from the collection YAML and checked inside the runner, no changes to `main.go` are needed for basic functionality. However, the `baseDir` (for resolving relative data file paths) needs to be available. The `VarSources.ProjectRoot` or the collection file's directory should be used. We need to verify this works end-to-end with a smoke test.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Pass collection directory to data-driven loader (use filepath.Dir of collection path, or VarSources.ProjectRoot) |
| `smoke/run.sh` | modify | Add data-driven smoke test case |

#### Current Code
The `VarSources.ProjectRoot` already exists and is passed through. We need to ensure the data file path is resolved relative to the collection file's directory, not ProjectRoot. This means `executePhase` needs the collection directory. Currently it doesn't receive it.

**Decision:** Add a `CollectionDir` field to `VarSources` (set from `filepath.Dir(file)` in main.go) or pass it separately. Since `VarSources` already has `ProjectRoot`, adding `CollectionDir` is cleaner.

#### New Code
```go
// In VarSources:
CollectionDir string // directory of the collection file (for resolving relative data file paths)
```

In `main.go`, set:
```go
CollectionDir: filepath.Dir(file),
```

#### Tests to Write FIRST (RED phase)

```go
// Integration test in runner_test.go
func TestRun_DataDriven_EndToEnd(t *testing.T) {
    // Create temp dir with CSV file and collection YAML
    // Parse collection, run with Professional tier
    // Verify correct number of iterations, correct interpolation
}
```

#### Impact on Existing Tests
- Adding `CollectionDir` to `VarSources` is additive — existing tests that don't set it will have empty string, which is fine (data-driven path resolution will use the empty string as base, same as current behavior for any path-relative logic).

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | `TestParseFile` | extended | add data_driven test cases |
| `internal/auth/gate_test.go` | (new test) | new | add data_driven gate test |
| `internal/runner/runner_test.go` | `TestRun` | extended | add data-driven execution tests |
| All existing tests | all | none | no changes needed |

## Risks and Edge Cases

- **Risk:** Data file path resolution across different working directories
  - **Mitigation:** Resolve relative to collection file directory (passed via `CollectionDir` in `VarSources`). Add test with nested directory structure.

- **Risk:** Large CSV files consuming excessive memory
  - **Mitigation:** For M2-019, load entire file into memory (streaming is Phase 3 per spec). Document the limitation. The spec says >10,000 rows triggers a warning in Phase 3.

- **Edge case:** CSV with quoted fields containing commas and newlines
  - **Handling:** Use Go's `encoding/csv` package which handles RFC 4180 correctly (quoted fields, embedded commas, embedded newlines).

- **Edge case:** JSON array with mixed types (some objects have extra fields)
  - **Handling:** Each object's keys become that row's columns. Missing keys in a row = missing variable (not injected, so the variable falls through to lower-precedence sources).

- **Edge case:** Row data variable name conflicts with special vars (_index, _iteration)
  - **Handling:** Row data is injected first, then special vars are set on top. Per spec, special vars are always available. If a CSV column is named `_index`, the special var wins (document this).

- **Edge case:** Extraction accumulation format
  - **Handling:** Accumulated values stored as JSON array string (e.g., `[1001,1002,1003]`). Access via `{{user_id}}` returns the full JSON array. Individual access (`{{user_id[0]}}`) is a future enhancement (M2-020+).

- **Risk:** `data_driven` with `path:` (external request file)
  - **Mitigation:** Data-driven configuration lives on the `RequestItem`, which already supports `path:` for external requests. Both features should compose naturally since external request resolution happens in the parser before the runner sees the item.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create test data file
cat > /tmp/test-users.csv << 'EOF'
email,expected_status
john@example.com,200
jane@example.com,200
invalid,400
EOF

# Create collection file
cat > /tmp/data-driven-test.yaml << 'EOF'
name: Data-Driven Test
requests:
  - name: Test Users
    data_driven:
      source: /tmp/test-users.csv
    request:
      method: GET
      url: "https://httpbin.org/status/{{expected_status}}"
EOF

# Run (requires Professional tier — will get feature gate on Free tier)
apitest run /tmp/data-driven-test.yaml

# Run tests
go test ./internal/datadriven/...
```
