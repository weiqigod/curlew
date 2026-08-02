# Implementation Plan: M6-002

## Overview
Add `SourceFile` / `SourceLine` fields to `parser.RequestItem` and
`runner.RequestResult`, populated from the YAML node positions at parse time
(including `include:` and `request_file:` / `path:` cases) and threaded
verbatim into every `RequestResult` — including each data-driven iteration.
Additive change only; no existing output format behaviour changes.

## Task Details
- **ID:** M6-002
- **Title:** Source-location plumbing: file and line onto parsed items and results
- **Phase:** M6: AI Agent Integration
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M6-001 | Error taxonomy + sentinel hint registry | done |

## Exploration Summary

### Data flow
- `parser.ParseFileWithOptions(path, opts)` loads a collection YAML file; it
  calls `parseCollectionBytes(path, data)` which does `yaml.Unmarshal(data, &col)`
  (so items arrive via the default reflection-based decoder), then resolves
  external refs via `resolveExternalReferences(...)`, GraphQL files, WebSocket
  templates, body files, and finally validates.
- After `parseCollectionBytes`, if `col.Include` is non-empty,
  `ParseFileWithOptions` calls `resolveIncludes(col, path, incCtx)` which walks
  each include path, calls `parseCollectionBytes(absChild, data)` on the child
  (so the child's own items come back already unmarshalled), recurses for
  transitive includes, and appends child `Setup`/`Requests`/`Teardown` items to
  the parent. Child items therefore need `SourceFile` pointing at the included
  file (its absolute path).
- External request files (`path:` field on a `RequestItem`) are handled by
  `resolveExternalReferences`. Each `externalRequest` returns a fresh
  `RequestItem` with no location; we need to stamp `SourceFile = absPath` and
  `SourceLine = 1` on that freshly constructed item.
- The runner's `executePhase` / `executeDataDriven` / `executeDataDrivenParallel`
  / `executeParallelMain` / WebSocket branch all build `RequestResult` values.
  Every construction site must copy `item.SourceFile` / `item.SourceLine` into
  the result. Data-driven iterations must copy verbatim (no synthetic offset).
- `parallel.executor.go`'s `RequestOutcome` is a separate struct; the parallel
  code path builds `RequestOutcome` first and the runner converts back to
  `RequestResult`. We need to thread location through `RequestOutcome` too so
  the parallel path preserves it.

### Key files touched
- `internal/parser/collection.go` — add `SourceFile`, `SourceLine` to
  `RequestItem` with YAML-ignored tags; also expose the populated fields via a
  custom `UnmarshalYAML` on `RequestItem` that captures `value.Line`.
- `internal/parser/parser.go` — pass the absolute collection path through the
  parser so every post-unmarshal pass can stamp `SourceFile` on items;
  normalize relative paths to absolute via `filepath.Abs(path)`.
- `internal/parser/external.go` — stamp `SourceFile = absPath`, `SourceLine = 1`
  on items resolved from `path:`.
- `internal/parser/include.go` — the child items already have
  `SourceFile = absChild` because child parsing stamps each item with the
  child's absolute path. No change needed beyond relying on
  `parseCollectionBytes(absChild, data)` receiving `absChild` as its path.
- `internal/parser/collection_test.go` — **new** file holding the three
  observable test functions (scoped tightly, distinct from `parser_test.go`).
- `internal/parser/parser_test.go` — update the `TestParseFile` DeepEqual table
  via a small `stripSourceLocations` helper so existing assertions remain
  behavioural, not sensitive to line numbers.
- `internal/runner/runner.go` — add `SourceFile`, `SourceLine` to
  `RequestResult`; populate at every `RequestResult{...}` construction site
  (found: lines 760, 860, 1075, 1081, 1087, 1174, 1197, 1316, 1412, 1498, 1606,
  1631, 1889, 1988, 2013). The `filterDataDrivenResults` strip paths must
  preserve `SourceFile` / `SourceLine`.
- `internal/runner/runner_test.go` — new test covering the threading contract.
- `internal/parallel/executor.go` — add `SourceFile`, `SourceLine` to
  `RequestOutcome`; populate in `executeOneRequest` and in the runner's
  conversion from outcome to result.
- `cmd/apitest/main.go` — no changes. The caller already passes the user-
  supplied collection path to `parser.ParseFileWithOptions`, and we record
  the absolute path inside the parser.

### Why additive works for `TestParseFile`
`TestParseFile` compares `reflect.DeepEqual(col, tt.wantCol)`. The `wantCol`
fixtures do not populate `SourceFile`/`SourceLine`; naively adding fields will
break the comparison. We add a small `stripSourceLocations(*Collection)`
helper used only from tests that zero the two new fields before DeepEqual.
This preserves the behavioural intent (parse round-trips semantic content)
without pinning line numbers into every fixture.

### Why a custom `UnmarshalYAML` on `RequestItem`
`yaml.v3`'s default reflect decoder does not pass the source node to value
types. To capture `value.Line` we implement
`func (r *RequestItem) UnmarshalYAML(node *yaml.Node) error` that:
1. Uses a type alias (`type requestItemRaw RequestItem`) to decode the node
   into a scratch value without re-entering the custom method (infinite
   recursion guard — standard yaml.v3 pattern).
2. Copies scratch fields back into the receiver.
3. Sets `r.SourceLine = node.Line`. `SourceFile` stays empty here and is
   stamped after unmarshal in `parseCollectionBytes` (where we know `path`).

### Why stamp `SourceFile` post-unmarshal
`UnmarshalYAML` on `RequestItem` has no access to the file path. The cleanest
place to stamp `SourceFile` is right after `yaml.Unmarshal(data, &col)` in
`parseCollectionBytes`, walking all three sections and setting each item's
`SourceFile = absPath` — but only when the field is still empty (so items
produced by nested external-file resolution or included children keep their
own `SourceFile` set earlier).

### Key edge cases
- **External file (`path:`)**: `resolveExternalReferences` constructs a fresh
  `RequestItem` from an `externalRequest`. Location stamping must be explicit
  there (`SourceFile = absPath`, `SourceLine = 1`) and must **not** be
  overwritten by the later "stamp unset items with parent's path" pass.
- **Include**: `parseCollectionBytes(absChild, data)` is invoked with the
  child's absolute path, so stamping items from that call uses `absChild` —
  already the correct answer.
- **Data-driven iterations**: `executeDataDriven` / `executeDataDrivenParallel`
  reference `item.SourceFile`/`item.SourceLine` — copy them verbatim onto
  every iteration's `RequestResult` (iteration identity already encoded by
  `IsDataDriven`, `IterationIndex`, `IterationTotal`).
