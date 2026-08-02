# Implementation Plan: M3-005

## Overview
Add a new `apitest import openapi <spec-path> [--output path]` subcommand that parses an
OpenAPI 3.0/3.1 spec via `github.com/getkin/kin-openapi/openapi3` and emits a minimal
`parser.Collection` skeleton (method, URL, name, path-parameter interpolation, and a
`base_url` variable derived from `servers[0]`). The command is gated behind the
`openapi_import` Professional-tier feature.

## Task Details
- **ID:** M3-005
- **Title:** apitest import openapi: parse spec and emit collection skeleton
- **Phase:** M3: Professional Tier
- **Priority:** 4
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| — | (no dependencies) | — |

## Architectural Decisions

Three design questions came up during exploration. I resolved them as follows:

1. **Return shape of `Import`.** The task scope says
   `Import(specPath string) (*parser.Collection, error)`. I keep that signature —
   `Import` builds and returns a `parser.Collection` value so unit tests can assert on
   structured data rather than scraping YAML strings. A separate `Emit(w io.Writer,
   col *parser.Collection) error` handles writing.

2. **YAML serialization of the collection.** `parser.Collection` uses custom
   `UnmarshalYAML` for `SensitiveVars` and `Section` but has no corresponding
   `MarshalYAML`. A round-trip through `yaml.Marshal(col)` therefore produces
   `variables: { values: { base_url: ... } }`, which is the wrong shape and will not
   re-parse. Rather than add `MarshalYAML` methods to the parser package (high blast
   radius — touches all existing parser tests and GraphQL/WebSocket nested types), I
   define a minimal unexported "writer view" struct (`writerCollection` /
   `writerRequestItem` / `writerRequest`) inside `internal/openapi/emit.go`. This
   struct uses plain map/string fields in the exact shape the collection parser
   expects, so a round-trip through `validate` succeeds. If later tasks (M3-006)
   need to write bodies/headers, the writer view grows alongside it.

3. **Name synthesis from method + path.** When `operationId` is absent, the task
   yaml says `GET /pets -> get_pets`. I implement this as:
   lowercase method + `_` + path with leading `/` stripped, `/` → `_`, `{...}` →
   `by_...`, and any other non-identifier character → `_`. Examples:
   - `GET /pets` → `get_pets`
   - `GET /pets/{petId}` → `get_pets_by_petId`
   - `POST /pets` → `post_pets`
   Collisions between two operations sharing the same synthesised name append a
   `_2`, `_3`, ... suffix based on discovery order.

4. **Path parameter interpolation.** OpenAPI uses `{petId}` in the path template. The
   collection format uses `{{petId}}`. I replace every `{name}` segment in the path
   with `{{name}}` via a regexp. The `base_url` variable carries `servers[0].url`;
   the emitted request URL is `{{base_url}}/path/with/{{petId}}`.

5. **Missing servers block.** Task scope implies `servers` is present. If the spec
   has no `servers` (valid OpenAPI — implementations assume the server hosting the
   spec), I default `base_url` to an empty string and still emit `variables:
   { base_url: "" }`. The user can then set it at run time via `--var
   base_url=...`. This behaviour is covered by a unit test but is not in the task
   YAML's observable section; documenting it here.

6. **Path ordering.** `openapi3.T.Paths` is a keyed container (map-like). I sort path
   keys lexicographically before iteration, then iterate HTTP methods in a fixed
   order (`GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS, TRACE`) so output is
   deterministic across runs. This matters for `validate` round-trip tests.

## Implementation Steps

### Step 1: Add `kin-openapi` dependency and create `internal/openapi` package skeleton
**Rationale:** Smallest possible change that gets the package to compile. No command
wiring yet, no YAML emission — just the module + an empty file. Keeps blast radius
near zero so the subsequent TDD steps land on a green baseline.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | modify | add `github.com/getkin/kin-openapi v0.135.0` require |
| `go.sum` | modify | checksums from `go mod tidy` |
| `internal/openapi/doc.go` | create | package comment |

#### New Code
```go
// Package openapi parses OpenAPI 3.0/3.1 specifications and emits minimal
// apitest collection skeletons. Skeletons include method, URL (with path
// parameters interpolated as {{name}}), request name, and a base_url variable
// derived from the spec's servers[0]. Request bodies, headers, and assertions
// are handled in M3-006.
package openapi
```

