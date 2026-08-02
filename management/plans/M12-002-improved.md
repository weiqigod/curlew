# Improvement Report: M12-002

**Task:** $base64 and $base64Decode dynamic functions
**Date:** 2026-04-28
**Review:** management/reviews/M12-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `docs/MANUAL.md` line 1091: "The 15 built-in helpers in the table above take no arguments" was ambiguous after the argument-bearing table was inserted between the no-argument table and this prose — "the table above" now pointed at the argument-bearing table, directly contradicting the "take no arguments" claim. | Changed to "The 15 no-argument built-in functions (from the first table in §3.7 above)" to unambiguously identify the correct table. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/variable`) | 96.1% |
| Coverage (total) | 87.0% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 43c5dd5 | fix(docs): clarify §3.7 no-arg table reference after argument-bearing table insertion | #1 |

## Summary

1/1 findings resolved. 0 deferred.