- **Skipped results**: even the `Skipped: true` stubs need location so
  downstream event streams (M6-004) can still attribute the skip. Update each
  skip-path `RequestResult{}` construction.
- **WebSocket synthesised result**: the WebSocket branch also needs the
  location stamp.
- **`filterDataDrivenResults`**: when store_results strips most fields, preserve
  `SourceFile` / `SourceLine` in the summary/failed_only sub-structs.
- **Parallel `RequestOutcome` → `RequestResult` conversion** (runner.go ~860):
  copy `outcome.SourceFile` / `outcome.SourceLine` alongside the existing
  fields.
- **`openapi.import.go` (line 111)** constructs a `parser.RequestItem` for
  collections generated from OpenAPI specs. Those won't have a source file/
  line (no YAML origin). Leave fields empty — ZV default. Not exercised by
  M6-002 tests; downstream consumers (M6-004) can treat zero values as
  "unknown origin" like today.

## Implementation Steps

### Step 1: Add `SourceFile` / `SourceLine` fields to `RequestItem` with custom YAML line capture
**Rationale:** Smallest blast radius — this is the pure struct/decoder change.
Add YAML-tagged ignored fields and a line-capturing `UnmarshalYAML`. Line
numbers show up in parser-level tests immediately; no runner changes yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `SourceFile string \`yaml:"-"\`` and `SourceLine int \`yaml:"-"\`` to `RequestItem`; add `UnmarshalYAML` method that captures `node.Line`. |
| `internal/parser/collection_test.go` | create | Three new test functions for the YAML line capture on a raw document, before any parser-level path stamping is in scope. |

#### Current Code (excerpt, collection.go)
```go
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

#### New Code
```go
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

	// SourceFile is the absolute path of the file that declared this item.
	// Populated by the parser (collections: the parent file; includes: the
	// included child file; external request_file: the external file path).
	// Zero value means the item was synthesised (e.g. OpenAPI import) and
	// has no source YAML origin.
	SourceFile string `yaml:"-"`

	// SourceLine is the 1-based YAML line number of this item's mapping node
	// inside SourceFile. For external request files SourceLine is 1
	// (the external file is a single request at the top of the document).
	// Zero value means unknown.
	SourceLine int `yaml:"-"`
}

