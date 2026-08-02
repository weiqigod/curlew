# Code Review: M16-010

**Task:** scheduled_runs.result_id FK migration and ingest-path linkage
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-010-scheduled-runs-result-id-fk

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error paths properly returned; no swallowed errors; no panics. Idempotency branch logs at `Information` level before returning `None`. `resultError != ResultError.None` path wraps into `ScheduleClaimError` without leaking internal error codes. |
| Input Validation | PASS | `ClaimToken` validated (null/empty/non-Guid returns `InvalidRequest`). Null entity guard before accessing properties. Idempotency branch triple-guards (status terminal, token match, result_id present) before short-circuiting. |
| Naming | PASS | The Low finding from iteration 1 — `Results.ResultId.TryParse(...)` unnecessary namespace qualifier on line 177 of `ScheduleExecutorService.cs` — is resolved. Bare `ResultId.TryParse(...)` is now used, consistent with `SchedulesService.cs`. All exported symbols have correct doc comments, no stuttering, proper casing. |
| Code Organization | PASS | Package boundaries clean. `ScheduleExecutorService` depends on `ResultsService` through constructor injection. `ScheduledRunDto` is a pure record with doc comments on every parameter. `ToRunDto` null-conditional `r.ResultId is { } guid ? ResultId.Format(guid) : null` correctly handles legacy NULL rows. |
| Correctness | PASS | Idempotency logic is sound: requires `Status == Completed/Failed AND ClaimToken == token AND ResultId is not null` — all three conditions must be true. After ShardReaper reset, `ClaimToken` is cleared on the DB row so old tokens cannot match the idempotency branch. `Guid?` nullable comparison works correctly in C#. `ToRunDto` null-conditional is correct for the optional FK. |
| Test Quality | PASS | All 6 task behaviors are covered. `ScheduledRunResultLinkTests` class satisfies the observable filter. Both service-layer and HTTP-layer idempotency are independently tested. `DeletingResult_sets_scheduled_runs_result_id_to_null` correctly tests ON DELETE SET NULL via EF Core 9's automatic `PRAGMA foreign_keys = ON`. Regression guard for different-token case present at both layers. |

## Test Coverage

- **ScheduledRunDto**: 100% line coverage
- **ScheduleExecutorService** (overall): 100% line coverage
- **ScheduleExecutorService.SubmitResultAsync**: 85.45% line, 80.95% branch — above the 80% threshold
- **SchedulesService**: 100% line coverage
- All new test files exercised by `dotnet test` (1542 passed, 0 failed)

## Behavior Coverage

| Behavior | Covered By |
|----------|-----------|
| Schema has nullable `result_id` FK to `results(id)` | `DeletingResult_sets_scheduled_runs_result_id_to_null` (calls `Database.Migrate()`) |
| Migration rollback | Acknowledged as `dotnet ef` manual operation (M16-009 migration; no new migration generated) |
| Worker posts result → `result_id` written atomically | `ListRuns_returns_result_id_after_worker_posts_result`, `SubmitResultAsync_persists_result_and_sets_result_id_on_completed`, `Result_persists_to_results_table_and_sets_result_id_FK` |
| Legacy NULL rows gracefully handled | `ListRuns_returns_null_result_id_for_queued_run`, `ListRuns_returns_null_result_id_for_legacy_completed_row` |
| ON DELETE SET NULL | `DeletingResult_sets_scheduled_runs_result_id_to_null` |
| Idempotent retry with same `claim_token` → 200, no double-insert | `SubmitResultAsync_idempotent_retry_with_same_claim_token_returns_None_and_does_not_double_insert`, `Result_post_idempotent_retry_with_same_claim_token_returns_200_and_keeps_one_row` |

## Summary

The implementation is correct, complete, and meets all six task behaviors. The sole finding from iteration 1 (unnecessary `Results.` qualifier on `ResultId.TryParse`) was resolved in the `/improve` pass. The idempotency logic is properly guarded — requiring a terminal status, a matching claim token, and a non-null result_id before short-circuiting — so adversarial cases (different token, reaper-reset row) correctly return 409. No findings remain.
