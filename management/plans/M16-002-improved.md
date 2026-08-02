# Improvement Report: M16-002

**Task:** Token tables and templates for password reset and email verification
**Date:** 2026-05-07
**Review:** management/reviews/M16-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Behavior #7 in `management/tasks/M16-002.yaml` incorrectly described the partial index as being on `(token_hash) WHERE consumed_at IS NULL AND revoked_at IS NULL`. The actual implementation (per spec:10979-10982) has two distinct indexes: an unconditional unique index on `(token_hash)` (`idx_password_reset_tokens_hash`) and a separate partial index on `(expires_at) WHERE consumed_at IS NULL AND revoked_at IS NULL` (`idx_password_reset_tokens_active`). | Updated behavior #7 to accurately state both indexes: the partial index on `(expires_at)` and the separate unique index on `(token_hash)`, naming both by their index names. | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS — 1322 passed, 0 failed, 8 skipped |
| `golangci-lint run` | PASS — 0 issues |
| Coverage | 94.8% (as reported by review; no new code added in this improve pass) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 9c5836c2 | fix(tasks): correct behavior #7 index description in M16-002 task YAML | #1 |

## Summary

1/1 findings resolved. 0 deferred.
