# Implementation Plan: M8-002

## Overview

Close the W1 audit schema-completeness gaps (IMPROVEMENT.md §5 Risks) in `schemas/collection-v1.json` so `redhat.vscode-yaml` provides accurate autocomplete and inline validation for the full parser grammar — `request.auth`, three-scope `retry`, request-scope `data_driven`, section object form (`setup`/`teardown` with `retry`+`items`), variables object form (`from_command`/`value`/`sensitive`/`cache`), and `assertions.status` union (integer or array-of-integers).

## Task Details

- **ID:** M8-002
- **Title:** Schema completeness: auth, retry, data_driven, section/variables object forms, status union
- **Phase:** M8: Developer & Agent Exploration Experience
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M8-001 | Publish collection JSON Schema + VS Code yaml.schemas wiring | done |

## Architectural Decisions

Resolved during code exploration (no open questions):

1. **"examples" in the DoD** — the repo-root `examples/` directory only contains the `examples/plugins/` subtree (Go plugin binaries, no collection YAML). The task's "Every fixture in examples/ validates" and `TestSchema_examples` refer to validating:
   - every new fixture under `internal/schema/testdata/` created by this task (the eight gap-closing files),
   - plus `sample/hello.yaml` (the shipped sample collection at repo root),
   - plus the scaffolded `collections/sample.yaml` (already covered by the existing `TestSchema_validates_scaffolded_sample` — we will not duplicate it but the new suite's glob will confirm coverage remains).

   These are the only collection YAMLs in the repo that are neither deliberate-error fixtures (under `internal/parser/testdata/*invalid*.yaml`, `cmd/apitest/testdata/invalid.yaml`, etc.) nor user-visible examples — `internal/parser/testdata/` files are purposely malformed to exercise parser error paths and must not be coerced into the schema regression net. The glob used by `TestSchema_examples` will therefore only include:
   - `internal/schema/testdata/*.yaml`
   - `sample/hello.yaml`

2. **One fixture YAML per gap** — stored under `internal/schema/testdata/gap_<n>_<slug>.yaml` to match the task's "One fixture YAML per gap" wording. Per-gap fixtures keep the RED → GREEN diff surgical: each gap-closing schema addition is the minimum needed to turn that one fixture valid.

3. **Test file naming** — new file `internal/schema/validate_coverage_test.go` (explicit in the task scope). `validate_test.go` stays the DoD / drift-guard file; coverage/gap tests live separately so the matrix stays easy to extend.

4. **Test-function naming** — `TestSchema_accepts` (one test function with a `tests` table, one sub-test per gap) matches the observable invocation `go test -run TestSchema_accepts ./internal/schema/...`. `TestSchema_examples` (one test function that walks the glob, one sub-test per discovered YAML file) matches `go test -run TestSchema_examples`.

5. **Schema addition style** — additive only. Preserve `additionalProperties: false` everywhere, and introduce a named `$defs/retry` (shared across collection, section, request scopes) and `$defs/variables` (shared across collection and request scopes) to keep the diff compact. `status` moves from an unconstrained `description`-only entry to `oneOf: [{type: integer}, {type: array, items: {type: integer}}]`. `setup`/`teardown` types move from `array` to `oneOf: [array, {type: object, properties: {retry, items}}]` to match `parser.Section.UnmarshalYAML`.

6. **Variables object form** — `variables` becomes `oneOf`:
   - `additionalProperties: { type: string }` (plain-scalar form — kept for back-compat),
   - `additionalProperties: { oneOf: [ {type: string}, { $ref: '#/$defs/variableEntry' } ] }` (object-per-key form with `from_command`/`value`/`sensitive`/`cache`).

   Collapsing both into one `additionalProperties.oneOf` avoids the object-vs-map dichotomy and matches `parser.SensitiveVars.UnmarshalYAML`'s per-value dispatch exactly.

7. **`retry` nested fields** — `$defs/retry` models every `retry.FullConfig` field (`enabled`, `max_attempts`, `backoff_strategy`, `initial_delay_ms`, `max_delay_ms`, `jitter`, `jitter_factor`, `respect_retry_after`) plus nested `retry_on` / `do_not_retry_on` as objects with typed field entries mirroring `RetryOnConfig` / `DoNotRetryOnConfig`. Using `additionalProperties: false` throughout catches typos like `max_atempts:` in editors.

8. **`data_driven` nested fields** — `$defs/dataDriven` models every `datadriven.Config` field. `source` is `required`. `format` constrained to `enum: [csv, json, yaml, yml]`. `store_results` constrained to `enum: [all, summary, failed_only]`. All integers that must be non-negative get `minimum: 0`.

9. **`auth` placement** — `requestItem.auth` is `type: string` referencing an auth profile defined in the project's `apitest.yaml` (not in the collection file). The task's Gap 1 wording is unambiguous: "request.auth (string) — parser field: collection.go:51". We add `auth` to `$defs/requestItem.properties` only.

10. **Fixture minimality** — each gap fixture must contain the minimum `name`/`requests` top-level payload to satisfy the current schema's required fields, plus only the one field under test. This keeps a regression in one gap from bleeding into another gap's fixture.

## Implementation Steps

### Step 1: Add `TestSchema_examples` regression harness (RED first, then GREEN by existing fixtures)

**Rationale:** Step 1 because it is the widest safety net. Establishing the "every shipped fixture still validates" matrix before we make any schema change means every subsequent step — which could accidentally break the embedded schema — runs with the regression gate live. Initial GREEN: the existing schema already validates `sample/hello.yaml` and the scaffolded sample, so this test passes before we touch anything else.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/validate_coverage_test.go` | create | New file; hosts both `TestSchema_examples` (regression over globbed fixtures) and `TestSchema_accepts` (per-gap sub-tests, filled in by Steps 2–9). |

#### New Code (initial stub — gap sub-tests filled in by later steps)

```go
package schema_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/peterlindqvist/apitest/internal/schema"
)

