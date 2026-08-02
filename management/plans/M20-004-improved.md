# Improvement Report: M20-004

**Task:** MANUAL.md locale reference + cross-locale seed reproducibility matrix
**Date:** 2026-06-12
**Review:** management/reviews/M20-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `MANUAL.md:1297` precedence chain header omits the `Environment` level — reads "Default (`en-US`) < Project < Collection < CLI flag" instead of the 5-level SPEC:1006 chain | Rewrote chain header to "Default (`en-US`) < Project < Environment < Collection < CLI flag (`--locale`)". Added `Environment` bullet to the description list. Retained and updated the parenthetical note clarifying the environment-file locale seam is reserved but not yet wired. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/variable`) | 97.2% |
| Coverage (total) | 84.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c28c427d | fix(docs): add Environment level to precedence chain in MANUAL.md | #1 |

## Summary

1/1 findings resolved. 0 deferred.