Then run:
```bash
go get github.com/getkin/kin-openapi@v0.135.0
go mod tidy
go build ./...
```

#### Tests to Write FIRST (RED phase)
None for this step — it is a dependency bump. The first real test lands in Step 2.

#### Impact on Existing Tests
None. Pure addition.

---

### Step 2: Build `Import(specPath string) (*parser.Collection, error)` with unit tests
**Rationale:** Import is the pure, side-effect-free core. Driving it from tests
first forces the name-synthesis, path-interpolation, and base-url logic to be
correct before the CLI glue lands.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/import.go` | create | `Import`, `synthName`, `interpolatePath` |
| `internal/openapi/import_test.go` | create | table-driven unit tests |
| `internal/openapi/testdata/petstore.yaml` | create | minimal 3.0 fixture |
| `internal/openapi/testdata/petstore_31.yaml` | create | minimal 3.1 fixture |
| `internal/openapi/testdata/no_servers.yaml` | create | fixture without `servers:` |
| `internal/openapi/testdata/no_operation_id.yaml` | create | fixture with missing operationIds |
| `internal/openapi/testdata/invalid.yaml` | create | malformed YAML |
| `internal/openapi/errors.go` | create | sentinel errors |

#### New Code (sketch)

```go
// internal/openapi/errors.go
package openapi

import "errors"

var (
    ErrSpecNotFound     = errors.New("openapi spec file not found")
    ErrSpecInvalid      = errors.New("openapi spec is invalid")
    ErrUnsupportedSpec  = errors.New("openapi spec version is not supported")
)
```

```go
// internal/openapi/import.go
package openapi

import (
    "fmt"
    "regexp"
    "sort"
    "strings"

    "github.com/getkin/kin-openapi/openapi3"
    apierrors "github.com/peterlindqvist/apitest/internal/errors"
    "github.com/peterlindqvist/apitest/internal/parser"
    "github.com/peterlindqvist/apitest/internal/variable"
)

// methodOrder is the fixed iteration order for HTTP methods within a path.
// Keeping this stable makes generated collections deterministic.
var methodOrder = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE"}

var pathParamRE = regexp.MustCompile(`\{([^{}/]+)\}`)

// Import parses an OpenAPI 3.0 or 3.1 spec file and returns a minimal
// parser.Collection skeleton with one RequestItem per operation. Only method,
// URL, name, path parameters, and base_url variable are populated; bodies,
// headers, and assertions are deferred to M3-006.
func Import(specPath string) (*parser.Collection, error) {
    loader := openapi3.NewLoader()
    loader.IsExternalRefsAllowed = false
    doc, err := loader.LoadFromFile(specPath)
    if err != nil {
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryParse,
            FilePath: specPath,
            Message:  fmt.Sprintf("loading openapi spec: %v", err),
            Inner:    fmt.Errorf("%w: %v", ErrSpecInvalid, err),
        }
    }
    if err := doc.Validate(loader.Context); err != nil {
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryParse,
            FilePath: specPath,
            Message:  fmt.Sprintf("validating openapi spec: %v", err),
            Inner:    fmt.Errorf("%w: %v", ErrSpecInvalid, err),
        }
    }

    baseURL := ""
    if len(doc.Servers) > 0 {
        baseURL = doc.Servers[0].URL
    }

    name := "Imported Collection"
    if doc.Info != nil && doc.Info.Title != "" {
        name = doc.Info.Title
    }

    col := &parser.Collection{
        Name: name,
        Variables: parser.SensitiveVars{
            Values:    map[string]string{"base_url": baseURL},
            Sensitive: variable.NewSensitiveSet(),
            Commands:  map[string]parser.CommandVar{},
        },
    }

    // Sort paths for determinism.
    var paths []string
    if doc.Paths != nil {
        for p := range doc.Paths.Map() {
            paths = append(paths, p)
        }
    }
    sort.Strings(paths)

    used := map[string]int{}
    var items []parser.RequestItem
    for _, path := range paths {
        pi := doc.Paths.Find(path)
        if pi == nil {
            continue
        }
        ops := pi.Operations()
        for _, method := range methodOrder {
            op, ok := ops[method]
            if !ok {
                continue
            }
            item := parser.RequestItem{
                Name: chooseName(op, method, path, used),
                Request: parser.Request{
                    Method: method,
                    URL:    "{{base_url}}" + interpolatePath(path),
                },
            }
            items = append(items, item)
        }
    }
    col.Requests = parser.Section{Items: items}
    return col, nil
}