// UnmarshalYAML captures the YAML node line for source-location plumbing.
// It decodes the node into a type alias (to avoid infinite recursion into
// this method) and sets SourceLine from node.Line. SourceFile is stamped
// by the parser after unmarshal, because the file path is not available
// at unmarshal time.
func (r *RequestItem) UnmarshalYAML(node *yaml.Node) error {
	type requestItemRaw RequestItem
	var raw requestItemRaw
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*r = RequestItem(raw)
	r.SourceLine = node.Line
	return nil
}
```

#### Tests to Write FIRST (RED phase)
`internal/parser/collection_test.go`:
```go
package parser

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRequestItem_UnmarshalYAML_capturesLine(t *testing.T) {
	doc := `
- name: alpha
  request:
    method: GET
    url: https://example.com/a
- name: beta
  request:
    method: GET
    url: https://example.com/b
`
	var items []RequestItem
	if err := yaml.Unmarshal([]byte(doc), &items); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].SourceLine != 2 { // 1-based, leading newline in backtick string
		t.Errorf("items[0].SourceLine = %d, want 2", items[0].SourceLine)
	}
	if items[1].SourceLine != 6 {
		t.Errorf("items[1].SourceLine = %d, want 6", items[1].SourceLine)
	}
	if items[0].SourceFile != "" {
		t.Errorf("items[0].SourceFile = %q, want empty (stamped at higher level)", items[0].SourceFile)
	}
}
```

#### Impact on Existing Tests
- `TestParseFile` (parser_test.go L117–133) — `reflect.DeepEqual(col, tt.wantCol)`
  will fail because parsed items now carry `SourceLine` (non-zero) while
  `wantCol` fixtures don't. Fix in Step 2 via a strip helper.

### Step 2: Stamp `SourceFile` post-unmarshal and repair `TestParseFile`
**Rationale:** After Step 1, the item knows its line but not its file. Adding
the file-stamping pass is the minimum change that satisfies the direct-file
behaviour before include/external cases. Doing it here also lets us ship the
DeepEqual fix for existing tests in the same step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | In `parseCollectionBytes`, after `yaml.Unmarshal` and `filepath.Abs`, walk all three sections and stamp `item.SourceFile = absPath` when still empty. |
| `internal/parser/parser_test.go` | modify | Add a `stripSourceLocations(*Collection)` helper and call it before every `reflect.DeepEqual(col, tt.wantCol)` in `TestParseFile`. |

#### Current Code (parser.go, around line 135)
```go
	// Resolve external file references before validation
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving collection path %q: %w", path, err)
	}
	visited := map[string]bool{absPath: true}
```

#### New Code
```go
	// Resolve external file references before validation
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving collection path %q: %w", path, err)
	}

	// M6-002: stamp SourceFile on every item that does not already have one.
	// Items produced by `path:` external-file resolution are stamped later
	// with their own external file path; we must not overwrite those.
	stampSourceFile(col, absPath)

	visited := map[string]bool{absPath: true}
