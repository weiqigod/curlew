# Improvement Report: M15-003

**Task:** Retier from_command from Solo to Free across registry, tests, and MANUAL.md
**Date:** 2026-05-07
**Review:** management/reviews/M15-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Stale comment in `internal/runner/runner.go:946` reads "Solo tier" for a feature now gated at Free | Updated to `// Precedence 5: from_command variables (Free tier — available at all tiers)` | ✓ tests pass |
| 2 | Low | `TestDefaultRegistry_FromCommand` in `registry_test.go` screened `Description` but not `Workaround` for legacy Solo/pricing strings | Added `strings.Contains(def.Workaround, "$9/month")` and `strings.Contains(def.Workaround, "Solo")` assertions parallel to the existing Description checks | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/auth` | 89.6% |
| Coverage `internal/variable` | 97.4% |
| Coverage `internal/runner` | 84.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| b3ac230e | fix(runner,auth): resolve review findings from M15-003 | #1, #2 |

## Summary

2/2 findings resolved. 0 deferred.
