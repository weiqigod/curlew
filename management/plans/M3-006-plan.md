# Implementation Plan: M3-006

## Overview
Extend the OpenAPI importer (`internal/openapi/`) to populate header/query parameters, request body placeholders, and status-code assertions — completing the collection skeleton started in M3-005.

## Task Details
- **ID:** M3-006
- **Title:** openapi import: headers, request bodies, and status assertions
- **Phase:** M3: Professional Tier
- **Priority:** 4
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M3-005 | openapi import: collection skeleton | done |

---

## Code Exploration Findings

### `internal/openapi/import.go` (current state)
`Import(specPath string) (*parser.Collection, error)` loads the spec via `kin-openapi`, validates it, then builds a `parser.Collection` with:
- `col.Name` from `doc.Info.Title`
- `col.Variables["base_url"]` from first server URL
- One `parser.RequestItem` per operation: `Name`, `Request.Method`, `Request.URL` (path params interpolated as `{{name}}`)

Line 24 comment explicitly says: _"bodies, headers, and assertions are deferred to M3-006"_.

### `internal/openapi/emit.go` (current state)
`writerCollection` → `writerRequestItem` → `writerRequest` only emit `method` and `url`. No headers, body, or assertions fields exist yet.

### `internal/parser/collection.go` (relevant fields)
```go
type Request struct {
    Method      string            `yaml:"method"`
    URL         string            `yaml:"url"`
    Headers     map[string]string `yaml:"headers,omitempty"`
    Body        any               `yaml:"body,omitempty"`
    QueryParams map[string]string `yaml:"query,omitempty"`
    // ...
}

type Assertions struct {
    Status  StatusCodes      `yaml:"status,omitempty"`
    // ...
}

// StatusCodes.UnmarshalYAML accepts both scalar 200 and sequence [200, 201]
type StatusCodes struct { Codes []int }
```

`parser.Request.Headers`, `Body`, and `RequestItem.Assertions.Status` are already defined — we only need to populate them in `import.go` and emit them in `emit.go`.

### Existing test fixtures (`internal/openapi/testdata/`)
`petstore.yaml`, `petstore_31.yaml`, `no_servers.yaml`, `no_operation_id.yaml`, `no_operations.yaml`, `invalid.yaml` — none have header params, request bodies, or multi-status responses.

### Top-level `testdata/openapi/`
Contains `petstore.yaml` (same minimal spec). Need to add `petstore-full.yaml`.

### No existing `$ref` resolution utilities
`kin-openapi` resolves all internal `$ref`s automatically when loading. If `ref.Value == nil` after loading, the ref is unresolvable. We only need cycle detection during _schema walking_ (when we recurse through a resolved schema to generate placeholders).

---

## Implementation Steps

Steps are ordered smallest-blast-radius first: new files before modifying existing ones; pure helpers before wiring; tests before implementation.

---

### Step 1: Create `testdata/openapi/petstore-full.yaml` fixture

**Rationale:** The integration test and observable verification both reference this file. Creating it first lets us write tests that fail (RED) before any code is written.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/openapi/petstore-full.yaml` | create | Rich OpenAPI 3.0 spec exercising all 8 behaviors |

#### New Code

```yaml
openapi: 3.0.3
info:
  title: Petstore Full
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
components:
  schemas:
    Pet:
      type: object
      properties:
        name:
          type: string
        tag:
          type: string
    NewPet:
      type: object
      required:
        - name
      properties:
        name:
          type: string
        tag:
          type: string
paths:
  /pets:
    get:
      operationId: listPets
      parameters:
        - name: X-API-Key
          in: header
          required: true
          schema: { type: string }
        - name: limit
          in: query
          schema: { type: integer }
      responses:
        '200':
          description: A list of pets
        '404':
          description: Not found
    post:
      operationId: createPet
      parameters:
        - name: X-API-Key
          in: header
          required: true
          schema: { type: string }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/NewPet'
      responses:
        '201':
          description: Created
        '404':
          description: Not found
  /pets/{petId}:
    get:
      operationId: showPetById
      parameters:
        - name: petId
          in: path
          required: true
          schema: { type: string }
        - name: X-API-Key
          in: header
          required: true
          schema: { type: string }
      responses:
        '200':
          description: A single pet
        '404':
          description: Not found
```

#### Tests to Write FIRST (RED phase)
No Go tests in this step — the file itself is the test fixture consumed in later steps.

#### Impact on Existing Tests
None.

---

### Step 2: Add `snakeCase` and `extractStatusCodes` helpers

**Rationale:** Pure, side-effect-free functions with no dependencies on other new code. Easy to TDD in isolation before wiring into the main import loop.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/import.go` | modify | Add `snakeCase` and `extractStatusCodes` functions |
| `internal/openapi/import_test.go` | modify | Add `TestSnakeCase` and `TestExtractStatusCodes` |

#### Current Code
```go
// end of file after synthName (line 149)
```

