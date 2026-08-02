# Implementation Plan: M3-004

## Overview

Add a `schema:` assertion that validates an HTTP response body against a
JSON Schema (Draft 2020-12) file referenced from the collection, gated to
Professional tier. The schema is loaded and compiled once per collection
run, validation errors are reported with JSONPath-style instance locations,
expected type, and actual value, and missing / invalid schema files fail
fast at parse time.

## Task Details

- **ID:** M3-004
- **Title:** JSON Schema response body validation assertion
- **Phase:** M3: Professional Tier
- **Priority:** 3
- **Complexity:** medium

## Dependencies

None. (Task YAML declares `dependencies: []`.)

| Task | Title | Status |
|------|-------|--------|
| —    | —     | —      |

## Architectural Decisions

1. **Library choice.** Use `github.com/santhosh-tekuri/jsonschema/v6`
   (v6.0.2 already present in the local module cache). It is the most
   widely-used Go JSON Schema library, supports Draft 2020-12 out of the
   box (`Draft2020` is `draftLatest`), returns a structured
   `*ValidationError` with `InstanceLocation []string`, `ErrorKind`
   (e.g. `*kind.Type` with `Got`/`Want`, `*kind.Required` with missing
   property), and a recursive `Causes` slice we can flatten to get every
   failing path (not just the first). This satisfies the "collect all
   validation errors" requirement.

2. **Where gating fires.** Gate at parse time, mirroring the existing
   `include_directive` pattern (M3-003). A `schema:` assertion present in
   any request triggers a `ParseOptions.SchemaGate` callback before any
   schema file is opened. Runtime never sees a gated collection, so the
   runner needs no gating code for this feature, matching the task's
   requirement that Free/Solo tier return exit code 6 with
   `schema_validation` gate.

3. **Where schemas are loaded.** Schema compilation happens during
   `parseCollectionBytes` after external references are resolved, so
   `path:`-referenced requests with `schema:` work identically to inline
   requests. Schemas are resolved relative to the collection file
   (using `filepath.Dir(absPath)` — same helper `resolveExternalReferences`
   uses). Parse errors (not found, invalid JSON Schema) are returned as
   `*apierrors.Structured` with `Category: CategoryParse` — fail-fast, not
   runtime.

4. **Schema cache scope.** A per-Collection cache keyed on the absolute
   schema path (populated during parse). The parser attaches the
   compiled `*jsonschema.Schema` onto a new unexported field on
   `parser.BodyAssertion` (or a sibling struct) so that the runner does
   not re-open or re-compile schemas on the hot path. Since
   `assertion.CheckBody` is called from three call sites
   (sequential runner, parallel executor, data-driven iteration), keeping
   compilation at parse time avoids duplicating cache plumbing into each
   caller.

5. **Schema assertion surface.** `schema:` lives alongside the existing
   `body:` map in `parser.Assertions` as a peer field (sibling, not
   operator). This keeps the YAML shape close to the task's example
   (`assertions: { schema: schemas/user.json }`) and avoids overloading
   the `body:` map-of-maps parsing. A new
   `assertion.CheckSchema(compiled *CompiledSchema, body []byte) []Result`
   function is added, invoked from `Evaluate` via a new field on
   `EvalInput` (carrying the opaque `*CompiledSchema`). Runner / parallel
   / data-driven call sites populate this field from
   `item.Assertions.Schema.Compiled`.

6. **Opaque wrapper.** The `assertion` package must not import
   `jsonschema/v6` directly through an exported type that returns the
   library's `*jsonschema.Schema`, to avoid leaking the dependency into
   every caller. Instead we expose an opaque
   `assertion.CompiledSchema` struct that stores the library handle
   as an unexported field, plus a constructor
   `assertion.CompileSchemaFile(path string) (*CompiledSchema, error)`
   invoked by the parser. `parser.BodyAssertion`-adjacent wiring holds
   `*assertion.CompiledSchema` (not the library type), so only `parser`
   and `assertion` import `jsonschema/v6`. (Parser imports it to call
   `assertion.CompileSchemaFile`; no direct library import from parser.)

7. **Result shape.** For each JSON Schema violation we produce one
   `assertion.Result` with:
   - `Type`  = `"schema <jsonpath>"` (e.g. `"schema $.id"`) — the instance
     location translated to JSONPath notation (`$` prefix, numeric indices
     `[0]`, string keys `.name`).
   - `Expected` = human-readable requirement: `"type integer"`,
     `"required: email"`, etc.
   - `Actual` = `jsonType(val)` + ": " + formatted actual value, or
     `"missing"` for required-field violations, or
     `"response body is not JSON"` when the body cannot be parsed.
   - `Passed` = `false`.
   On success (no errors), one `Result{Type: "schema <path>", Expected:
   "valid", Actual: "valid", Passed: true}` is produced so schema
   assertions always contribute exactly one visible line in the report.

8. **Path translation.** `jsonschema.ValidationError.InstanceLocation` is
   `[]string` (already decoded JSON Pointer tokens with no `/` or `~`
   escaping). We translate it to JSONPath by joining with `.` for object
   keys and `[N]` for array indices. Numeric tokens that round-trip to
   an integer become `[N]`; everything else becomes `.key`. A helper
   `toJSONPath(loc []string) string` lives in `assertion/schema.go`.

9. **Walk order.** Flatten `Causes` recursively, yielding only leaf
   errors (errors whose `Causes` is empty) to avoid duplicated group
   errors. This mirrors the library's own "detailed" output model.

10. **Registry entry.** Register `schema_validation` in
    `internal/auth/registry.go` at Professional tier with the wording
    "JSON Schema response body assertions require Professional tier".

11. **JSON Schema for the YAML schema itself.** `collection.json` needs
    the `assertions.schema` property added (string, optional,
    description: "Path to a JSON Schema file (Professional tier)").

## Implementation Steps

### Step 1: Add `schema_validation` to the feature registry

**Rationale:** Smallest change; registering a feature is side-effect free
and unlocks gate test coverage before any parser / assertion code changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | add `schema_validation` feature definition |
| `internal/auth/registry_test.go` | modify | assert the new definition exists at Professional tier |

#### Current Code

```go
r.Register(FeatureDefinition{
    Name:         "include_directive",
    RequiredTier: TierProfessional,
    Description:  "include: directive requires Professional tier ($19/month)",
    Workaround:   "Inline shared requests into each collection, or use path: external references for individual requests",
})
return r
```