// interpolatePath rewrites OpenAPI path parameters {name} into {{name}}.
func interpolatePath(path string) string {
    return pathParamRE.ReplaceAllString(path, "{{$1}}")
}

// chooseName returns the operationId if set; otherwise synthesises a name from
// method and path. Collisions are disambiguated with _2, _3, ... suffixes.
func chooseName(op *openapi3.Operation, method, path string, used map[string]int) string {
    name := op.OperationID
    if name == "" {
        name = synthName(method, path)
    }
    if n := used[name]; n > 0 {
        disambiguated := fmt.Sprintf("%s_%d", name, n+1)
        used[name] = n + 1
        used[disambiguated] = 1
        return disambiguated
    }
    used[name] = 1
    return name
}

// synthName converts "GET /pets/{petId}" -> "get_pets_by_petId".
func synthName(method, path string) string {
    var b strings.Builder
    b.WriteString(strings.ToLower(method))
    b.WriteByte('_')
    trimmed := strings.TrimPrefix(path, "/")
    for i, seg := range strings.Split(trimmed, "/") {
        if i > 0 {
            b.WriteByte('_')
        }
        if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
            b.WriteString("by_")
            b.WriteString(seg[1 : len(seg)-1])
            continue
        }
        b.WriteString(seg)
    }
    return b.String()
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/openapi/import_test.go
package openapi

import (
    "errors"
    "strings"
    "testing"
)

func TestSynthName(t *testing.T) {
    tests := []struct {
        name   string
        method string
        path   string
        want   string
    }{
        {"simple get", "GET", "/pets", "get_pets"},
        {"post simple", "POST", "/pets", "post_pets"},
        {"single path param", "GET", "/pets/{petId}", "get_pets_by_petId"},
        {"nested path param", "GET", "/users/{userId}/pets/{petId}", "get_users_by_userId_pets_by_petId"},
        {"root", "GET", "/", "get_"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := synthName(tt.method, tt.path); got != tt.want {
                t.Errorf("synthName(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
            }
        })
    }
}

func TestInterpolatePath(t *testing.T) {
    tests := []struct{ in, want string }{
        {"/pets", "/pets"},
        {"/pets/{petId}", "/pets/{{petId}}"},
        {"/users/{userId}/pets/{petId}", "/users/{{userId}}/pets/{{petId}}"},
    }
    for _, tt := range tests {
        if got := interpolatePath(tt.in); got != tt.want {
            t.Errorf("interpolatePath(%q) = %q, want %q", tt.in, got, tt.want)
        }
    }
}

func TestImport_Petstore(t *testing.T) {
    col, err := Import("testdata/petstore.yaml")
    if err != nil {
        t.Fatalf("Import returned error: %v", err)
    }
    if col.Name != "Petstore" {
        t.Errorf("Name = %q, want %q", col.Name, "Petstore")
    }
    if got := col.Variables.Values["base_url"]; got != "https://api.example.com/v1" {
        t.Errorf("base_url = %q, want %q", got, "https://api.example.com/v1")
    }
    if len(col.Requests.Items) != 3 {
        t.Fatalf("Requests.Items len = %d, want 3", len(col.Requests.Items))
    }

    // Expect deterministic order: sorted paths, methods in methodOrder.
    want := []struct{ name, method, url string }{
        {"listPets", "GET", "{{base_url}}/pets"},
        {"createPet", "POST", "{{base_url}}/pets"},
        {"showPetById", "GET", "{{base_url}}/pets/{{petId}}"},
    }
    for i, w := range want {
        got := col.Requests.Items[i]
        if got.Name != w.name || got.Request.Method != w.method || got.Request.URL != w.url {
            t.Errorf("item[%d] = {%s %s %s}, want {%s %s %s}",
                i, got.Name, got.Request.Method, got.Request.URL,
                w.name, w.method, w.url)
        }
    }
}

