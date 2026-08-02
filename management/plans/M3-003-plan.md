# Implementation Plan: M3-003

## Overview

Add a top-level `include:` directive to collection YAML that recursively splices
child collection setup/requests/teardown items into the parent in declaration
order, with a snapshot-based variable scoping rule (parent variables flow into
the child; child variables override locally but never leak back to parent), a
cycle detector, and a Professional-tier feature gate (`include_directive`).

## Task Details

- **ID:** M3-003
- **Title:** include directive: compose collections with snapshot variable scoping
- **Phase:** M3: Professional Tier
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| _(none)_ | — | — |

The task declares no dependencies. The implementation uses existing variable
override plumbing (`RequestItem.Variables` + `variable.Scope.WithOverrides`),
the existing per-collection `ExternalFiles` tracking, and the existing feature
gate infrastructure (`internal/auth.Registry`, `auth.CheckFeature`).

---

## Architectural Decisions (Resolved Up-Front)

The Plan agent surfaced four open questions. Decisions made in pipeline mode:

### Decision 1 — Where the feature gate fires

The `include` directive must be gated **inside the parser**, before any child
file is opened. Reasons:

- The parent file may declare an include that itself triggers cycle detection,
  missing-file errors, or YAML errors. The task says *"Given an `include:`
  directive at Free or Solo tier, when parsed, then exit code 6"* — the gate
  must fire even when the includes would otherwise error.
- Resolving includes happens in `parser.ParseFile`. The runner already runs
  late and sees only fully-spliced items; gating there would mean parsing has
  already done file I/O.

To keep the parser package free of an `internal/auth` import (it currently has
none), we add a small **`ParseOptions` struct** with an `IncludeGate func()
error` callback. `ParseFile(path)` is preserved as a thin wrapper that calls
the new `ParseFileWithOptions(path, ParseOptions{})` with no gate (i.e. the
behavior existing tests/validators rely on is unchanged). The CLI and watch
mode pass an `IncludeGate` that calls `auth.CheckFeature(reg,
"include_directive", currentTier())`.

### Decision 2 — Variable propagation mechanism

Snapshot scoping is implemented at parse time by **stamping per-request
variable overrides on every spliced child request**. Specifically, the include
walker carries a "cumulative override map" representing the variables added
by all enclosing includes relative to the *root* parent's declared collection
variables. For each child request item we splice into the parent, we set:

```
splicedItem.Variables.Values = merge(cumulativeOverrides, item.Variables.Values)
```

with `item.Variables.Values` (any per-request overrides the child file
declared at the request site) winning over the cumulative include overrides.

Why this works:

- The parent's runtime base scope is `parent.Variables.Values`. Spliced child
  requests live in the parent's `Items` slices, so their base scope at runtime
  is exactly the parent's variables — that is the *snapshot* the spec calls
  for. Parent variables defined after the include statement still flow into
  the base because YAML is order-independent for top-level fields, and the
  spec only requires order-sensitivity for the **request items**, not the
  variables block itself.
- `RequestItem.Variables.Values` is already applied at request execution time
  via `variable.Scope.WithOverrides(item.Variables.Values)` (see
  `internal/runner/runner.go:819`, `:1251`, `:1415`). `WithOverrides` allows
  override values to reference base-scope variables (see existing test case
  `override_references_base_variable` in
  `internal/variable/variable_test.go:409`), so a child-declared variable
  like `child_url: "{{base_url}}/v2"` resolves correctly against the parent's
  `base_url` at runtime.
- For grandchild requests, the cumulative overrides accumulate as we recurse:
  `grandchild_overrides = merge(child_collection_vars, grandchild_collection_vars)`,
  with grandchild winning. The grandchild request item is spliced into the
  parent, so it gets the parent's snapshot for everything else.
- Parent's own requests defined after the include are **not** stamped with
  any overrides — they execute with parent's vanilla scope. Behavior 3 holds
  trivially.

This avoids any change to `runner` or `variable` and keeps the include feature
strictly a parser-time transformation.

### Decision 3 — Variable shadowing in cumulative overrides

When recursing `parent → child → grandchild`, the cumulative override map
uses the rule **"deeper child wins over shallower child"**. Concretely:

- Enter child: `cumulative = merge({}, child.Variables.Values)` —
  child overrides win over the empty cumulative map (no parent vars in here;
  parent vars come from base scope).
- Enter grandchild: `cumulative_g = merge(cumulative, grandchild.Variables.Values)`
  — grandchild overrides win over child overrides.

