# Implementation Plan: M1-015

## Overview
Add external request file references to the parser, allowing collections to reference standalone request YAML files via `path:` instead of defining requests inline. Includes relative path resolution, circular reference detection, and inline variable overrides at the reference site.

## Task Details
- **ID:** M1-015
- **Title:** External request file references
- **Phase:** M1: Core CLI
- **Priority:** 15
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-010 | Variable extraction from responses | done |

## Architecture Decision

External file resolution happens **in the parser layer** — after YAML unmarshaling but before method normalization/validation. This means the runner never sees external references; it receives fully populated `RequestItem` entries. This keeps the runner unchanged and maintains the invariant that all items the runner processes are complete.

**ParseFile flow changes:**
1. Read file → 2. Unmarshal YAML → 3. Validate name → 4. **Resolve external references** → 5. Normalize methods → 6. Validate requests

## Implementation Steps

### Step 1: Add `Path` field to `RequestItem` and new sentinel errors
**Rationale:** Smallest structural change — adds the data model without changing any behavior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Path` field to `RequestItem`, add `ExternalRequest` type |
| `internal/parser/errors.go` | modify | Add `ErrExternalFileNotFound`, `ErrCircularFileReference` |

#### Current Code
```go
// collection.go - RequestItem struct
type RequestItem struct {
	Name       string            `yaml:"name"`
	Request    Request           `yaml:"request"`
	Variables  map[string]string `yaml:"variables,omitempty"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### New Code
```go
// collection.go - RequestItem struct with Path
type RequestItem struct {
	Name       string            `yaml:"name"`
	Path       string            `yaml:"path,omitempty"`
	Request    Request           `yaml:"request"`
	Variables  map[string]string `yaml:"variables,omitempty"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}

// ExternalRequest represents a standalone request file referenced via path:.
type ExternalRequest struct {
	Name       string            `yaml:"name"`
	Request    Request           `yaml:"request"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

```go
// errors.go - new sentinel errors
var (
	ErrExternalFileNotFound  = errors.New("external request file not found")
	ErrCircularFileReference = errors.New("circular file reference")
)
```

#### Tests to Write FIRST (RED phase)

No behavioral tests yet — this is a pure data model change. Tests come in Steps 2 and 3.

#### Impact on Existing Tests
- No existing tests affected. `Path` has `omitempty` and no existing testdata uses it.

---

### Step 2: Implement `parseExternalFile` function
**Rationale:** Isolated parsing of external files — can be fully unit-tested without touching collection parsing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | Add `parseExternalFile(path string) (*ExternalRequest, error)` |
| `internal/parser/parser_test.go` | modify | Add table-driven tests for external file parsing |
| `internal/parser/testdata/requests/` | create | Testdata external request files |

#### New Code
```go
func parseExternalFile(path string) (*ExternalRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("external request file %s: %w", path, ErrExternalFileNotFound)
		}
		return nil, fmt.Errorf("reading external request file %s: %w", path, err)
	}

	var ext ExternalRequest
	if err := yaml.Unmarshal(data, &ext); err != nil {
		return nil, fmt.Errorf("parsing external request file %s: %w", path, ErrInvalidYAML)
	}

	if ext.Request.URL == "" {
		return nil, fmt.Errorf("external request %q in %s: %w", ext.Name, path, ErrMissingRequiredField)
	}

	return &ext, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseExternalFile(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		wantErr   error
		wantName  string
		wantURL   string
	}{
		{"valid minimal external request", "testdata/requests/get-user.yaml", nil, "Get User", "https://example.com/users/1"},
		{"external request with assertions and extract", "testdata/requests/create-user.yaml", nil, "Create User", "https://example.com/users"},
		{"external file not found", "testdata/requests/nonexistent.yaml", ErrExternalFileNotFound, "", ""},
		{"external file invalid YAML", "testdata/requests/invalid.yaml", ErrInvalidYAML, "", ""},
		{"external request missing url", "testdata/requests/missing-url.yaml", ErrMissingRequiredField, "", ""},
		{"external request method defaults handled", "testdata/requests/no-method.yaml", nil, "No Method", "https://example.com/test"},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — new function, new test file data.

---

### Step 3: Implement `resolveExternalReferences` and integrate into `ParseFile`
**Rationale:** Core behavior — depends on Step 2's `parseExternalFile`. This is where path resolution, circular detection, and variable overrides happen.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | Add `resolveExternalReferences`, integrate into `ParseFile` |
| `internal/parser/parser_test.go` | modify | Add table-driven tests for collection-level external references |
| `internal/parser/testdata/` | create | Collection testdata files with external references |

#### New Code
```go
func resolveExternalReferences(collectionPath string, items []RequestItem, visited map[string]bool) ([]RequestItem, error) {
	collectionDir := filepath.Dir(collectionPath)
	resolved := make([]RequestItem, 0, len(items))

	for _, item := range items {
		if item.Path == "" {
			resolved = append(resolved, item)
			continue
		}

		extPath := item.Path
		if !filepath.IsAbs(extPath) {
			extPath = filepath.Join(collectionDir, extPath)
		}

		absPath, err := filepath.Abs(extPath)
		if err != nil {
			return nil, fmt.Errorf("resolving path %q: %w", item.Path, err)
		}

		if visited[absPath] {
			return nil, fmt.Errorf("circular file reference: %s references %s: %w", collectionPath, item.Path, ErrCircularFileReference)
		}
		visited[absPath] = true

		ext, err := parseExternalFile(extPath)
		if err != nil {
			return nil, err
		}

		ri := RequestItem{
			Name:       ext.Name,
			Request:    ext.Request,
			Assertions: ext.Assertions,
			Extract:    ext.Extract,
		}

		// Reference site variable overrides
		if len(item.Variables) > 0 {
			ri.Variables = item.Variables
		}

		// Reference site name override
		if item.Name != "" {
			ri.Name = item.Name
		}

		resolved = append(resolved, ri)
	}

	return resolved, nil
}
```

**Integration into `ParseFile`:** Insert `resolveExternalReferences` call after YAML unmarshaling and name validation, before method normalization loop. The collection's own absolute path is seeded into the `visited` map.

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_ExternalReferences(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		wantErr     error
		wantCount   int
		checkResult func(t *testing.T, col *Collection)
	}{
		{"external reference loads and merges", "testdata/with_external_ref.yaml", nil, 1, nil},
		{"external reference relative to collection dir", "testdata/subdir/with_relative_ref.yaml", nil, 1, nil},
		{"external reference not found", "testdata/with_external_ref_not_found.yaml", ErrExternalFileNotFound, 0, nil},
		{"circular self-reference detected", "testdata/with_external_self_ref.yaml", ErrCircularFileReference, 0, nil},
		{"external with variable overrides", "testdata/with_external_ref_overrides.yaml", nil, 1, checkVariables},
		{"external with extract block", "testdata/with_external_ref_extract.yaml", nil, 1, checkExtract},
		{"external behaves identically to inline", "testdata/with_external_ref.yaml", nil, 1, checkMatchesInline},
		{"external reference name override", "testdata/with_external_ref_name_override.yaml", nil, 1, checkNameOverride},
		{"mixed inline and external requests", "testdata/with_mixed_inline_external.yaml", nil, 2, checkMixedOrder},
		{"both path and request specified", "testdata/with_path_and_request.yaml", ErrMutuallyExclusive, 0, nil},
	}
}
```

#### Impact on Existing Tests
- No existing tests should break. The `resolveExternalReferences` call is a no-op when no items have `Path` set (returns items unchanged).
- Existing `ParseFile` tests use fixtures without `path:` fields, so the resolution step produces identical output.

---

### Step 4: CLI integration tests
**Rationale:** Validates the full pipeline (parse → resolve → run) with the real binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main_test.go` | modify | Add integration tests for external request references |

#### Tests to Write FIRST (RED phase)

```go
func TestCLIIntegration_ExternalRequestReference(t *testing.T) {
	tests := []struct {
		name     string
		// setup function creates temp files
		wantExit int
		wantOut  string
	}{
		{"external reference executes successfully", /* ... */, 0, "Get User"},
		{"external reference not found exits 3", /* ... */, 3, "external request file not found"},
		{"external with variable interpolation", /* ... */, 0, ""},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — new test cases only.

---

### Step 5: Update smoke test and CHANGELOG
**Rationale:** Completion contract — every new capability needs smoke coverage and documentation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add external request reference smoke test |
| `CHANGELOG.md` | modify | Add M1-015 entry |

#### Impact on Existing Tests
- No existing tests affected.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | All existing | none | — |
| `internal/runner/runner_test.go` | All existing | none | — |
| `cmd/apitest/main_test.go` | All existing | none | — |

## Risks and Edge Cases

- **Risk:** Both `path:` and inline `request:` specified → **Mitigation:** Validate mutual exclusivity — return error if both `Path != ""` and `Request.URL != ""`.
- **Risk:** Same external file referenced multiple times in one collection → **Mitigation:** This is valid (e.g., same endpoint with different variables). The `visited` set prevents cycles but not reuse — after processing an external file, it remains in `visited` to prevent circular chains, but since external files are leaf nodes this only blocks self-reference.
- **Risk:** Symlinks creating circular paths → **Mitigation:** `filepath.Abs()` canonicalizes paths. Could add `filepath.EvalSymlinks()` if needed later.
- **Risk:** External file has no `name:` field → **Mitigation:** Accept empty name (consistent with inline requests where name is optional).
- **Risk:** Empty `path: ""` value → **Mitigation:** Treated as no path (zero value check `item.Path == ""`).
- **Edge case:** External file with its own `extract:` block → **Handling:** Preserved in merged `RequestItem` — runner processes extractions normally.
- **Edge case:** Windows path separators → **Handling:** Use `filepath.Join` and `filepath.Abs` throughout.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create external request file
mkdir -p /tmp/apitest-test/requests
cat > /tmp/apitest-test/requests/get-user.yaml << 'EOF'
name: Get User
request:
  method: GET
  url: https://httpbin.org/get
EOF

# Create collection referencing it
cat > /tmp/apitest-test/collection.yaml << 'EOF'
name: External Reference Test
requests:
  - path: requests/get-user.yaml
EOF

# Run and verify
./apitest run /tmp/apitest-test/collection.yaml
```