#### New Code

```go
r.Register(FeatureDefinition{
    Name:         "include_directive",
    RequiredTier: TierProfessional,
    Description:  "include: directive requires Professional tier ($19/month)",
    Workaround:   "Inline shared requests into each collection, or use path: external references for individual requests",
})
r.Register(FeatureDefinition{
    Name:         "schema_validation",
    RequiredTier: TierProfessional,
    Description:  "JSON Schema response body assertions require Professional tier ($19/month)",
    Workaround:   "Use body: JSONPath assertions to check individual fields (type, equals, exists)",
})
return r
```

#### Tests to Write FIRST (RED phase)

```go
func TestDefaultRegistry_schema_validation(t *testing.T) {
    r := DefaultRegistry()
    def, ok := r.Lookup("schema_validation")
    if !ok {
        t.Fatal("schema_validation feature not registered")
    }
    if def.RequiredTier != TierProfessional {
        t.Errorf("RequiredTier = %v, want Professional", def.RequiredTier)
    }
    if def.Description == "" || def.Workaround == "" {
        t.Error("Description and Workaround must be set")
    }
}
```

#### Impact on Existing Tests

- None. Pure additive change.

---

### Step 2: Add jsonschema dependency to go.mod

**Rationale:** Must happen before any code references the library.
Isolated commit keeps the dependency change auditable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | modify | add `github.com/santhosh-tekuri/jsonschema/v6` |
| `go.sum` | modify | lock checksum |

#### Current Code

```
require (
    github.com/fsnotify/fsnotify v1.9.0
    gopkg.in/yaml.v3 v3.0.1
)
```

#### New Code

```
require (
    github.com/fsnotify/fsnotify v1.9.0
    github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
    gopkg.in/yaml.v3 v3.0.1
)
```

Run `go get github.com/santhosh-tekuri/jsonschema/v6@v6.0.2` and
`go mod tidy` to update `go.sum`. Verify `go build ./...` still succeeds
with no code yet calling the library.

#### Tests to Write FIRST

- No test. `go build` is the gate.

#### Impact on Existing Tests

- None. Dependency addition only.

---

### Step 3: Compiled-schema wrapper and JSONPath translator in assertion pkg

**Rationale:** Introduces the new types and pure helpers behind a
fully-tested unit surface. No other package references these yet, so
the blast radius is zero.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/schema.go` | create | `CompiledSchema`, `CompileSchemaFile`, `CheckSchema`, `toJSONPath` |
| `internal/assertion/schema_test.go` | create | table-driven tests for all schema behaviours |
| `internal/assertion/testdata/user.json` | create | Draft 2020-12 schema: id:int, email:string, required |
| `internal/assertion/testdata/user_bad.json` | create | schema with `"type": "not-a-type"` for compile-error case |

#### New Code (skeleton)

```go
// Package-private doc:
// schema.go implements JSON Schema validation for response bodies.
// Compilation is eager (at parse time) and yields a CompiledSchema
// opaque wrapper so callers do not import the third-party library.

package assertion

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "strconv"
    "strings"

    "github.com/santhosh-tekuri/jsonschema/v6"
    "github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// ErrSchemaFileNotFound is returned when a schema path cannot be resolved.
var ErrSchemaFileNotFound = errors.New("schema file not found")

// ErrSchemaInvalid is returned when a schema file is not a valid JSON Schema.
var ErrSchemaInvalid = errors.New("invalid JSON Schema")

// CompiledSchema is an opaque wrapper around a compiled JSON Schema.
// It is produced once per unique schema file and reused across every
// request (and every parallel/data-driven iteration) that references it.
type CompiledSchema struct {
    path   string             // absolute path, for error messages
    sch    *jsonschema.Schema // compiled schema
}

// Path returns the absolute path of the source schema file.
func (c *CompiledSchema) Path() string { return c.path }

// CompileSchemaFile reads absPath and compiles it as a JSON Schema.
// Returns ErrSchemaFileNotFound when the file is missing or
// ErrSchemaInvalid (with a wrapped cause) when the file is not a valid
// JSON Schema document. The returned *CompiledSchema is safe for
// concurrent use by multiple goroutines.
func CompileSchemaFile(absPath string) (*CompiledSchema, error) {
    f, err := os.Open(absPath)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil, fmt.Errorf("%w: %s", ErrSchemaFileNotFound, absPath)
        }
        return nil, fmt.Errorf("reading schema %s: %w", absPath, err)
    }
    defer f.Close()
    doc, err := jsonschema.UnmarshalJSON(f)
    if err != nil {
        return nil, fmt.Errorf("%w: %s: %w", ErrSchemaInvalid, absPath, err)
    }
    c := jsonschema.NewCompiler()
    if err := c.AddResource(absPath, doc); err != nil {
        return nil, fmt.Errorf("%w: %s: %w", ErrSchemaInvalid, absPath, err)
    }
    sch, err := c.Compile(absPath)
    if err != nil {
        return nil, fmt.Errorf("%w: %s: %w", ErrSchemaInvalid, absPath, err)
    }
    return &CompiledSchema{path: absPath, sch: sch}, nil
}

// CheckSchema validates body against compiled. Returns nil when compiled
// is nil (no assertion configured). On success returns a single passing
// Result; on failure returns one Result per leaf validation error. When
// body is not valid JSON a single failing Result is returned with
// Actual = "response body is not JSON".
func CheckSchema(compiled *CompiledSchema, body []byte) []Result {
    if compiled == nil {
        return nil
    }
    typeTag := fmt.Sprintf("schema %s", compiled.path)
    var doc any
    if err := json.Unmarshal(body, &doc); err != nil {
        return []Result{{
            Type:     typeTag,
            Expected: "valid JSON",
            Actual:   "response body is not JSON",
            Passed:   false,
        }}
    }
    err := compiled.sch.Validate(doc)
    if err == nil {
        return []Result{{
            Type:     typeTag,
            Expected: "valid",
            Actual:   "valid",
            Passed:   true,
        }}
    }
    var verr *jsonschema.ValidationError
    if !errors.As(err, &verr) {
        return []Result{{
            Type:     typeTag,
            Expected: "valid",
            Actual:   err.Error(),
            Passed:   false,
        }}
    }
    var results []Result
    for _, leaf := range flattenLeafErrors(verr) {
        results = append(results, leafToResult(leaf, doc))
    }
    if len(results) == 0 {
        results = append(results, Result{
            Type: typeTag, Expected: "valid",
            Actual: "validation failed", Passed: false,
        })
    }
    return results
}

