# Improvement Report: M6-006

**Task:** Event schema documentation and stability policy
**Date:** 2026-04-22
**Review:** management/reviews/M6-006-review.md

## Resolved Findings

### Iteration 1 (finding from first review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Section heading "Error codes by producing package" did not match content (organised by category, not Go package). Tables lacked a "Producing package" column. | Renamed section heading to "Error codes by category". Added a "Producing package" column to every error-code table. Corrected stale/invented codes to match actual registry entries. | ✓ tests pass |

### Iteration 2 (finding from second review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Table of Contents entry at line 25 still read "Error codes by producing package" with anchor `#error-codes-by-producing-package`, but the actual section heading was renamed to "Error codes by category" (anchor `#error-codes-by-category`). The ToC link was broken. | Updated line 25 from `[Error codes by producing package](#error-codes-by-producing-package)` to `[Error codes by category](#error-codes-by-category)` to match the renamed section heading. | ✓ tests pass |

### Iteration 3 (finding from third review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestSchema_DocInSyncWithCode` did not include `EventError` in its `cases` slice. The plan explicitly stated "The `EventError` definition is itself validated in a separate sub-case" but that case was never added. Optional-field drift in `EventError` would go undetected. | Added `{"EventError", reflect.TypeOf(events.EventError{}), "EventError"}` to the `cases` slice. The new `EventError` sub-case now runs and verifies that all 6 EventError fields (category, code, message, hint, file, line) are present in `definitions/EventError`, and that required (non-omitempty) fields appear in the `required` array. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | 95.7% (`internal/output/events`) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| a70e930 | fix(docs): rename section heading and add producing-package column | Iteration 1 #1 |
| 25be375 | fix(docs): correct stale ToC anchor for error codes section | Iteration 2 #1 |
| 414eb48 | fix(events): add EventError to TestSchema_DocInSyncWithCode cases | Iteration 3 #1 |

## Summary
3/3 total findings resolved across 3 improvement iterations. 0 deferred.
