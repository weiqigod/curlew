# Improvement Report: M16-012

**Task:** schedules.timezone migration and web schedules dashboard page
**Date:** 2026-05-11
**Review:** management/reviews/M16-012-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `422 invalid-timezone` E2E test had no assertion — could never fail even if feature broken | Added `expect(page.getByTestId('sched-form-error')).toBeVisible()` + text match for timezone/Invalid/422 | ✓ tests pass |
| 2 | High | `submitting = false` permanently — loading/disabled state never activated, double-submit not prevented | Set `submitting = true` before `dispatch('submit', ...)` in `handleSubmit` | ✓ tests pass |
| 3 | High | `handleCreateSchedule` re-threw error as unhandled promise rejection — modal had no mechanism to receive it | Added `apiError` prop to `CreateScheduleModal`; parent catches error, sets `createModalError`, calls `createModalRef?.resetSubmitting()` instead of re-throwing | ✓ tests pass |
| 4 | Medium | Only `ListSchedules` had a 402 tier-gate test; Create/Get/RunNow/ListRuns untested for free-tier orgs | Added `CreateFreeTierClientAsync` helper and four new 402 tests: `Post_schedule_returns_402_for_free_tier_org`, `Get_schedule_returns_402_for_free_tier_org`, `RunNow_returns_402_for_free_tier_org`, `ListRuns_returns_402_for_free_tier_org` | ✓ tests pass |
| 5 | Medium | Swagger test did not assert `timezone` field in request schema or `422` response code | Extended `Swagger_json_lists_schedules_endpoints` to assert: (a) `timezone`/`Timezone` field present in `CreateScheduleRequest` schema, (b) `422` response key exists on POST `/schedules` operation | ✓ tests pass |
| 6 | Low | `computeDuration` returned e.g. `-100ms` on clock skew (completedAt < startedAt); no test coverage | Added `if (ms < 0) return '—'` guard before branch comparisons; added `'returns em dash for negative duration (clock skew)'` test case | ✓ tests pass |
| 7 | Low | Silent `catch {}` in `EnqueueDueAsync` swallowed `TimeZoneNotFoundException` with no log output | Injected `ILogger<SchedulesService>` via primary constructor; added `logger.LogWarning(...)` call in the catch block; updated direct test instantiations to pass `NullLogger<SchedulesService>.Instance` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `dotnet test --filter FullyQualifiedName~Schedules` | PASS (113/113) |
| `npm run test:unit` | PASS (198/198) |
| Backend coverage (SchedulesService) | ~94% line-rate |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 2147cf37 | fix(schedules): resolve all M16-012 review findings | #1, #2, #3, #4, #5, #6, #7 |

## Summary
7/7 findings resolved. 0 deferred.
