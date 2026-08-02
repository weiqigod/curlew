# Code Review: M3-006

**Task:** openapi import: headers, request bodies, and status assertions
**Reviewer:** AI
**Date:** 2026-04-14
**Branch:** feature/M3-006-openapi-headers-bodies-assertions
**Round:** 3 (post-improve round 2)

## Verdict: PASS

## Findings

No findings.

## Resolved Findings History

### Round 1 (5 findings — all resolved)

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | High | Smoke test `grep -q "name:"` always passes (matched collection name field, not body) | Replaced with `grep -q "body:" && grep -q "name: string"` |
| 2 | Medium | Behavior 4 named `examples` map path completely untested | Added `TestImport_RequestBody_FromNamedExamples` |
| 3 | Medium | Named examples map iterated non-deterministically | Keys sorted with `sort.Strings(exKeys)` before ranging |
| 4 | Low | Fallback content-type loop untested | Added `TestImport_RequestBody_FallbackContentType` |
| 5 | Low | `strRef(s string)` had unused parameter | Removed parameter; updated all call-sites |

### Round 2 (1 finding — resolved)

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | Low | Content-type fallback loop iterated `map[string]*MediaType` without sorting — non-deterministic when multiple json-variant types present | Collect keys, `sort.Strings`, iterate in order; added `TestImport_RequestBody_FallbackContentType_Deterministic` (20 iterations) |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `ErrSpecInvalid` and `ErrNoOperations` sentinels; all `fmt.Errorf` use `%w`; `apierrors.Structured.Unwrap()` returns `Inner` enabling `errors.Is` traversal; no swallowed errors; no panics |
| Input Validation | PASS | nil-guards on all `SchemaRef`, `PathItem`, `Operation`, `Parameter` values; missing file, invalid YAML, and empty spec all return structured errors |
| Naming | PASS | No stuttering; all exported symbols (`Import`, `Emit`, `ErrSpecInvalid`, `ErrNoOperations`) have doc comments; unexported helpers are concise and descriptive |
| Code Organization | PASS | `schema.go` owns schema walking, `emit.go` owns YAML output, `import.go` owns the pipeline, `errors.go` owns sentinels; `internal/` boundaries respected; no circular deps |
| Correctness | PASS | All map iterations over non-trivial sets are sorted (paths, methods, parameters, examples, properties, status codes, content-type keys); cycle detection via cloned visited set; sibling-branch independence verified |
| Test Quality | PASS | All 8 behaviors covered; both happy-path and error-path tested; table-driven tests with `t.Run()`; integration fixture (`petstore_full.yaml`) round-trips through parser and validator; race detector passes |

## Test Coverage

- Package coverage: **96.5%** (well above 80% threshold)
- Accepted gaps (documented):
  - `stderrWarn` 0.0% — thin one-liner, exercised at integration level via `Import()` but tests use `importWithWarn` seam
  - `Emit` 92.9% — deferred `enc.Close()` error branch; explicitly commented in source as accepted gap
  - Remaining sub-100% functions (94–98%) cover minor nil-path and unmatched-switch branches

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| 1. Header params → `headers` + collection var | `TestImport_HeaderParameters`, `TestImport_SharedHeaderDedup`, `TestImport_FullFixture` | PASS |
| 2. Query params → URL + collection var | `TestImport_QueryParameters`, `TestImport_FullFixture` | PASS |
| 3. JSON body from schema placeholder | `TestImport_RequestBody_FromSchema`, `TestImport_FullFixture` | PASS |
| 4. Body from `example`/`examples` field | `TestImport_RequestBody_FromExample`, `TestImport_RequestBody_FromNamedExamples` | PASS |
| 5. Status assertions from responses | `TestImport_StatusAssertions`, `TestImport_FullFixture` | PASS |
| 6. `$ref` resolution before body generation | `TestImport_FullFixture` (uses `$ref: '#/components/schemas/NewPet'`, verifies `body["name"]` present) | PASS |
| 7. Shared header param deduplication | `TestImport_SharedHeaderDedup`, `TestImport_FullFixture` | PASS |
| 8. Recursive `$ref` cycle detection + warning | `TestImport_CycleWarning`, `TestWalkSchema_CycleWarn` | PASS |

## Quality Gates

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./internal/openapi/...` | PASS |
| `go test -race ./internal/openapi/...` | PASS |
| `golangci-lint run ./internal/openapi/...` | PASS — 0 issues |
| Coverage | PASS — 96.5% |
| CHANGELOG.md updated | PASS |
| Help text updated | PASS |
| Smoke test updated | PASS |

## Summary

All six findings from the prior two review rounds have been resolved. The implementation correctly handles all eight task behaviors: header and query parameter extraction with snake_case variable names, shared-parameter deduplication, schema-walked request body placeholders with example preference, $ref cycle detection with warn-once semantics, and status code aggregation. The code is deterministic across all map iterations, well-tested at 96.5% coverage, lint-clean, and race-free.

→ Run `/verify M3-006` to complete the task.