```

Also add (private helper, same file):
```go
// stampSourceFile sets SourceFile on every RequestItem that does not already
// have one. Called by parseCollectionBytes with the absolute path of the
// collection file being parsed. Items loaded from external request files
// (resolveExternalReferences) or from included collections (resolveIncludes)
// will already have their own SourceFile set and must be left alone.
func stampSourceFile(col *Collection, absPath string) {
	for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
		for i := range *section {
			if (*section)[i].SourceFile == "" {
				(*section)[i].SourceFile = absPath
			}
		}
	}
}
```

#### Test helper (parser_test.go)
```go
// stripSourceLocations zeros SourceFile/SourceLine across every request item
// in c, making c amenable to DeepEqual against fixtures that predate the
// M6-002 source-location fields. Use only in tests whose intent is content
// equality, not line-sensitive equality.
func stripSourceLocations(c *Collection) {
	if c == nil {
		return
	}
	for _, s := range []*[]RequestItem{&c.Setup.Items, &c.Requests.Items, &c.Teardown.Items} {
		for i := range *s {
			(*s)[i].SourceFile = ""
			(*s)[i].SourceLine = 0
		}
	}
}
```
And update `TestParseFile`:
```go
if err != nil {
	t.Fatalf("unexpected error: %v", err)
}
stripSourceLocations(col)
if !reflect.DeepEqual(col, tt.wantCol) {
	t.Errorf("got %+v, want %+v", col, tt.wantCol)
}
```

#### New Observable Test (`internal/parser/collection_test.go`, add to same file)
```go
func TestParse_CarriesSourceLocation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "src-demo.yaml")
	content := `name: demo
requests:
  - name: alpha
    request:
      method: GET
      url: https://example.com/a
  - name: beta
    request:
      method: GET
      url: https://example.com/b
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	col, err := ParseFile(file)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(col.Requests.Items))
	}

	absFile, _ := filepath.Abs(file)
	for i, want := range []struct {
		name string
		line int
	}{
		{"alpha", 3},
		{"beta", 7},
	} {
		ri := col.Requests.Items[i]
		if ri.Name != want.name {
			t.Fatalf("Requests.Items[%d].Name = %q, want %q", i, ri.Name, want.name)
		}
		if ri.SourceFile != absFile {
			t.Errorf("Requests.Items[%d].SourceFile = %q, want %q", i, ri.SourceFile, absFile)
		}
		if ri.SourceLine != want.line {
			t.Errorf("Requests.Items[%d].SourceLine = %d, want %d", i, ri.SourceLine, want.line)
		}
	}
}
```

Add imports `"os"`, `"path/filepath"` at the top of `collection_test.go`.

#### Impact on Existing Tests
- `TestParseFile` table — passes once `stripSourceLocations` is in place.
- All other `parser_test.go` tests that access fields directly (not DeepEqual on
  full `Collection`) keep working — they never read `SourceFile`/`SourceLine`.

### Step 3: Stamp `SourceFile` / `SourceLine` on externally-loaded requests
**Rationale:** External-reference resolution (`path:`) creates fresh
`RequestItem` values from `externalRequest`. Location stamping there is
self-contained and unlocks the `TestParse_IncludesCarryIncludedFilePath`-
adjacent behaviour for `request_file:` (formally `path:` in code; the
observable uses the `request_file:` spelling as the user-facing alias — see
M1-015 where `path:` is the internal YAML field).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/external.go` | modify | In `resolveExternalReferences`, set `ri.SourceFile = absPath` and `ri.SourceLine = 1` on the freshly constructed `RequestItem`. |
| `internal/parser/collection_test.go` | modify | Add `TestParse_ExternalRequestCarriesExternalFilePath`. |

#### Current Code (external.go)
```go
		ri := RequestItem{
			Name:       ext.Name,
			Request:    ext.Request,
			Assertions: ext.Assertions,
			Extract:    ext.Extract,
		}
```

#### New Code
```go
		ri := RequestItem{
			Name:       ext.Name,
			Request:    ext.Request,
			Assertions: ext.Assertions,
			Extract:    ext.Extract,
			SourceFile: absPath,
			SourceLine: 1,
		}
```

#### New Tests (collection_test.go)
```go
func TestParse_ExternalRequestCarriesExternalFilePath(t *testing.T) {
	dir := t.TempDir()
	extFile := filepath.Join(dir, "get-user.yaml")
	ext := `name: Get User
request:
  method: GET
  url: https://example.com/users/1
`
	if err := os.WriteFile(extFile, []byte(ext), 0o600); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, "parent.yaml")
	pc := `name: parent
requests:
  - path: get-user.yaml
`
	if err := os.WriteFile(parent, []byte(pc), 0o600); err != nil {
		t.Fatal(err)
	}

	col, err := ParseFile(parent)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(col.Requests.Items))
	}
	absExt, _ := filepath.Abs(extFile)
	if col.Requests.Items[0].SourceFile != absExt {
		t.Errorf("SourceFile = %q, want %q", col.Requests.Items[0].SourceFile, absExt)
	}
	if col.Requests.Items[0].SourceLine != 1 {
		t.Errorf("SourceLine = %d, want 1", col.Requests.Items[0].SourceLine)
	}
}
```

#### Impact on Existing Tests
- None. `TestParseFile_ExternalReferences` inspects specific fields, never the
  new ones.

### Step 4: Verify (and add a test for) included-file source path
**Rationale:** Include child items arrive via a recursive
`parseCollectionBytes(absChild, data)` call from `resolveIncludes`; because
Step 2 stamps with the child's absolute path, included items already inherit
the correct `SourceFile`. This step is the regression test that locks the
behaviour in.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection_test.go` | modify | Add `TestParse_IncludesCarryIncludedFilePath`. |

