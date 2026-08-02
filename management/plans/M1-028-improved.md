# Improvement Report: M1-028

**Task:** Feature gate framework
**Date:** 2026-03-19
**Review:** management/reviews/M1-028-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `WriteGateJSON` error silently discarded with `_ =` | Check error return and log to stderr, consistent with success-path JSON writes | ✓ tests pass |
| 2 | Low | Missing `--no-color` unit test for gated commands (`parseGatedArgs` at 88.9% coverage) | Added `"vault --no-color still exits 6"` test case to `TestGatedCmd_vault`, bringing `parseGatedArgs` to 100% coverage | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 90.8% total, 100% internal/auth, 100% parseGatedArgs |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| d2f611b | fix(gate): handle WriteGateJSON error and add --no-color test | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