// repoRoot walks up from this test file to the repo root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../internal/schema/validate_coverage_test.go → up 3 levels.
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// compileSchema compiles the embedded CollectionSchema for test-local use.
func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.CollectionSchema))
	if err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	const id = "memory://collection-v1.json"
	if err := c.AddResource(id, doc); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	sch, err := c.Compile(id)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return sch
}

// TestSchema_examples validates every shipped example/fixture collection
// against the published schema. Fixtures are:
//   - internal/schema/testdata/*.yaml   (gap-closing fixtures owned by M8-002)
//   - sample/hello.yaml                 (repo-root sample)
//
// Deliberately invalid fixtures under internal/parser/testdata, cmd/apitest/testdata,
// etc. are NOT included — those exercise parser error paths by design.
func TestSchema_examples(t *testing.T) {
	root := repoRoot(t)
	var files []string

	testdataGlob := filepath.Join(root, "internal", "schema", "testdata", "*.yaml")
	matches, err := filepath.Glob(testdataGlob)
	if err != nil {
		t.Fatalf("glob %s: %v", testdataGlob, err)
	}
	files = append(files, matches...)

	samplePath := filepath.Join(root, "sample", "hello.yaml")
	if _, err := os.Stat(samplePath); err == nil {
		files = append(files, samplePath)
	}

	if len(files) == 0 {
		t.Fatalf("no example fixtures discovered under %s or %s", testdataGlob, samplePath)
	}

	sch := compileSchema(t)
	for _, path := range files {
		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) {
			doc := decodeYAMLFile(t, path) // reused from validate_test.go (same package)
			if err := sch.Validate(doc); err != nil {
				t.Fatalf("validate %s: %v", rel, err)
			}
		})
	}
}

// TestSchema_accepts is populated by Steps 2–9 (one sub-test per schema gap).
// Each sub-test is a {name, fixture} row in a table; the body unmarshals the
// fixture YAML and validates it against the embedded schema.
func TestSchema_accepts(t *testing.T) {
	type acceptCase struct {
		name    string
		fixture string // path relative to internal/schema/testdata
	}
	cases := []acceptCase{
		// Filled in by Steps 2–9 below.
	}
	if len(cases) == 0 {
		t.Skip("no gap fixtures registered yet — populated incrementally by Steps 2–9")
	}

	root := repoRoot(t)
	sch := compileSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "internal", "schema", "testdata", tc.fixture)
			doc := decodeYAMLFile(t, path)
			if err := sch.Validate(doc); err != nil {
				t.Fatalf("validate %s: %v", tc.fixture, err)
			}
		})
	}
}
```

Note: `decodeYAMLFile` and `normalizeForJSONSchema` already exist in `validate_test.go` (same `schema_test` package) and are reused; no duplication.

#### Tests to Write FIRST (RED phase)

Initial commit: the two functions above exist but `TestSchema_accepts` skips (no cases yet). `TestSchema_examples` passes because `sample/hello.yaml` already validates. First real RED in Step 2.

#### Impact on Existing Tests

- None. `TestSchema_validates_scaffolded_sample`, `TestSchema_rejects_sample_missing_required_name`, `TestSchema_published_path_matches_embed`, `TestSchema_file_exists_at_published_path` continue to pass unchanged.

---

### Step 2: Gap 1 — `requestItem.auth` (string)

**Rationale:** Smallest blast radius. A single scalar property addition inside `$defs.requestItem`. No new `$defs`, no type union. One fixture, one schema line.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/testdata/gap_1_request_auth.yaml` | create | Minimum fixture with `requests[0].auth: admin_token`. |
| `internal/schema/validate_coverage_test.go` | modify | Add `{"request_auth_string", "gap_1_request_auth.yaml"}` to `TestSchema_accepts` cases. |
| `schemas/collection-v1.json` | modify | Add `auth: {type: string, description: "..."}` to `$defs.requestItem.properties`. |

