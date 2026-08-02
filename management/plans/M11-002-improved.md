# Improvement Report: M11-002

**Task:** JSON output completeness: per-iteration data-driven entries + root summary block
**Date:** 2026-04-26
**Review:** management/reviews/M11-002-review.md

## Resolved Findings (iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `json:"summary,omitempty"` tag on `JSONOutput.Summary` was misleading: `buildSummaryJSON` guarantees a non-nil pointer so `omitempty` never fires in practice, contradicting the documented "always populated" contract. | Removed `omitempty` from the tag (`json:"summary"` plain), added inline comment pointing to `buildSummaryJSON`. Updated `TestWriteJSON_Summary` "summary omitted when nil" case to document that a nil pointer (unreachable from production code) now serialises as `"summary": null` rather than being absent. | ✓ tests pass |

## Resolved Findings (iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `exec --dry-run --format json` emitted `"summary": null` — dry-run path at `main.go:3330` constructed `&output.JSONOutput{}` without setting `Summary`, violating spec "never null" promise. | Added `Summary: buildSummaryJSON(nil)` to the dry-run `JSONOutput` literal so all JSON output paths (run, exec, exec dry-run) honour the always-non-null contract. | ✓ tests pass |
| 2 | Low | `TestExecCmd_dryRunJsonSchema` lacked assertion on `out.Summary != nil`, leaving the dry-run regression undetected. | Added `if out.Summary == nil { t.Error(...) }` assertion to `TestExecCmd_dryRunJsonSchema`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`cmd/apitest`) | 81.7% |
| Coverage (`internal/output`) | 93.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 873c633 | fix(output): remove omitempty from Summary field, update test to match | iter1 #1 |
| f33f3ee | fix(exec): populate Summary in dry-run JSON output path | iter2 #1, #2 |

## Summary

3/3 total findings resolved across 2 improvement iterations. 0 deferred.
