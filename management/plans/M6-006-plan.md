# Implementation Plan: M6-006

## Overview
Ship a self-contained, agent-grade reference for the Curlew v0.1 event stream: a prose document (`docs/EVENTS_SCHEMA_v0.1.md`) that covers every event kind, field semantics, ordering/timing guarantees, body-truncation rules, error taxonomy, and the v0.x → v1.0 stability policy — plus two sync tests in `internal/output/events` that keep the checked-in JSON Schema and the doc's examples structurally pinned to the Go struct definitions.

## Task Details
- **ID:** M6-006
- **Title:** Event schema documentation and stability policy
- **Phase:** M6: AI Agent Integration
- **Priority:** 2
- **Complexity:** medium
- **Estimated effort:** 1 day

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M6-004 | Events emitter package with NDJSON event types | done |
| M6-005 | Wire --events flag into the run subcommand end-to-end | done |

## Key Architectural Decisions

1. **Hand-written JSON Schema stays authoritative; the sync test is structural, not generative.** The existing `docs/events-schema/v0.1.json` (written during M6-004) is already high-quality: `additionalProperties: false`, `oneOf` discriminator, per-kind required fields. Rather than build a Go-to-JSON-Schema generator (which would duplicate hand-authored descriptions and risk regressions), `TestSchema_DocInSyncWithCode` will use Go reflection over the exported event structs in `internal/output/events/events.go` to enforce an invariant: **every JSON-tagged struct field maps 1:1 to a property in the corresponding schema definition, and every required (non-`omitempty`) field appears in that definition's `required` array.** This catches "added a field in Go, forgot to mirror it in the schema" — the only drift that actually matters — without coupling prose text to code.
2. **Examples are single-source in the markdown, and validated against the schema at test time.** `TestSchema_MarkdownExamplesValidate` walks the fenced code blocks in `docs/EVENTS_SCHEMA_v0.1.md` tagged ` ```json ` or ` ```ndjson ` and validates each line against the compiled v0.1 schema. This keeps examples honest: edit the doc, run the tests, see it fail if a sample drifts. No runtime code changes, no generated artifacts.
3. **The doc is structured for agent consumption, not for humans browsing a tutorial.** Agents read top-to-bottom once and need deterministic lookup paths. So the layout is: (a) invariants that hold across the whole stream, (b) one section per `Kind` with a canonical schema table and a minimal example, (c) the error taxonomy as a flat code-to-category-to-hint-pattern reference, (d) ordering and timing guarantees, (e) stability policy. Every section is reachable in ≤2 clicks from a table of contents at the top.
4. **Error category/code reference is enumerated explicitly from the codebase.** The task behaviour calls out "given an agent looks up `PARSE_INVALID_YAML`, the doc points it to the category, the producing package, and the expected hint structure." Rather than hand-maintain this (the registry now has ~80 codes across ~25 packages), I'll embed a compact reference table derived from `apierrors.RegisteredNames()` and the individual `hints_init.go` files. The table is hand-written into the doc — but the sync test verifies no `Code` string in the doc is missing from the registry, so stale entries surface immediately.
5. **Stability policy is prescriptive.** Exactly what is allowed in v0.x (additive new kinds, additive optional fields, new error codes), what requires a major bump (rename/remove field, narrowing enum, changing required-ness, changing meaning), and the v1.0 gate (M6-007 harness passes). The policy lives inside the doc; M6-007 will update it when it promotes the schema.
6. **Doc filename commits to `v0.1` in the path.** Once v1.0 lands (M6-007), the v0.1 artefacts are retained unchanged as a deprecation anchor. So the filename isn't "current" — it's versioned forever. This matches the checked-in JSON Schema naming.
7. **No runtime or CLI changes whatsoever.** This slice is pure documentation plus two reflective tests. No imports added outside `internal/output/events/schema_test.go`'s test package. No new sentinels, no new error codes, no new exported API.

## Implementation Steps

### Step 1: Write docs/EVENTS_SCHEMA_v0.1.md
**Rationale:** Documentation is the user-observable deliverable and has zero code risk. Writing it first lets the sync tests (step 2) target a concrete file with concrete examples. Starting with tests on an empty doc means the tests have nothing to validate and guide nothing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `docs/EVENTS_SCHEMA_v0.1.md` | create | New reference doc (agent-oriented, ~600 lines) |

