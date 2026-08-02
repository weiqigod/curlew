# Code Review: M16-012

**Task:** schedules.timezone migration and web schedules dashboard page
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-012-schedules-timezone-dashboard
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All 7 findings from the first review iteration were verified as resolved:

| # | Severity | Finding (Iteration 1) | Fix Verified |
|---|----------|-----------------------|-------------|
| 1 | High | 422 E2E test had no assertion | `expect(page.getByTestId('sched-form-error')).toBeVisible()` + text match added at lines 73–74 of `org-schedules.spec.ts` |
| 2 | High | `submitting = false` permanently — loading state never activated | `submitting = true` set at line 47 of `CreateScheduleModal.svelte` before dispatch |
| 3 | High | Error from `handleCreateSchedule` was unhandled promise rejection invisible to modal | `apiError` prop added to modal; parent catches, sets `createModalError`, calls `createModalRef?.resetSubmitting()` — confirmed in `+page.svelte` lines 40–43 |
| 4 | Medium | Only ListSchedules had a 402 tier-gate test | 4 new 402 tests added: `Post_schedule_returns_402`, `Get_schedule_returns_402`, `RunNow_returns_402`, `ListRuns_returns_402` via `CreateFreeTierClientAsync` helper |
| 5 | Medium | Swagger test didn't assert `timezone` field or `422` response code | Both assertions added to `Swagger_json_lists_schedules_endpoints` at lines 307–341 |
| 6 | Low | `computeDuration` returned negative strings on clock skew | `if (ms < 0) return '—'` guard at line 11 of `duration.ts`; negative-case test at line 29 of `duration.test.ts` |
| 7 | Low | Silent bare `catch {}` in `EnqueueDueAsync` with no log output | `ILogger<SchedulesService>` injected via primary constructor; `logger.LogWarning(...)` call in catch block at lines 253–257 of `SchedulesService.cs` |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped and propagated correctly. `InvalidTimezone`, `InvalidCron`, etc. returned as typed errors. `EnqueueDueAsync` now logs timezone resolution failures at warning level. |
| Input Validation | PASS | Timezone validated via `TimeZoneInfo.FindSystemTimeZoneById`; cron validated via Cronos; required fields validated before processing. Web form validates locally before dispatch. |
| Naming | PASS | No stuttering, doc comments on all exported symbols. `ScheduleProblem`, `ScheduleDto`, `ScheduleError` all clean. |
| Code Organization | PASS | Backend packages respected; `ScheduleProblem` is a focused RFC 7807 factory; web components are correctly scoped. |
| Correctness | PASS | `submitting` flag properly managed; API errors flow correctly from parent to modal; negative duration handled; DST-aware Cronos invocation with `DateTimeKind.Utc` explicit; tier-gate covers all 5 endpoints. |
| Test Quality | PASS | Spring-forward gap test present; table-driven DST theory for 5 timezone cases; all 5 endpoints have 402 tier-gate coverage; swagger test validates both `timezone` field and `422` response; negative-duration edge case covered. |

## Test Coverage
- Go: all tests pass (no Go files changed in this task)
- C# backend: new tests cover timezone persistence, DST-aware Cronos (5 cases + spring-forward), CreateAsync timezone validation (3 bad-tz cases), EnqueueDueAsync TZ recompute, HTTP endpoint 422/201/402 on all 5 endpoints, swagger schema
- Web unit: `schedules.test.ts`, `duration.test.ts`, `page-server.test.ts` all present with good coverage; `duration.test.ts` includes 5 cases including negative-ms guard
- Web E2E: 6 tests; the 422 test now has an assertion (`sched-form-error` visible + text match)

## Summary

The backend half is solid: migration, entity, service, and endpoint changes are all correct and well-tested. The DST-aware Cronos integration, timezone validation, RFC 7807 problem-detail response, and tier-gate enforcement on all five endpoints are correctly implemented. The web half had three correctness/test gaps in the first review; all three were properly fixed in the improvement iteration. The modal now correctly tracks submission state, API errors are surfaced to the user via the `apiError` prop, and the E2E test that previously had no assertion now verifies the expected UI behaviour.
