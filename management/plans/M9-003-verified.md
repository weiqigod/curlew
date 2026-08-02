# Verification Report: M9-003

**Task:** markdown content-type matrix, volatile-header discipline, 1 MiB body cap
**Verified by:** AI
**Date:** 2026-04-25
**Branch:** feature/M9-003-content-type-matrix
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 43 packages, all pass |
| `go test -race ./internal/output/markdown/...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage (`internal/output/markdown/...`) | 91.2% | Meets >= 80% threshold |
| `./scripts/ci-local.sh` | PASS | Go gate passed; backend/web/e2e not triggered |

## Observable Output

The task observable references `collections/` fixture files run against live
HTTP endpoints. These do not exist as static files — the plan (§ Verification)
states that integration tests in `cmd/apitest/run_test.go` spin up
`httptest.NewServer` instances to cover each Kind. All DoD-named tests (see
table below) serve as the authoritative observable verification. The full unit
+ integration test run command from the task YAML:

```
go test -run 'TestMarkdown_ContentType|TestMarkdown_Render_(Text|Empty|HEAD|Binary|YAML|XML|HTML)|TestMarkdown_BodyCap|TestMarkdown_VolatileHeaders|TestMarkdown_RedactionInvariant' ./...
```

Result: PASS (all 13 named tests pass, plus 27 sub-tests)

Expected: PASS
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `Classify(body, method, contentType) Kind` — HEAD wins, Empty before MIME, MIME parse, heuristic fallback | `TestMarkdown_ContentType`, `TestMarkdown_ContentType_FallbackSniff` | PASS |
| 2 | JSON bodies render via `json.Indent` in ```json fence; M9-002 pretty-printer reused | `TestMarkdown_Render_PassJSON`, `TestMarkdown_Render_FailJSON` | PASS |
| 3 | YAML bodies render via `yaml.Node` round-trip in ```yaml fence (stable ordering) | `TestMarkdown_Render_YAML` | PASS |
| 4 | XML bodies render verbatim in ```xml fence | `TestMarkdown_Render_XML` | PASS |
| 5 | HTML bodies render verbatim in ```html fence | `TestMarkdown_Render_HTML` | PASS |
| 6 | Plain text bodies render verbatim in ```text fence | `TestMarkdown_Render_Text` | PASS |
| 7 | Empty bodies render as `_(empty body)_` | `TestMarkdown_Render_Empty` | PASS |
| 8 | HEAD responses render as `_(HEAD — no body)_` | `TestMarkdown_Render_HEAD` | PASS |
| 9 | Binary bodies render as `hex.Dump` (512-byte preview) in ```hexdump fence + `_N bytes total_` footer | `TestMarkdown_Render_Binary` | PASS |
| 10 | 1 MiB cap applied post-redaction; order: redact → classify → truncate → format | `TestMarkdown_BodyCap`, `TestMarkdown_BodyCap_PostRedaction`, `TestMarkdown_Render_TruncatedBody` | PASS |
| 11 | Capped bodies get `_... truncated (body was N bytes, showing first 1048576)_` footer | `TestMarkdown_Render_TruncatedBody` | PASS |
| 12 | `### Response metadata` subsection below response body; Content-Type, headers alphabetical, post-redaction | `TestMarkdown_VolatileHeaders` | PASS |
| 13 | Volatile headers (Date, X-Request-Id, Set-Cookie, Etag, Server, Age) filtered from signal blocks, appear only in metadata | `TestMarkdown_VolatileHeaders`, `TestMarkdown_VolatileRequestHeaders` | PASS |
| 14 | Redaction invariant: `--allow-sensitive` does not leak secrets into markdown (formatter + pipeline) | `TestMarkdown_RedactionInvariant`, `TestRun_MarkdownFormat_RedactionInvariant` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — 43 packages PASS | PASS |
| 2 | TestMarkdown_ContentType passes | `--- PASS: TestMarkdown_ContentType (0.00s)` | PASS |
| 3 | TestMarkdown_ContentType_FallbackSniff passes | `--- PASS: TestMarkdown_ContentType_FallbackSniff (0.00s)` | PASS |
| 4 | TestMarkdown_Render_Text passes | `--- PASS: TestMarkdown_Render_Text (0.00s)` | PASS |
| 5 | TestMarkdown_Render_Empty passes | `--- PASS: TestMarkdown_Render_Empty (0.00s)` | PASS |
| 6 | TestMarkdown_Render_HEAD passes | `--- PASS: TestMarkdown_Render_HEAD (0.00s)` | PASS |
| 7 | TestMarkdown_Render_Binary passes | `--- PASS: TestMarkdown_Render_Binary (0.00s)` | PASS |
| 8 | TestMarkdown_Render_YAML passes | `--- PASS: TestMarkdown_Render_YAML (0.00s)` | PASS |
| 9 | TestMarkdown_Render_XML and TestMarkdown_Render_HTML pass | Both PASS | PASS |
| 10 | TestMarkdown_BodyCap passes: 1.1 MiB truncated with footer | `--- PASS: TestMarkdown_BodyCap (0.00s)` + `TestMarkdown_Render_TruncatedBody` | PASS |
| 11 | TestMarkdown_BodyCap_PostRedaction passes | `--- PASS: TestMarkdown_BodyCap_PostRedaction (0.00s)` | PASS |
| 12 | TestMarkdown_VolatileHeaders passes | `--- PASS: TestMarkdown_VolatileHeaders (0.00s)` | PASS |
| 13 | TestMarkdown_RedactionInvariant passes | `--- PASS: TestMarkdown_RedactionInvariant (0.00s)` | PASS |
| 14 | Regression: all M9-001 and M9-002 tests pass unmodified | `go test ./...` — all cached packages PASS | PASS |
| 15 | go test ./... passes with no regressions | 43 packages, 0 failures | PASS |
| 16 | go test -cover ./internal/output/markdown/... >= 80% | 91.2% | PASS |
| 17 | golangci-lint run passes with 0 issues | ci-local.sh lint gate PASS | PASS |
| 18 | ./smoke/run.sh passes | ci-local.sh smoke gate PASS | PASS |
| 19 | ./scripts/ci-local.sh passes | `=== ci-local PASS ===` | PASS |

