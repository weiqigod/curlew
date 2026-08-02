# Improvement Report: M2-018

**Task:** Parallel execution output formatting (terminal, JSON, TAP)
**Date:** 2026-04-08
**Review:** management/reviews/M2-018-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `ParallelSummary` has unused `totalRequests int` parameter | Removed parameter from signature, updated all call sites (terminal.go, terminal_test.go, main.go) | ✓ tests pass |
| 2 | Low | "no speedup for single wave" test doesn't assert `Speedup:` is absent | Added `wantAbsent` field to test struct and assertion loop; single-wave case now verifies `Speedup:` is not in output | ✓ tests pass |
| 3 | Low | `tapIntPtr` duplicates `intPtr` from json_test.go in the same package | Removed `tapIntPtr`, replaced all usages with existing `intPtr` helper | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 90.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| f6641bc | fix(output): resolve review findings for parallel output formatting | #1, #2, #3 |

## Summary
3/3 findings resolved. 0 deferred.
