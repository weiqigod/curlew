# Improvement Report: M16-010

**Task:** scheduled_runs.result_id FK migration and ingest-path linkage
**Date:** 2026-05-11
**Review:** management/reviews/M16-010-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `Results.ResultId.TryParse(...)` in `ScheduleExecutorService.cs:177` uses an unnecessary namespace qualifier. The `using ApiTool.Backend.Results;` directive already resolves `ResultId` directly; the extra `Results.` creates a visual collision with the lowercase `results` field (the `ResultsService` instance) and is inconsistent with `SchedulesService.cs` which uses bare `ResultId.Format(...)`. | Replaced `Results.ResultId.TryParse(resultDto.Id, out var resultGuid)` with `ResultId.TryParse(resultDto.Id, out var resultGuid)` — one-word removal. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln -warnaserror` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| Coverage | 1542 passed, 0 failed, 8 skipped |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 9bc2c231 | fix(schedules): remove unnecessary Results. qualifier on ResultId.TryParse | #1 |

## Summary

1/1 findings resolved. 0 deferred.
