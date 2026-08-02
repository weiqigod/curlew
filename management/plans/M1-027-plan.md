# Implementation Plan: M1-027

## Overview

Add `info` and `schema` commands to the CLI. `info` shows project metadata (collections, environments, root path) in human-readable or JSON format. `schema` outputs the JSON Schema for the collection file format.

## Task Details
- **ID:** M1-027
- **Title:** AI introspection commands (info, schema)
- **Phase:** M1: Core CLI
- **Priority:** 27
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-025 | Validate command | done |

## Implementation Steps

### Step 1: Add Collection Discovery to Config Package
**Rationale:** Smallest blast radius — a pure utility function with no side effects, tested in isolation before wiring into any command.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/discovery.go` | create | `ListCollections()` function |
| `internal/config/discovery_test.go` | create | Table-driven tests for collection discovery |

#### New Code
```go
// ListCollections returns sorted relative paths of collection files
// found in the collections/ subdirectory of baseDir.
// Returns an empty slice if the directory doesn't exist or contains no YAML files.
func ListCollections(baseDir string) []string
```

Scans `collections/` for `.yaml`/`.yml` files (top-level only), returns sorted relative paths like `collections/sample.yaml`.

#### Tests to Write FIRST (RED phase)

```go
func TestListCollections(t *testing.T) {
    tests := []struct {
        name     string
        setup    func(t *testing.T, dir string)
        expected []string
    }{
        {"collections_dir_with_yaml_files", /* setup creates collections/a.yaml, collections/b.yml */, []string{"collections/a.yaml", "collections/b.yml"}},
        {"collections_dir_empty", /* setup creates empty collections/ dir */, nil},
        {"no_collections_dir", /* no setup */, nil},
        {"ignores_non_yaml_files", /* setup creates collections/readme.md */, nil},
        {"nested_subdirectories_not_included", /* setup creates collections/sub/nested.yaml */, nil},
        {"yml_extension_included", /* setup creates collections/test.yml */, []string{"collections/test.yml"}},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 2: Add InfoJSONOutput to Output Package
**Rationale:** Define the output struct before the command that uses it, keeping output formatting separate from command logic (follows existing pattern with `WriteValidationJSON`).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/json.go` | modify | Add `InfoJSONOutput` struct and `WriteInfoJSON()` |
| `internal/output/json_test.go` | modify | Add tests for info JSON serialization |

#### New Code
```go
// InfoJSONOutput is the JSON structure for curlew info --format json.
type InfoJSONOutput struct {
    ProjectRoot  string   `json:"project_root"`
    ProjectName  string   `json:"project_name"`
    Collections  []string `json:"collections"`
    Environments []string `json:"environments"`
    Version      string   `json:"version"`
}

// WriteInfoJSON serializes info output to indented JSON.
func WriteInfoJSON(w io.Writer, out *InfoJSONOutput) error {
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(out)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteInfoJSON(t *testing.T) {
    tests := []struct {
        name  string
        input *InfoJSONOutput
        check func(t *testing.T, output string)
    }{
        {"valid_JSON_output", /* ... */, /* json.Valid check */},
        {"all_fields_present", /* ... */, /* check project_root, project_name, etc. */},
        {"empty_collections_not_null", /* Collections: []string{} */, /* check "collections": [] not null */},
        {"empty_environments_not_null", /* Environments: []string{} */, /* check "environments": [] not null */},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — additive only

---

### Step 3: Create Embedded JSON Schema Package
**Rationale:** The schema is a static artifact needed by the `schema` command. Creating it before the command allows testing the schema content independently.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/schema.go` | create | `go:embed` for collection JSON Schema |
| `internal/schema/collection.json` | create | The JSON Schema itself |
| `internal/schema/schema_test.go` | create | Validate embedded schema is correct JSON |

#### New Code

`schema.go`:
```go
package schema

import _ "embed"

//go:embed collection.json
var CollectionSchema []byte
```

`collection.json`: A JSON Schema describing the collection YAML format per the specification. Key properties:
- `name` (string, required)
- `description` (string, optional)
- `variables` (object, optional)
- `setup` (array of request items, optional)
- `requests` (array of request items, required)
- `teardown` (array of request items, optional)
- `options` (object with `stop_on_failure`, optional)

Request items define: `name`, `path`, `required`, `request` (method, url, headers, body, query), `variables`, `assertions` (status, headers, body, timing), `extract`.

#### Tests to Write FIRST (RED phase)

```go
func TestCollectionSchema(t *testing.T) {
    tests := []struct {
        name  string
        check func(t *testing.T)
    }{
        {"embedded_schema_is_valid_json", /* json.Valid(CollectionSchema) */},
        {"schema_has_schema_keyword", /* unmarshal, check "$schema" exists */},
        {"schema_has_required_properties", /* check properties.name, properties.requests */},
        {"schema_type_is_object", /* check type == "object" */},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — new package

---

### Step 4: Wire `info` Command in CLI
**Rationale:** With discovery and output helpers in place, wire the full `info` command supporting both JSON and human-readable output.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `infoCmd`, `parseInfoArgs`, wire in `run()` switch |
| `cmd/curlew/main_test.go` | modify | Add integration tests for info command |

#### Current Code (switch statement in `run()`):
```go
switch command {
case "run":
    return runCmd(args[1:])
case "validate":
    return validateCmd(args[1:])
case "init":
    return initCmd(args[1:])
default:
    // ...
}
```

#### New Code (add cases):
```go
case "info":
    return infoCmd(args[1:])
case "schema":
    return schemaCmd(args[1:])
```

```go
func infoCmd(args []string) int
func parseInfoArgs(args []string) (format string, noColor bool, err error)
```

**Behavior:**
- Uses `os.Getwd()` → `config.FindProjectRoot()` for project discovery
- No project found → exit 5 with error: `"no curlew project found (no curlew.yaml in current or parent directories)"`
- `--format json` → JSON via `WriteInfoJSON`
- Default (no `--format`) → human-readable terminal output
- `--format <invalid>` → exit 1

**Human-readable format:**
```
Project: MyApp
Root:    /path/to/project

Collections:
  collections/sample.yaml

Environments:
  dev
  staging
```

If no collections or environments, show `(none)`.

#### Tests to Write FIRST (RED phase)

```go
func TestInfoCmd(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        setup    func(t *testing.T) string // returns dir to chdir into
        wantExit int
        check    func(t *testing.T, stdout, stderr string)
    }{
        {"json_in_project_dir_exit_0", ...},
        {"json_lists_project_root", ...},
        {"json_lists_project_name", ...},
        {"json_lists_collections", ...},
        {"json_lists_environments", ...},
        {"json_empty_project_empty_arrays", ...},
        {"json_output_is_valid_json", ...},
        {"human_readable_default_exit_0", ...},
        {"human_readable_shows_project_name", ...},
        {"human_readable_shows_root_path", ...},
        {"human_readable_shows_collections", ...},
        {"human_readable_shows_environments", ...},
        {"human_readable_no_collections_shows_none", ...},
        {"human_readable_no_environments_shows_none", ...},
        {"outside_project_dir_exit_5", ...},
        {"outside_project_dir_error_message", ...},
        {"invalid_format_exit_1", ...},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — additive switch case

---

### Step 5: Wire `schema` Command in CLI
**Rationale:** Simple command that outputs the embedded schema. Depends on Step 3 (schema package) and Step 4 (wiring pattern).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `schemaCmd`, `parseSchemaArgs` |
| `cmd/curlew/main_test.go` | modify | Add integration tests for schema command |

#### New Code
```go
func schemaCmd(args []string) int
func parseSchemaArgs(args []string) (format string, err error)
```

**Behavior:**
- Outputs `schema.CollectionSchema` to stdout
- `--format json` or no flag → JSON Schema output
- `--format <invalid>` → exit 1
- No project discovery needed (schema is always available)

#### Tests to Write FIRST (RED phase)

```go
func TestSchemaCmd(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        wantExit int
        check    func(t *testing.T, stdout, stderr string)
    }{
        {"json_exit_0", ...},
        {"json_output_is_valid_json", ...},
        {"output_contains_schema_keyword", ...},
        {"default_format_outputs_json", ...},
        {"invalid_format_exit_1", ...},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 6: Update Help Text
**Rationale:** Help text must list all available commands per the completeness contract.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `info` and `schema` to `printHelp()` |
| `cmd/curlew/main_test.go` | modify | Add help text assertions |

#### Tests to Write FIRST (RED phase)

```go
{"help_contains_info", ...},   // help output mentions "info"
{"help_contains_schema", ...}, // help output mentions "schema"
```

#### Impact on Existing Tests
- Existing help tests check for presence of specific strings and should not break

---

### Step 7: Update Smoke Test
**Rationale:** Completeness contract requires integration verification via the real binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add info and schema command sections |

#### New Code
```bash
echo "=== Info command ==="
echo "--- Info: in project directory ---"
# Create temp project, run curlew info --format json, verify exit 0

echo "--- Info: outside project directory (expect error) ---"
# Run in temp dir without curlew.yaml, verify exit 5

echo "=== Schema command ==="
echo "--- Schema: valid JSON Schema output ---"
# Run curlew schema --format json, verify valid JSON
```

#### Impact on Existing Tests
- No existing smoke test sections affected

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| All existing tests | All | none | none |

No existing tests will break. All changes are additive (new switch cases, new packages, new functions).

## Risks and Edge Cases

- **Risk:** `info` run outside project directory → **Mitigation:** Return exit code 5 with clear error: "no curlew project found (no curlew.yaml in current or parent directories)"
- **Edge case:** Empty project (curlew.yaml exists but no collections/environments) → **Handling:** Return valid output with empty arrays `[]`, never `null`
- **Edge case:** Malformed curlew.yaml → **Handling:** Return exit 5 with parse error from `LoadProjectConfig`
- **Risk:** JSON Schema correctness/completeness → **Mitigation:** Write schema against current `Collection` struct fields in `internal/parser/collection.go`; test that schema has expected top-level properties
- **Edge case:** `collections/` directory doesn't exist → **Handling:** `ListCollections` returns empty slice
- **Risk:** `go:embed` requires file to exist at compile time → **Mitigation:** Create `collection.json` in Step 3 before any build step
- **Edge case:** Working directory determination → **Handling:** Use `os.Getwd()` as starting point for `FindProjectRoot`

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# In a project directory:
./curlew info --format json | jq .
# Verify: JSON with project_root, project_name, collections, environments

./curlew info
# Verify: human-readable summary

# Anywhere:
./curlew schema --format json | jq .
# Verify: valid JSON Schema output with $schema keyword

# Outside project directory:
cd /tmp && ./curlew info
# Verify: error message about no project found, exit code 5
```