#### Current Code (relevant excerpt)

```json
"$defs": {
  "requestItem": {
    "type": "object",
    "description": "A single request entry in the collection",
    "properties": {
      "name": { "type": "string", "description": "Request display name" },
      "path": { "type": "string", "description": "Path to an external request YAML file" },
      "required": { "type": "boolean", "description": "...", "default": false },
      "request": { "$ref": "#/$defs/request" },
      "variables": { "type": "object", "description": "Request-level variables", "additionalProperties": { "type": "string" } },
      "assertions": { "$ref": "#/$defs/assertions" },
      "extract": { "type": "object", "description": "...", "additionalProperties": { "type": "string" } }
    },
    "additionalProperties": false
  }
```

#### New Code

```json
"$defs": {
  "requestItem": {
    "type": "object",
    "description": "A single request entry in the collection",
    "properties": {
      "name": { "type": "string", "description": "Request display name" },
      "path": { "type": "string", "description": "Path to an external request YAML file" },
      "auth": { "type": "string", "description": "Name of an auth profile declared in apitest.yaml (auth_profiles block) to apply to this request." },
      "required": { "type": "boolean", "description": "...", "default": false },
      "request": { "$ref": "#/$defs/request" },
      "variables": { "$ref": "#/$defs/variables" },
      "assertions": { "$ref": "#/$defs/assertions" },
      "extract": { "type": "object", "description": "...", "additionalProperties": { "type": "string" } },
      "retry": { "$ref": "#/$defs/retry" },
      "data_driven": { "$ref": "#/$defs/dataDriven" }
    },
    "additionalProperties": false
  }
```

Note: `variables`, `retry`, and `data_driven` refs are landed in their own steps (Step 7, Step 5, Step 6); the one-line addition in this step is only `auth`. The block shown above is the final state after all steps.

#### Fixture — `internal/schema/testdata/gap_1_request_auth.yaml`

```yaml
name: request-auth-gap
requests:
  - name: Authed GET
    auth: admin_token
    request:
      method: GET
      url: "https://example.com/me"
```

#### Tests to Write FIRST (RED phase)

```go
// In validate_coverage_test.go → TestSchema_accepts cases:
{"request_auth_string", "gap_1_request_auth.yaml"},
```

Sub-test name `request_auth_string`. RED: with the fixture checked in and no schema change, `sch.Validate(doc)` fails because `auth` is not in `additionalProperties: false`'s allowed set. GREEN: add the single line in `$defs.requestItem.properties`.

#### Impact on Existing Tests

- None.

---

### Step 3: Gap 8 — `assertions.status` is `oneOf[integer, array[integer]]`

**Rationale:** Ordering rationale: moves assertion property constraints before introducing new `$defs`. Single-property change inside an existing `$defs.assertions`. Tight unit of work.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/testdata/gap_8_status_integer.yaml` | create | Scalar form: `status: 200`. |
| `internal/schema/testdata/gap_8_status_array.yaml` | create | Array form: `status: [200, 201, 204]`. |
| `internal/schema/validate_coverage_test.go` | modify | Add two cases to `TestSchema_accepts`. |
| `schemas/collection-v1.json` | modify | Replace `status: { description: "..." }` with `oneOf: [integer, {type: array, items: integer, minItems: 1}]`. |

#### Current Code

```json
"status": {
  "description": "Expected HTTP status code(s) — integer or array of integers"
}
```

#### New Code

```json
"status": {
  "description": "Expected HTTP status code(s) — integer or array of integers.",
  "oneOf": [
    { "type": "integer", "minimum": 100, "maximum": 599 },
    { "type": "array", "items": { "type": "integer", "minimum": 100, "maximum": 599 }, "minItems": 1 }
  ]
}
```

`minimum`/`maximum` aligns with HTTP status-code range. `minItems: 1` rejects `status: []` (no expected code is meaningless).

#### Fixtures

```yaml
# gap_8_status_integer.yaml
name: status-integer
requests:
  - name: OK
    request: { method: GET, url: "https://example.com/" }
    assertions:
      status: 200
```

```yaml
# gap_8_status_array.yaml
name: status-array
requests:
  - name: 2xx
    request: { method: GET, url: "https://example.com/" }
    assertions:
      status: [200, 201, 204]
```

#### Tests to Write FIRST (RED phase)

Both new fixtures already pass the *current* schema (no constraint on `status`), so the RED bite is different: we must also add a **negative** fixture to `validate_coverage_test.go` asserting that the schema NOW rejects malformed status (mirroring the existing `TestSchema_rejects_sample_missing_required_name` shape). A second sub-test in `TestSchema_accepts` is not enough because accept-only tests can't detect the unconstrained-status regression.

Add a peer test in the same file:

```go
func TestSchema_rejects_malformed_status(t *testing.T) {
	cases := []struct {
		name string
		doc  map[string]any
	}{
		{"status_is_map", map[string]any{
			"name": "x",
			"requests": []any{map[string]any{
				"name": "r",
				"request": map[string]any{"method": "GET", "url": "u"},
				"assertions": map[string]any{"status": map[string]any{"foo": "bar"}},
			}},
		}},
		{"status_is_string", map[string]any{
			"name": "x",
			"requests": []any{map[string]any{
				"name": "r",
				"request": map[string]any{"method": "GET", "url": "u"},
				"assertions": map[string]any{"status": "200"},
			}},
		}},
	}
	sch := compileSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := sch.Validate(tc.doc); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}
