# Improvement Report: M2-014

**Task:** Retry configuration precedence (global, collection, section, request)
**Date:** 2026-04-02
**Review:** management/reviews/M2-014-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `%s` instead of `%w` for inner error in defaults parsing (line 109) | Changed to `%w` so inner YAML error is part of the error chain | ✓ tests pass |
| 1b | Low | Same `%s` pattern in auth_profiles parsing (line 81, pre-existing) | Fixed for consistency per review recommendation | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 90.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 25ce3cc | fix(config): use %w instead of %s for wrapped errors in project config | #1, #1b |

## Summary
1/1 findings resolved (plus 1 pre-existing fixed for consistency). 0 deferred.
