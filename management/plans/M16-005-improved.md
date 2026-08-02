# Improvement Report: M16-005

**Task:** trials table migration and entity
**Date:** 2026-05-10
**Review:** management/reviews/M16-005-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Dead `HasIndex` declaration for `idx_trials_notify_3day` in `AppDbContext.OnModelCreating`. EF Core tracks only one `HasIndex` per property; the 3-day declaration was silently overridden by the 1-day declaration that followed it, misleading maintainers into thinking EF manages both partial indexes. | Removed the dead `HasIndex` block (3 lines). Replaced the comment above the remaining `idx_trials_notify_1day` declaration with a note explaining that `idx_trials_notify_3day` is raw-SQL-only (created in the migration) and pointing to the migration file for context. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| Coverage | 1419/1427 tests pass (8 skipped — Stripe live integration tests) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 65bf2e4e | fix(db): remove dead HasIndex for idx_trials_notify_3day | #1 |

## Summary
1/1 findings resolved. 0 deferred.