## Code Review

| Check | Status | Notes |
|-------|--------|-------|
| Error handling | PASS | `fmt.Errorf("write %s.md: %w", ...)` in formatter.go. yaml encode errors trigger verbatim-fence fallback. |
| Naming conventions | PASS | No stuttering. Doc comments on all exports: `Kind`, `BodyCapBytes`, `IsVolatileHeader`, `Classify`. |
| Code organization | PASS | Three new single-concern files: `contenttype.go`, `bodycap.go`, `metadata.go`. |
| Test quality | PASS | Table-driven tests. TDD pattern visible in commit history. Integration tests via `httptest.NewServer`. |

Branch A: Review PASS trusted (verdict PASS in `management/reviews/M9-003-review.md`), spot-check clean.

## Commits

| Hash | Message |
|------|---------|
| `9b3c25e` | docs(plan): add implementation plan for M9-003 |
| `02d3e56` | chore(task): mark M9-003 as planned |
| `67fdc93` | chore(task): mark M9-003 as in_progress |
| `4f42408` | test(output): add failing tests for content-type classifier |
| `48df8c7` | feat(output): implement content-type classifier with Kind enum |
| `8532230` | test(output): add failing tests for body-cap helper |
| `f4adfd7` | feat(output): implement body-cap helper with 1 MiB default |
| `7bf35c6` | test(output): add failing tests for volatile-header set and metadata renderer |
| `3f3f5ff` | feat(output): implement volatile-header set and metadata renderer |
| `74fd347` | test(output): add failing tests for content-type renderers and volatile headers |
| `b02a5c2` | feat(output): wire content-type renderers, volatile-header discipline, body cap, metadata section into formatter |
| `f512dc4` | refactor(output): fix gofumpt alignment in formatter_test.go |
| `055cc3b` | chore(task): mark M9-003 as review |
| `8db970f` | docs(review): add review with findings for M9-003 |
| `aade119` | fix(markdown): rename cap param to limit in truncateForMarkdown |
| `4e95193` | test(markdown): add missing truncated-body and volatile-request-header tests |
| `533c2c4` | docs(review): add improvement report for M9-003 |
| `01f8bb3` | docs(review): add passing review for M9-003 |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/output/markdown/contenttype.go` | created | Kind enum + Classify function |
| `internal/output/markdown/contenttype_test.go` | created | TestMarkdown_ContentType + FallbackSniff |
| `internal/output/markdown/bodycap.go` | created | BodyCapBytes + truncateForMarkdown |
| `internal/output/markdown/bodycap_test.go` | created | TestMarkdown_BodyCap + PostRedaction |
| `internal/output/markdown/metadata.go` | created | volatileHeaders set + IsVolatileHeader + renderMetadata |
| `internal/output/markdown/metadata_test.go` | created | TestMarkdown_VolatileHeaderSet + RenderMetadata |
| `internal/output/markdown/formatter.go` | modified | renderBody dispatch, renderResponse, renderResponseMetadata, volatile filter in renderRequest |
| `internal/output/markdown/formatter_test.go` | modified | 9 new test functions + golden regen |
| `internal/output/markdown/testdata/golden/*.md` | modified | Regenerated to include ### Response metadata section |
| `cmd/apitest/main.go` | modified | Body cap integration in buildMarkdownReport |
| `cmd/apitest/run_test.go` | modified | TestRun_MarkdownFormat_RedactionInvariant added, HappyPath updated |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
