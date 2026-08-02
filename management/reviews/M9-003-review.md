# Code Review: M9-003

**Task:** markdown content-type matrix, volatile-header discipline, 1 MiB body cap
**Reviewer:** AI
**Date:** 2026-04-25
**Branch:** feature/M9-003-content-type-matrix

## Verdict: PASS

## Findings

_No findings. All prior review findings resolved. No new issues identified._

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No errors swallowed. `fmt.Errorf("context: %w", err)` used in `WriteReport`. `json.Marshal` on in-memory data correctly ignores error. `yaml.Unmarshal`/`enc.Encode` errors trigger verbatim-fence fallback. `enc.Close` discard is correct (yaml.v3 docs: always nil after Encode). |
| Input Validation | PASS | `Classify` handles nil/empty body, empty method, empty/unparseable contentType. `truncateForMarkdown` handles empty body and body at/below cap. `renderMetadata` handles nil/empty header map. `lookupHeader` handles nil map. `renderRequest` handles nil RequestBody. |
| Naming | PASS | All exported symbols have doc comments (`Kind`, `BodyCapBytes`, `IsVolatileHeader`, `Classify`). Inline comments on iota constants are standard Go practice. `cap` builtin shadowing fixed in Iteration 1 (parameter renamed to `limit`). No stuttering. |
| Code Organization | PASS | Three new files each own a single concern: `contenttype.go` (classifier), `bodycap.go` (cap helper), `metadata.go` (volatile set + renderer). No circular deps. `defer` not needed (no file handles or connections in changed code). No unused imports or variables. Exported surface is minimal. |
| Correctness | PASS | Classifier signal hierarchy matches spec (HEAD → Empty → MIME → heuristic). YAML round-trip uses `yaml.Node` to preserve source ordering (plan risk item). Double-redaction in `main.go` (general pass at line 1442, markdown-specific pass at 1619) is intentional and correct. Body cap order: redact (main.go) → cap → classify → render (correct). Binary `_N bytes total_` footer uses `original` (pre-cap length), not `len(capped)`. Volatile header filtering correctly applied in `renderRequest` but NOT needed in `renderResponse` (response signal block contains body only, no raw headers). `renderMetadata` receives all headers unfiltered — correct since metadata section is the intended home for volatile headers. |
| Test Quality | PASS | All DoD-named tests present and passing. Three findings from Iteration 1 all resolved: (1) `TestMarkdown_Render_TruncatedBody` exercises the `renderBody` truncated branch end-to-end; (2) `TestMarkdown_VolatileRequestHeaders` proves request-side volatile filtering; (3) `cap` → `limit` rename. Coverage 91.2% exceeds 80% threshold. Integration test `TestRun_MarkdownFormat_RedactionInvariant` covers the full --allow-sensitive pipeline. |

## Test Coverage
- Coverage: 91.2% (`internal/output/markdown/...`, via `go test -coverprofile`)
- Previously uncovered branches now covered: `renderBody` truncated path (Finding #1), `renderRequest` IsVolatileHeader path (Finding #2)
- Remaining sub-100% paths: `renderYAMLBody` encode-error fallback (78.6%), `parseSentinels` (66.7%), `writeAtomic` (56.2%), `filePath("")` branch (66.7%) — all are either error injection paths or legacy paths outside M9-003 scope, and overall coverage remains well above 80%

## DoD Compliance

| DoD Item | Status |
|----------|--------|
| All behavior tests pass | PASS |
| TestMarkdown_ContentType passes | PASS |
| TestMarkdown_ContentType_FallbackSniff passes | PASS |
| TestMarkdown_Render_Text passes | PASS |
| TestMarkdown_Render_Empty passes | PASS |
| TestMarkdown_Render_HEAD passes | PASS |
| TestMarkdown_Render_Binary passes | PASS |
| TestMarkdown_Render_YAML passes | PASS |
| TestMarkdown_Render_XML and TestMarkdown_Render_HTML pass | PASS |
| TestMarkdown_BodyCap passes: 1.1 MiB body truncated with footer marker | PASS (TestMarkdown_Render_TruncatedBody added) |
| TestMarkdown_BodyCap_PostRedaction passes | PASS |
| TestMarkdown_VolatileHeaders passes | PASS |
| TestMarkdown_RedactionInvariant passes | PASS |
| Regression: all M9-001 and M9-002 tests pass | PASS |
| go test ./... passes with no regressions | PASS |
| go test -cover ./internal/output/markdown/... >= 80% | PASS (91.2%) |
| golangci-lint run passes with 0 issues | PASS |
| ./smoke/run.sh passes | PASS |
| ./scripts/ci-local.sh passes | PASS |

## Summary

The implementation is architecturally sound and all three findings from the Iteration 1 review have been resolved. The content-type classifier, body-cap helper, volatile-header set, and formatter integration are correct. Error handling is clean. YAML canonicalisation uses `yaml.Node` to preserve source ordering. The redaction invariant is enforced both at the formatter level (unit test) and at the pipeline level (integration test). Coverage is 91.2%, exceeding the 80% threshold, with the remaining uncovered paths being error-injection paths outside M9-003 scope.
