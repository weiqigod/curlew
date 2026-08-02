# Improvement Report: M1-021

**Task:** TAP output format (--format tap)
**Date:** 2026-03-16
**Review:** management/reviews/M1-021-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `summary.Passed`/`summary.Failed` accessed before nil-check in TAP block (main.go:269-270) | Added `var passed, failed int` with `if summary != nil` guard before field reads, consistent with the JSON block pattern | ✓ tests pass |
| 2 | Low | Empty-collection TAP write error silently discarded, returning exit 0 with potentially corrupt output (main.go:163) | Changed `_ = output.WriteTAP(...)` to capture and log write errors to stderr before returning | ✓ tests pass |
| 3 | Low | Missing test for `TAPResult{Passed:false}` with no Error and no Failures (empty diagnostic block) | Added `TestWriteTAP/failing_request_with_no_error_and_no_failures_emits_empty_diagnostic_block` subcase | ✓ tests pass |
| 4 | Low | `sanitizeTAPName` leaves double-space after stripping `#` (e.g. `"req # with hash"` → `"req  with hash"`) | Replaced `strings.TrimSpace` with `strings.Join(strings.Fields(name), " ")` to collapse internal whitespace runs | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage `internal/output` | 87.6% |
| Coverage `cmd/apitest` | 85.1% |
| Coverage overall | 91.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 051033b | fix(output): normalize whitespace in sanitizeTAPName and add empty-diagnostic test | #3, #4 |
| 591d3ba | fix(cli): guard summary nil-check before field access and log TAP write errors | #1, #2 |

## Summary

4/4 findings resolved. 0 deferred.