// flattenLeafErrors walks ValidationError.Causes depth-first and yields
// nodes with no further Causes. Each returned node corresponds to a
// single failing schema keyword at a single instance location.
func flattenLeafErrors(e *jsonschema.ValidationError) []*jsonschema.ValidationError {
    if len(e.Causes) == 0 {
        return []*jsonschema.ValidationError{e}
    }
    var out []*jsonschema.ValidationError
    for _, c := range e.Causes {
        out = append(out, flattenLeafErrors(c)...)
    }
    return out
}

// leafToResult converts a single ValidationError leaf to an assertion.Result.
// For "type" errors it names the expected + actual type and value.
// For "required" errors it names the missing property.
// For all other kinds it falls back to the library's LocalizedString.
func leafToResult(leaf *jsonschema.ValidationError, root any) Result {
    path := toJSONPath(leaf.InstanceLocation)
    typeTag := fmt.Sprintf("schema %s", path)

    switch k := leaf.ErrorKind.(type) {
    case *kind.Type:
        val, _ := lookupAt(root, leaf.InstanceLocation)
        return Result{
            Type:     typeTag,
            Expected: fmt.Sprintf("type %s", strings.Join(k.Want, " or ")),
            Actual:   fmt.Sprintf("%s (%v)", k.Got, val),
            Passed:   false,
        }
    case *kind.Required:
        // Required reports the missing property names as a slice.
        missing := strings.Join(k.Missing, ", ")
        return Result{
            Type:     typeTag,
            Expected: fmt.Sprintf("required: %s", missing),
            Actual:   "missing",
            Passed:   false,
        }
    default:
        val, found := lookupAt(root, leaf.InstanceLocation)
        actual := "missing"
        if found {
            actual = fmt.Sprintf("%v", val)
        }
        return Result{
            Type:     typeTag,
            Expected: leaf.ErrorKind.LocalizedString(nil),
            Actual:   actual,
            Passed:   false,
        }
    }
}

// toJSONPath converts a ValidationError.InstanceLocation slice into a
// JSONPath-style string. Numeric tokens become "[N]" (array index),
// all other tokens become ".key". Empty input yields "$".
func toJSONPath(loc []string) string {
    var b strings.Builder
    b.WriteByte('$')
    for _, tok := range loc {
        if _, err := strconv.Atoi(tok); err == nil {
            b.WriteByte('[')
            b.WriteString(tok)
            b.WriteByte(']')
            continue
        }
        b.WriteByte('.')
        b.WriteString(tok)
    }
    return b.String()
}