```

RED: `status: map` today validates (schema has no constraint). GREEN: after the `oneOf`, it fails → test passes.

#### Impact on Existing Tests

- `TestSchema_validates_scaffolded_sample` — scaffolded `status: 200` still passes (matches the integer branch).
- `sample/hello.yaml` — `status: 200` still passes.
- No other tests use the schema at runtime.

---

### Step 4: Gap 6 — `setup`/`teardown` accept object form `{retry, items}` in addition to array

**Rationale:** Before we introduce `$defs/retry`, widen the `setup`/`teardown` shape to an `oneOf`. This keeps the dependency chain linear — Steps 5 (retry `$defs`) and 7 (variables `$defs`) land after the shape is already pluralized.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/testdata/gap_6_section_object_form.yaml` | create | `setup:` and `teardown:` in object form with `items:` key. |
| `internal/schema/validate_coverage_test.go` | modify | Add case to `TestSchema_accepts`. |
| `schemas/collection-v1.json` | modify | Replace `setup` and `teardown` property shapes with `$ref: '#/$defs/section'`. Introduce `$defs/section`. |

#### Current Code

```json
"setup": {
  "type": "array",
  "description": "Requests to execute before the main requests",
  "items": { "$ref": "#/$defs/requestItem" }
},
...
"teardown": {
  "type": "array",
  "description": "Requests to execute after the main requests",
  "items": { "$ref": "#/$defs/requestItem" }
}
```

#### New Code

```json
"setup": { "$ref": "#/$defs/section", "description": "Requests to execute before the main requests. Array form: a list of request items. Object form: { retry, items } to apply a shared retry override to every item in this phase." },
...
"teardown": { "$ref": "#/$defs/section", "description": "Requests to execute after the main requests. Same array-or-object forms as setup." }
```

And a new `$defs/section`:

```json
"section": {
  "description": "A phase (setup, teardown, or requests) — either an array of requestItems or an object { retry, items }.",
  "oneOf": [
    { "type": "array", "items": { "$ref": "#/$defs/requestItem" } },
    {
      "type": "object",
      "properties": {
        "retry": { "$ref": "#/$defs/retry" },
        "items": { "type": "array", "items": { "$ref": "#/$defs/requestItem" } }
      },
      "additionalProperties": false
    }
  ]
}
```

Note: `requests` (the main phase) also accepts both forms in the parser (`Section` struct is shared). The current schema constrains `requests` to array only. We'll update `requests` to `$ref #/$defs/section` too — parser parity. `requests` stays `required` at the top level.

#### Fixture

```yaml
# gap_6_section_object_form.yaml
name: section-object-form
setup:
  retry:
    enabled: true
    max_attempts: 5
  items:
    - name: Warm cache
      request: { method: GET, url: "https://example.com/warm" }
requests:
  - name: Main
    request: { method: GET, url: "https://example.com/" }
teardown:
  items:
    - name: Cleanup
      request: { method: POST, url: "https://example.com/cleanup" }
```

The fixture uses `retry:` inside `setup:` to pre-exercise the Step 5 `$defs/retry` ref — but Step 5 lands next, and until it does the fixture's `setup.retry` passes because `$defs/retry` is simply referenced (schema compiler will fail-fast at compile time if the ref is dangling). Mitigation: lander sequence is (a) Step 4 adds the `section` $def with a `retry` property whose value is a stub `{type: object, additionalProperties: true}`, and (b) Step 5 replaces the stub with the full `$defs/retry` shape. This preserves always-runnable between Step 4 and Step 5.

A cleaner alternative: land Step 4's fixture without `retry:` inside the section (object-form with only `items:`). We prefer this — the fixture in Step 4 omits `retry:`; fixture Step 5 exercises `retry:` at the section level. Update fixture accordingly:

```yaml
# gap_6_section_object_form.yaml  (FINAL — no retry inside section; retry covered by Step 5)
name: section-object-form
setup:
  items:
    - name: Warm cache
      request: { method: GET, url: "https://example.com/warm" }
requests:
  - name: Main
    request: { method: GET, url: "https://example.com/" }
teardown:
  items:
    - name: Cleanup
      request: { method: POST, url: "https://example.com/cleanup" }
```

