# Improvement Report: M9-003

**Task:** markdown content-type matrix, volatile-header discipline, 1 MiB body cap
**Date:** 2026-04-25
**Review:** management/reviews/M9-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `renderBody` truncated branch (lines 270-272) never exercised by a test; DoD item "TestMarkdown_BodyCap passes: 1.1 MiB body truncated to 1 MiB with footer marker" not met at the formatter output level | Added `TestMarkdown_Render_TruncatedBody` which builds a 1.1 MiB text body, calls `WriteReport`, reads the output file, and asserts the `_... truncated (body was N bytes, showing first 1048576)_` marker is present alongside a `text` fence. `renderBody` now reaches 100% coverage. | ✓ tests pass |
| 2 | Medium | Request-side volatile header filtering (`IsVolatileHeader` call in `renderRequest` line 218) present in code but not assertion-verified by any test | Added `TestMarkdown_VolatileRequestHeaders` which sets `X-Request-Id` in `entry.RequestHdr`, calls `WriteReport`, extracts the `### Request` section, and asserts the volatile header is absent while non-volatile headers (`Authorization`, `Accept`) are present. | ✓ tests pass |
| 3 | Low | Parameter `cap int` in `truncateForMarkdown` (bodycap.go:13) shadows the predeclared builtin `cap()` | Renamed parameter from `cap` to `limit` throughout `truncateForMarkdown`. Doc comment updated to match. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/output/markdown/...`) | 91.2% (up from 90.6%) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| aade119 | fix(markdown): rename cap param to limit in truncateForMarkdown | #3 |
| 4e95193 | test(markdown): add missing truncated-body and volatile-request-header tests | #1, #2 |

## Summary

3/3 findings resolved. 0 deferred.