This matches behavior 2 (child overrides parent's `b`) and is the natural
nesting for transitive includes (behavior 5).

### Decision 4 — Cycle detection key

We reuse the same approach as `resolveExternalReferences` in
`internal/parser/external.go`: a `map[string]bool` of **absolute, symlink-free
file paths** keyed via `filepath.Abs(filepath.Clean(path))`. We seed the map
with the parent file's absolute path, mark each child as visited before
recursing, and return `ErrCircularInclude` (new sentinel) with both file paths
in the error message when re-entry is detected.

We do **not** unmark on return: once a file appears in any include subtree, a
sibling subtree may legitimately include it again? — The spec defines a cycle
as `A → B → A`. The conservative reading is "cycles only", so siblings
re-including the same file are allowed. We therefore use a **per-recursion-
path** visited set (cloned and extended at each descent), not a global
visited set. This matches how `resolveExternalReferences` threads `visited`.

### Decision 5 — Schema permissiveness

`internal/schema/collection.json` gets a new top-level `include` property
typed as `array of strings`. Per-tier gating is enforced at runtime by the
parser, not by the JSON schema (which is tier-agnostic).

---

## Implementation Steps

Steps are ordered by **smallest blast radius first**: data structures →
sentinel errors → schema → parser internals → wiring → smoke/CHANGELOG.

### Step 1: Add the sentinel error and `Include` field on `Collection`

**Rationale:** Pure additive change with zero downstream callers — establishes
the data model the rest of the code will populate.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/errors.go` | modify | Add `ErrCircularInclude` and `ErrIncludeNotFound` sentinels |
| `internal/parser/collection.go` | modify | Add `Include []string` field on `Collection` |

#### Current Code

`internal/parser/errors.go`:
```go
var (
    ErrFileNotFound          = errors.New("collection file not found")
    ErrInvalidYAML           = errors.New("invalid YAML syntax")
    ErrEmptyCollection       = errors.New("collection has no name")
    ErrUnsupportedMethod     = errors.New("unsupported HTTP method")
    ErrMissingRequiredField  = errors.New("missing required field")
    ErrExternalFileNotFound  = errors.New("external request file not found")
    ErrCircularFileReference = errors.New("circular file reference")
    ErrMutuallyExclusive     = errors.New("mutually exclusive fields")
    ErrUnsupportedProtocol   = errors.New("unsupported protocol")
    ErrInvalidFieldValue     = errors.New("invalid field value")
)
```

`internal/parser/collection.go` (Collection struct):
```go
type Collection struct {
    Name         string            `yaml:"name"`
    Description  string            `yaml:"description,omitempty"`
    Variables    SensitiveVars     `yaml:"variables,omitempty"`
    Retry        *retry.FullConfig `yaml:"retry,omitempty"`
    Setup        Section           `yaml:"setup,omitempty"`
    Requests     Section           `yaml:"requests"`
    Teardown     Section           `yaml:"teardown,omitempty"`
    Options      Options           `yaml:"options,omitempty"`
    RateLimitRPS int               `yaml:"rate_limit_rps,omitempty"`

    ExternalFiles []string `yaml:"-"`
}
```

#### New Code

`internal/parser/errors.go` (additions only):
```go
var (
    // ... existing ...
    ErrCircularInclude  = errors.New("circular include")
    ErrIncludeNotFound  = errors.New("include file not found")
)
```

`internal/parser/collection.go` (Collection struct):
```go
type Collection struct {
    Name         string            `yaml:"name"`
    Description  string            `yaml:"description,omitempty"`
    Variables    SensitiveVars     `yaml:"variables,omitempty"`
    Retry        *retry.FullConfig `yaml:"retry,omitempty"`
    Include      []string          `yaml:"include,omitempty"` // M3-003
    Setup        Section           `yaml:"setup,omitempty"`
    Requests     Section           `yaml:"requests"`
    Teardown     Section           `yaml:"teardown,omitempty"`
    Options      Options           `yaml:"options,omitempty"`
    RateLimitRPS int               `yaml:"rate_limit_rps,omitempty"`

    ExternalFiles []string `yaml:"-"`
}
```

#### Tests to Write FIRST (RED phase)

A trivial unmarshal test that proves the new field is wired:

```go
func TestParseFile_include_field_unmarshal(t *testing.T) {
    // Given a parent file declaring `include: [./shared/auth.yaml]`
    // and an empty `requests:` block, when ParseFile runs,
    // then col.Include == ["./shared/auth.yaml"] before any resolution.
}
```

Plus an `errors.Is` test asserting the new sentinels exist (compile-time
safety).

#### Impact on Existing Tests

- None. New field is additive; existing collections do not declare `include:`
  so unmarshal yields a nil slice. Existing `TestParseFile` table cases that
  use `reflect.DeepEqual` on `Collection` continue to pass (zero-value `nil`
  Include slice matches).

---

### Step 2: Add `ParseOptions` and `ParseFileWithOptions` shell

**Rationale:** Introduce the new entry point with the gate hook **before**
writing any include resolution logic. `ParseFile(path)` becomes a thin wrapper
calling `ParseFileWithOptions(path, ParseOptions{})`. This step is a no-op
refactor — all existing tests must still pass after it lands.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | Extract body of `ParseFile` into `ParseFileWithOptions`; add `ParseOptions` struct |

#### New Code

```go
// ParseOptions configures collection parsing. Zero value disables tier gating.
type ParseOptions struct {
    // IncludeGate, when non-nil, is invoked exactly once before any `include:`
    // directive is resolved. If it returns a non-nil error, parsing aborts and
    // the error is returned unwrapped (callers can errors.As it to detect a
    // *auth.GateError). Callers that do not care about gating leave it nil.
    IncludeGate func() error
}

// ParseFile reads and parses a collection YAML file with default options
// (no include gating). Backwards-compatible with all existing callers.
func ParseFile(path string) (*Collection, error) {
    return ParseFileWithOptions(path, ParseOptions{})
}

// ParseFileWithOptions reads and parses a collection YAML file with the given
// options. Pass ParseOptions{IncludeGate: ...} to enforce the Professional-
// tier gate on the `include:` directive.
func ParseFileWithOptions(path string, opts ParseOptions) (*Collection, error) {
    // ... existing ParseFile body ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFileWithOptions_no_options_matches_ParseFile(t *testing.T) {
    // Parsing testdata/minimal.yaml via ParseFile and ParseFileWithOptions
    // with zero ParseOptions yields equal Collection structs.
}

func TestParseFileWithOptions_gate_not_called_when_no_include(t *testing.T) {
    // Given a collection without `include:`, when ParseFileWithOptions runs
    // with an IncludeGate that records its invocations, then the gate is
    // never called.
}
```

#### Impact on Existing Tests

- None — `ParseFile` semantics unchanged.

---

### Step 3: Implement `resolveIncludes` recursion (no gate yet)

**Rationale:** Build the resolver as a pure helper with full unit coverage
before plumbing it into `ParseFileWithOptions`. Keeps blast radius local until
the next step.

#### Files to Create

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/include.go` | create | New file housing include resolution helpers |
| `internal/parser/include_test.go` | create | Table-driven include tests |

#### New Code

```go
package parser

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"

    apierrors "github.com/peterlindqvist/apitest/internal/errors"
    "gopkg.in/yaml.v3"
)

// includeContext threads the cumulative variable overrides, visited file set,
// and accumulated external file list through recursive include resolution.
type includeContext struct {
    cumulativeVars map[string]string // overrides accumulated from enclosing includes
    visited        map[string]bool   // absolute paths visited on the current recursion path
    extFiles       *[]string         // appended-to: every successfully loaded include path
}

// resolveIncludes walks col.Include, parses each child collection, recursively
// resolves its includes, stamps cumulativeVars onto each child request item,
// and appends the spliced items into col.Setup/Requests/Teardown in include
// order. cumulativeVars at the root level should be nil.
//
// The function mutates col in place. col.Include is left intact for caller
// inspection (e.g. tests); runtime code ignores it.
func resolveIncludes(col *Collection, parentPath string, ctx *includeContext) error {
    if len(col.Include) == 0 {
        return nil
    }
    parentDir := filepath.Dir(parentPath)
    for idx, raw := range col.Include {
        childPath := raw
        if !filepath.IsAbs(childPath) {
            childPath = filepath.Join(parentDir, childPath)
        }
        absChild, absErr := filepath.Abs(childPath)
        if absErr != nil {
            return &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: parentPath,
                Message:  fmt.Sprintf("include[%d]: cannot resolve path %q: %s", idx, raw, absErr),
                Inner:    absErr,
            }
        }
        if ctx.visited[absChild] {
            return &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: parentPath,
                Message: fmt.Sprintf(
                    "circular include: %q includes %q which is already on the include path",
                    parentPath, absChild,
                ),
                Inner: ErrCircularInclude,
            }
        }
        // Read child raw bytes and parse without re-entering ParseFileWithOptions
        // (we don't want to re-fire the gate on a child).
        data, readErr := os.ReadFile(absChild)
        if readErr != nil {
            if errors.Is(readErr, os.ErrNotExist) {
                return &apierrors.Structured{
                    Category: apierrors.CategoryParse,
                    FilePath: parentPath,
                    Line:     findIncludeLine(parentPath, idx),
                    Message:  fmt.Sprintf("include[%d]: file not found: %s", idx, raw),
                    Inner:    ErrIncludeNotFound,
                }
            }
            return &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: parentPath,
                Message:  fmt.Sprintf("include[%d]: reading %s: %s", idx, raw, readErr),
                Inner:    readErr,
            }
        }
        // Parse child as a Collection via the same code path as ParseFile but
        // without re-running ParseOptions.IncludeGate. We funnel through a
        // local helper that performs structural unmarshal + per-request
        // validation, returning a validated *Collection.
        child, parseErr := parseCollectionBytes(absChild, data)
        if parseErr != nil {
            return parseErr
        }
        // Compute the override map this child contributes to its descendants
        // and to its own spliced request items.
        childOverrides := mergeStringMaps(ctx.cumulativeVars, child.Variables.Values)
        childCtx := &includeContext{
            cumulativeVars: childOverrides,
            visited:        cloneVisited(ctx.visited, absChild),
            extFiles:       ctx.extFiles,
        }
        if err := resolveIncludes(child, absChild, childCtx); err != nil {
            return err
        }
        // Stamp overrides onto every spliced request item, then append.
        stampOverrides(child.Setup.Items, childOverrides)
        stampOverrides(child.Requests.Items, childOverrides)
        stampOverrides(child.Teardown.Items, childOverrides)
        col.Setup.Items = append(col.Setup.Items, child.Setup.Items...)
        col.Requests.Items = append(col.Requests.Items, child.Requests.Items...)
        col.Teardown.Items = append(col.Teardown.Items, child.Teardown.Items...)
        // Track the loaded child file (and its transitively loaded includes).
        *ctx.extFiles = append(*ctx.extFiles, absChild)
        *ctx.extFiles = append(*ctx.extFiles, child.ExternalFiles...)
    }
    return nil
}

// stampOverrides merges overrides into each item's Variables.Values. Existing
// per-request overrides on the item win over include-supplied overrides.
func stampOverrides(items []RequestItem, overrides map[string]string) {
    if len(overrides) == 0 {
        return
    }
    for i := range items {
        merged := make(map[string]string, len(overrides)+len(items[i].Variables.Values))
        for k, v := range overrides {
            merged[k] = v
        }
        for k, v := range items[i].Variables.Values {
            merged[k] = v // request-site override wins
        }
        items[i].Variables.Values = merged
        if items[i].Variables.Sensitive == nil {
            // Preserve a non-nil SensitiveSet so downstream code can inspect
            // it without nil checks. We do not propagate include-level
            // sensitivity (the snapshot is values-only).
            items[i].Variables.Sensitive = variable.NewSensitiveSet()
        }
    }
}

// mergeStringMaps returns a new map containing all keys from base, overridden
// by all keys from over.
func mergeStringMaps(base, over map[string]string) map[string]string {
    if len(base) == 0 && len(over) == 0 {
        return nil
    }
    out := make(map[string]string, len(base)+len(over))
    for k, v := range base {
        out[k] = v
    }
    for k, v := range over {
        out[k] = v
    }
    return out
}

// cloneVisited returns a copy of v with extra added.
func cloneVisited(v map[string]bool, extra string) map[string]bool {
    out := make(map[string]bool, len(v)+1)
    for k := range v {
        out[k] = true
    }
    out[extra] = true
    return out
}

// findIncludeLine returns the YAML line number of the include[idx] entry in
// the parent file. Returns 0 on any parse error or out-of-range index.
func findIncludeLine(parentPath string, idx int) int {
    data, err := os.ReadFile(parentPath)
    if err != nil {
        return 0
    }
    var doc yaml.Node
    if err := yaml.Unmarshal(data, &doc); err != nil {
        return 0
    }
    if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
        return 0
    }
    mapping := doc.Content[0]
    if mapping.Kind != yaml.MappingNode {
        return 0
    }
    for i := 0; i+1 < len(mapping.Content); i += 2 {
        if mapping.Content[i].Value == "include" {
            seq := mapping.Content[i+1]
            if seq.Kind != yaml.SequenceNode || idx < 0 || idx >= len(seq.Content) {
                return mapping.Content[i].Line
            }
            return seq.Content[idx].Line
        }
    }
    return 0
}
```

The `parseCollectionBytes(path, data)` helper is a new private function in
`parser.go` that performs the same structural validation as the body of
`ParseFileWithOptions` (everything from `yaml.Unmarshal` through
`validateRequests` and the rate_limit check) but takes raw bytes instead of
re-reading the file. This avoids both code duplication and re-firing the
include gate when descending. **`parseCollectionBytes` itself does NOT call
`resolveIncludes`** — recursion is driven explicitly by `resolveIncludes` so
each child collection's own `Include` slice is processed within the same
`includeContext`.

#### Tests to Write FIRST (RED phase)

Table-driven test in `internal/parser/include_test.go`:

```go
func TestResolveIncludes(t *testing.T) {
    tests := []struct {
        name          string
        parentFile    string
        wantErr       error
        wantCount     int               // total request items after splice
        wantOverrides map[string]string // expected Variables.Values on a sentinel item
    }{
        {
            name:       "single include splices child requests in order",
            parentFile: "testdata/include/parent_simple.yaml",
            wantCount:  2, // 1 parent + 1 child
        },
        {
            name:          "child variable overrides parent for child request only",
            parentFile:    "testdata/include/parent_override.yaml",
            wantOverrides: map[string]string{"b": "99", "c": "3"},
        },
        {
            name:          "child resolves base_url from parent snapshot",
            parentFile:    "testdata/include/parent_base_url.yaml",
            wantOverrides: nil, // child declared no vars; nil overrides
        },
        {
            name:       "transitive include resolves with cumulative overrides",
            parentFile: "testdata/include/parent_grandchild.yaml",
            wantCount:  3,
        },
        {
            name:       "circular include detected",
            parentFile: "testdata/include/circular_a.yaml",
            wantErr:    ErrCircularInclude,
        },
        {
            name:       "include relative path resolved from including file",
            parentFile: "testdata/include/subdir/parent_relative.yaml",
            wantCount:  1,
        },
        {
            name:       "include file not found",
            parentFile: "testdata/include/parent_missing.yaml",
            wantErr:    ErrIncludeNotFound,
        },
        {
            name:       "child request own override wins over include overrides",
            parentFile: "testdata/include/parent_request_override.yaml",
            // request_b should win over child collection b override
        },
    }
    // ... loop ...
}
```

Plus a focused unit test for the **parent-after-include leak guard**:

```go
func TestResolveIncludes_parent_requests_unchanged_by_child_overrides(t *testing.T) {
    // parent has variables {a: 1, b: 2} and a request "ParentReq".
    // child has variables {b: 99} and a request "ChildReq".
    // After resolveIncludes: ParentReq.Variables.Values must be empty (no
    // overrides stamped), ChildReq.Variables.Values must contain b=99.
}
```

Plus a test for the cumulative override propagation through grandchildren:

```go
func TestResolveIncludes_grandchild_sees_child_overrides(t *testing.T) {
    // parent vars: {a:1, b:2}
    // child vars:  {b:99, c:3}
    // grandchild vars: {d:4}
    // After resolution, the grandchild request's overrides must contain
    // b=99, c=3, d=4 (so it sees the child's b override even though the
    // grandchild is spliced into the root parent's items).
}
```

#### Test Fixtures to Create

Create the following YAML files under
`internal/parser/testdata/include/`:

| File | Purpose |
|------|---------|
| `parent_simple.yaml` | Parent with one include and one own request |
| `child_simple.yaml` | Child with one request |
| `parent_override.yaml` | Parent vars `{a:1, b:2}`, includes `./child_override.yaml` |
| `child_override.yaml` | Vars `{b:99, c:3}`, one request |
| `parent_base_url.yaml` | Parent vars `{base_url: https://api.example.com}`, includes `./child_base_url.yaml` |
| `child_base_url.yaml` | One request with URL `{{base_url}}/v1/ping` (no own vars) |
| `parent_grandchild.yaml` | Parent → child → grandchild chain |
| `child_grandchild.yaml` | Includes `./grandchild.yaml`, vars `{b:99, c:3}` |
| `grandchild.yaml` | Vars `{d:4}`, one request |
| `circular_a.yaml` | Includes `./circular_b.yaml` |
| `circular_b.yaml` | Includes `./circular_a.yaml` (cycle) |
| `subdir/parent_relative.yaml` | Includes `./../include/child_simple.yaml` to prove relative resolution |
| `parent_missing.yaml` | Includes `./does_not_exist.yaml` |
| `parent_request_override.yaml` | Parent includes child whose request declares its own `variables: {b: req_b_wins}` |
| `parent_unchanged.yaml` | Parent vars `{a:1, b:2}` with parent request and one include of child_override.yaml |

#### Impact on Existing Tests

- None — new test file, new fixtures; no existing path collisions.

---

### Step 4: Wire `resolveIncludes` into `ParseFileWithOptions`

**Rationale:** With the resolver and tests green at the helper level, plug it
into the public parse function. The gate fires here, **before** any child
file is opened.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | Add gate check + `resolveIncludes` call inside `ParseFileWithOptions` |

#### New Code (insertion site)

After the post-unmarshal `col.Name == ""` check and before
`resolveExternalReferences`:

```go
// M3-003: include directive resolution (Professional tier).
if len(col.Include) > 0 {
    if opts.IncludeGate != nil {
        if gateErr := opts.IncludeGate(); gateErr != nil {
            return nil, gateErr
        }
    }
    absParent, absErr := filepath.Abs(path)
    if absErr != nil {
        return nil, fmt.Errorf("resolving parent path %q: %w", path, absErr)
    }
    var includeExt []string
    incCtx := &includeContext{
        cumulativeVars: nil,
        visited:        map[string]bool{absParent: true},
        extFiles:       &includeExt,
    }
    if err := resolveIncludes(&col, path, incCtx); err != nil {
        return nil, err
    }
    col.ExternalFiles = append(col.ExternalFiles, includeExt...)
}
```

The existing `resolveExternalReferences` block remains unchanged and runs
**after** include resolution — so spliced child requests that themselves use
`path:`-style external references are processed with the parent's collection
directory as the base. The Plan agent flagged this as a subtle interaction:

- Child requests loaded via `path:` *inside the child file* should resolve
  relative to the **child**'s directory, not the parent's. Because spliced
  items already carry their `path:` field unmodified, this would produce
  wrong resolution if `resolveExternalReferences` runs at the parent level.

To handle this correctly, `resolveIncludes` must call
`resolveExternalReferences(absChild, child.Setup.Items, ...)` (and likewise
for Requests/Teardown) on the **child** before splicing — i.e. fully resolve
the child's own external references against the child's directory, then
splice the resolved items. We update `parseCollectionBytes` to perform this
resolution as part of "fully parsing a child", so by the time
`resolveIncludes` appends items into the parent, all `path:` references are
already inlined as fully-formed `RequestItem`s.

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_include_gate_blocks_at_free_tier(t *testing.T) {
    // Construct a parent with `include: [./shared/auth.yaml]`.
    // Call ParseFileWithOptions with an IncludeGate that returns a
    // *auth.GateError. Assert err is the same gate error and Collection
    // is nil.
}

func TestParseFile_include_no_gate_at_default(t *testing.T) {
    // ParseFile (no gate) on the same parent succeeds and returns spliced
    // items. Confirms gate is opt-in.
}

func TestParseFile_include_resolves_and_splices(t *testing.T) {
    // End-to-end through ParseFile: parent + 2 children produce the merged
    // request list with parent's order preserved and includes spliced after.
}

func TestParseFile_include_path_relative_to_parent_not_cwd(t *testing.T) {
    // Parse parent in subdir/; include path "./child.yaml" must resolve
    // to subdir/child.yaml regardless of process CWD. Use t.Chdir to
    // confirm.
}

func TestParseFile_include_child_path_external_resolves_against_child_dir(t *testing.T) {
    // Child includes a request via `path: ./requests/get.yaml`.
    // The get.yaml lives in the child's directory, NOT the parent's.
    // After ParseFile, the spliced request must have the correct URL.
}
```

#### Impact on Existing Tests

- `TestParseFile_ExternalReferences` (in `external_test.go`) and similar
  tests must continue to pass. Because we only **add** an include-resolution
  block (the existing `resolveExternalReferences` call site stays in place
  for the **parent** file's own `path:` items), no existing test loses
  coverage. We do refactor `resolveExternalReferences` invocation to also be
  called from `parseCollectionBytes`, but the contract is identical when
  there are no includes.

---

### Step 5: Register the `include_directive` feature in the auth registry

**Rationale:** The gate check needs a registered feature definition.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Register `include_directive` as Professional tier |
| `internal/auth/registry_test.go` | modify | Add test row asserting the new entry exists |

#### New Code (added to `DefaultRegistry()`)

```go
r.Register(FeatureDefinition{
    Name:         "include_directive",
    RequiredTier: TierProfessional,
    Description:  "include: directive requires Professional tier ($19/month)",
    Workaround:   "Inline shared requests into each collection, or use path: external references for individual requests",
})
```

#### Tests to Write FIRST (RED phase)

```go
func TestDefaultRegistry_include_directive_registered(t *testing.T) {
    reg := DefaultRegistry()
    def, ok := reg.Lookup("include_directive")
    if !ok {
        t.Fatal("include_directive feature not registered")
    }
    if def.RequiredTier != TierProfessional {
        t.Errorf("RequiredTier = %v, want Professional", def.RequiredTier)
    }
}
```

#### Impact on Existing Tests

- `internal/auth/registry_test.go` likely has a test that counts registered
  features or asserts a specific list. Need to update the count / list.

---

### Step 6: Wire `IncludeGate` from the CLI entry points

**Rationale:** Hook the new gate callback into both `cmd/apitest/main.go` and
`internal/watch/paths.go` so real users hit the Professional-tier check.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Replace `parser.ParseFile(file)` at line 275 with `parser.ParseFileWithOptions(file, parser.ParseOptions{IncludeGate: includeGateFor(currentTier())})` |
| `internal/watch/paths.go` | modify | Same swap at line 76, using `currentTier()` from the watch caller (or accept a tier arg) |
| `cmd/apitest/main.go` | modify | Add `includeGateFor(tier auth.Tier) func() error` helper that returns nil when allowed or a `*auth.GateError` otherwise |

#### New Code (in `main.go`)

```go
// includeGateFor returns a parser include-gate callback that enforces the
// Professional-tier feature gate when the user is below Professional.
func includeGateFor(tier auth.Tier) func() error {
    reg := auth.DefaultRegistry()
    return func() error {
        return auth.CheckFeature(reg, "include_directive", tier)
    }
}

// ... in runCmdInner, replace:
col, err := parser.ParseFile(file)
// ... with:
col, err := parser.ParseFileWithOptions(file, parser.ParseOptions{
    IncludeGate: includeGateFor(currentTier()),
})
```

The error path that handles `parser.ParseFile` failure already exists (lines
276–296). We extend it to detect a `*auth.GateError` and return exit code 6
(matching the `parallel_execution` gate handling at line 436):

```go
if err != nil {
    var gateErr *auth.GateError
    if errors.As(err, &gateErr) {
        switch format {
        case "json":
            jsonOut := &output.GateJSONOutput{
                Status: "feature_gated", ExitCode: 6,
                Feature:          gateErr.Result.Feature,
                RequiredTier:     string(gateErr.Result.RequiredTier),
                CurrentTier:      string(gateErr.Result.CurrentTier),
                Message:          gateErr.Result.Message,
                UpgradeURL:       gateErr.Result.UpgradeURL,
                TrialAvailable:   gateErr.Result.TrialAvailable,
                RegisterForTrial: gateErr.Result.RegisterURL,
                Workaround:       gateErr.Result.Workaround,
            }
            _ = output.WriteGateJSON(os.Stdout, jsonOut)
        case "tap":
            _ = writeTAPBailout(os.Stdout, err)
        case "junit":
            _ = writeJUnitError(os.Stdout, err)
        case "html":
            _ = writeHTMLError(report, err)
        default:
            errOut.StructuredError(err)
        }
        return 6, nil
    }
    // ... existing error handling (returns 3) ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_include_gate_blocks_at_free_tier(t *testing.T) {
    // Override currentTier() to TierFree. Run apitest run on a parent file
    // with `include: [./child.yaml]`. Expect exit code 6 and that stderr
    // contains "include_directive".
}

func TestRunCmd_include_runs_at_professional_tier(t *testing.T) {
    // Override currentTier() to TierProfessional. Run apitest run on a
    // parent that includes a child with one request hitting an httptest
    // server. Expect exit 0 and the request to have actually been sent.
}

func TestWatch_include_gate_blocks_at_free_tier(t *testing.T) {
    // Same as above but via internal/watch path discovery.
}
```

#### Impact on Existing Tests

- Existing `cmd/apitest/main_test.go` tests that use `parser.ParseFile` via
  the CLI will now go through `ParseFileWithOptions`. As long as none of
  those collections declare `include:`, behavior is unchanged.
- `cmd/apitest/main_test.go:5297` uses `parser.ParseFile` directly in a test
  — leave it alone (it's a test helper, no gate needed).
- `internal/watch/paths.go:76` (and its tests) — sweep call sites; those that
  already work without includes are unaffected.

---

### Step 7: Update the JSON schema

**Rationale:** Schema is a leaf concern; updating last avoids churn during
earlier iterations.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/collection.json` | modify | Add `include` top-level property |
| `internal/schema/schema_test.go` | modify | Add a positive case (collection with include passes validation) and a negative case (include items must be strings) |

#### New Code

```json
"include": {
  "type": "array",
  "description": "Other collection files to include (Professional tier). Items are spliced into setup/requests/teardown in order. Variables in this collection form a snapshot the included children inherit; child variable changes never leak back to this collection.",
  "items": { "type": "string" }
},
```

#### Tests to Write FIRST (RED phase)

```go
func TestSchema_validates_include_directive(t *testing.T) {
    // YAML with include: [./a.yaml, ./b.yaml] passes validation.
}

func TestSchema_rejects_non_string_include_item(t *testing.T) {
    // YAML with include: [{path: ./a.yaml}] fails validation because items
    // must be strings.
}
```

#### Impact on Existing Tests

- None — additive property under `additionalProperties: false`. Existing
  collection fixtures without `include:` continue to validate.

---

### Step 8: Smoke test, CHANGELOG, and observable verification

**Rationale:** Wire the new feature into the user-visible smoke flow.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add a smoke case that exercises a parent + include scenario |
| `CHANGELOG.md` | modify | Add entry under "Unreleased" describing M3-003 |

The smoke test will create a small `parent.yaml` that includes a
`shared/auth.yaml` and a `shared/common.yaml`, run it, and assert exit 0. It
will then re-run with `APITEST_TIER=free` and assert exit 6.

#### Impact on Existing Tests

- None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | `TestParseFile` | none — additive `Include` field defaults to nil | none |
| `internal/parser/parser_test.go` | new `TestParseFileWithOptions_*` | new | write |
| `internal/parser/parser_test.go` | new `TestParseFile_include_*` | new | write |
| `internal/parser/include_test.go` | `TestResolveIncludes` (table) | new file | write |
| `internal/parser/include_test.go` | `TestResolveIncludes_parent_requests_unchanged_by_child_overrides` | new | write |
| `internal/parser/include_test.go` | `TestResolveIncludes_grandchild_sees_child_overrides` | new | write |
| `internal/parser/external_test.go` | `TestParseFile_ExternalReferences` | none — `path:` resolution path unchanged when no `include:` is present | none |
| `internal/auth/registry_test.go` | feature-list assertion (if any) | likely needs +1 to count | update |
| `internal/auth/registry_test.go` | new `TestDefaultRegistry_include_directive_registered` | new | write |
| `cmd/apitest/main_test.go` | new `TestRunCmd_include_gate_*`, `TestRunCmd_include_runs_*` | new | write |
| `internal/watch/paths_test.go` | new `TestWatch_include_gate_*` | new | write |
| `internal/schema/schema_test.go` | new `TestSchema_validates_include_directive`, `TestSchema_rejects_non_string_include_item` | new | write |
| `internal/runner/runner.go` | (no changes) | none | — |
| `internal/variable/variable.go` | (no changes) | none | — |

---

## Risks and Edge Cases

- **Risk:** Recursive includes opening symlink loops outside the visited set
  (file `a` symlinks to file `b`, `b` includes `a`).
  → **Mitigation:** Use `filepath.Abs` + `filepath.Clean` for the visited
  key. For symlink resolution, evaluate symlinks via `filepath.EvalSymlinks`
  before keying. Document this in the helper godoc.

- **Risk:** Sensitive variables from the parent are passed into child request
  overrides as plain values, losing the `!sensitive` tag.
  → **Mitigation:** This is acceptable for M3-003 because `RequestItem.Variables.Sensitive`
  is per-item and not consulted by interpolation; sensitivity is tracked
  globally via the merged scope's `SensitiveSet`. We deliberately do **not**
  copy parent's sensitive flags into the spliced item's per-item
  `SensitiveSet` because the flags will already exist in the runtime base
  scope when the parent's `Variables.UnmarshalYAML` runs. The override map
  only carries values; redaction is driven by the base-scope sensitive set.
  Add a code comment in `stampOverrides` documenting this.

- **Risk:** `parseCollectionBytes` (the helper extracted from `ParseFile`)
  drifts from `ParseFile`'s validation as the latter evolves.
  → **Mitigation:** Make `ParseFileWithOptions` call `parseCollectionBytes`
  as its only validation entry point. The two share one body by construction.

- **Risk:** Child includes its own `path:` external request, and the
  reference resolves to the wrong directory.
  → **Mitigation:** `parseCollectionBytes(absChild, data)` runs
  `resolveExternalReferences(absChild, ...)` against the child's directory
  before returning. The fully-resolved items are then spliced into the
  parent. Step 4 includes a regression test
  (`TestParseFile_include_child_path_external_resolves_against_child_dir`).

- **Risk:** `findIncludeLine` opens the parent file a second time. Minor I/O
  cost; only triggered on the error path.
  → **Mitigation:** Acceptable. Alternative would be to pass the parent's
  raw bytes through `includeContext`, which complicates the API for a cold-
  path optimisation.

- **Edge case:** `include: []` (empty list) — handled by the `len(col.Include) == 0`
  short-circuit; gate is **not** fired (no actual include directive in
  effect). Add a test asserting empty include slice neither gates nor errors.

- **Edge case:** A child collection sets `name:` to empty string — the
  recursive `parseCollectionBytes` will reject it with `ErrEmptyCollection`
  just like a top-level parse would. Test confirms the error surfaces.

- **Edge case:** A child collection declares its own `rate_limit_rps`. The
  child's value is **ignored** because we splice items, not the
  `RateLimitRPS` field. Document this in the godoc on `Collection.Include`.

- **Edge case:** A parent's `setup:` block is empty but a child contributes
  `setup:` entries. The spliced result has the child's setup items as the
  parent's setup items in order. Test confirms this.

- **Edge case:** Parent's `Requests` is nil (unlikely; YAML `requests:` is
  required) but child contributes requests. Behavior: the spliced child
  requests become the parent's requests. The existing `requests:` required
  marker on the schema still applies — parent must have at least an empty
  `requests: []` list.