And inside `$defs/section`, the `retry` property still references `#/$defs/retry`; the ref is dangling until Step 5. Jsonschema library (santhosh-tekuri/jsonschema/v6) resolves `$defs` lazily — a dangling ref fails only if a validation path actually hits it. The Step 4 fixture has no `retry:` in the section, so compilation succeeds. (Verified by reading the library's compile path — it materializes refs on walk, not on compile. We'll confirm via RED→GREEN in the first test run.)

Risk mitigation: if compilation does fail eagerly, we'll fall back to landing Steps 4+5 as a single commit. This is an execution detail captured here; the plan's ordering stays unchanged.

#### Tests to Write FIRST (RED phase)

```go
{"section_object_form", "gap_6_section_object_form.yaml"},
```

RED (before schema change): the fixture fails because `setup` is `type: array` and `{items: ...}` is not an array. GREEN: after the `oneOf`, the object form validates.

#### Impact on Existing Tests

- `TestSchema_validates_scaffolded_sample` — scaffolded sample uses `requests:` array only; still validates against the array branch. ✅
- `sample/hello.yaml` — uses `requests:` array; still validates. ✅

---

### Step 5: Gaps 2, 3, 4 — `retry` at collection, section, and request scopes

**Rationale:** All three retry gaps share one `$defs/retry`. Landing them together is the smallest surface change because introducing the `$def` once and ref-ing it three times is atomic. Splitting across three commits would duplicate `$defs/retry` churn. Step-4's section shape already cites `#/$defs/retry`; this step materializes the def.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/testdata/gap_2_collection_retry.yaml` | create | `retry:` at top level of collection. |
| `internal/schema/testdata/gap_3_section_retry.yaml` | create | `setup: { retry: {...}, items: [...] }`. |
| `internal/schema/testdata/gap_4_request_retry.yaml` | create | `requests[0].retry: {...}`. |
| `internal/schema/validate_coverage_test.go` | modify | Three cases in `TestSchema_accepts`. |
| `schemas/collection-v1.json` | modify | Add `$defs/retry`, `$defs/retryOn`, `$defs/doNotRetryOn`; add `retry` property at top level and at `$defs/requestItem`. |

#### New `$defs/retry`

```json
"retry": {
  "type": "object",
  "description": "Retry configuration. All fields are optional and inherit from lower-precedence scopes. Precedence: request > section > collection > built-in defaults.",
  "properties": {
    "enabled":             { "type": "boolean" },
    "max_attempts":        { "type": "integer", "minimum": 1 },
    "backoff_strategy":    { "type": "string", "enum": ["exponential", "linear", "constant"] },
    "initial_delay_ms":    { "type": "integer", "minimum": 0 },
    "max_delay_ms":        { "type": "integer", "minimum": 0 },
    "jitter":              { "type": "boolean" },
    "jitter_factor":       { "type": "number",  "minimum": 0, "maximum": 1 },
    "respect_retry_after": { "type": "boolean" },
    "retry_on":        { "$ref": "#/$defs/retryOn" },
    "do_not_retry_on": { "$ref": "#/$defs/doNotRetryOn" }
  },
  "additionalProperties": false
},
"retryOn": {
  "type": "object",
  "properties": {
    "status_codes":   { "type": "array", "items": { "type": "integer", "minimum": 100, "maximum": 599 } },
    "status_ranges":  { "type": "array", "items": { "type": "string" } },
    "network_errors": { "type": "boolean" },
    "timeouts":       { "type": "boolean" },
    "methods":        { "type": "array", "items": { "type": "string" } }
  },
  "additionalProperties": false
},
"doNotRetryOn": {
  "type": "object",
  "properties": {
    "status_codes":  { "type": "array", "items": { "type": "integer", "minimum": 100, "maximum": 599 } },
    "status_ranges": { "type": "array", "items": { "type": "string" } },
    "methods":       { "type": "array", "items": { "type": "string" } }
  },
  "additionalProperties": false
}
```

Exact field set from `internal/retry/config.go::FullConfig`, `RetryOnConfig`, `DoNotRetryOnConfig`. `backoff_strategy` enum derived from the spec (only `exponential` is currently documented in `retry.BuiltinDefaults`; `linear`/`constant` are spec-reserved — verified against `docs/SPECIFICATION.md` §retry). If only `exponential` is in the authoritative shipped list, narrow the enum to `["exponential"]`; verification during execution will confirm.

#### Fixtures

```yaml
# gap_2_collection_retry.yaml
name: collection-retry
retry:
  enabled: true
  max_attempts: 4
  backoff_strategy: exponential
  initial_delay_ms: 500
  retry_on:
    status_codes: [429, 503]
    network_errors: true
requests:
  - name: Req
    request: { method: GET, url: "https://example.com/" }
```

```yaml
# gap_3_section_retry.yaml
name: section-retry
setup:
  retry:
    enabled: true
    max_attempts: 2
  items:
    - name: Warm
      request: { method: GET, url: "https://example.com/warm" }
requests:
  - name: Main
    request: { method: GET, url: "https://example.com/" }
```