#### New Code
```go
// snakeCase converts an arbitrary parameter name into a snake_case variable
// name suitable for collection variable placeholders.
// Examples: "X-API-Key" → "x_api_key", "userId" → "user_id", "limit" → "limit"
func snakeCase(s string) string {
    var b strings.Builder
    for i, r := range s {
        switch {
        case r == '-' || r == '.' || r == ' ':
            b.WriteRune('_')
        case unicode.IsUpper(r) && i > 0:
            b.WriteRune('_')
            b.WriteRune(unicode.ToLower(r))
        default:
            b.WriteRune(unicode.ToLower(r))
        }
    }
    // Collapse multiple underscores and trim leading/trailing.
    result := strings.Trim(reMultiUnderscore.ReplaceAllString(b.String(), "_"), "_")
    if result == "" {
        return "param"
    }
    return result
}

var reMultiUnderscore = regexp.MustCompile(`_+`)

// extractStatusCodes returns the sorted numeric HTTP status codes from an
// operation's responses map. Non-numeric keys like "default" and pattern keys
// like "2XX" are ignored.
func extractStatusCodes(responses *openapi3.Responses) []int {
    if responses == nil {
        return nil
    }
    var codes []int
    for key := range responses.Map() {
        code, err := strconv.Atoi(key)
        if err != nil {
            continue // skip "default", "2XX", etc.
        }
        codes = append(codes, code)
    }
    sort.Ints(codes)
    return codes
}
```

Add to imports: `"strconv"`, `"unicode"`.

#### Tests to Write FIRST (RED phase)

```go
func TestSnakeCase(t *testing.T) {
    tests := []struct{ in, want string }{
        {"limit", "limit"},
        {"X-API-Key", "x_api_key"},
        {"userId", "user_id"},
        {"Content-Type", "content_type"},
        {"x.forwarded.for", "x_forwarded_for"},
        {"already_snake", "already_snake"},
        {"HTTPMethod", "h_t_t_p_method"}, // uppercase run: each upper gets _
        {"", "param"},
    }
    for _, tt := range tests {
        t.Run(tt.in, func(t *testing.T) {
            if got := snakeCase(tt.in); got != tt.want {
                t.Errorf("snakeCase(%q) = %q, want %q", tt.in, got, tt.want)
            }
        })
    }
}

func TestExtractStatusCodes(t *testing.T) {
    tests := []struct {
        name  string
        keys  []string
        want  []int
    }{
        {"single", []string{"200"}, []int{200}},
        {"multiple sorted", []string{"404", "200", "201"}, []int{200, 201, 404}},
        {"skips default", []string{"200", "default"}, []int{200}},
        {"skips pattern", []string{"200", "2XX"}, []int{200}},
        {"nil responses", nil, nil},
    }
    // ... build openapi3.Responses from keys and call extractStatusCodes
}
```

#### Impact on Existing Tests
None — new functions only.

---

### Step 3: Create `internal/openapi/schema.go` with `schemaWalker`

**Rationale:** New file, zero changes to existing code. The schema walker is the most complex piece and must exist before body generation (Step 5) can be wired in.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/schema.go` | create | `schemaWalker` type and `placeholder` method |
| `internal/openapi/schema_test.go` | create | `TestWalkSchema` table-driven test |

#### New Code (`schema.go`)

```go
package openapi

import (
    "fmt"
    "sort"

    "github.com/getkin/kin-openapi/openapi3"
)

// schemaWalker generates placeholder values from resolved OpenAPI 3 schemas.
// It detects $ref cycles via a visited-set passed through recursion and emits
// a warning (via warn) on first occurrence of each cycle.
type schemaWalker struct {
    warn       func(string)
    warnedRefs map[string]bool
}