---

## Risks Surfaced and Resolved

The Plan agent flagged two open questions resolved by Decisions 1 and 2:

1. **Where does the gate fire?** → In the parser, via an opt-in `IncludeGate`
   callback in `ParseOptions`, so parser stays decoupled from `internal/auth`.
2. **How are variables propagated without changing runner/scope code?** →
   By stamping per-request `Variables.Values` overrides at parse time,
   reusing the existing `WithOverrides` plumbing in `internal/runner`.

No open questions remain.

---

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from the task YAML):

```bash
# Setup: create parent and shared files
mkdir -p /tmp/m3-003-obs/shared
cat > /tmp/m3-003-obs/parent.yaml <<'YAML'
name: Parent Collection
variables:
  base_url: https://api.example.com
include:
  - ./shared/auth.yaml
  - ./shared/common.yaml
requests:
  - name: Parent ping
    request:
      method: GET
      url: "{{base_url}}/parent-ping"
YAML
cat > /tmp/m3-003-obs/shared/auth.yaml <<'YAML'
name: Shared Auth
variables:
  auth_path: /auth/login
requests:
  - name: Login
    request:
      method: POST
      url: "{{base_url}}{{auth_path}}"
YAML
cat > /tmp/m3-003-obs/shared/common.yaml <<'YAML'
name: Shared Common
requests:
  - name: Health
    request:
      method: GET
      url: "{{base_url}}/health"
YAML

# Build and run at Professional tier (default in dev)
go build -o /tmp/apitest ./cmd/apitest
APITEST_TIER=professional /tmp/apitest run /tmp/m3-003-obs/parent.yaml
# expect: exit 0, three requests interpolating https://api.example.com/...

# Re-run at Free tier
APITEST_TIER=free /tmp/apitest run /tmp/m3-003-obs/parent.yaml
echo "exit=$?"
# expect: exit 6 with feature_gated message naming "include_directive"

# Run the targeted parser tests
go test ./internal/parser/... -run TestParseFile_include -v
go test ./internal/parser/... -run TestResolveIncludes -v
```