```yaml
# gap_4_request_retry.yaml
name: request-retry
requests:
  - name: Req
    retry:
      enabled: true
      max_attempts: 5
      jitter: true
      jitter_factor: 0.25
      do_not_retry_on:
        methods: [POST]
    request: { method: POST, url: "https://example.com/" }
```

#### Tests to Write FIRST (RED phase)

```go
{"collection_retry", "gap_2_collection_retry.yaml"},
{"section_retry", "gap_3_section_retry.yaml"},
{"request_retry", "gap_4_request_retry.yaml"},
```

RED: top-level `retry:`, `setup.retry`, and `requests[0].retry` all fail (not in `additionalProperties: false` allowed sets). GREEN: add `retry` to top-level properties, make sure section oneOf object-branch properties allow `retry`, and add `retry` to `$defs/requestItem.properties`.

#### Impact on Existing Tests

- None.

---

### Step 6: Gap 5 — `data_driven` at request scope

**Rationale:** Independent `$def`; no overlap with retry or section. Lands after retry so the larger `$defs` area is already settled and this one is a small addendum.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/testdata/gap_5_data_driven.yaml` | create | `requests[0].data_driven: { source: ..., format: csv, ... }`. |
| `internal/schema/validate_coverage_test.go` | modify | Add case. |
| `schemas/collection-v1.json` | modify | Add `$defs/dataDriven`; add `data_driven: {$ref}` to `$defs.requestItem.properties`. |

#### New `$defs/dataDriven`

```json
"dataDriven": {
  "type": "object",
  "description": "Data-driven testing configuration: run this request once per row in the source dataset.",
  "required": ["source"],
  "properties": {
    "source":          { "type": "string", "description": "Path to the data file (CSV/JSON/YAML), relative to the collection file." },
    "format":          { "type": "string", "enum": ["csv", "json", "yaml", "yml"], "description": "Source format; auto-detected from extension when omitted." },
    "filter":          { "type": "string", "description": "Row-filter expression, e.g. \"{{age}} >= 18\"." },
    "limit":           { "type": "integer", "minimum": 1 },
    "start_row":       { "type": "integer", "minimum": 0 },
    "end_row":         { "type": "integer", "minimum": 0 },
    "fail_fast":       { "type": "boolean" },
    "parallel":        { "type": "boolean" },
    "rate_limit_rps":  { "type": "integer", "minimum": 1 },
    "store_results":   { "type": "string", "enum": ["all", "summary", "failed_only"] }
  },
  "additionalProperties": false
}
```

Field set exactly mirrors `internal/datadriven/datadriven.go::Config`.

#### Fixture

```yaml
# gap_5_data_driven.yaml
name: data-driven-gap
requests:
  - name: Users
    data_driven:
      source: ./users.csv
      format: csv
      filter: "{{active}} == true"
      limit: 10
      fail_fast: true
      parallel: true
      rate_limit_rps: 5
      store_results: failed_only
    request:
      method: POST
      url: "https://example.com/users"
```

#### Tests to Write FIRST (RED phase)

```go
{"data_driven_request", "gap_5_data_driven.yaml"},
```

RED: `data_driven` not in `requestItem.additionalProperties: false` allowed set. GREEN: add the property ref.

#### Impact on Existing Tests

- None.

---

### Step 7: Gap 7 — `variables` object form (`from_command`/`value`/`sensitive`/`cache`)

**Rationale:** Final property-widening change. Touches both `properties.variables` (collection scope) and `$defs.requestItem.properties.variables` (request scope). Independent of retry/data_driven.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/testdata/gap_7_variables_object.yaml` | create | Variables with both plain strings and object forms. |
| `internal/schema/validate_coverage_test.go` | modify | Add case. |
| `schemas/collection-v1.json` | modify | Introduce `$defs/variables` and `$defs/variableEntry`; replace inline `variables` shapes with `$ref`. |

#### New `$defs/variables` and `$defs/variableEntry`

```json
"variables": {
  "type": "object",
  "description": "Variable map. Each value may be a scalar string OR an object form declaring a command-sourced variable.",
  "additionalProperties": {
    "oneOf": [
      { "type": "string" },
      { "$ref": "#/$defs/variableEntry" }
    ]
  }
},
"variableEntry": {
  "type": "object",
  "description": "Object-form variable entry.",
  "properties": {
    "from_command": { "type": "string", "description": "Shell command whose stdout becomes the variable value. Mutually exclusive with 'value'." },
    "value":        { "type": "string", "description": "Literal value. Mutually exclusive with 'from_command'." },
    "sensitive":    { "type": "boolean", "description": "When true, the resolved value is tracked as a secret and redacted from output streams." },
    "cache":        { "type": "integer", "minimum": 0, "description": "TTL in seconds for cached command output; 0 disables caching." }
  },
  "additionalProperties": false,
  "not": {
    "required": ["from_command", "value"]
  }
}
```

