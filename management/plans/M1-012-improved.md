# Improvement Report: M1-012

**Task:** Environment files with --env flag
**Date:** 2026-03-12
**Review:** management/reviews/M1-012-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Doc comment on `flatten` says "dot-separated keys" but implementation uses underscore-separated keys | Changed comment to "underscore-separated keys" | tests pass |
| 2 | Low | Custom `contains`/`containsSubstring` helpers re-implement `strings.Contains` | Replaced with `strings.Contains`, removed custom helpers | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 93.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| b94713d | fix(config): correct doc comment and use stdlib strings.Contains | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