#### New Tests
```go
func TestParse_IncludesCarryIncludedFilePath(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child.yaml")
	cc := `name: child
requests:
  - name: child_req
    request:
      method: GET
      url: https://example.com/child
`
	if err := os.WriteFile(child, []byte(cc), 0o600); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, "parent.yaml")
	pc := `name: parent
include:
  - ./child.yaml
requests:
  - name: parent_req
    request:
      method: GET
      url: https://example.com/parent
`
	if err := os.WriteFile(parent, []byte(pc), 0o600); err != nil {
		t.Fatal(err)
	}

	col, err := ParseFile(parent)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	absParent, _ := filepath.Abs(parent)
	absChild, _ := filepath.Abs(child)
	// Parent's own request appears first; child item is appended.
	var parentItem, childItem *RequestItem
	for i := range col.Requests.Items {
		switch col.Requests.Items[i].Name {
		case "parent_req":
			parentItem = &col.Requests.Items[i]
		case "child_req":
			childItem = &col.Requests.Items[i]
		}
	}
	if parentItem == nil || childItem == nil {
		t.Fatalf("parent or child request missing: %+v", col.Requests.Items)
	}
	if parentItem.SourceFile != absParent {
		t.Errorf("parent SourceFile = %q, want %q", parentItem.SourceFile, absParent)
	}
	if childItem.SourceFile != absChild {
		t.Errorf("child SourceFile = %q, want %q", childItem.SourceFile, absChild)
	}
	if childItem.SourceLine == 0 {
		t.Errorf("child SourceLine = 0, want > 0")
	}
}
```

#### Impact on Existing Tests
- None; this is a new observable case. The existing include tests don't check
  source location.

### Step 5: Thread `SourceFile` / `SourceLine` through `parallel.RequestOutcome`
**Rationale:** The parallel executor converts items into `RequestOutcome`,
then the runner converts those back to `RequestResult`. Add the fields to
`RequestOutcome` and populate at every outcome-construction site so Step 6
can forward them into `RequestResult`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | modify | Add `SourceFile`, `SourceLine` to `RequestOutcome`; populate at every `RequestOutcome{}` site in `ExecuteWaves` / `executeOneRequest`. |

#### Current Code (excerpt)
```go
type RequestOutcome struct {
	Index            int
	Name             string
	Method           string
	URL              string
	...
	WaveIndex        int
	Warnings         []string
}
```

#### New Code
```go
type RequestOutcome struct {
	Index            int
	Name             string
	Method           string
	URL              string
	...
	WaveIndex        int
	Warnings         []string

	// SourceFile / SourceLine propagate the originating RequestItem's
	// location (M6-002). Copied verbatim on every wave-level outcome,
	// including skipped / context-cancelled entries.
	SourceFile string
	SourceLine int
}
```

Every construction site (`executor.go` lines 106, 133, 144, 159, 210, 311, 341,
369) sets:
```go
SourceFile: cfg.Items[idx].SourceFile,
SourceLine: cfg.Items[idx].SourceLine,
```
For `executeOneRequest` (which already has `item := cfg.Items[idx]`):
```go
SourceFile: item.SourceFile,
SourceLine: item.SourceLine,
```

#### Impact on Existing Tests
- `internal/parallel/executor_test.go` — none. Tests check fields explicitly;
  no DeepEqual on the full outcome struct.

### Step 6: Thread `SourceFile` / `SourceLine` through `runner.RequestResult`
**Rationale:** Last step in the thread — each `RequestResult` now records
the `(SourceFile, SourceLine)` of its originating item, including every
skip path and every data-driven iteration.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add fields to `RequestResult`; stamp at every `RequestResult{}` site (15 sites). Preserve in `filterDataDrivenResults`. Forward in parallel-wave conversion. |
| `internal/runner/runner_test.go` | modify | Add `TestRunner_RequestResultCarriesSourceLocation` (sequential + data-driven). |

#### Current Code (excerpt)
```go
type RequestResult struct {
	Name             string
	Phase            Phase
	Method           string
	URL              string
	...
	IsDataDriven     bool
	DataDrivenName   string
	IterationIndex   int
	IterationTotal   int
	IterationData    map[string]string
}
```

