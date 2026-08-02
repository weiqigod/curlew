# Improvement Report: M1-024

**Task:** Init command (apitest init)
**Date:** 2026-03-17
**Review:** management/reviews/M1-024-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `fmt.Errorf("%w", ErrProjectExists)` wraps sentinel with no added message — needless allocation | Replaced with `return ErrProjectExists` directly | ✓ tests pass |
| 2 | Low | Bare `os.WriteFile` return in `ensureGitignore` append path lacks operation context, inconsistent with `writeFile` helper | Wrapped with `fmt.Errorf("appending to .gitignore: %w", err)` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/scaffold`) | 80.9% |
| Coverage (`cmd/apitest`) | 86.5% |
| Coverage (total) | 91.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 7d4398e | fix(scaffold): resolve review findings #1 and #2 | #1, #2 |

## Summary

2/2 findings resolved. 0 deferred.
