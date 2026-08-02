# Code Review: M4-006

**Task:** Backend: scheduled run trigger service
**Reviewer:** AI
**Date:** 2026-04-16
**Branch:** feature/M4-006-scheduled-run-trigger-service
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All 7 findings from the first review iteration were resolved correctly.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Null `Cron` guard added before `CronExpression.Parse` — null input now returns `ScheduleError.InvalidCron` (400) instead of throwing `ArgumentNullException` (500). All other error paths properly wrapped and returned. No swallowed errors. |
| Input Validation | PASS | `Name`, `CollectionRef`, and `Cron` all validated for null/whitespace before use. `request == null` is caught at the first check via `request?.Name` pattern. |
| Naming | PASS | No stuttering, doc comments on all exported types and methods, `ISchedulerEnqueuer` interface uses the `-er` suffix correctly. |
| Code Organization | PASS | Proper namespace separation, `ISchedulerEnqueuer` interface creates clean testability seam, `SchedulerHost` uses scoped DI correctly. TDD commit pattern followed throughout. |
| Correctness | PASS | Null cron input handled correctly. RBAC checks work at service and endpoint layer. `EnqueueDueAsync` atomically updates `LastRunAt` + recomputes `NextRunAt`. Cancellation propagated correctly throughout all async calls. |
| Test Quality | PASS | All 8 task behaviors are verified by at least one test. Integration tests now cover `ListSchedules` and `GetSchedule` handlers. RBAC integration test correctly seeds a user with `OrgRole.Member` in the org. Swagger test verifies the `cron` field in request body schema. `CountingLogger` asserts the specific "scheduler tick" message by content. Unit test for null `Cron` field added to `SchedulesServiceTests`. |

## Test Coverage

- Overall line coverage: **93.3%** (135 tests, 0 failures)
- Schedules test filter: 41 tests pass
- `SchedulesService` and all async state machines: **100%** line coverage
- `SchedulerHost.ExecuteAsync`: **88.9%** — `OperationCanceledException` catch block in inner `Task.Delay` is not exercised, acceptable defensive path
- `SchedulesEndpoints` handlers: **45–68%** on individual async state machines — uncovered paths are defensive error handlers for malformed JWT (handled by auth middleware before reaching handlers) and invalid org-id format (would require crafting a non-`org_`-prefixed org id in requests), plus switch-expression arms for error codes not triggered at HTTP level
- Missing areas: `JsonException` catch in `CreateSchedule`, invalid-orgId branches, `RunNow` PermissionDenied/NotFound endpoint-level paths — all are covered at the service level and are edge cases in the HTTP transport layer

## Behavior Compliance

All 8 behaviors from the task YAML are verified:

| Behavior | Verified By |
|----------|-------------|
| POST /schedules → 201 with computed next_run_at | `Post_schedule_returns_201_with_next_run_at_in_future` (endpoint) + `CreateAsync_persists_schedule_with_computed_next_run_at` (service) |
| Malformed cron → 400 invalid_cron | `Post_schedule_returns_400_invalid_cron_for_bad_expression` (endpoint) + `CreateAsync_returns_invalid_cron_for_malformed_expression` (service) |
| Duplicate name → 409 schedule_name_taken | `Post_schedule_returns_409_schedule_name_taken_for_duplicate` (endpoint) |
| Scheduler tick creates queued run | `EnqueueDueAsync_creates_runs_for_due_schedules` (service) + `ExecuteAsync_calls_enqueue_due_on_tick` (host) |
| POST run-now → 202, run appears in list | `Post_run_now_returns_202_with_queued_run` + `Get_schedule_runs_returns_queued_run_after_run_now` (endpoint) |
| Member (non-admin) → 403 permission_denied | `Post_schedule_returns_403_for_member_non_admin` (endpoint, correct OrgRole.Member scenario) |
| Service restart re-hydrates past-due schedules | `EnqueueDueAsync_creates_runs_for_due_schedules` (past NextRunAt picked up on first tick) |
| Swagger lists schedules endpoints with cron documented | `Swagger_json_lists_schedules_endpoints` (verifies cron field in request body schema) |

## Summary

The implementation is complete and correct. All 7 findings from the first review iteration were resolved: the critical null Cron guard prevents the `ArgumentNullException` 500 response, integration tests now exercise all five endpoint handlers, the RBAC test seeds the correct `OrgRole.Member` scenario, the Swagger test verifies the `cron` field in the request body schema, and the scheduler host test asserts the specific "scheduler tick" log message. Overall coverage is 93.3% (above the 80% threshold). The code meets all project standards.