#### Section outline (mandatory — tests verify this structure exists)

```markdown
# Curlew Agent Event Stream — v0.1

## Audience and scope
<one paragraph: who reads this, what stream is, when emitted>

## Table of contents
<links to every H2 below>

## Stream invariants
- schema_version contract (every event has "schema_version": "0.1")
- run_id is stable for the whole stream and opaque (32 lowercase hex)
- id is a monotonic counter, starts at 1, no gaps
- at_ms monotonic per producer; may skew in parallel (see Ordering)
- every line is one complete JSON object terminated by \n (NDJSON)
- encoding is UTF-8

## Event kinds
### run.start
<schema table: field | type | required | description>
<minimal ```json example>
<fuller ```json example with optional fields>

### run.error
<same sub-structure>

### request.start
<same>

### request.end
<same — including the body-truncation fields cluster>

### assertion.result
<same>

### run.end
<same>

## Error taxonomy (for run.error and request.end.error)
### Categories
<table: category | when used | recommended agent action>
### Error codes (by producing package)
<subsection per package, with code | short description | typical hint shape>

## Ordering guarantees
- Within a single run, event ids are strictly monotonic.
- run.start is always id=1 and at_ms=0.
- run.end is always the last event, its event_count matches its id.
- request.start for a given request_id precedes every other event for that id.
- request.end for a given request_id follows every assertion.result for that id.
- In parallel waves, at_ms values across different request_ids MAY interleave
  (they share the same monotonic clock origin but are scheduled independently).
  ids remain strictly monotonic regardless.

## Body truncation and encoding
- Default limit: 2048 bytes.
- Text body ≤ limit: body emitted inline, no truncation fields.
- Text body > limit: truncated at byte boundary to valid UTF-8, *_truncated=true, *_size set.
- Binary body: base64-encoded, *_encoding="base64".
- Binary body > limit: base64 of the first N bytes, *_truncated=true, *_size set.
- Sensitive redaction (see M6-003): values matching redaction rules are
  replaced with "***REDACTED***" before truncation is evaluated. Truncation
  works on the redacted bytes.

## run.error vs request.end with error
- run.error: a failure that prevents normal completion (parse, config, auth
  pre-flight, unsupported feature, etc.). Only run.start precedes it in the
  stream. run.end still follows with exit_code != 0.
- request.end with outcome=error: a failure during a single request's
  execution (network, plugin, extraction). Other requests may still run and
  have their own request.end events.

## Stability policy (v0.x and v1.0)
### Additive changes (allowed in v0.x)
- New event Kind values
- New optional fields on existing kinds
- New enum values in non-discriminator fields (category, network code)
- New error codes
### Breaking changes (require a major bump)
- Renaming or removing any existing field
- Changing a field's type or units
- Narrowing an enum (removing a value)
- Changing a field from optional to required
- Changing the meaning of an existing value
### v1.0 promotion gate
v1.0 may be cut only after M6-007's validation harness passes against every
canonical broken fixture (missing-variable, bad-yaml, failing-assertion,
unreachable-host, auth-missing, feature-gate-denied, circular-include).
M6-007 is the only task that mutates SchemaVersion; its plan documents the
v0.1 → v1.0 diff.

## Consumer guidance (for agents)
- Parse one line at a time; do not buffer the whole stream.
- Dispatch on "kind" before inspecting other fields.
- Treat unknown optional fields as forward-compatible; do not error.
- Treat unknown Kind values as forward-compatible; skip with a warning.
- Use (run_id, id) as the unique event identity across merged streams.
```

#### Tests to Write FIRST (RED phase)

See Step 2. Step 1 produces the doc content; Step 2 adds the tests that validate it.

#### Impact on Existing Tests
- None. This step adds a new file.

### Step 2: Add TestSchema_DocInSyncWithCode and TestSchema_MarkdownExamplesValidate
**Rationale:** The tests come last in commit order because they require the doc to exist, but they must be written to fail first on an empty-or-wrong doc — that's the RED phase. Land them in the same commit as the doc for atomicity.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/events/schema_test.go` | modify | Add two test functions + small helpers |

#### Tests to Write FIRST (RED phase)

**Test 1: `TestSchema_DocInSyncWithCode`** — reflection-based sync check between Go structs and the JSON Schema.