#### New Code
```go
type RequestResult struct {
	Name             string
	Phase            Phase
	Method           string
	URL              string
	...
	IsDataDriven     bool
	DataDrivenName   string
	IterationIndex   int
	IterationTotal   int
	IterationData    map[string]string

	// SourceFile / SourceLine are copied verbatim from the originating
	// parser.RequestItem. For data-driven iterations all iterations share
	// the base item's values (iteration identity lives in IsDataDriven /
	// IterationIndex / IterationTotal).
	SourceFile string
	SourceLine int
}
```

Stamp at every construction site. Concrete changes:
1. **Skip-path stubs** — all `RequestResult{Name: item.Name, ...}` at runner.go
   L760, L1075, L1081, L1087, L1174. Add `SourceFile: item.SourceFile, SourceLine: item.SourceLine`.
2. **Parallel wave conversion** (runner.go L860) — copy from `outcome.SourceFile` /
   `outcome.SourceLine` (populated in Step 5).
3. **WebSocket synthesised result** (L1197) — add the two fields from `item`.
4. **Exec-error result** (L1316) — add from `item`.
5. **Normal result** (L1412) — add from `item`.
6. **Data-driven exec-error & success** (L1498, L1606, L1631, L1889) — add from
   `item` (the base RequestItem).
7. **`filterDataDrivenResults` summary/failed-only branches** (L1988, L2013) —
   preserve `SourceFile` / `SourceLine` in the stripped `RequestResult{}`.

#### New Tests (runner_test.go)
```go
func TestRunner_RequestResultCarriesSourceLocation(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:       "alpha",
			Request:    parser.Request{Method: "GET", URL: "https://example.com/a"},
			SourceFile: "/abs/path/demo.yaml",
			SourceLine: 3,
		},
		{
			Name:       "beta",
			Request:    parser.Request{Method: "GET", URL: "https://example.com/b"},
			SourceFile: "/abs/path/demo.yaml",
			SourceLine: 7,
		},
	}
	col := &parser.Collection{Name: "demo", Requests: parser.Section{Items: items}}

	results, _, err := Run(context.Background(), col, nil, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for i, want := range []struct {
		file string
		line int
	}{
		{"/abs/path/demo.yaml", 3},
		{"/abs/path/demo.yaml", 7},
	} {
		if results[i].SourceFile != want.file {
			t.Errorf("results[%d].SourceFile = %q, want %q", i, results[i].SourceFile, want.file)
		}
		if results[i].SourceLine != want.line {
			t.Errorf("results[%d].SourceLine = %d, want %d", i, results[i].SourceLine, want.line)
		}
	}
}

func TestRunner_DataDrivenIterationsCarrySourceLocation(t *testing.T) {
	// Inline CSV via data_driven inline data to avoid filesystem coupling.
	// Each iteration must copy the base item's (SourceFile, SourceLine).
	csvPath := filepath.Join(t.TempDir(), "rows.csv")
	if err := os.WriteFile(csvPath, []byte("user_id\n1\n2\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	items := []parser.RequestItem{
		{
			Name:    "fetch",
			Request: parser.Request{Method: "GET", URL: "https://example.com/{{user_id}}"},
			DataDriven: &datadriven.Config{
				Source: csvPath,
			},
			SourceFile: "/abs/path/demo.yaml",
			SourceLine: 12,
		},
	}
	col := &parser.Collection{Name: "dd", Requests: parser.Section{Items: items}}
	vars := VarSources{
		Tier: auth.TierProfessional, // data_driven is gated
	}
	results, _, err := Run(context.Background(), col, nil, successExecutor, vars)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	for i, r := range results {
		if r.SourceFile != "/abs/path/demo.yaml" {
			t.Errorf("iter %d: SourceFile = %q, want %q", i, r.SourceFile, "/abs/path/demo.yaml")
		}
		if r.SourceLine != 12 {
			t.Errorf("iter %d: SourceLine = %d, want 12", i, r.SourceLine)
		}
	}
}
```
(Use `auth.TierProfessional` if that is the tier constant; otherwise whichever
unlocks data_driven. The existing runner_test.go already imports `auth`,
`datadriven`, `os`, `path/filepath`.)

#### Impact on Existing Tests
- `internal/runner/runner_test.go` — existing tests compare specific fields,
  never the full `RequestResult`. No breaks.
