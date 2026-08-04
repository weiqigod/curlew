# Code Review: M21-001

**Task:** Collection/project JSON Schema completeness: 9 parser fields missing, editors flag valid collections
**Reviewer:** AI
**Date:** 2026-08-04
**Branch:** fix/M21-001-schema-completeness

## Verdict: FAIL

Four findings, none critical. The functional change is correct and verified from
three independent directions; the findings are test-strength and clarity issues.

## Pre-audit Gate

`./scripts/ci-local.sh --go` — **PASS** (build, `go test`, race, lint, smoke).

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Medium | Test Quality | `internal/parser/closedsets_test.go` | 78 | `TestParser_protocol_hint_lists_only_accepted_values` guards only the literal string `https`. Its second loop reduces to "the hint must not contain the word https", so reintroducing any *other* rejected protocol (`ws`, `grpc`, `http2`) into the hint passes. The test is named as a general guard but implements a single-value one. | Check a set of plausible-but-rejected protocol values, not one hardcoded string. |
| 2 | Low | Code Organization | `internal/schema/parity_test.go` | 195-196 | `collectionSchemaBytes()` and `projectSchemaBytes()` are one-line wrappers returning `schema.CollectionSchema` / `schema.ProjectSchema` unchanged. They add a layer of indirection over an already-exported package var and earn nothing. | Inline them and reference the schema vars directly. |
| 3 | Low | Correctness | `internal/schema/validate_coverage_test.go` | 233 | `collectionAround` mutates its argument in place (via `setdefault`-style writes) *and* returns a new wrapper map. A reader at the call site sees only the return value, so the mutation is invisible. It is safe today only because every call site passes a fresh literal. | Build and return a copy rather than writing into the caller's map. |
| 4 | Low | Spec Compliance | — | — | Behaviour 4 says the `$id` "returns the schema rather than 404 when resolved over HTTP". `TestSchema_ids_match_module_path` pins the string but cannot resolve it; nothing in the repo records the post-push HTTP check, so the behaviour is only half-verified and the other half is undocumented. | Record the `curl` check in the verification report as an explicit post-merge step. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error paths. The four extracted comparisons preserve their existing `apierrors.Structured` returns, sentinels, and categories exactly; hints are now derived from the same lists they describe. |
| Input Validation | PASS | `inClosedSet` treats `""` as unset, matching each original `x != "" && x != …` guard. WebSocket `action` deliberately uses bare `slices.Contains` because the original `switch`/`default` rejected `""`, and that rejection is preserved (covered by `TestParser_websocket_closed_sets`). |
| Naming | PASS | `SupportedProtocols`, `WebSocketActions`, `WebSocketBackoffStrategies`, `GraphQLErrorHandlingValues` all carry doc comments, no stuttering, and follow the existing `output.SupportedFormats` precedent. |
| Code Organization | PASS with finding #2 | No new package dependencies. `internal/parser` does not import `internal/schema`, so the parity test in `schema_test` creates no cycle. |
| Correctness | PASS | Behaviour-preserving refactor confirmed by the pre-existing parser suite plus new closed-set tests. No concurrency, no resources, no context plumbing touched. |
| Test Quality | FAIL — finding #1 | Table-driven throughout with `t.Run` subtests and explicit `error case -` rows. Weakness is #1's narrow assertion. |

## Verification Independence

The schema change is confirmed from three directions that do not share a code path:

1. **Go tests** — reflection parity against the parser structs, plus
   `santhosh-tekuri/jsonschema` validation of seven fixtures.
2. **An independent validator** — Python `jsonschema` `Draft202012Validator`
   accepts the task's own observable document with 0 errors, and still rejects
   six typo cases, proving `additionalProperties: false` was not weakened to buy
   the completeness.
3. **The real parser** — `TestSchema_dod_fixture_parses` runs the
   all-nine-fields fixture through `parser.ParseFile` and asserts the decoded
   values, so schema and parser agree on a document rather than a name list.

## Behaviour Coverage

| # | Behaviour | Covering test | Status |
|---|-----------|---------------|--------|
| 1 | Collection using the nine fields validates cleanly | `TestSchema_accepts` (7 fixtures), `TestSchema_examples`, observable 2 | ✅ |
| 2 | Every yaml-tagged parser field is described | `TestSchema_parser_fields_are_described` (+ converse and table-completeness guards) | ✅ |
| 3 | Committed files match `curlew schema` output | `TestSchema_published_path_matches_embed`, `TestSchema_project_published_path_matches_embed` | ⚠️ tautological — see below |
| 4 | `$id` resolves rather than 404s | `TestSchema_ids_match_module_path` (string only) | ⚠️ finding #4 |
| 5 | MANUAL §1.5 caveat corrected | prose; no automated guard | ✅ (manual) |

**On behaviour 3.** `curlew schema` writes the `go:embed`ed bytes of the very
file being diffed, and `go test` recompiles the embed from that file, so this
check cannot fail. It was already satisfied before this task. It is left in place
because the DoD names it, and it is *not* the recurrence guard — that is
`internal/schema/parity_test.go`. Called out here so the sign-off is honest
rather than implied.

## Test Coverage

- Total: **85.0%** (threshold 80%)
- New test files: `internal/schema/parity_test.go`, `internal/schema/astsource_test.go`,
  `internal/parser/closedsets_test.go`
- Missing coverage: none introduced. The schema files are data, not statements.

## Summary

The defect is fixed and, more importantly, made non-recurring: reflection parity
in both directions, table-completeness so a new nested struct cannot slip past,
`go/ast` guards pinning the two tagless decoders, and enum pinning across five
closed sets. Scope grew beyond the nine fields by three verified defects of the
same class — `$defs.request` requiring `method`, `project-v1.json` omitting
`config:`, and an error hint offering a protocol the parser rejects — each fixed
with its own test. The four findings are test-strength and clarity issues in code
this task added; none affects shipped behaviour.