```go
// TestSchema_DocInSyncWithCode verifies that every exported field on the Go
// event structs in events.go appears in the corresponding schema definition in
// docs/events-schema/v0.1.json, and that every field without an ",omitempty"
// JSON tag is listed in that definition's "required" array.
//
// This test exists to prevent silent drift: when a developer adds, removes, or
// renames a field on an event struct, the test fails unless the schema file
// is updated to match.
func TestSchema_DocInSyncWithCode(t *testing.T) {
    // Load the raw JSON schema document (not the compiled jsonschema.Schema —
    // we need to introspect "required" and "properties" text directly).
    path := schemaPath(t)
    data, err := os.ReadFile(path)
    if err != nil { t.Fatalf("read schema: %v", err) }
    var root map[string]any
    if err := json.Unmarshal(data, &root); err != nil {
        t.Fatalf("unmarshal schema: %v", err)
    }
    defs, ok := root["definitions"].(map[string]any)
    if !ok { t.Fatal("schema missing definitions") }

    // Mapping: Go struct type -> schema definition name.
    cases := []struct {
        name       string
        structType reflect.Type
        defName    string
    }{
        {"RunStart", reflect.TypeOf(events.RunStart{}), "RunStart"},
        {"RunError", reflect.TypeOf(events.RunError{}), "RunError"},
        {"RequestStart", reflect.TypeOf(events.RequestStart{}), "RequestStart"},
        {"RequestEnd", reflect.TypeOf(events.RequestEnd{}), "RequestEnd"},
        {"AssertionResult", reflect.TypeOf(events.AssertionResult{}), "AssertionResult"},
        {"RunEnd", reflect.TypeOf(events.RunEnd{}), "RunEnd"},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            def, ok := defs[tc.defName].(map[string]any)
            if !ok {
                t.Fatalf("definition %q missing from schema", tc.defName)
            }

            wantProps, wantRequired := collectJSONFields(tc.structType)

            gotProps := keysOf(def["properties"])
            gotRequired := stringsOf(def["required"])

            if diff := diffStringSets(wantProps, gotProps); diff != "" {
                t.Errorf("%s: properties mismatch (struct vs schema):\n%s",
                    tc.defName, diff)
            }
            if diff := diffStringSets(wantRequired, gotRequired); diff != "" {
                t.Errorf("%s: required mismatch (struct vs schema):\n%s",
                    tc.defName, diff)
            }
        })
    }
}

// collectJSONFields walks an event struct (including its embedded Header) and
// returns the set of JSON field names and the subset that are required
// (no ",omitempty" tag).
func collectJSONFields(t reflect.Type) (all, required []string) { /* ... */ }

// keysOf returns the sorted key set of a JSON object value.
func keysOf(v any) []string { /* ... */ }

// stringsOf converts a JSON array value to a sorted []string.
func stringsOf(v any) []string { /* ... */ }

// diffStringSets returns a human-readable diff when two sets differ, or "" if
// equal. Used for failure messages only.
func diffStringSets(want, got []string) string { /* ... */ }
```

Table-driven test sub-cases (named above): `RunStart`, `RunError`, `RequestStart`, `RequestEnd`, `AssertionResult`, `RunEnd`.

**Test 2: `TestSchema_MarkdownExamplesValidate`** — extract fenced code blocks from the doc and validate against the compiled schema.

```go
// TestSchema_MarkdownExamplesValidate extracts every fenced code block tagged
// ```json or ```ndjson from docs/EVENTS_SCHEMA_v0.1.md and validates each
// non-blank line against the v0.1 JSON Schema.
//
// Ensures the examples embedded in the agent-facing documentation stay
// consistent with the checked-in schema. A broken example should fail fast.
func TestSchema_MarkdownExamplesValidate(t *testing.T) {
    sch := compileEventSchema(t)

    docPath := eventSchemaDocPath(t) // walks up to docs/EVENTS_SCHEMA_v0.1.md
    src, err := os.ReadFile(docPath)
    if err != nil { t.Fatalf("read doc: %v", err) }

    blocks := extractJSONFences(string(src))
    if len(blocks) == 0 {
        t.Fatal("no ```json or ```ndjson fences found — doc is missing examples")
    }

    for i, block := range blocks {
        t.Run(fmt.Sprintf("block_%d_%s_line_%d", i, block.lang, block.startLine), func(t *testing.T) {
            // Each block may contain multiple NDJSON lines.
            for _, line := range splitDocLines(block.body) {
                line = strings.TrimSpace(line)
                if line == "" { continue }
                validateLine(t, sch, line)
            }
        })
    }
}

// fenceBlock carries one fenced code block's metadata.
type fenceBlock struct {
    lang      string // "json" | "ndjson"
    startLine int    // 1-based, for error reporting
    body      string
}

// extractJSONFences scans src for ```json and ```ndjson fences and returns
// the contained bodies. Indentation inside the fence is preserved.
// Unknown languages are skipped.
func extractJSONFences(src string) []fenceBlock { /* ... */ }

// splitDocLines splits body on "\n" and returns the slice.
func splitDocLines(s string) []string { return strings.Split(s, "\n") }
```