// placeholder returns a Go value suitable for YAML serialisation that represents
// a plausible placeholder for the given schema. It prefers example values when
// present, otherwise synthesises type-appropriate defaults.
//
// visited is the set of $ref names seen on the current recursion path; pass nil
// for the root call. It is cloned before descent so sibling branches do not
// pollute each other's cycle sets.
func (w *schemaWalker) placeholder(ref *openapi3.SchemaRef, visited map[string]bool) any {
    if ref == nil || ref.Value == nil {
        return "string"
    }

    // Cycle detection: if we've already descended through this named ref on the
    // current path, stop and return a placeholder.
    refName := ref.Ref
    if refName != "" {
        if visited[refName] {
            msg := fmt.Sprintf("recursive $ref detected: %s — using placeholder", refName)
            if !w.warnedRefs[refName] {
                w.warnedRefs[refName] = true
                w.warn(msg)
            }
            return "string"
        }
        // Clone visited before descending so sibling branches are independent.
        next := make(map[string]bool, len(visited)+1)
        for k, v := range visited {
            next[k] = v
        }
        next[refName] = true
        visited = next
    }

    s := ref.Value

    // Prefer explicit example on the schema.
    if s.Example != nil {
        return s.Example
    }
    // Prefer first enum value.
    if len(s.Enum) > 0 {
        return s.Enum[0]
    }

    // allOf: merge all object sub-schemas (non-objects are ignored).
    if len(s.AllOf) > 0 {
        merged := map[string]any{}
        for _, sub := range s.AllOf {
            if v, ok := w.placeholder(sub, visited).(map[string]any); ok {
                for k, val := range v {
                    merged[k] = val
                }
            }
        }
        if len(merged) > 0 {
            return merged
        }
    }

    // oneOf/anyOf: use first branch.
    if len(s.OneOf) > 0 {
        return w.placeholder(s.OneOf[0], visited)
    }
    if len(s.AnyOf) > 0 {
        return w.placeholder(s.AnyOf[0], visited)
    }

    // Type-based dispatch.
    switch {
    case s.Type.Is("string"):
        return defaultForStringFormat(s.Format)
    case s.Type.Is("integer") || s.Type.Is("number"):
        return 0
    case s.Type.Is("boolean"):
        return false
    case s.Type.Is("array"):
        if s.Items != nil {
            return []any{w.placeholder(s.Items, visited)}
        }
        return []any{}
    default: // object (or untyped)
        if len(s.Properties) == 0 {
            return map[string]any{}
        }
        obj := make(map[string]any, len(s.Properties))
        for _, k := range sortedKeys(s.Properties) {
            obj[k] = w.placeholder(s.Properties[k], visited)
        }
        return obj
    }
}

// defaultForStringFormat returns a format-appropriate placeholder string.
func defaultForStringFormat(format string) string {
    switch format {
    case "date-time":
        return "2006-01-02T15:04:05Z"
    case "date":
        return "2006-01-02"
    case "uuid":
        return "00000000-0000-0000-0000-000000000000"
    case "email":
        return "user@example.com"
    case "uri":
        return "https://example.com"
    default:
        return "string"
    }
}

// sortedKeys returns the keys of a Properties map in lexicographic order.
func sortedKeys(m openapi3.Schemas) []string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    return keys
}
```

#### Tests to Write FIRST (RED phase) — `schema_test.go`

Table-driven `TestWalkSchema` with rows:

| Row name | Input | Want |
|---|---|---|
| `nil_ref` | `nil` | `"string"` |
| `nil_value` | `&SchemaRef{}` | `"string"` |
| `type_string` | `{type: string}` | `"string"` |
| `type_integer` | `{type: integer}` | `0` |
| `type_number` | `{type: number}` | `0` |
| `type_boolean` | `{type: boolean}` | `false` |
| `format_date_time` | `{type: string, format: date-time}` | `"2006-01-02T15:04:05Z"` |
| `format_uuid` | `{type: string, format: uuid}` | `"00000000-..."` |
| `format_email` | `{type: string, format: email}` | `"user@example.com"` |
| `object_one_field` | `{type: object, properties: {name: {type: string}}}` | `{"name": "string"}` |
| `object_sorted_keys` | `{properties: {z: str, a: str}}` | `{"a": "string", "z": "string"}` |
| `array_of_string` | `{type: array, items: {type: string}}` | `["string"]` |
| `nested_object` | `{type: object, properties: {pet: {type: object, properties: {name: {type: string}}}}}` | `{"pet": {"name": "string"}}` |
| `example_wins` | `{type: string, example: "Fluffy"}` | `"Fluffy"` |
| `enum_wins_over_type` | `{type: string, enum: ["a", "b"]}` | `"a"` |
| `cycle_terminates` | named ref pointing to itself | `"string"` + warn emitted |
| `siblings_no_cycle` | two siblings both referencing same non-recursive ref | both resolve, no warning |
| `allof_merges` | `{allOf: [{properties: {a: str}}, {properties: {b: int}}]}` | `{"a": "string", "b": 0}` |
| `oneof_picks_first` | `{oneOf: [{type: string}, {type: integer}]}` | `"string"` |

#### Impact on Existing Tests
None — new file only.

---

### Step 4: Add `importWithWarn` test seam and header/query parameter extraction

**Rationale:** Refactoring `Import` to an inner `importWithWarn` before adding features keeps the test seam clean. Header and query extraction belong together since both iterate `op.Parameters`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/import.go` | modify | Refactor to `importWithWarn`; add `collectParameters`, `stderrWarn` |
| `internal/openapi/import_test.go` | modify | Add `TestImport_HeaderParameters`, `TestImport_QueryParameters` |

#### Current Code (import.go lines 25-107)
```go
func Import(specPath string) (*parser.Collection, error) {
    loader := openapi3.NewLoader()
    // ... full body ...
    item := parser.RequestItem{
        Name: chooseName(op, method, path, used),
        Request: parser.Request{
            Method: method,
            URL:    "{{base_url}}" + interpolatePath(path),
        },
    }
    // ...
}
```

#### New Code

