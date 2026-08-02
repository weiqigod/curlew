# Improvement Report: M2-027 (Iteration 2)

**Task:** HTML report generation
**Date:** 2026-04-09
**Review:** management/reviews/M2-027-review.md (Iteration 2)

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Empty collection path silently discards `writeHTMLFile` error at line 279 (`_ = writeHTMLFile(report, emptyReport)`). User gets exit code 0 with no failure indication if report file cannot be created. | Replaced `_ = writeHTMLFile(...)` with proper error checking: `if writeErr := writeHTMLFile(report, emptyReport); writeErr != nil { errOut.StructuredError(...); return 1, nil }`. Added `TestRunCmd_format_html_empty_collection_write_error` test. | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (total) | 89.1% |
| Coverage (`internal/output`) | 92.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 9e3b1b8 | fix(output): check writeHTMLFile error in empty collection path | #1 |

## Previous Iteration Fixes (Iteration 1)

All 5 findings from the iteration 1 review were resolved in iteration 1:
1. `tmpl.Execute` error wrapping (646988a)
2. `os.Create` error wrapping (cb48e40)
3. Missing `WriteHTML` error path test (646988a)
4. Smoke test referencing deleted directory (50deb3d)
5. Nil report guard (646988a)

## Summary
1/1 findings resolved (iteration 2). 0 deferred.
6/6 total findings resolved across both iterations.
