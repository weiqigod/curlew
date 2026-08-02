# Improvement Report: M9-005

**Task:** markdown shipment: init --output flag, docs, CHANGELOG, IMPROVEMENT.md W4 status
**Date:** 2026-04-25
**Review:** management/reviews/M9-005-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Behavior 2 in task YAML says "no output: block" but implementation keeps the M8-003 `output: { format: terminal, verbosity: normal }` baseline — spec text and implementation disagree. | Updated behavior 2 in `management/tasks/M9-005.yaml` to reflect the "keep M8-003 baseline" decision: wording now reads "produces the same scaffold as the M8-003 baseline" and clarifies that no markdown-related output block is present. Spec and implementation are now consistent. | ✓ tests pass |
| 2 | Medium | DoD item "config validator accepts end-to-end" not satisfied — `TestInit_OutputAllFormats` only does string-substring checks, no schema validation. No markdown fixtures in `internal/schema/testdata/`. | Added `TestSchema_scaffolded_all_output_formats_validate` in `internal/schema/validate_test.go` calling `schema.Validate()` on each format's scaffolded `curlew.yaml` against the project schema. Added `output_collection_markdown.yaml` and `output_project_markdown.yaml` fixtures in `internal/schema/testdata/`; registered both in `TestSchema_accepts_output`. | ✓ tests pass |
| 3 | Low | `outputBlock` default branch untested — silent fallback to terminal block invisible when a new format is added without a corresponding `case`. | Added `TestOutputBlock_DefaultFallback` in `internal/scaffold/scaffold_test.go` calling `outputBlock("unrecognised-format")` directly and asserting the defensive fallback. `outputBlock` coverage is now 100%. | ✓ tests pass |
| 4 | Low | CHANGELOG observable awk range `/^## \[/` terminates on the opening `## [Unreleased]` line — grep always returned empty. | Fixed the closing range pattern to `/^## \[0/` in `management/tasks/M9-005.yaml`. Verified the fixed expression returns the expected two matching lines. | ✓ observable verified |
| 5 | Low | W4 status observable `grep -A 1` does not reach the `**Status:**` line (blank line between heading and Status). | Changed to `grep -A 2 … \| head -3` in `management/tasks/M9-005.yaml`. Verified the fixed expression returns the Status: Shipped line. | ✓ observable verified |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/scaffold`) | 83.9% |
| Coverage (`cmd/curlew`) | 81.3% |
| Coverage (total) | 86.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6658557 | fix(tasks): align M9-005 spec with implementation and fix broken observables | #1, #4, #5 |
| 8a586a0 | fix(scaffold,schema): add schema validation tests and markdown fixtures | #2, #3 |

## Summary
5/5 findings resolved. 0 deferred.