Table-driven sub-cases (dynamic): one per fenced block, named `block_<i>_<lang>_line_<N>`.

**Helper: `eventSchemaDocPath`** — mirrors the `schemaPath` helper already in `schema_test.go`.

```go
// eventSchemaDocPath returns the absolute path to docs/EVENTS_SCHEMA_v0.1.md.
func eventSchemaDocPath(t *testing.T) string {
    t.Helper()
    _, thisFile, _, ok := runtime.Caller(0)
    if !ok { t.Fatal("runtime.Caller failed") }
    root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
    return filepath.Join(root, "docs", "EVENTS_SCHEMA_v0.1.md")
}
```

#### Impact on Existing Tests
- `schema_test.go` already imports `jsonschema/v6`, `os`, `filepath`, `runtime`, `bufio`, `strings`, `bytes`, `encoding/json`, `testing`, `time`. New additions: `reflect`, `fmt`. Both are stdlib.
- The existing `TestEmitter_AllKindsValidateAgainstSchema` and `TestEmitter_GoldenSchemaValidates` tests continue to pass unchanged.
- No existing test breaks.

### Step 3: Verify and commit
**Rationale:** Run the full event-package test suite plus build and lint before committing, so the RED-GREEN state is always committed atomically.

```bash
go build ./cmd/curlew
go test ./internal/output/events/...
~/go/bin/golangci-lint run ./internal/output/events/...
./smoke/run.sh
```

All must pass; on success, commit the doc and the tests together.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/output/events/schema_test.go` | `TestEmitter_AllKindsValidateAgainstSchema` | none | — |
| `internal/output/events/schema_test.go` | `TestEmitter_GoldenSchemaValidates` | none | — |
| `internal/output/events/schema_test.go` | `TestEmitter_GoldenRunHappy` | none | — |
| `internal/output/events/schema_test.go` | `TestEmitter_GoldenRunError` | none | — |
| `internal/output/events/schema_test.go` | `TestEmitter_GoldenRunFailedAssertion` | none | — |
| `internal/output/events/schema_test.go` | `TestSchema_DocInSyncWithCode` | new | add |
| `internal/output/events/schema_test.go` | `TestSchema_MarkdownExamplesValidate` | new | add |
| any other package | — | none | — |

## Risks and Edge Cases

- **Risk:** Reflection walks the `Header` embed and returns its fields, but the schema places `schema_version` / `run_id` / `id` / `at_ms` / `kind` on each concrete definition (via `allOf` + explicit repetition). → **Mitigation:** `collectJSONFields` must follow anonymous embedded struct fields and splice their JSON tags into the parent set. Schema comparison is performed against the concrete definition's `properties` / `required`, which already repeats the Header fields.
- **Risk:** A schema property has `additionalProperties: false` plus `const: "0.1"` / `const: "run.start"` style constraints; reflection only tells us the JSON key exists, not the const value. → **Mitigation:** Out of scope for this sync test. The const-value check is handled by `TestEmitter_AllKindsValidateAgainstSchema` which validates a real emitted event matches the schema.
- **Risk:** `EventError` is embedded by pointer (`*EventError`) in `RequestEnd` and as a value in `RunError`. → **Mitigation:** `collectJSONFields` dereferences pointer field types when producing the property-list entry for the parent. The `EventError` definition is itself validated in a separate sub-case.
- **Risk:** Extracting fenced code blocks by string parsing is brittle — indented fences, nested fences, escaped backticks. → **Mitigation:** Keep the fence parser minimal and strict: it recognises only lines matching `^```(json|ndjson)\s*$` (opening) and `^```\s*$` (closing). Anything exotic breaks the test loudly — which we want.
- **Risk:** A fenced block example deliberately shows an invalid event (e.g. demonstrating what *not* to emit). → **Mitigation:** Don't include such examples. Use prose to describe invalid shapes and tag them as inline code (`` ` ``) rather than fenced. The test enforces "every fenced example is valid". This is a documentation contract.
- **Edge case:** Markdown contains non-JSON fenced blocks (e.g. ` ```yaml ` or ` ```bash `). → **Handling:** `extractJSONFences` filters by language tag and skips everything else.
- **Edge case:** A fenced block is a multi-line single JSON object pretty-printed across lines. → **Handling:** The splitter treats blank lines as separators; pretty-printed JSON within one block validates as one object (collect lines until a blank line, then validate the joined string). The simpler approach: require every fenced block to be **either** a single single-line JSON object **or** NDJSON (one object per line). Document this rule in the doc itself. Test enforces it.
- **Edge case:** An agent reads the doc and only sees the examples — some examples must be self-sufficient (include all required fields). → **Handling:** The section-per-kind layout gives every kind a minimal example plus a maximal example. The "minimal" one uses only required fields.
- **Risk:** Running tests from different working directories breaks the `runtime.Caller`-based path resolution. → **Mitigation:** Already proven to work: the existing `schemaPath` helper uses the same pattern. Mirror it exactly.
- **Risk:** The error-code reference in the doc goes stale as new codes are added. → **Mitigation:** Scope note — the doc is a v0.1 snapshot. The stability policy states that "new error codes are additive and allowed in v0.x without a doc update"; the inventory is illustrative, not exhaustive. The doc explicitly points agents at `internal/errors` / `hints_init.go` as the source of truth. No sync test for the code list (would be churn-heavy and add no behavioural guarantee).