- `internal/runner/distributed`, `internal/runner/shard` — scan usage:
  neither DeepEqual's `RequestResult` nor `RequestOutcome`. Safe.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | `TestParseFile` | breaks on DeepEqual | add `stripSourceLocations(col)` before compare |
| `internal/parser/collection_test.go` | `TestRequestItem_UnmarshalYAML_capturesLine` | new | write |
| `internal/parser/collection_test.go` | `TestParse_CarriesSourceLocation` | new | write |
| `internal/parser/collection_test.go` | `TestParse_ExternalRequestCarriesExternalFilePath` | new | write |
| `internal/parser/collection_test.go` | `TestParse_IncludesCarryIncludedFilePath` | new | write |
| `internal/runner/runner_test.go` | `TestRunner_RequestResultCarriesSourceLocation` | new | write |
| `internal/runner/runner_test.go` | `TestRunner_DataDrivenIterationsCarrySourceLocation` | new | write |
| `internal/runner/runner_test.go` | existing cases | none | compare specific fields already |
| `internal/parallel/executor_test.go` | existing cases | none | no DeepEqual on full outcome |
| `internal/parser/external_test.go` | existing cases | none | field-specific checks |
| `internal/parser/include_test.go` | existing cases | none | field-specific checks |

## Risks and Edge Cases
- **Risk:** Custom `UnmarshalYAML` on `RequestItem` could break when the YAML
  document encodes a `RequestItem` in an unexpected node kind (e.g., an alias).
  → **Mitigation:** The type-alias `node.Decode(&raw)` preserves all existing
  decoding behaviour (it's what the default decoder already does). If the
  node is of a surprising kind, `Decode` returns the same error it would have
  before; we only add `r.SourceLine = node.Line` afterwards.
- **Risk:** `openapi.import.go` synthesises `parser.RequestItem` without a
  source YAML node → `SourceFile` / `SourceLine` remain zero.
  → **Mitigation:** This is the intended behaviour. Downstream consumers
  (M6-004 event stream) treat zero values as "no origin file." Not covered by
  M6-002 tests.
- **Risk:** `resolveExternalReferences` might overwrite `Name` / `Auth` from
  the reference site. We must not overwrite `SourceFile` / `SourceLine`, which
  point at the external file, not the reference site.
  → **Mitigation:** The fresh `ri := RequestItem{...}` uses the external path
  directly. The existing code only overwrites `Name`, `Variables`, `Auth` from
  the reference-site item. No other field is touched.
- **Edge case:** Empty collection path (e.g., in-memory decode). `filepath.Abs("")`
  returns the current working directory — acceptable, since any in-memory path
  that reaches `parseCollectionBytes` already passed a real path through
  `ParseFileWithOptions`.
- **Edge case:** `RequestItem` decoded outside `ParseFileWithOptions`
  (e.g., `Section.UnmarshalYAML` path in tests). `SourceLine` is still captured
  (`UnmarshalYAML` fires regardless of the caller); `SourceFile` stays empty,
  which matches our contract ("unknown when not loaded from a file"). Existing
  `TestSection_UnmarshalYAML_*` tests do not touch these fields, so no impact.
- **Edge case:** Windows path separators. `filepath.Abs` gives a platform-
  native absolute path; the observable in the task YAML uses Unix-style paths
  via `/tmp`, consistent with the rest of the codebase.
- **Risk:** Adding fields to `RequestResult` could break external JSON/YAML
  output goldens (JSON format exposes arbitrary fields via encoding/json).
  → **Check:** `output/json` and friends. The M6-002 scope line says "no
  behaviour change to existing formats" — we'll verify during `/execute` that
  the JSON output does not serialise `RequestResult` directly; it uses a
  dedicated DTO. If it turns out the JSON formatter reflects through
  `RequestResult`, we tag the new fields with `json:"-"` to suppress them.

## Verification

```bash
go build ./cmd/apitest
go test ./internal/parser/... ./internal/runner/... ./internal/parallel/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):
```bash
# Parser-level: direct YAML file carries source location.
go test -run TestParse_CarriesSourceLocation ./internal/parser/

# Parser-level: included file contributes requests with child's SourceFile.
go test -run TestParse_IncludesCarryIncludedFilePath ./internal/parser/

# Runner-level: RequestResult threads location verbatim from RequestItem.
go test -run TestRunner_RequestResultCarriesSourceLocation ./internal/runner/
```
All three expected to PASS after Steps 1–6.
