# Improvement Report: M1-015

**Task:** External request file references
**Date:** 2026-03-12
**Review:** management/reviews/M1-015-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `visited[absPath] = true` prevents valid reuse of same external file from multiple references | Removed line — external files are leaf nodes, only the collection's own path (seeded by `ParseFile`) is needed for self-reference detection. Added test for reuse with different variables. | ✓ tests pass |
| 2 | Medium | YAML parse error details discarded in `parseExternalFile` | Changed to double-`%w` wrapping (`fmt.Errorf("...: %w: %w", ErrInvalidYAML, err)`) matching `ParseFile`'s pattern | ✓ tests pass |
| 3 | Low | `ExternalRequest` exported but only used within parser package | Renamed to `externalRequest` (unexported) | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 91.1% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| fa73dce | refactor(parser): unexport ExternalRequest type | #3 |
| 9a104d0 | fix(parser): preserve YAML error details in parseExternalFile | #2 |
| 2dfcb41 | fix(parser): allow reusing same external file from multiple references | #1 |

## Summary
3/3 findings resolved. 0 deferred.