// lookupAt walks root by the given InstanceLocation tokens and returns
// the value at that position, or (nil, false) when the path does not
// resolve (e.g. the error is about a missing required field).
func lookupAt(root any, loc []string) (any, bool) {
    cur := root
    for _, tok := range loc {
        switch v := cur.(type) {
        case map[string]any:
            next, ok := v[tok]
            if !ok {
                return nil, false
            }
            cur = next
        case []any:
            idx, err := strconv.Atoi(tok)
            if err != nil || idx < 0 || idx >= len(v) {
                return nil, false
            }
            cur = v[idx]
        default:
            return nil, false
        }
    }
    return cur, true
}
```

#### Tests to Write FIRST (RED phase)

`internal/assertion/schema_test.go`:

```go
func TestCompileSchemaFile(t *testing.T) {
    tests := []struct {
        name    string
        write   string // contents; "" = don't create file
        wantErr error  // sentinel to check with errors.Is (nil = success)
    }{
        {"valid draft 2020-12", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`, nil},
        {"file missing", "", ErrSchemaFileNotFound},
        {"invalid json", `{not json`, ErrSchemaInvalid},
        {"invalid schema syntax", `{"type":"not-a-type"}`, ErrSchemaInvalid},
    }
    for _, tt := range tests { /* write to t.TempDir(), call, assert */ }
}

func TestCheckSchema(t *testing.T) {
    schemaJSON := `{
        "$schema":"https://json-schema.org/draft/2020-12/schema",
        "type":"object",
        "required":["id","email"],
        "properties":{
            "id":{"type":"integer"},
            "email":{"type":"string"}
        }
    }`
    compiled := mustCompile(t, schemaJSON)

    tests := []struct {
        name       string
        body       string
        wantPassed bool
        wantCount  int
        wantType   string // substring match against Result.Type
        wantExp    string // substring match against Result.Expected
        wantAct    string // substring match against Result.Actual
    }{
        {"valid body", `{"id":1,"email":"a@b"}`, true, 1, "schema ", "valid", "valid"},
        {"missing required email", `{"id":1}`, false, 1, "schema $", "required: email", "missing"},
        {"type mismatch id string for int", `{"id":"not-an-int","email":"a@b"}`, false, 1, "schema $.id", "type integer", "string"},
        {"body not JSON", `<html/>`, false, 1, "schema ", "valid JSON", "response body is not JSON"},
        {"empty body", ``, false, 1, "schema ", "valid JSON", "response body is not JSON"},
        {"nested array index", /* schema with items */, false, 1, "schema $.tags[0]", "type string", "number"},
    }
    for _, tt := range tests { /* run, assert Results */ }
}

func TestCheckSchema_nil_compiled(t *testing.T) {
    // nil compiled => nil results (no assertion configured)
    if got := CheckSchema(nil, []byte("{}")); got != nil {
        t.Errorf("expected nil results for nil compiled, got %v", got)
    }
}

func TestToJSONPath(t *testing.T) {
    tests := []struct {
        name string
        in   []string
        want string
    }{
        {"empty", nil, "$"},
        {"single key", []string{"id"}, "$.id"},
        {"nested key", []string{"user", "email"}, "$.user.email"},
        {"array index", []string{"tags", "0"}, "$.tags[0]"},
        {"mixed", []string{"users", "2", "name"}, "$.users[2].name"},
    }
    for _, tt := range tests { /* assert */ }
}

func TestFlattenLeafErrors_collects_all(t *testing.T) {
    // Validate a body with multiple violations and assert that
    // CheckSchema returns one Result per violation (not just the first).
    schemaJSON := `{
        "type":"object",
        "required":["a","b","c"],
        "properties":{"a":{"type":"integer"},"b":{"type":"integer"},"c":{"type":"integer"}}
    }`
    compiled := mustCompile(t, schemaJSON)
    results := CheckSchema(compiled, []byte(`{"a":"s","b":"s"}`))
    // Expect >= 3 failing results: two type errors + one required error
    failed := 0
    for _, r := range results {
        if !r.Passed { failed++ }
    }
    if failed < 3 {
        t.Errorf("want >= 3 failing results, got %d: %+v", failed, results)
    }
}
```

#### Impact on Existing Tests

- None. New file, new symbols.

---

### Step 4: Extend `assertion.Evaluate` / `EvalInput` to include schema

**Rationale:** Small, local extension — adds one field plus a passthrough
call. Necessary glue before the runner can pass compiled schemas.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | modify | add `Schema *CompiledSchema` to `EvalInput`, call `CheckSchema` inside `Evaluate` |
| `internal/assertion/assertion_test.go` | modify | add an `Evaluate` test case that includes schema |

#### Current Code

```go
type EvalInput struct {
    StatusCodes      []int
    ActualStatus     int
    BodyAssertions   []BodyInput
    Body             []byte
    HeaderAssertions []HeaderInput
    Headers          http.Header
    MaxDurationMs    int
    ActualDuration   time.Duration
}

func Evaluate(in EvalInput) *Results {
    var items []Result
    if r := CheckStatus(in.StatusCodes, in.ActualStatus); r != nil { ... }
    if headerResults := CheckHeaders(...); headerResults != nil { ... }
    if bodyResults := CheckBody(in.BodyAssertions, in.Body); bodyResults != nil { ... }
    if r := CheckTiming(...); r != nil { ... }
    ...
}
```

#### New Code

```go
type EvalInput struct {
    StatusCodes      []int
    ActualStatus     int
    BodyAssertions   []BodyInput
    Body             []byte
    HeaderAssertions []HeaderInput
    Headers          http.Header
    MaxDurationMs    int
    ActualDuration   time.Duration
    Schema           *CompiledSchema // nil = no schema assertion
}

func Evaluate(in EvalInput) *Results {
    var items []Result
    if r := CheckStatus(in.StatusCodes, in.ActualStatus); r != nil {
        items = append(items, *r)
    }
    if headerResults := CheckHeaders(in.HeaderAssertions, in.Headers); headerResults != nil {
        items = append(items, headerResults...)
    }
    if bodyResults := CheckBody(in.BodyAssertions, in.Body); bodyResults != nil {
        items = append(items, bodyResults...)
    }
    if schemaResults := CheckSchema(in.Schema, in.Body); schemaResults != nil {
        items = append(items, schemaResults...)
    }
    if r := CheckTiming(in.MaxDurationMs, in.ActualDuration); r != nil {
        items = append(items, *r)
    }
    if len(items) == 0 {
        return nil
    }
    passed := true
    for _, r := range items {
        if !r.Passed { passed = false; break }
    }
    return &Results{Items: items, Passed: passed}
}
```

#### Tests to Write FIRST

```go
func TestEvaluate_with_schema(t *testing.T) {
    compiled := mustCompile(t, `{"type":"object","required":["id"]}`)
    in := EvalInput{
        StatusCodes:  []int{200},
        ActualStatus: 200,
        Body:         []byte(`{}`),
        Schema:       compiled,
    }
    got := Evaluate(in)
    if got == nil {
        t.Fatal("expected non-nil results")
    }
    if got.Passed {
        t.Error("expected Passed=false due to missing required field")
    }
    // Must contain both the status pass and the schema fail
    sawStatus, sawSchema := false, false
    for _, r := range got.Items {
        if r.Type == "status" && r.Passed { sawStatus = true }
        if strings.HasPrefix(r.Type, "schema ") && !r.Passed { sawSchema = true }
    }
    if !sawStatus || !sawSchema {
        t.Errorf("missing expected result rows: %+v", got.Items)
    }
}

func TestEvaluate_schema_nil_no_body_no_results(t *testing.T) {
    // Evaluate with no assertions at all must still return nil.
    got := Evaluate(EvalInput{ActualStatus: 200})
    if got != nil {
        t.Errorf("expected nil, got %+v", got)
    }
}
```

#### Impact on Existing Tests

- None. Existing `TestEvaluate_*` tests leave `Schema` as zero value
  (nil) and go through the same code paths as before.

---

### Step 5: Parser — `schema:` field on Assertions, parse-time compile, gate

**Rationale:** Now we plug the loaded assertion into the parse tree.
The gate must fire before any schema file is opened (parallel to
`IncludeGate`), so we thread a new `ParseOptions.SchemaGate` through.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | add `Schema SchemaAssertion` field to `Assertions` struct |
| `internal/parser/parser.go` | modify | add `SchemaGate` to `ParseOptions`; resolve + compile + gate schemas; wire for main + external + included collections |
| `internal/parser/errors.go` | modify | add `ErrSchemaFileNotFound`, `ErrSchemaInvalid` sentinels |
| `internal/parser/parser_test.go` | modify | new tests for schema parse paths |
| `internal/parser/testdata/schemas/user.json` | create | fixture schema |
| `internal/parser/testdata/schemas/bad.json` | create | fixture invalid schema |
| `internal/parser/testdata/schema_valid.yaml` | create | collection referencing `schemas/user.json` |
| `internal/parser/testdata/schema_missing.yaml` | create | references `schemas/missing.json` |
| `internal/parser/testdata/schema_invalid.yaml` | create | references `schemas/bad.json` |

#### Current Code (collection.go)

```go
type Assertions struct {
    Status  StatusCodes      `yaml:"status,omitempty"`
    Headers HeaderAssertions `yaml:"headers,omitempty"`
    Body    BodyAssertions   `yaml:"body,omitempty"`
    Timing  TimingAssertion  `yaml:"timing,omitempty"`
}
```

#### New Code (collection.go)

```go
type Assertions struct {
    Status  StatusCodes      `yaml:"status,omitempty"`
    Headers HeaderAssertions `yaml:"headers,omitempty"`
    Body    BodyAssertions   `yaml:"body,omitempty"`
    Timing  TimingAssertion  `yaml:"timing,omitempty"`
    Schema  SchemaAssertion  `yaml:"schema,omitempty"`
}

// SchemaAssertion holds a reference to a JSON Schema file plus the
// pre-compiled schema handle. Path is the raw YAML value (as written,
// relative to the collection file). Compiled is populated during parse
// and is nil when no schema is configured.
type SchemaAssertion struct {
    Path     string                     // raw path as written in YAML
    Compiled *assertion.CompiledSchema  // nil unless parser resolved & compiled it
}

// UnmarshalYAML lets schema: accept a plain string scalar.
func (s *SchemaAssertion) UnmarshalYAML(value *yaml.Node) error {
    if value.Kind != yaml.ScalarNode {
        return fmt.Errorf("schema: expected string path, got %v", value.Kind)
    }
    s.Path = value.Value
    return nil
}
```

Note: this pulls `internal/assertion` into `internal/parser`. There is
currently **no** `parser → assertion` edge. We must verify there is no
`assertion → parser` edge (confirmed: `assertion.go` imports only
`internal/jsonpath`). Safe to add.

#### New Code (parser.go — ParseOptions & post-parse compilation)

```go
// ParseOptions configures collection parsing.
type ParseOptions struct {
    IncludeGate func() error
    // SchemaGate, when non-nil, is invoked exactly once before any
    // schema: file is opened if the collection references at least one
    // schema assertion. Behaviour mirrors IncludeGate.
    SchemaGate func() error
}
```

Inside `parseCollectionBytes` (after external references and GraphQL/WS
resolution, before return), add a new pass:

```go
// M3-004: compile schema: assertions once per unique path.
// Gate fires before any file is opened.
schemaCache := map[string]*assertion.CompiledSchema{}
var needsGate bool
for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
    for i := range *section {
        if (*section)[i].Assertions.Schema.Path != "" {
            needsGate = true
            break
        }
    }
    if needsGate { break }
}
if needsGate {
    if opts.SchemaGate != nil {
        if gateErr := opts.SchemaGate(); gateErr != nil {
            return nil, gateErr
        }
    }
    for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
        for i := range *section {
            ref := &(*section)[i].Assertions.Schema
            if ref.Path == "" { continue }
            absPath := ref.Path
            if !filepath.IsAbs(absPath) {
                absPath = filepath.Join(collectionDir, absPath)
            }
            absResolved, absErr := filepath.Abs(absPath)
            if absErr != nil {
                return nil, &apierrors.Structured{
                    Category: apierrors.CategoryParse,
                    FilePath: path,
                    Message:  fmt.Sprintf("request %q: resolving schema path %q: %s", (*section)[i].Name, ref.Path, absErr),
                    Inner:    absErr,
                }
            }
            if cached, ok := schemaCache[absResolved]; ok {
                ref.Compiled = cached
                col.ExternalFiles = append(col.ExternalFiles, absResolved)
                continue
            }
            compiled, compErr := assertion.CompileSchemaFile(absResolved)
            if compErr != nil {
                hint := "Check assertions.schema path is relative to the collection file"
                inner := ErrSchemaInvalid
                if errors.Is(compErr, assertion.ErrSchemaFileNotFound) {
                    inner = ErrSchemaFileNotFound
                    hint = "Create the file or fix the path (resolved against the collection's directory)"
                }
                return nil, &apierrors.Structured{
                    Category: apierrors.CategoryParse,
                    FilePath: path,
                    Message:  fmt.Sprintf("request %q: %s", (*section)[i].Name, compErr),
                    Hint:     hint,
                    Inner:    inner,
                }
            }
            schemaCache[absResolved] = compiled
            ref.Compiled = compiled
            col.ExternalFiles = append(col.ExternalFiles, absResolved)
        }
    }
}
```

The gate must also fire for included collections (M3-003). Thread
`opts.SchemaGate` through `resolveIncludes` / `includeContext` similarly
to `IncludeGate`, or call `parseCollectionBytes` inside the include
walker with the same `ParseOptions`. Simpler path: pass `opts` through
`resolveIncludes` so nested collections parse with the same gate. (Audit
`include.go` in Step 5.5 below.)

#### Files Sub-Modify (Step 5.5 — include plumbing)

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/include.go` | modify | accept `ParseOptions` and pass into nested parses so nested schemas also gate |
| `internal/parser/include_test.go` | modify (if needed) | update to reflect new signature |

> If `resolveIncludes` already re-parses children via
> `parseCollectionBytes` without `ParseOptions`, we need to add a variant
> that forwards them. Verify during Step 5.5 and patch minimally.

#### Current Code (errors.go)

```go
var (
    ErrFileNotFound          = errors.New("collection file not found")
    ...
    ErrIncludeNotFound       = errors.New("include file not found")
)
```

#### New Code (errors.go)

```go
var (
    ErrFileNotFound          = errors.New("collection file not found")
    ...
    ErrIncludeNotFound       = errors.New("include file not found")
    ErrSchemaFileNotFound    = errors.New("schema file not found")
    ErrSchemaInvalid         = errors.New("invalid JSON Schema")
)
```

#### Tests to Write FIRST (parser_test.go)

```go
func TestParseFile_schema_valid(t *testing.T) {
    col, err := ParseFile("testdata/schema_valid.yaml")
    if err != nil {
        t.Fatalf("ParseFile: %v", err)
    }
    got := col.Requests.Items[0].Assertions.Schema
    if got.Path != "schemas/user.json" {
        t.Errorf("Path = %q, want %q", got.Path, "schemas/user.json")
    }
    if got.Compiled == nil {
        t.Error("Compiled must be populated after ParseFile")
    }
}

func TestParseFile_schema_missing_file(t *testing.T) {
    _, err := ParseFile("testdata/schema_missing.yaml")
    if err == nil {
        t.Fatal("expected error")
    }
    if !errors.Is(err, ErrSchemaFileNotFound) {
        t.Errorf("want ErrSchemaFileNotFound, got %v", err)
    }
    var se *apierrors.Structured
    if !errors.As(err, &se) || se.Category != apierrors.CategoryParse {
        t.Errorf("want CategoryParse structured error, got %T", err)
    }
}

func TestParseFile_schema_invalid_syntax(t *testing.T) {
    _, err := ParseFile("testdata/schema_invalid.yaml")
    if err == nil {
        t.Fatal("expected error")
    }
    if !errors.Is(err, ErrSchemaInvalid) {
        t.Errorf("want ErrSchemaInvalid, got %v", err)
    }
}

func TestParseFile_schema_relative_to_collection_dir(t *testing.T) {
    // schema path must resolve to the directory of the YAML file,
    // not os.Getwd(). Run from a temp CWD to be explicit.
    cwd, _ := os.Getwd()
    t.Cleanup(func() { _ = os.Chdir(cwd) })
    if err := os.Chdir(t.TempDir()); err != nil {
        t.Fatal(err)
    }
    abs, _ := filepath.Abs(filepath.Join(cwd, "testdata/schema_valid.yaml"))
    col, err := ParseFile(abs)
    if err != nil {
        t.Fatalf("ParseFile: %v", err)
    }
    if col.Requests.Items[0].Assertions.Schema.Compiled == nil {
        t.Error("schema should have been resolved relative to collection file")
    }
}

func TestParseFile_schema_gate_below_professional(t *testing.T) {
    var called int
    gate := func() error {
        called++
        return &auth.GateError{/* ... */}
    }
    _, err := ParseFileWithOptions("testdata/schema_valid.yaml", ParseOptions{SchemaGate: gate})
    if err == nil {
        t.Fatal("expected gate error")
    }
    if called != 1 {
        t.Errorf("gate should fire exactly once, called=%d", called)
    }
    var ge *auth.GateError
    if !errors.As(err, &ge) {
        t.Errorf("want *auth.GateError, got %T", err)
    }
}

func TestParseFile_schema_gate_no_schema_not_called(t *testing.T) {
    // Collection without any schema: assertions must NOT invoke the gate.
    var called int
    gate := func() error { called++; return nil }
    _, err := ParseFileWithOptions("testdata/basic_collection.yaml", ParseOptions{SchemaGate: gate})
    if err != nil { t.Fatal(err) }
    if called != 0 {
        t.Errorf("gate should not fire for collections without schema: assertions, called=%d", called)
    }
}

func TestParseFile_schema_cache_reused(t *testing.T) {
    // Two requests referencing the same schema path must share one
    // *CompiledSchema (pointer equality) after parse.
    col, err := ParseFile("testdata/schema_shared.yaml")
    if err != nil { t.Fatal(err) }
    a := col.Requests.Items[0].Assertions.Schema.Compiled
    b := col.Requests.Items[1].Assertions.Schema.Compiled
    if a == nil || b == nil { t.Fatal("both must be compiled") }
    if a != b {
        t.Error("parser should cache compiled schemas per absolute path")
    }
}
```

Fixtures:

- `testdata/schemas/user.json`:
  ```json
  {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "required": ["id", "email"],
    "properties": {
      "id": { "type": "integer" },
      "email": { "type": "string" }
    }
  }
  ```
- `testdata/schemas/bad.json`: `{"type": "definitely-not-a-valid-type"}`
- `testdata/schema_valid.yaml`:
  ```yaml
  name: schema-valid
  requests:
    - name: get user
      request: { method: GET, url: http://example.com/u }
      assertions:
        schema: schemas/user.json
  ```
- `testdata/schema_shared.yaml`: two requests both referencing `schemas/user.json`.

#### Impact on Existing Tests

- Adding `SchemaGate` field to `ParseOptions` is additive; existing
  callers continue to pass `ParseOptions{IncludeGate: ...}` and leave
  `SchemaGate` nil (equivalent to "no gate"). No breakage.
- Existing parser tests that build `Assertions` literals don't set
  `Schema` and stay at the zero value. No breakage.
- `include.go` signature change (if needed) touches
  `include_*_test.go`; confirm during Step 5.5 and update locally.

---

### Step 6: Runner call-site wiring — pass schema to `Evaluate`

**Rationale:** Runner is the hot path and has three call sites for
`assertion.Evaluate`. Each gets the compiled schema from the parse tree.
No new state, just an extra field copied through.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | populate `EvalInput.Schema` in the 3 `Evaluate` call sites |
| `internal/parallel/executor.go` | modify | populate `EvalInput.Schema` in its 1 call site |
| `internal/websocket/executor.go` | modify (if applicable) | pass-through, see below |
| `internal/runner/runner_test.go` | modify | add an end-to-end test: schema pass / fail via a fake exec |

#### Current Code (representative site)

```go
ar := assertion.Evaluate(assertion.EvalInput{
    StatusCodes:      item.Assertions.Status.Codes,
    ActualStatus:     result.StatusCode,
    HeaderAssertions: requtil.ToHeaderInputs(item.Assertions.Headers.Items),
    Headers:          result.Headers,
    BodyAssertions:   requtil.ToBodyInputs(item.Assertions.Body.Items),
    Body:             result.Body,
    MaxDurationMs:    item.Assertions.Timing.MaxDurationMs,
    ActualDuration:   result.Duration,
})
```

#### New Code

```go
ar := assertion.Evaluate(assertion.EvalInput{
    StatusCodes:      item.Assertions.Status.Codes,
    ActualStatus:     result.StatusCode,
    HeaderAssertions: requtil.ToHeaderInputs(item.Assertions.Headers.Items),
    Headers:          result.Headers,
    BodyAssertions:   requtil.ToBodyInputs(item.Assertions.Body.Items),
    Body:             result.Body,
    MaxDurationMs:    item.Assertions.Timing.MaxDurationMs,
    ActualDuration:   result.Duration,
    Schema:           item.Assertions.Schema.Compiled,
})
```

(Same one-line addition in all 4 call sites.)

#### WebSocket note

`internal/websocket/executor.go` evaluates `ExpectAssertions` via
`CheckBody` directly and does not currently call `assertion.Evaluate`.
The task's behaviors only concern HTTP response bodies, so WebSocket
steps are out of scope — we do **not** apply schema validation to
WebSocket messages in this slice. Add a brief code comment noting this.

#### Tests to Write FIRST (runner_test.go)

```go
func TestRun_schema_pass(t *testing.T) {
    // Use a fake exec that returns a valid JSON body; load a collection
    // with a schema: assertion and assert summary.Passed == 1 and
    // AssertionFailures == 0.
}

func TestRun_schema_fail(t *testing.T) {
    // Fake exec returns `{"id":"not-an-int"}`; load collection;
    // assert AssertionFailures >= 1 and a result row with
    // Type "schema $.id".
}

func TestRun_schema_gate_free_tier(t *testing.T) {
    // Use a fake `includeGateFor`-style ParseOptions with a SchemaGate
    // that returns auth.GateError. Call ParseFileWithOptions directly
    // (mirrors the cmd layer) and assert the error chain resolves to
    // *auth.GateError with Feature "schema_validation".
}
```

#### Impact on Existing Tests

- No assertion behaviour changes for collections without `schema:`.
  Existing runner tests keep passing unchanged.

---

### Step 7: CLI wiring — `schemaGateFor` and pass into parser

**Rationale:** Final user-visible surface. Mirrors `includeGateFor`
exactly, so it's a low-risk 1:1 pattern copy.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | add `schemaGateFor(tier)`; pass into both `ParseFileWithOptions` call sites |

#### Current Code

```go
func includeGateFor(tier auth.Tier) func() error {
    reg := auth.DefaultRegistry()
    return func() error {
        return auth.CheckFeature(reg, "include_directive", tier)
    }
}
...
col, err := parser.ParseFileWithOptions(file, parser.ParseOptions{
    IncludeGate: includeGateFor(currentTier()),
})
...
ParseOpts: parser.ParseOptions{IncludeGate: includeGateFor(currentTier())},
```

#### New Code

```go
func schemaGateFor(tier auth.Tier) func() error {
    reg := auth.DefaultRegistry()
    return func() error {
        return auth.CheckFeature(reg, "schema_validation", tier)
    }
}
...
col, err := parser.ParseFileWithOptions(file, parser.ParseOptions{
    IncludeGate: includeGateFor(currentTier()),
    SchemaGate:  schemaGateFor(currentTier()),
})
...
ParseOpts: parser.ParseOptions{
    IncludeGate: includeGateFor(currentTier()),
    SchemaGate:  schemaGateFor(currentTier()),
},
```

The existing error-handling block (`errors.As(err, &ge)` → exit code 6 +
`writeGateForFormat`) already handles the new gate without change: the
gate returns `*auth.GateError{Result.Feature: "schema_validation"}`, and
the output helpers read `Feature` from the wrapped result.

#### Tests to Write FIRST

No new test. Verified end-to-end by the observable smoke scenario and
by `TestRun_schema_gate_free_tier` in Step 6.

#### Impact on Existing Tests

- None. `validate` and `watch` command tests that construct
  `ParseOptions{}` directly continue to work (zero value `SchemaGate`
  means no gating). Any test that uses `parser.ParseFileWithOptions` with
  fixtures lacking `schema:` assertions is unaffected.

---

### Step 8: Collection JSON Schema (`internal/schema/collection.json`)

**Rationale:** Required by the task's scope bullet. Adding a single
optional property keeps the validate command's schema in sync with the
YAML surface. No code change beyond the JSON edit.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/collection.json` | modify | add `schema` property under `assertions` |

#### Current Code

```json
"assertions": {
  "type": "object",
  "properties": {
    "status": { ... },
    "headers": { ... },
    "body": { ... },
    "timing": { ... }
  },
  "additionalProperties": false
}
```

#### New Code

```json
"assertions": {
  "type": "object",
  "properties": {
    "status": { ... },
    "headers": { ... },
    "body": { ... },
    "timing": { ... },
    "schema": {
      "type": "string",
      "description": "Path to a JSON Schema (Draft 2020-12) file, resolved relative to the collection file. Professional tier."
    }
  },
  "additionalProperties": false
}
```

#### Tests to Write FIRST

```go
// internal/schema/schema_test.go — add assertion that the embedded
// schema contains assertions.schema as a string property.
func TestCollectionSchema_contains_assertions_schema(t *testing.T) {
    var doc map[string]any
    if err := json.Unmarshal(CollectionSchema, &doc); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    // Walk $defs.assertions.properties.schema and assert type == string.
    // ...
}
```

#### Impact on Existing Tests

- Any existing `validate`/schema tests that enforce
  `additionalProperties: false` under `assertions` now accept `schema`.
  If a negative test asserted that `schema:` was rejected, it would need
  updating — none exists today (`grep -n schema management/parser_test.go`
  returns nothing).

---

### Step 9: Smoke test update

**Rationale:** Locks in the observable verification (Section below).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | add a block that runs a temporary schema scenario and asserts exit codes |
| `sample/schema/user.json` | create | shared sample schema |
| `sample/schema/collection.yaml` | create | sample collection using the schema |

The smoke block should:

1. Start a local test server (using `python3 -m http.server` with a JSON
   fixture, or an ad-hoc Go helper embedded in a tiny `go run` script,
   whichever is already the smoke idiom).
2. Run with `CURLEW_TIER=professional` and assert exit 0.
3. Corrupt the fixture (or use a `schema_fail.yaml` pointing at an
   endpoint returning the bad body) and assert exit 1 with stderr
   containing `$.id`.
4. Run with `CURLEW_TIER=free` on the valid scenario and assert exit 6
   and stderr containing `schema_validation`.

Inspect the existing smoke script for the test-server pattern and match
it. If the current smoke script has no test server at all, scope the
smoke update to a dry-run that only exercises the parse-time gate
(Free tier exit 6) and leave the HTTP-backed verification for manual
observable verification. Decision: **scope to the parse-time gate
check** in smoke to keep the script self-contained and stable; the full
HTTP scenario is documented in the Observable section below as the
manual verification.

#### Impact on Existing Tests

- None.

---

### Step 10: Docs and changelog

**Rationale:** Task DoD requires "help text updated (if user-facing)".
Keep the user-facing surface discoverable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | add M3-004 entry under Unreleased |
| `docs/SPECIFICATION.md` | modify (if it documents assertions) | add `schema:` reference |

Check both files in Step 10; if SPECIFICATION.md does not enumerate
assertion operators, skip that edit.

#### Impact on Existing Tests

- None.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/registry_test.go` | new `TestDefaultRegistry_schema_validation` | add | new |
| `internal/assertion/schema_test.go` | new | add | create file + ~6 tests |
| `internal/assertion/assertion_test.go` | existing | none | add 2 new `Evaluate`-level tests with schema |
| `internal/parser/parser_test.go` | existing | none | add ~7 new schema-specific tests |
| `internal/parser/include_*_test.go` | existing | possible signature change | re-check after Step 5.5; update if `resolveIncludes` signature changes |
| `internal/runner/runner_test.go` | existing | none | add 3 new end-to-end schema tests |
| `internal/parallel/executor_test.go` | existing | none | parallel path gets a test that a schema failure in a parallel wave is surfaced |
| `internal/schema/schema_test.go` | existing | none | add `TestCollectionSchema_contains_assertions_schema` |
| `cmd/curlew/main_test.go` | existing (if present) | none | no change; CLI behaviour covered by gate tests |

Total tests added: ~18. Total files created: 4 assertion source/test + 3 parser testdata fixtures + 2 sample files = 9. Files modified: 8.

## Risks and Edge Cases

- **Risk: import cycle parser ↔ assertion.** `parser` does not currently
  import `assertion`; `assertion` imports only `jsonpath`. Adding
  `parser → assertion` is safe as long as `assertion` never imports
  `parser`. **Mitigation:** Step 3 keeps `assertion/schema.go` free of
  any `parser` symbol; lint will catch a regression.

- **Risk: `$schema` in the collection JSON Schema collides with `schema`
  field.** The outer envelope key `$schema` is a meta-keyword; the new
  property is `schema` (no `$`). No collision.

- **Edge case: `schema:` present but empty string.** YAML `schema: ""`
  should be treated as "not set". **Handling:** The resolve loop already
  checks `if ref.Path == "" { continue }`.

- **Edge case: schema file is an empty file or pure whitespace.**
  `jsonschema.UnmarshalJSON` returns an error, which we map to
  `ErrSchemaInvalid`. Covered by the invalid-syntax test case.

- **Edge case: schema with `$ref` to another file.** The library will
  try to load the referenced file via the default loader. If the ref is
  relative to the schema file, this works. If it's a URL, it will try to
  fetch it — which breaks our "fail-fast at parse" contract. **Mitigation:**
  Out of scope for this slice. Document the limitation in the schema
  assertion's description text. (A future task can install a restricted
  `URLLoader`.)

- **Edge case: response body is an empty byte slice (HEAD request, 204).**
  `json.Unmarshal([]byte{}, &doc)` returns an error → one failing result
  "response body is not JSON". This matches the task's "not JSON"
  behavior; users who want to skip schema validation on empty bodies can
  omit the `schema:` assertion on those requests.

- **Edge case: response body is `null`.** Valid JSON. The schema decides
  whether `null` is acceptable; the library handles it correctly.

- **Edge case: very large response.** The library validates against the
  already-decoded `any` tree. Memory usage is bounded by the body size.
  No additional guard needed beyond existing request limits.

- **Edge case: duplicate schema paths across requests.** Step 5's
  per-collection cache ensures each unique absolute path compiles once.
  Verified by `TestParseFile_schema_cache_reused`.

- **Risk: `kind.Required.Missing` field name.** The plan assumes the
  library exposes missing property names on `*kind.Required`. If the
  actual field is named differently, swap the case arm at Step 3 to use
  `leaf.ErrorKind.LocalizedString(nil)` (fallback path) — one-line fix
  discovered during RED-phase implementation. The failing test case
  remains valid because the fallback still produces a non-empty
  `Expected` containing "required" substring (which the test asserts as
  a substring, not an exact match).

- **Edge case: `testdata/basic_collection.yaml` may not exist in
  parser/testdata.** Step 5's gate-not-called test needs any schema-less
  fixture. **Mitigation:** Use any existing fixture
  (`grep -l "name:" internal/parser/testdata/*.yaml`) or add a tiny new
  one.

## Proposed Go Function Signatures

```go
// internal/assertion/schema.go
var ErrSchemaFileNotFound = errors.New("schema file not found")
var ErrSchemaInvalid      = errors.New("invalid JSON Schema")

type CompiledSchema struct { /* unexported fields */ }
func (c *CompiledSchema) Path() string

func CompileSchemaFile(absPath string) (*CompiledSchema, error)
func CheckSchema(compiled *CompiledSchema, body []byte) []Result
// (internal) toJSONPath(loc []string) string
// (internal) flattenLeafErrors(e *jsonschema.ValidationError) []*jsonschema.ValidationError
// (internal) leafToResult(leaf *jsonschema.ValidationError, root any) Result
// (internal) lookupAt(root any, loc []string) (any, bool)

// internal/assertion/assertion.go
type EvalInput struct {
    // ...existing fields...
    Schema *CompiledSchema
}

// internal/parser/collection.go
type Assertions struct {
    // ...existing fields...
    Schema SchemaAssertion `yaml:"schema,omitempty"`
}
type SchemaAssertion struct {
    Path     string
    Compiled *assertion.CompiledSchema
}
func (s *SchemaAssertion) UnmarshalYAML(value *yaml.Node) error

// internal/parser/parser.go
type ParseOptions struct {
    IncludeGate func() error
    SchemaGate  func() error
}

// internal/parser/errors.go
var ErrSchemaFileNotFound = errors.New("schema file not found")
var ErrSchemaInvalid      = errors.New("invalid JSON Schema")

// internal/auth/registry.go
// (no signature change, registers new FeatureDefinition)

// cmd/curlew/main.go
func schemaGateFor(tier auth.Tier) func() error
```

All new exported symbols will get doc comments per the project's Go
standards.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (manual, per task YAML):

```bash
# 1. Valid schema + valid body at Professional tier (exit 0)
mkdir -p /tmp/m3-004 && cd /tmp/m3-004
cat > schemas/user.json <<'EOF'
{"$schema":"https://json-schema.org/draft/2020-12/schema",
 "type":"object","required":["id","email"],
 "properties":{"id":{"type":"integer"},"email":{"type":"string"}}}
EOF
# (spin up a trivial local server returning {"id":1,"email":"a@b"})
cat > collection.yaml <<'EOF'
name: schema-check
requests:
  - name: get user
    request: { method: GET, url: http://127.0.0.1:8080/user }
    assertions:
      status: 200
      schema: schemas/user.json
EOF
CURLEW_TIER=professional curlew run collection.yaml   # expect exit 0

# 2. Same but server returns {"id":"not-an-int"} — expect exit 1,
#    stderr contains $.id, "type integer", and "string".
CURLEW_TIER=professional curlew run collection.yaml

# 3. Free tier on the valid scenario — expect exit 6 and
#    "schema_validation" in the gate output.
CURLEW_TIER=free curlew run collection.yaml

# 4. Unit tests
go test ./internal/assertion/...
```