```go
// stderrWarn writes a warning message to os.Stderr.
func stderrWarn(msg string) {
    fmt.Fprintln(os.Stderr, "warning:", msg)
}

// Import parses an OpenAPI 3.0 or 3.1 spec file and returns a collection with
// headers, request bodies, and status-code assertions derived from the spec.
func Import(specPath string) (*parser.Collection, error) {
    return importWithWarn(specPath, stderrWarn)
}

// importWithWarn is the testable core of Import; warn receives warning messages
// instead of writing to os.Stderr.
func importWithWarn(specPath string, warn func(string)) (*parser.Collection, error) {
    // ... same loader/validate/name/col setup ...
    
    walker := &schemaWalker{warn: warn, warnedRefs: map[string]bool{}}

    used := map[string]int{}
    collVarsSeen := map[string]bool{} // dedup across operations
    var items []parser.RequestItem
    for _, path := range paths {
        pi := doc.Paths.Find(path)
        // ...
        for _, method := range methodOrder {
            op, ok := ops[method]
            // ...
            url, headers, newVars := collectParameters(op, pi, "{{base_url}}"+interpolatePath(path), collVarsSeen)
            for k, v := range newVars {
                col.Variables.Values[k] = v
            }
            body := buildRequestBody(op, walker)
            statusCodes := extractStatusCodes(op.Responses)
            
            item := parser.RequestItem{
                Name: chooseName(op, method, path, used),
                Request: parser.Request{
                    Method:  method,
                    URL:     url,
                    Headers: headers,
                    Body:    body,
                },
            }
            if len(statusCodes) > 0 {
                item.Assertions.Status = parser.StatusCodes{Codes: statusCodes}
            }
            items = append(items, item)
        }
    }
    // ...
}

// collectParameters extracts header and query parameters from an operation
// (merging path-level and operation-level params, operation wins on conflict).
// Returns the (possibly query-appended) URL, request headers map, and new
// collection variables to add. collVarsSeen tracks already-added variable names
// so shared params across operations become a single collection variable.
func collectParameters(
    op *openapi3.Operation,
    pi *openapi3.PathItem,
    baseURL string,
    collVarsSeen map[string]bool,
) (url string, headers map[string]string, newVars map[string]string) {
    // Merge path-level params with operation-level (operation overrides).
    merged := map[string]*openapi3.Parameter{}
    for _, ref := range pi.Parameters {
        if ref != nil && ref.Value != nil {
            merged[ref.Value.Name] = ref.Value
        }
    }
    for _, ref := range op.Parameters {
        if ref != nil && ref.Value != nil {
            merged[ref.Value.Name] = ref.Value
        }
    }

    headers = map[string]string{}
    newVars = map[string]string{}
    var queryVars []string

    // Sort for determinism.
    var names []string
    for n := range merged {
        names = append(names, n)
    }
    sort.Strings(names)

    for _, name := range names {
        p := merged[name]
        switch p.In {
        case "header":
            varName := snakeCase(name)
            headers[name] = "{{" + varName + "}}"
            if !collVarsSeen[varName] {
                collVarsSeen[varName] = true
                newVars[varName] = ""
            }
        case "query":
            varName := snakeCase(name)
            queryVars = append(queryVars, name+"={{"+varName+"}}")
            if !collVarsSeen[varName] {
                collVarsSeen[varName] = true
                newVars[varName] = ""
            }
        }
        // "path" and "cookie" params are ignored for M3-006.
    }

    url = baseURL
    if len(queryVars) > 0 {
        sort.Strings(queryVars) // deterministic order
        url += "?" + strings.Join(queryVars, "&")
    }
    return url, headers, newVars
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestImport_HeaderParameters(t *testing.T) {
    // Uses petstore-full.yaml — all 3 ops share X-API-Key
    col, err := Import("testdata/petstore-full.yaml")  // path relative to package
    // But this is in internal/openapi, so testdata relative to that package?
    // No — we need a fixture in internal/openapi/testdata/ or use importWithWarn directly.
    // Use a small inline spec via a temp file helper.
    tests := []struct {
        name       string
        spec       string
        wantHeader string
        wantVar    string
    }{
        {"single required header", inlineSpecWithHeader("X-API-Key"), "x_api_key", ""},
        {"camel case header", inlineSpecWithHeader("ContentType"), "content_type", ""},
        {"dedup across ops", inlineSpecWithTwoOpsSharedHeader(), "x_api_key once in vars", ""},
    }
    // ...
}
```

Test helper: `writeSpecToTemp(t, yamlContent string) string` — writes to `t.TempDir()`, returns path.

| Row name | Description |
|---|---|
| `single_required_header` | one op, one required header → `headers: {X-API-Key: '{{x_api_key}}'}` + var added |
| `multiple_sorted` | one op, two headers `A-Header` and `Z-Header` → sorted in output |
| `shared_across_ops` | two ops both have `X-API-Key` → var appears once in collection |
| `camel_case_header` | `ContentType` → var `content_type` |
| `path_level_param_inherited` | header defined at path item level → appears in op |
| `single_query_param` | `?limit={{limit}}` in URL + var added |
| `multiple_query_sorted` | `?from={{from}}&limit={{limit}}` (alphabetical) |
| `shared_query_across_ops` | same query var across two ops → var added once |

