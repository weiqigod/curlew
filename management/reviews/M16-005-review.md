# Code Review: M16-005

**Task:** trials table migration and entity
**Reviewer:** AI
**Date:** 2026-05-10
**Branch:** feature/M16-005-trials-table

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No error handling needed in pure entity/migration code; `TrialKindConverter` correctly throws `InvalidOperationException` for unknown enum values — appropriate since an unknown DB value is a programming defect, not an expected failure |
| Input Validation | PASS | `IsRequired()` and `HasMaxLength(64)` on feature column; `HasMaxLength(32)` on kind column; CHECK constraint enforces enum values at DDL level; FK cascade on user delete |
| Naming | PASS | No stuttering; doc comments on all exported types and properties; `TrialKindConverter` is `internal sealed` (correct visibility); enum values match C# PascalCase conventions |
| Code Organization | PASS | Entity in `Data/Entities/`, converter co-located, `DbSet<Trial>` added in block order, `OnModelCreating` wiring follows existing ordering conventions; `idx_trials_notify_3day` correctly placed in raw SQL (migration) with explanatory comment in `AppDbContext` |
| Correctness | PASS | Previous finding (dead `HasIndex` for `idx_trials_notify_3day`) resolved — dead block removed, comment added pointing to migration raw SQL; snapshot reflects only the managed `idx_trials_notify_1day` index; rollback targets the correct preceding migration (`AddGitlabInstallationsAndEvents`) |
| Test Quality | PASS | All 6 behaviors covered; happy path + error paths + boundary cases (same-feature-different-user, different-features-same-user); raw ADO.NET bypass to test CHECK constraint; DDL inspection for partial indexes; down-migration rollback test; `[InlineData("trials")]` added to schema theory |

## Test Coverage
- Coverage: all behavior tests pass; CI gate green
- Missing coverage: none — all 6 task behaviors have at least one test; EXPLAIN index-use test correctly deferred to M16-008 per plan decision #15

## Summary

All code is correct and well-structured. The one finding from the first review (dead `HasIndex` declaration for `idx_trials_notify_3day` that EF Core silently ignored) has been resolved: the dead block was removed and a clear comment was added near `idx_trials_notify_1day` explaining that the 3-day index is raw-SQL-only in the migration. The migration, entity, converter, `AppDbContext` wiring, and tests are all consistent with each other and with the spec. No issues remain.
