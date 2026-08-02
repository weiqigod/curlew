# Improvement Report: M1-026

**Task:** AI exec command
**Date:** 2026-03-19
**Review round:** 2
**Review:** management/reviews/M1-026-review.md

## Resolved Findings

### Round 1

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `AppendJSONL` deferred close modifies local `err` but no named return — close errors silently swallowed | Changed signature to `(err error)`, renamed OpenFile error to `openErr` to avoid shadowing named return | ✓ tests pass |
| 2 | Low | `StatusCode`/`DurationMs` lack `omitempty` — dry-run entries show `"status_code":0,"duration_ms":0` | Added `omitempty` to both struct tags; added test verifying dry-run entries omit zero-valued fields | ✓ tests pass |

### Round 2

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Dry-run JSON path constructs `JSONRequest` without initializing `Assertions` field — produces `"assertions":null` instead of `"assertions":[]`, breaking schema compatibility with `run` command | Initialized `Assertions` to `make([]output.JSONAssertion, 0)` in dry-run `JSONRequest` literal; added `TestExecCmd_dryRunJsonSchema` regression test | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 90.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| fa1a571 | fix(output): omit zero-valued status_code/duration_ms in JSONL entries | Round 1 #2 |
| e1a27af | fix(output): use named return in AppendJSONL for close-error propagation | Round 1 #1 |
| 8a00af5 | fix(exec): initialize Assertions slice in dry-run JSON output | Round 2 #1 |

## Summary
3/3 findings resolved across 2 review rounds. 0 deferred.
