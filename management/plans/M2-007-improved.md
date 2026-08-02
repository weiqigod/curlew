# Improvement Report: M2-007

**Task:** Vault integration with runner and variable precedence
**Date:** 2026-03-27
**Review:** management/reviews/M2-007-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `resolve_uses_bulk_fetch_for_multiple_distinct_paths` verified result values but not BulkFetch call count — B6 "single API call" contract not directly asserted | Extracted unexported `resolveSecrets(ctx, provider, refs)` helper from `Resolve`; added `mockProvider` struct with `bulkFetchCount`; updated test to call `resolveSecrets` with mock and assert `bulkFetchCount == 1` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (vault) | 96.0% |
| Coverage (runner) | 90.0% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c34c476 | fix(vault): assert BulkFetch call count in B6 test via mock provider | #1 |

## Summary
1/1 findings resolved. 0 deferred.