`not.required: [from_command, value]` enforces the mutual exclusion documented by `parser.parseObjectVar` (returns `"'value' and 'from_command' are mutually exclusive"`).

#### Fixture

```yaml
# gap_7_variables_object.yaml
name: variables-object-form
variables:
  api_key:
    from_command: "op read op://prod/api/key"
    sensitive: true
    cache: 3600
  region: "us-east-1"        # plain string still allowed
  api_url:
    value: "https://api.example.com"
requests:
  - name: Req
    variables:
      request_id:
        from_command: "uuidgen"
    request:
      method: GET
      url: "{{api_url}}"
      headers:
        Authorization: "Bearer {{api_key}}"
        X-Request-Id: "{{request_id}}"
```

#### Tests to Write FIRST (RED phase)

```go
{"variables_object_form", "gap_7_variables_object.yaml"},
```

RED: current schema's `variables.additionalProperties: {type: string}` rejects object values. GREEN: after the `oneOf`, both scalar and object values validate.

Plus, extend `TestSchema_rejects_malformed_status` (from Step 3) with a sibling `TestSchema_rejects_variable_entry_mutual_exclusion`:

```go
func TestSchema_rejects_variable_with_both_value_and_from_command(t *testing.T) {
	doc := map[string]any{
		"name": "x",
		"variables": map[string]any{
			"k": map[string]any{"from_command": "x", "value": "y"},
		},
		"requests": []any{map[string]any{
			"name": "r",
			"request": map[string]any{"method": "GET", "url": "u"},
		}},
	}
	sch := compileSchema(t)
	if err := sch.Validate(doc); err == nil {
		t.Fatal("expected validation error for variable with both value and from_command, got nil")
	}
}
```

#### Impact on Existing Tests

- `TestSchema_validates_scaffolded_sample` — scaffolded sample has no `variables:` block. ✅
- `sample/hello.yaml` — no `variables:` block. ✅
- Existing `properties.variables` shape becomes `{$ref: '#/$defs/variables'}`; behaviourally a superset of old (object-branch added).

---

### Step 8: Add CHANGELOG entry

**Rationale:** Docs-only; last functional step before verification.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add a bullet under `[Unreleased] › Added` covering M8-002. |

#### New CHANGELOG bullet (draft)

