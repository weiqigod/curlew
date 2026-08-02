# Improvement Report: M7-004

**Task:** CI regression gate: stream-discipline matrix across commands and formats
**Date:** 2026-04-22
**Review:** management/reviews/M7-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `run_html_happy_with_report` cell only asserted `stdoutIsEmpty` but never read the generated HTML file to verify content — Behavior 4 and plan Decision 3 markers (`<!DOCTYPE html>`, `curlew report`, `status-bar`, `summary-card`) unverified | Added `assertHTMLReportContent` helper that reads the on-disk file via `os.ReadFile` and checks all four structural markers. Added `checkHTMLFile string` field to the case struct and `htmlReportPath` variable threaded from the `--report` arg into the post-run assertion. | ✓ tests pass |
| 2 | Low | `wantExitCode: -1` comment claimed "any non-zero" but the guard `if tc.wantExitCode >= 0` silently skips the check entirely for -1, not asserting non-zero — false expectation for future authors | Updated comment to `-1 = skip exit-code check (any code accepted)` to accurately describe the guard's behaviour | ✓ tests pass |
| 3 | Low | `assertValidJSONRunOutput` lacked a doc comment explaining why `total`/`passed` fields (from Behavior 1 in the task YAML) are not checked — misleading spec-vs-implementation gap | Added doc comment cross-referencing plan Decision 2: single-collection JSON output uses `name/status/duration_ms/requests`; `total`/`passed` only appear in MultiJSONOutput (glob mode) | ✓ tests pass |
| 4 | Low | `tc := tc // capture loop variable` is dead code in Go 1.24 (per-iteration loop variable semantics since Go 1.22) | Removed the redundant capture line | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 86.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| fa99be5 | fix(test): resolve M7-004 review findings in stream_discipline_matrix_test | #1, #2, #3, #4 |

## Summary
4/4 findings resolved. 0 deferred.