## Go Function Signatures (new, all in `internal/output/events/schema_test.go`)

```go
// In package events_test.

func TestSchema_DocInSyncWithCode(t *testing.T)
func TestSchema_MarkdownExamplesValidate(t *testing.T)

// Helpers (unexported).
func collectJSONFields(t reflect.Type) (all []string, required []string)
func keysOf(v any) []string
func stringsOf(v any) []string
func diffStringSets(want, got []string) string
func eventSchemaDocPath(t *testing.T) string
func extractJSONFences(src string) []fenceBlock
type fenceBlock struct { lang string; startLine int; body string }
```

No new exports from the `events` package itself. No changes to `events.go`, `emitter.go`, or `hints_init.go`.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
ls docs/EVENTS_SCHEMA_v0.1.md docs/events-schema/v0.1.json
go test -run TestSchema_DocInSyncWithCode ./internal/output/events/...
go test -run TestSchema_MarkdownExamplesValidate ./internal/output/events/...
```

All three must succeed.

## Open Questions (resolved by plan author)

- **Q: Should the JSON Schema itself be regenerated from code, or kept hand-authored?** → **A:** Hand-authored, with a reflective sync test. Preserving the hand-authored descriptions in the schema is worth more than generation purity, and the reflective test catches the only drift that causes breakage (missing or extra fields).
- **Q: Should the stability policy live in the doc or in a separate `STABILITY.md`?** → **A:** In the doc. Agents reading the schema need the stability contract adjacent; a separate file is one more thing to find. M6-007 will update this same file when promoting to v1.0 and rename it to `EVENTS_SCHEMA_v1.0.md`.
- **Q: Should the error-code reference table be exhaustive or illustrative?** → **A:** Illustrative. There are ~80 codes across the codebase and they grow; exhaustive reference is tooling-worthy but out of scope. The doc gives a per-category summary and canonical examples, then points to `internal/errors` for the full set. The sync test does **not** check code-list completeness.
- **Q: What NDJSON fence language tag should examples use — ` ```json ` or ` ```ndjson `?** → **A:** Single-object examples use ` ```json `. Multi-line streams (full-run samples) use ` ```ndjson `. The test accepts both.
- **Q: Should we pin the doc filename to `v0.1` or use `current`?** → **A:** Pin to `v0.1`. Versioned filenames match the JSON Schema convention and make archival trivial when v1.0 lands. M6-007 will create `v1.0.md` alongside, not in place of, `v0.1.md`.