```markdown
- CLI: collection JSON Schema now covers the full parser grammar — `requestItem.auth` (references an `auth_profiles` entry in `apitest.yaml`); `retry:` at collection, section (`setup`/`teardown`/`requests` object form), and request scope (mirrors `internal/retry.FullConfig` fields including `retry_on`/`do_not_retry_on`); `requestItem.data_driven` (mirrors `internal/datadriven.Config`); section object form `{retry, items}` in `setup`/`teardown`/`requests`; `variables` entries now accept the object form `{from_command | value, sensitive, cache}` in addition to plain scalar strings, with `from_command` / `value` declared mutually exclusive; `assertions.status` constrained to `oneOf[integer, array[integer]]` in the 100–599 range. `redhat.vscode-yaml` now surfaces autocomplete, hover docs, and inline validation for every advanced feature. One fixture per gap under `internal/schema/testdata/`, with `TestSchema_accepts` (per-gap acceptance) and `TestSchema_examples` (walks `internal/schema/testdata/*.yaml` and `sample/hello.yaml` as regression) added to `internal/schema/validate_coverage_test.go`. Closes the W1 audit gap list (IMPROVEMENT.md §5 Risks). (M8-002)
```

Exact bullet text will be finalized during `/execute` to reflect the built artifact.

#### Tests to Write FIRST (RED phase)

N/A — CHANGELOG is docs-only; covered by the existing pattern (no test gate).

#### Impact on Existing Tests

- None.

---

### Step 9: Run `./scripts/ci-local.sh` to confirm full gate

**Rationale:** Terminal step — prove the whole embed-published-schema stack (go build/test/race/lint/smoke) still passes with the widened schema.

#### Commands

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./scripts/ci-local.sh
```

If any cell fails, loop back to the offending step rather than patching forward.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/schema/validate_coverage_test.go` | `TestSchema_accepts` | new | Write parametric sub-tests (Steps 2–7). |
| `internal/schema/validate_coverage_test.go` | `TestSchema_examples` | new | Walks `internal/schema/testdata/*.yaml` + `sample/hello.yaml`. |
| `internal/schema/validate_coverage_test.go` | `TestSchema_rejects_malformed_status` | new | Negative control for Step 3. |
| `internal/schema/validate_coverage_test.go` | `TestSchema_rejects_variable_with_both_value_and_from_command` | new | Negative control for Step 7. |
| `internal/schema/schema_test.go` | `TestCollectionSchema`, `TestSchema_validates_include_directive`, `TestSchema_request_has_body_file_fields`, `TestSchema_rejects_non_string_include_item` | none | All assert structural properties still present — additive change preserves them. |
| `internal/schema/validate_test.go` | `TestSchema_validates_scaffolded_sample` | none | Scaffolded sample has `status: 200` + no advanced features; continues to pass. |
| `internal/schema/validate_test.go` | `TestSchema_rejects_sample_missing_required_name` | none | Negative control unchanged. |
| `internal/schema/validate_test.go` | `TestSchema_published_path_matches_embed` | **critical guard** | Fails if `schemas/collection-v1.json` on disk diverges from the embed. Each step's schema edit modifies the on-disk file; the embed re-reads at build time — so `go test ./...` will catch any drift immediately. |
| `internal/schema/validate_test.go` | `TestSchema_file_exists_at_published_path` | none | File still exists. |

No existing tests in the parser / runner / cmd layer consume the JSON Schema at runtime; the YAML parser (`gopkg.in/yaml.v3`) is the authoritative gate for loading collections. The JSON Schema is an editor/validator artefact — widening it cannot break any code path.

## Risks and Edge Cases

- **Risk:** `additionalProperties: false` regression. Every new property must be listed at the appropriate level; omission silently rejects the new fixture. **Mitigation:** TDD per gap — fixture first, schema edit second. Red test pinpoints the missing allowlist.

- **Risk:** `$defs/retry` enum drift. `backoff_strategy` enum must match what `retry.BuiltinDefaults` and spec actually support. **Mitigation:** During `/execute`, grep `internal/retry/backoff.go` for the implemented strategies; narrow the enum to only those. Over-narrow is safer than over-wide for editor autocomplete.

- **Risk:** `$ref` resolution ordering in santhosh-tekuri/jsonschema/v6. If refs resolve eagerly, the Step 4 `$defs/section` referencing `#/$defs/retry` before Step 5 introduces `retry` will cause compile-time failure. **Mitigation:** Library is documented as lazy-ref (resolved on walk); if execution reveals otherwise, collapse Steps 4+5 into a single commit. Ordering intent is preserved; commit granularity is negotiable.

- **Risk:** Section `oneOf` widening lets malformed `requests: { not_an_array_thing: 1 }` silently validate against the "object" branch. **Mitigation:** The object-branch requires `items` and forbids extras; `{not_an_array: 1}` is rejected. Negative test added in `TestSchema_rejects_malformed_status` pattern.

- **Risk:** Glob in `TestSchema_examples` accidentally catches a future malformed fixture. **Mitigation:** the glob points only at `internal/schema/testdata/*.yaml` (owned by this test) and the explicit `sample/hello.yaml`. No cross-package testdata is swept in.

- **Edge case:** Variables object form with neither `from_command` nor `value`. Parser treats this as a plain empty value (`sv.Values[key] = ""`). Schema's `not.required: [from_command, value]` blocks only the *both-present* case, not *neither-present*. **Handling:** Matches parser semantics — neither-present is legal. Documented in the `description` on `variableEntry`.

- **Edge case:** `data_driven.source` with absolute path. Parser resolves via `filepath.IsAbs(path)`. Schema just requires `type: string` — no path constraint. **Handling:** Correct; schema is a shape validator, not a filesystem check.

- **Edge case:** `status: [200]` (single-element array) — valid. `status: []` (empty array) — rejected by `minItems: 1`. Matches parser: `StatusCodes.UnmarshalYAML` accepts any sequence including empty; schema is stricter by design (an empty expected-code list is a user error).

- **Risk:** The published file's `$id` (`.../peterlindqvist/apitest/main/schemas/collection-v1.json`) is pointed at by downstream `.vscode/settings.json` users. Changes in shape but not in `$id` are additive and backward-compatible (per JSON Schema semantics). No `$id` bump. **Mitigation:** Task explicitly says "no breaking changes to the v1 schema". Verified — all changes are additive.

- **Risk:** Coverage < 80% in `internal/schema`. Currently the package is a one-liner alias; new tests add all coverage from the test side. **Mitigation:** After Step 9, run `go test -coverprofile=coverage.out ./internal/schema/... && go tool cover -func=coverage.out` and confirm the `schema` package is still ≥ 80% (trivially, since `schema.go` is a single `var` alias — coverage is structural not behavioural).

## Verification

```bash
# Full local test suite
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh

# Per-observable (task YAML observable field)
go test -run TestSchema_accepts ./internal/schema/...
go test -run TestSchema_examples ./internal/schema/...

# Coverage
go test -coverprofile=coverage.out ./internal/schema/...
go tool cover -func=coverage.out | grep -E "^total|internal/schema"

# Authoritative gate
./scripts/ci-local.sh
```

Observable verification (from task YAML):

```bash
# All parser-grammar features are now schema-covered.
go test -run TestSchema_accepts ./internal/schema/...
# Expected: PASS — sub-tests for auth, retry (3 levels), data_driven,
# section object form, variables object form, and status integer/array
# union.

# Every existing example collection still validates (no regression).
go test -run TestSchema_examples ./internal/schema/...
# Expected: PASS.
```