func TestImport_OpenAPI31(t *testing.T) {
    col, err := Import("testdata/petstore_31.yaml")
    if err != nil {
        t.Fatalf("Import returned error: %v", err)
    }
    if len(col.Requests.Items) == 0 {
        t.Fatal("expected at least one request from 3.1 spec")
    }
}

func TestImport_NoServers(t *testing.T) {
    col, err := Import("testdata/no_servers.yaml")
    if err != nil {
        t.Fatalf("Import returned error: %v", err)
    }
    if col.Variables.Values["base_url"] != "" {
        t.Errorf("expected empty base_url, got %q", col.Variables.Values["base_url"])
    }
}

func TestImport_NoOperationId(t *testing.T) {
    col, err := Import("testdata/no_operation_id.yaml")
    if err != nil {
        t.Fatalf("Import returned error: %v", err)
    }
    names := map[string]bool{}
    for _, it := range col.Requests.Items {
        names[it.Name] = true
    }
    if !names["get_pets"] {
        t.Errorf("expected synthesised name get_pets; got %v", names)
    }
}

func TestImport_FileNotFound(t *testing.T) {
    _, err := Import("testdata/does_not_exist.yaml")
    if err == nil {
        t.Fatal("expected error for missing file")
    }
    if !errors.Is(err, ErrSpecInvalid) && !strings.Contains(err.Error(), "no such file") {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestImport_InvalidSpec(t *testing.T) {
    _, err := Import("testdata/invalid.yaml")
    if err == nil {
        t.Fatal("expected error for invalid spec")
    }
    if !errors.Is(err, ErrSpecInvalid) {
        t.Errorf("expected ErrSpecInvalid, got %v", err)
    }
}
```

Fixture sketch `internal/openapi/testdata/petstore.yaml`:

```yaml
openapi: 3.0.3
info:
  title: Petstore
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: OK
    post:
      operationId: createPet
      responses:
        '201':
          description: Created
  /pets/{petId}:
    get:
      operationId: showPetById
      parameters:
        - name: petId
          in: path
          required: true
          schema: { type: string }
      responses:
        '200':
          description: OK
```

#### Impact on Existing Tests
None. New package, new tests.

---

### Step 3: Add `Emit(w io.Writer, col *parser.Collection) error` and round-trip test
**Rationale:** YAML emission is a second, independently testable concern. Separating
it from `Import` lets us verify that the emitted bytes round-trip through
`validator.Validate` without dragging CLI or OpenAPI fixtures into the assertion.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/emit.go` | create | writer-view structs + `Emit` |
| `internal/openapi/emit_test.go` | create | marshal + round-trip tests |

#### New Code (sketch)

```go
// internal/openapi/emit.go
package openapi

import (
    "fmt"
    "io"

    "github.com/peterlindqvist/apitest/internal/parser"
    "gopkg.in/yaml.v3"
)

// writerCollection is a YAML-friendly projection of parser.Collection used
// strictly for emission. parser.Collection has custom UnmarshalYAML on
// SensitiveVars and Section that would not round-trip via yaml.Marshal, so we
// define our own outbound shape here. If later tasks need to emit additional
// fields (headers, body, assertions), they grow this struct — not the parser.
type writerCollection struct {
    Name      string               `yaml:"name"`
    Variables map[string]string    `yaml:"variables,omitempty"`
    Requests  []writerRequestItem  `yaml:"requests"`
}

type writerRequestItem struct {
    Name    string        `yaml:"name"`
    Request writerRequest `yaml:"request"`
}

type writerRequest struct {
    Method string `yaml:"method"`
    URL    string `yaml:"url"`
}

// Emit writes col to w as a YAML collection suitable for apitest run/validate.
func Emit(w io.Writer, col *parser.Collection) error {
    out := writerCollection{
        Name:      col.Name,
        Variables: col.Variables.Values,
        Requests:  make([]writerRequestItem, 0, len(col.Requests.Items)),
    }
    for _, it := range col.Requests.Items {
        out.Requests = append(out.Requests, writerRequestItem{
            Name: it.Name,
            Request: writerRequest{
                Method: it.Request.Method,
                URL:    it.Request.URL,
            },
        })
    }
    enc := yaml.NewEncoder(w)
    enc.SetIndent(2)
    if err := enc.Encode(&out); err != nil {
        return fmt.Errorf("encoding collection: %w", err)
    }
    return enc.Close()
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/openapi/emit_test.go
package openapi

import (
    "bytes"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/peterlindqvist/apitest/internal/parser"
    "github.com/peterlindqvist/apitest/internal/validator"
)

func TestEmit_ContainsExpectedKeys(t *testing.T) {
    col, err := Import("testdata/petstore.yaml")
    if err != nil {
        t.Fatalf("Import: %v", err)
    }
    var buf bytes.Buffer
    if err := Emit(&buf, col); err != nil {
        t.Fatalf("Emit: %v", err)
    }
    out := buf.String()
    wantSubstrings := []string{
        "name: Petstore",
        "variables:",
        "base_url: https://api.example.com/v1",
        "method: GET",
        "method: POST",
        "url: '{{base_url}}/pets'",
        "url: '{{base_url}}/pets/{{petId}}'",
        "name: listPets",
    }
    for _, s := range wantSubstrings {
        if !strings.Contains(out, s) {
            t.Errorf("emitted YAML missing %q:\n%s", s, out)
        }
    }
}

func TestEmit_RoundTripsThroughParser(t *testing.T) {
    col, err := Import("testdata/petstore.yaml")
    if err != nil {
        t.Fatalf("Import: %v", err)
    }
    tmp := filepath.Join(t.TempDir(), "petstore.collection.yaml")
    f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    if err := Emit(f, col); err != nil {
        t.Fatalf("Emit: %v", err)
    }
    _ = f.Close()

    // Must parse back cleanly.
    reparsed, err := parser.ParseFile(tmp)
    if err != nil {
        t.Fatalf("ParseFile: %v", err)
    }
    if reparsed.Name != "Petstore" {
        t.Errorf("Name = %q, want Petstore", reparsed.Name)
    }
    if len(reparsed.Requests.Items) != 3 {
        t.Fatalf("re-parsed len = %d, want 3", len(reparsed.Requests.Items))
    }
    if reparsed.Variables.Values["base_url"] != "https://api.example.com/v1" {
        t.Errorf("base_url did not round-trip: %v", reparsed.Variables.Values)
    }

    // Must validate clean.
    res := validator.Validate(tmp, nil)
    if !res.Valid {
        t.Errorf("validate failed: %+v", res.Issues)
    }
}
```

> Note on single-quoted URLs: yaml.v3 quotes values containing `{{` because the
> braces could be flow scalars. Both `url: '{{base_url}}/pets'` and
> `url: "{{base_url}}/pets"` parse back to the same string, so the assertion
> above uses single-quote form that yaml.v3 emits by default. If a future
> yaml.v3 bump changes the quoting style, the substring check gets updated.

#### Impact on Existing Tests
None.

---

### Step 4: Register `openapi_import` feature in auth registry
**Rationale:** The feature gate is a tiny, isolated change in `internal/auth`. Landing
it before the CLI wiring means the CLI can call `auth.CheckFeature(..., "openapi_import", ...)`
without racing the registry update.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | register `openapi_import` feature |
| `internal/auth/registry_test.go` | modify | assert new feature is registered |

#### Current Code
```go
	r.Register(FeatureDefinition{
		Name:         "schema_validation",
		RequiredTier: TierProfessional,
		Description:  "JSON Schema response body assertions require Professional tier ($19/month)",
		Workaround:   "Use body: JSONPath assertions to check individual fields (type, equals, exists)",
	})
	return r
}
```

#### New Code
```go
	r.Register(FeatureDefinition{
		Name:         "schema_validation",
		RequiredTier: TierProfessional,
		Description:  "JSON Schema response body assertions require Professional tier ($19/month)",
		Workaround:   "Use body: JSONPath assertions to check individual fields (type, equals, exists)",
	})
	r.Register(FeatureDefinition{
		Name:         "openapi_import",
		RequiredTier: TierProfessional,
		Description:  "OpenAPI import requires Professional tier ($19/month)",
		Workaround:   "Author collections by hand, or use the validate command to check hand-written collections",
	})
	return r
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestDefaultRegistry_RegistersOpenAPIImport(t *testing.T) {
    r := DefaultRegistry()
    def, ok := r.Lookup("openapi_import")
    if !ok {
        t.Fatal("expected openapi_import to be registered")
    }
    if def.RequiredTier != TierProfessional {
        t.Errorf("RequiredTier = %v, want %v", def.RequiredTier, TierProfessional)
    }
}
```

#### Impact on Existing Tests
None directly, but `registry_test.go` may have a "count of registered features"
assertion — if so, bump the expected count by one.

---

### Step 5: Wire `import openapi` subcommand in `cmd/apitest/main.go`
**Rationale:** This is the biggest blast radius (top-level command dispatch, help
text, gate handling). Doing it last means every underlying piece is already proven.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | add `import` case, `importCmd`, `importOpenAPICmd`, help text |
| `cmd/apitest/main_test.go` | modify | integration test for the new command |
| `cmd/apitest/testdata/openapi/petstore.yaml` | create | minimal 3.0 fixture |
| `testdata/openapi/petstore.yaml` | create | repo-level fixture used by the observable command |

#### Current Code (dispatch)
```go
	case "vault":
		return vaultCmd(args[1:])
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		printHelp()
		return 1
	}
```

#### New Code (dispatch)
```go
	case "vault":
		return vaultCmd(args[1:])
	case "import":
		return importCmd(args[1:])
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		printHelp()
		return 1
	}
```

Plus a new function block at the end of the file:

```go
// importCmd dispatches import subcommands.
func importCmd(args []string) int {
    if len(args) == 0 {
        _, _ = fmt.Fprintln(os.Stderr, "Usage: apitest import <format> <spec-path> [--output <file>]")
        _, _ = fmt.Fprintln(os.Stderr, "Formats:")
        _, _ = fmt.Fprintln(os.Stderr, "  openapi    Import an OpenAPI 3.0/3.1 spec (Professional tier)")
        return 1
    }
    switch args[0] {
    case "openapi":
        return importOpenAPICmd(args[1:])
    default:
        _, _ = fmt.Fprintf(os.Stderr, "Unknown import format: %s\n", args[0])
        return 1
    }
}

// importOpenAPICmd imports an OpenAPI spec and writes a collection skeleton.
func importOpenAPICmd(args []string) int {
    specPath, outputPath, err := parseImportOpenAPIArgs(args)
    if err != nil {
        _, _ = fmt.Fprintln(os.Stderr, "Usage: apitest import openapi <spec-path> [--output <file>]")
        _, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        return 1
    }

    // Feature gate.
    reg := auth.DefaultRegistry()
    tier := currentTier()
    if gateErr := auth.CheckFeature(reg, "openapi_import", tier); gateErr != nil {
        var ge *auth.GateError
        if !errors.As(gateErr, &ge) {
            _, _ = fmt.Fprintf(os.Stderr, "unexpected error: %v\n", gateErr)
            return 1
        }
        printer := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, false))
        printer.FeatureGate(&ge.Result)
        return 6
    }

    col, err := openapi.Import(specPath)
    if err != nil {
        _, _ = fmt.Fprintf(os.Stderr, "Error importing OpenAPI spec: %v\n", err)
        return 3
    }

    var w io.Writer = os.Stdout
    if outputPath != "" {
        f, openErr := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
        if openErr != nil {
            _, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", openErr)
            return 3
        }
        defer f.Close()
        w = f
    }
    if err := openapi.Emit(w, col); err != nil {
        _, _ = fmt.Fprintf(os.Stderr, "Error writing collection: %v\n", err)
        return 1
    }
    return 0
}

// parseImportOpenAPIArgs extracts the spec path and optional --output flag.
func parseImportOpenAPIArgs(args []string) (specPath, outputPath string, err error) {
    var positional []string
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--output", "-o":
            i++
            if i >= len(args) {
                return "", "", fmt.Errorf("--output requires a file path")
            }
            outputPath = args[i]
        default:
            positional = append(positional, args[i])
        }
    }
    if len(positional) == 0 {
        return "", "", fmt.Errorf("missing openapi spec path")
    }
    if len(positional) > 1 {
        return "", "", fmt.Errorf("unexpected extra arguments: %v", positional[1:])
    }
    return positional[0], outputPath, nil
}
```

And update `printHelp()`:

```go
	fmt.Println("  vault           Manage vault provider profiles (requires Solo tier)")
	fmt.Println("  vault list      List configured vault provider profiles")
	fmt.Println("  import openapi  Import OpenAPI 3.x spec into a collection skeleton (Professional tier)")
```

Plus an `"github.com/peterlindqvist/apitest/internal/openapi"` import at the top of
`main.go`.

#### Tests to Write FIRST (RED phase)

Integration tests in `cmd/apitest/main_test.go`:

```go
func TestImportOpenAPI_WritesCollection(t *testing.T) {
    t.Setenv("APITEST_TIER", "professional")
    dir := t.TempDir()
    out := filepath.Join(dir, "petstore.collection.yaml")

    code := run([]string{"import", "openapi", "testdata/openapi/petstore.yaml", "--output", out})
    if code != 0 {
        t.Fatalf("exit code = %d, want 0", code)
    }
    info, err := os.Stat(out)
    if err != nil {
        t.Fatalf("stat: %v", err)
    }
    if info.Mode().Perm() != 0644 {
        t.Errorf("mode = %v, want 0644", info.Mode().Perm())
    }

    // Round-trip: validate the emitted file.
    code = run([]string{"validate", out})
    if code != 0 {
        t.Errorf("validate exit = %d, want 0", code)
    }
}

func TestImportOpenAPI_FreeTierGated(t *testing.T) {
    t.Setenv("APITEST_TIER", "free")
    code := run([]string{"import", "openapi", "testdata/openapi/petstore.yaml"})
    if code != 6 {
        t.Errorf("exit code = %d, want 6", code)
    }
}

func TestImportOpenAPI_MissingSpec(t *testing.T) {
    t.Setenv("APITEST_TIER", "professional")
    code := run([]string{"import", "openapi", "testdata/openapi/does_not_exist.yaml"})
    if code != 3 {
        t.Errorf("exit code = %d, want 3", code)
    }
}

func TestImportCmd_UnknownFormat(t *testing.T) {
    t.Setenv("APITEST_TIER", "professional")
    code := run([]string{"import", "graphql", "foo.graphql"})
    if code != 1 {
        t.Errorf("exit code = %d, want 1", code)
    }
}
```

`cmd/apitest/testdata/openapi/petstore.yaml` is a copy of the fixture from Step 2
so the integration test does not reach into another package's testdata.

Also create `testdata/openapi/petstore.yaml` at the repository root for the manual
observable command in the task YAML.

#### Impact on Existing Tests
- `cmd/apitest/main_test.go`: any "all commands listed in help" test needs to
  include the new `import` entry. Grep for `printHelp` / `apitest --help` / `schema` /
  `vault` in `main_test.go` first; update assertions if an exact-match help-text
  test exists.
- `cmd/apitest/run_test.go`: no known impact; this is a new top-level command,
  not a modification of `run`.

---

### Step 6: Update help text, CHANGELOG, and smoke test
**Rationale:** Completeness contract items. Landing them last keeps the main
implementation commits focused on behaviour.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | add Unreleased entry for M3-005 |
| `smoke/run.sh` | modify | add import openapi smoke checks (Professional + Free) |

#### New Code

`CHANGELOG.md` (prepend to Unreleased > Added):

```markdown
- `apitest import openapi <spec-path> [--output path]` imports an OpenAPI 3.0/3.1
  spec into a minimal collection skeleton: one request per operation with method,
  URL, name (from `operationId` or synthesised `method_path_by_param`), and a
  `variables: { base_url: ... }` entry derived from `servers[0]`; path parameters
  `{petId}` are rewritten to `{{petId}}`; deterministic output order (paths sorted
  lexicographically, methods iterated GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS/TRACE);
  `--output` writes to file with mode 0644, otherwise stdout; `openapi_import`
  registered as a Professional-tier feature gate (exit code 6 at Free/Solo tier);
  invalid or missing spec files return exit code 3 with a structured parse error;
  `internal/openapi` package backed by `github.com/getkin/kin-openapi/openapi3`;
  request bodies, headers, and assertions deferred to M3-006 (M3-005)
```

`smoke/run.sh` additions at the end:

```bash
# --- M3-005: apitest import openapi ---
echo "--- Import OpenAPI at Professional tier ---"
mkdir -p /tmp/apitest-smoke-openapi
cat > /tmp/apitest-smoke-openapi/petstore.yaml <<'OAI'
openapi: 3.0.3
info:
  title: Petstore
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: OK
  /pets/{petId}:
    get:
      operationId: showPetById
      parameters:
        - name: petId
          in: path
          required: true
          schema: { type: string }
      responses:
        '200':
          description: OK
OAI

APITEST_TIER=professional ./apitest import openapi /tmp/apitest-smoke-openapi/petstore.yaml \
    --output /tmp/apitest-smoke-openapi/generated.yaml \
  && echo "PASS: import openapi writes file" \
  || { echo "FAIL: import openapi failed"; exit 1; }

./apitest validate /tmp/apitest-smoke-openapi/generated.yaml \
  && echo "PASS: generated collection validates" \
  || { echo "FAIL: generated collection failed to validate"; exit 1; }

echo "--- Import OpenAPI at Free tier returns exit 6 ---"
APITEST_TIER=free ./apitest import openapi /tmp/apitest-smoke-openapi/petstore.yaml
CODE=$?
if [ "$CODE" = "6" ]; then
    echo "PASS: Free tier gated with exit 6"
else
    echo "FAIL: expected exit 6, got $CODE"
    exit 1
fi
```

#### Impact on Existing Tests
None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/registry_test.go` | existing count/list asserts (if any) | may break | bump expected feature count by 1 |
| `cmd/apitest/main_test.go` | help-text assertion (if any) | may break | include `import` in the expected commands list |
| `cmd/apitest/main_test.go` | new `TestImportOpenAPI_*` | new | create per Step 5 |
| `internal/openapi/*_test.go` | all | new | created in Steps 2 and 3 |

## Risks and Edge Cases

- **Risk:** `kin-openapi` pulls in a large transitive dependency tree.
  **Mitigation:** run `go mod graph | head -50` after the get; if more than 10
  new modules appear, document them in the plan update and keep an eye on binary
  size (`go build -ldflags="-s -w" ./cmd/apitest` before/after).

- **Risk:** OpenAPI 3.1 schema validation diverges from 3.0 in ways kin-openapi
  rejects. **Mitigation:** the 3.1 fixture test (`TestImport_OpenAPI31`) covers
  this; if kin-openapi does not load 3.1, fall back to
  `loader.IsExternalRefsAllowed = true` plus a strict-mode bypass, and document
  the exact version behaviour in the plan.

- **Risk:** yaml.v3 emits map keys in arbitrary order for `map[string]string`.
  Since we only emit one variable (`base_url`) today the order does not matter,
  but M3-006 and later additions need the writer view to switch from `map[...]`
  to a slice of key/value pairs if ordering becomes observable.
  **Mitigation:** documented here; Step 3 tests only check substring presence.

- **Edge case:** path param names containing dots or dashes (e.g., `{user.id}`
  or `{user-id}`). OpenAPI allows them; the synthName helper already preserves
  them as-is under `by_`. A test case covers this.
  **Handling:** add `{"dotted param", "GET", "/a/{user.id}", "get_a_by_user.id"}`
  to the `TestSynthName` table.

- **Edge case:** empty `paths:` section. **Handling:** `Import` returns a
  Collection with `Requests.Items == nil`; `Emit` produces `requests: null`,
  which the collection parser rejects. Add a unit test asserting `Import`
  returns an error for zero-operation specs, and mirror the behaviour with a
  sentinel `ErrNoOperations`.

- **Edge case:** OpenAPI `servers` with templated URLs (`{scheme}://example.com`).
  **Handling:** out of scope for M3-005. Accept the literal string as `base_url`
  and document the limitation in the help text.

- **Edge case:** duplicate synthesised names. **Handling:** disambiguation loop
  in `chooseName` (covered in unit tests).

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
# Place minimal spec:
mkdir -p testdata/openapi
# (petstore.yaml created as part of Step 5 fixtures)

# At Professional tier:
APITEST_TIER=professional go build ./cmd/apitest \
  && APITEST_TIER=professional ./apitest import openapi testdata/openapi/petstore.yaml \
     --output petstore.collection.yaml
APITEST_TIER=professional ./apitest validate petstore.collection.yaml
grep -q "base_url: https://api.example.com/v1" petstore.collection.yaml
grep -q "{{petId}}" petstore.collection.yaml

# At Free tier:
APITEST_TIER=free ./apitest import openapi testdata/openapi/petstore.yaml
echo "exit: $?"   # expect 6

# Unit tests:
go test ./internal/openapi/...
```