#### Impact on Existing Tests
- `TestImport_Petstore` — `petstore.yaml` has no header/query params → `item.Request.Headers` will be empty map (or nil). Test checks specific fields by name, not deep equality. If `headers` is empty map vs nil, the YAML output changes. We ensure `collectParameters` returns `nil` (not empty map) when no header params. Actually, let's return `nil` when len(headers)==0 and len(queryVars)==0.
- All other existing tests: check Name/Method/URL only, not affected.

---

### Step 5: Add request body generation

**Rationale:** Depends on `schemaWalker` (Step 3) and the `importWithWarn` refactor (Step 4).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/import.go` | modify | Add `buildRequestBody` function |
| `internal/openapi/import_test.go` | modify | Add `TestImport_RequestBody_*` tests |

#### New Code

```go
// buildRequestBody generates a placeholder request body from the operation's
// requestBody definition. Returns nil if no requestBody is defined.
// Prefers explicit examples over schema-derived placeholders.
// Only application/json media type is used; others are ignored.
func buildRequestBody(op *openapi3.Operation, w *schemaWalker) any {
    if op.RequestBody == nil || op.RequestBody.Value == nil {
        return nil
    }
    content := op.RequestBody.Value.Content
    // Prefer application/json; fall back to first JSON-like media type.
    mt := content.Get("application/json")
    if mt == nil {
        for key, val := range content {
            if strings.Contains(key, "json") {
                mt = val
                break
            }
        }
    }
    if mt == nil || mt.Schema == nil {
        return nil
    }
    // Prefer explicit example on the MediaType.
    if mt.Example != nil {
        return mt.Example
    }
    // Prefer first named example.
    for _, ex := range mt.Examples {
        if ex.Value != nil {
            return ex.Value.Value
        }
    }
    // Walk schema.
    return w.placeholder(mt.Schema, nil)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestImport_RequestBody_FromSchema(t *testing.T) {
    tests := []struct {
        name     string
        specYAML string
        want     any
    }{
        {"primitive_string_field", specWithBody(`{type: object, properties: {name: {type: string}}}`), map[string]any{"name": "string"}},
        {"integer_field", ..., map[string]any{"count": 0}},
        {"nested_object", ..., map[string]any{"pet": map[string]any{"name": "string"}}},
        {"array_field", ..., map[string]any{"tags": []any{"string"}}},
        {"ref_to_components", ..., map[string]any{"name": "string", "tag": "string"}},
        {"no_body", specWithNoBody(), nil},
        {"non_json_media_type", specWithXMLBody(), nil},
    }
}

func TestImport_RequestBody_FromExample(t *testing.T) {
    tests := []struct {
        name     string
        specYAML string
        want     any
    }{
        {"media_type_example", specWithMediaTypeExample(`{name: Fluffy}`), map[string]any{"name": "Fluffy"}},
        {"named_examples_map", specWithNamedExamples(), /* first named example value */},
        {"example_beats_schema", ..., /* example wins over schema walk */},
    }
}
```

#### Impact on Existing Tests
None — `petstore.yaml` has no requestBody definitions, so `buildRequestBody` returns nil for all existing operations.

---

### Step 6: Add status code aggregation

**Rationale:** Simplest extension to the import loop. `extractStatusCodes` already exists from Step 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/import.go` | modify | Wire `extractStatusCodes` into the main loop |
| `internal/openapi/import_test.go` | modify | Add `TestImport_StatusAssertions` |

#### Current Code (main loop, item creation)
```go
item := parser.RequestItem{
    Name: chooseName(op, method, path, used),
    Request: parser.Request{
        Method: method,
        URL:    "{{base_url}}" + interpolatePath(path),
    },
}
```

#### New Code (already shown in Step 4 diff — adding status)
```go
if len(statusCodes) > 0 {
    item.Assertions.Status = parser.StatusCodes{Codes: statusCodes}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestImport_StatusAssertions(t *testing.T) {
    tests := []struct {
        name       string
        specYAML   string
        opName     string
        wantCodes  []int
    }{
        {"single_200", specWithResponses(`{'200': {description: OK}}`), "op", []int{200}},
        {"multi_sorted", specWithResponses(`{'404': ..., '200': ..., '201': ...}`), "op", []int{200, 201, 404}},
        {"skips_default", specWithResponses(`{'200': ..., 'default': ...}`), "op", []int{200}},
        {"skips_2xx_pattern", specWithResponses(`{'200': ..., '2XX': ...}`), "op", []int{200}},
        {"no_responses", specWithNoResponses(), "op", nil},
    }
}
```

#### Impact on Existing Tests
`TestImport_Petstore` — `listPets` has responses `{'200': ...}`, `createPet` has `{'201': ...}`, `showPetById` has `{'200': ...}`. The test only checks `Name`, `Method`, `URL` — not assertions — so no breakage.

---

### Step 7: Extend `emit.go` to output headers, body, and assertions

**Rationale:** Only after the import side populates these fields does it make sense to extend the emitter. Keeping import and emit in sync.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/emit.go` | modify | Extend writer structs with headers, body, assertions |
| `internal/openapi/emit_test.go` | modify | Extend `TestEmit_ContainsExpectedKeys`; add `TestEmit_FullFixture` |

#### Current Code
```go
type writerRequest struct {
    Method string `yaml:"method"`
    URL    string `yaml:"url"`
}

type writerRequestItem struct {
    Name    string        `yaml:"name"`
    Request writerRequest `yaml:"request"`
}

// In Emit():
out.Requests = append(out.Requests, writerRequestItem{
    Name: it.Name,
    Request: writerRequest{
        Method: it.Request.Method,
        URL:    it.Request.URL,
    },
})
```

#### New Code
```go
type writerAssertions struct {
    Status []int `yaml:"status,omitempty"`
}

type writerRequest struct {
    Method  string            `yaml:"method"`
    URL     string            `yaml:"url"`
    Headers map[string]string `yaml:"headers,omitempty"`
    Body    any               `yaml:"body,omitempty"`
}

type writerRequestItem struct {
    Name       string           `yaml:"name"`
    Request    writerRequest    `yaml:"request"`
    Assertions writerAssertions `yaml:"assertions,omitempty"`
}

// In Emit():
wi := writerRequestItem{
    Name: it.Name,
    Request: writerRequest{
        Method:  it.Request.Method,
        URL:     it.Request.URL,
        Headers: nilIfEmpty(it.Request.Headers),
        Body:    it.Request.Body,
    },
}
if len(it.Assertions.Status.Codes) > 0 {
    wi.Assertions = writerAssertions{Status: it.Assertions.Status.Codes}
}
out.Requests = append(out.Requests, wi)
```

Add helper:
```go
func nilIfEmpty[K comparable, V any](m map[K]V) map[K]V {
    if len(m) == 0 {
        return nil
    }
    return m
}
```

#### Tests to Write FIRST (RED phase)

Extend `TestEmit_ContainsExpectedKeys` to import `petstore-full.yaml` (adjust path) and check:
```go
wantSubstrings := []string{
    "X-API-Key: '{{x_api_key}}'",  // or without quotes
    "x_api_key: ''",
    "limit={{limit}}",
    "name: string",                 // body placeholder
    "status:",
    "- 200",
    "- 404",
}
```

Add `TestEmit_FullFixture`:
```go
func TestEmit_FullFixture(t *testing.T) {
    // Import petstore-full.yaml, emit to temp file,
    // re-parse with parser.ParseFile, validate with validator.Validate,
    // check specific fields on re-parsed collection.
}
```

#### Impact on Existing Tests
- `TestEmit_ContainsExpectedKeys` — still passes (existing substrings unchanged; new ones added for full fixture)
- `TestEmit_RoundTripsThroughParser` — imports `petstore.yaml` (no headers/body/assertions) → `writerRequest.Headers` will be nil, body nil, assertions empty → YAML unchanged → still round-trips cleanly. No breakage.
- `TestEmit_WriteError` — same path, unaffected.

---

### Step 8: Add cycle warning test (`TestImport_CycleWarningToStderr`)

**Rationale:** Verifying that cycle detection emits to stderr requires a test seam. Now that `importWithWarn` exists, this test can capture warnings via a custom `warn` function without OS pipe complexity.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/import_test.go` | modify | Add `TestImport_CycleWarning` using `importWithWarn` |
| `internal/openapi/schema_test.go` | modify | Add `TestWalkSchema_CycleWarn` to verify `warnedRefs` not repeated |

#### New Code

```go
func TestImport_CycleWarning(t *testing.T) {
    // Inline recursive spec: Pet.$ref → Pet
    specYAML := `... schema with recursive $ref ...`
    path := writeSpecToTemp(t, specYAML)

    var warnings []string
    col, err := importWithWarn(path, func(msg string) { warnings = append(warnings, msg) })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    _ = col
    found := false
    for _, w := range warnings {
        if strings.Contains(w, "recursive $ref") {
            found = true
            break
        }
    }
    if !found {
        t.Errorf("expected recursive $ref warning; got %v", warnings)
    }
    // Warn-once: same ref should not appear twice in warnings.
    count := 0
    for _, w := range warnings {
        if strings.Contains(w, "recursive $ref") {
            count++
        }
    }
    if count > 1 {
        t.Errorf("expected warn-once; got %d warnings for same ref", count)
    }
}
```

#### Impact on Existing Tests
None.

---

### Step 9: Full integration test `TestImport_FullFixture`

**Rationale:** Validates all 8 behaviors together against the real fixture. Runs last to confirm all wiring works end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/openapi/import_test.go` | modify | Add `TestImport_FullFixture` |

#### New Code

```go
func TestImport_FullFixture(t *testing.T) {
    // petstore-full.yaml is in internal/openapi/testdata/ — need to add it there too,
    // or use a relative path from the package directory.
    // Convention: internal/openapi tests reference internal/openapi/testdata/
    // So we also need testdata/petstore_full.yaml under internal/openapi/testdata/.
    col, err := Import("testdata/petstore_full.yaml")
    if err != nil {
        t.Fatalf("Import: %v", err)
    }

    // Behavior 7: shared X-API-Key → single collection variable
    if _, ok := col.Variables.Values["x_api_key"]; !ok {
        t.Error("expected x_api_key collection variable")
    }

    // Behavior 1: all 3 ops have X-API-Key header
    for _, it := range col.Requests.Items {
        if it.Request.Headers["X-API-Key"] != "{{x_api_key}}" {
            t.Errorf("%s: missing X-API-Key header", it.Name)
        }
    }

    // Behavior 2: listPets URL has ?limit={{limit}}
    listPets := findItem(t, col, "listPets")
    if !strings.Contains(listPets.Request.URL, "limit={{limit}}") {
        t.Errorf("listPets URL missing limit query param: %s", listPets.Request.URL)
    }

    // Behavior 3: createPet body has name field
    createPet := findItem(t, col, "createPet")
    body, ok := createPet.Request.Body.(map[string]any)
    if !ok || body["name"] == nil {
        t.Errorf("createPet body missing name field: %v", createPet.Request.Body)
    }

    // Behavior 5: all ops have status assertions
    for _, it := range col.Requests.Items {
        if len(it.Assertions.Status.Codes) == 0 {
            t.Errorf("%s: expected status assertions", it.Name)
        }
    }
    // listPets: [200, 404]
    assertCodes(t, "listPets", listPets.Assertions.Status.Codes, []int{200, 404})
    // createPet: [201, 404]
    assertCodes(t, "createPet", createPet.Assertions.Status.Codes, []int{201, 404})
}
```

#### Impact on Existing Tests
None.

---

### Step 10: Update smoke test and help text

**Rationale:** DoD requires smoke test covers new capability and help text is updated.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add M3-006 block testing petstore-full.yaml |
| `cmd/curlew/main.go` | modify | Update import openapi help text |

#### New Smoke Test Block (after existing M3-005 block, before "Smoke Test Complete")

```bash
# --- M3-006: headers, request bodies, and status assertions ---
echo "--- Import OpenAPI with headers/body/status (M3-006) ---"
cat > /tmp/curlew-smoke-openapi/petstore-full.yaml <<'OAI'
openapi: 3.0.3
info:
  title: Petstore Full
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
components:
  schemas:
    NewPet:
      type: object
      properties:
        name: { type: string }
paths:
  /pets:
    post:
      operationId: createPet
      parameters:
        - name: X-API-Key
          in: header
          required: true
          schema: { type: string }
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/NewPet'
      responses:
        '201': { description: Created }
        '404': { description: Not found }
OAI

CURLEW_TIER=professional ./curlew import openapi /tmp/curlew-smoke-openapi/petstore-full.yaml \
    --output /tmp/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: import openapi full spec writes file" \
  || { echo "FAIL: import openapi full spec failed"; exit 1; }

grep -q "X-API-Key" /tmp/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: generated collection contains X-API-Key header" \
  || { echo "FAIL: missing X-API-Key header in generated collection"; exit 1; }

grep -q "name:" /tmp/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: generated collection contains request body" \
  || { echo "FAIL: missing request body in generated collection"; exit 1; }

grep -q "status:" /tmp/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: generated collection contains status assertions" \
  || { echo "FAIL: missing status assertions in generated collection"; exit 1; }

./curlew validate /tmp/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: full generated collection validates" \
  || { echo "FAIL: full generated collection failed to validate"; exit 1; }
```

#### Help Text Update (`cmd/curlew/main.go`)
Find the `import openapi` command description and extend to mention headers, bodies, and assertions are now populated.

#### Impact on Existing Tests
Smoke test additions only; no test changes.

---

## Test Impact Summary

| Test file | Test function | Impact | Action |
|-----------|--------------|--------|--------|
| `internal/openapi/import_test.go` | `TestSynthName` | none | — |
| `internal/openapi/import_test.go` | `TestInterpolatePath` | none | — |
| `internal/openapi/import_test.go` | `TestImport_Petstore` | none | — |
| `internal/openapi/import_test.go` | `TestImport_OpenAPI31` | none | — |
| `internal/openapi/import_test.go` | `TestImport_NoServers` | none | — |
| `internal/openapi/import_test.go` | `TestImport_NoOperationId` | none | — |
| `internal/openapi/import_test.go` | `TestChosenName_Collision` | none | — |
| `internal/openapi/import_test.go` | `TestImport_NoOperations` | none | — |
| `internal/openapi/import_test.go` | `TestImport_FileNotFound` | none | — |
| `internal/openapi/import_test.go` | `TestImport_InvalidSpec` | none | — |
| `internal/openapi/emit_test.go` | `TestEmit_ContainsExpectedKeys` | extend | add full-fixture substring checks |
| `internal/openapi/emit_test.go` | `TestEmit_RoundTripsThroughParser` | none | petstore.yaml has no headers/body — round-trip unchanged |
| `internal/openapi/emit_test.go` | `TestEmit_WriteError` | none | — |
| `internal/parser/parser_test.go` | all | none | parser untouched |

---

## Proposed Go Function Signatures

```go
// import.go
func Import(specPath string) (*parser.Collection, error)
func importWithWarn(specPath string, warn func(string)) (*parser.Collection, error)
func collectParameters(
    op *openapi3.Operation,
    pi *openapi3.PathItem,
    baseURL string,
    collVarsSeen map[string]bool,
) (url string, headers map[string]string, newVars map[string]string)
func buildRequestBody(op *openapi3.Operation, w *schemaWalker) any
func extractStatusCodes(responses *openapi3.Responses) []int
func snakeCase(s string) string
func stderrWarn(msg string)

// schema.go
type schemaWalker struct {
    warn       func(string)
    warnedRefs map[string]bool
}
func (w *schemaWalker) placeholder(ref *openapi3.SchemaRef, visited map[string]bool) any
func defaultForStringFormat(format string) string
func sortedKeys(m openapi3.Schemas) []string

// emit.go (new types)
type writerAssertions struct { Status []int }
// (writerRequest and writerRequestItem extended with new fields)
func nilIfEmpty[K comparable, V any](m map[K]V) map[K]V
```

---

## Risks and Edge Cases

| Risk | Mitigation |
|------|-----------|
| `kin-openapi` returns `ref.Value == nil` for unresolvable refs | Walker checks `ref.Value == nil` and returns `"string"` — no crash |
| Recursive `$ref` cycle → stack overflow | Cycle check runs _before_ descent; named ref added to `visited` before recursing; terminates on first re-visit |
| `warn` called multiple times for same ref | `warnedRefs` map on walker; warn-once per ref name |
| Non-deterministic YAML output (map key order) | `sortedKeys` for properties; `sort.Strings(queryVars)` for query params; `sort.Strings(headerNames)` for headers |
| `TestImport_Petstore` URL changes if empty headers map emits differently | `collectParameters` returns `nil` headers map (not empty map) when no header params → `writerRequest.Headers` is nil → omitempty suppresses field → no YAML change |
| `allOf` with non-object branches | Walker only merges object-typed results; non-objects silently ignored (documented) |
| `oneOf`/`anyOf` first branch is null-typed | Walker recurses into it normally; null schema returns `nil`; YAML serialises as `null` — parser accepts |
| Multi-type schemas (`type: [string, null]` in OAS 3.1) | `s.Type.Is("string")` returns false for multi-type; falls through to object default; acceptable for M3-006 |
| Header casing: `x_api_key` var vs `X-API-Key` wire name | Variable is snake_case; header key uses original OpenAPI name to preserve wire casing |
| Query param order across different Go map iterations | Explicit `sort.Strings` before joining ensures `?from={{from}}&limit={{limit}}` always |
| `petstore-full.yaml` needs to be in both `testdata/openapi/` (for smoke/observable) and `internal/openapi/testdata/` (for unit tests) | Create both — they can be identical files |

---

## Verification

```bash
go build ./cmd/curlew
go test ./internal/openapi/...
~/go/bin/golangci-lint run ./internal/openapi/...
./smoke/run.sh
```

Observable verification:
```bash
CURLEW_TIER=professional ./curlew import openapi testdata/openapi/petstore-full.yaml \
    --output out.yaml
# Verify out.yaml contains:
#   headers: { X-API-Key: '{{x_api_key}}' }
#   url ending with ?limit={{limit}}
#   body: { name: string }
#   assertions: { status: [200, 404] }  (or [201, 404] for createPet)
./curlew validate out.yaml
go test ./internal/openapi/...
```

---

## Deviations

### snakeCase algorithm — `HTTPMethod` → `http_method` (not `h_t_t_p_method`)

**What changed:** The plan's `snakeCase` implementation (naive "insert `_` before every uppercase at i>0") produced `x_a_p_i_key` for `X-API-Key`, which contradicts behavior #1 and #7. The corrected algorithm uses acronym-boundary detection (insert `_` before an uppercase letter only when prev is lowercase OR prev is uppercase AND next is lowercase). This gives `x_api_key` for `X-API-Key` (correct) but `http_method` for `HTTPMethod` instead of `h_t_t_p_method`.

**Why it's correct:** `X-API-Key` → `x_api_key` is the core user-facing behavior (task behaviors #1 and #7). `HTTPMethod` is an artificial test case with no real-world equivalent in OpenAPI headers; `http_method` is a more reasonable output. Test updated accordingly.
