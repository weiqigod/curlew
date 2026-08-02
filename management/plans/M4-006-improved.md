# Improvement Report: M4-006

**Task:** Backend: scheduled run trigger service
**Date:** 2026-04-16
**Review:** management/reviews/M4-006-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Null `Cron` field bypasses `CronFormatException` catch → 500 instead of 400 | Added `if (string.IsNullOrWhiteSpace(request.Cron)) return (null, ScheduleError.InvalidCron, "cron is required.");` before the try/catch block in `SchedulesService.CreateAsync` | Tests pass |
| 2 | High | No integration test for `GET /organizations/{orgId}/schedules` (ListSchedules handler) | Added `Get_schedules_returns_200_list_for_org_member()` and `Get_schedules_returns_403_for_non_member()` to `SchedulesEndpointsTests` | Tests pass |
| 3 | High | No integration test for `GET /organizations/{orgId}/schedules/{name}` (GetSchedule handler) | Added `Get_schedule_by_name_returns_200_for_org_member()` and `Get_schedule_by_name_returns_404_for_unknown_name()` to `SchedulesEndpointsTests` | Tests pass |
| 4 | Medium | `ExecuteAsync_logs_info_when_schedules_are_due` assertion `InfoCount > 0` was trivially satisfied by the "SchedulerHost started" message | Changed `CountingLogger` to collect all info messages in a `ConcurrentBag<string>` and assert that the collection contains a message with "scheduler tick" | Tests pass |
| 5 | Medium | RBAC integration test used a non-member user, not a user with `OrgRole.Member` | Added `CreateMemberClientAsync(OrgRole)` helper to `SchedulesEndpointsTests` that triggers user upsert via `CurrentUserAccessor` then seeds an `OrganizationMember` row with the given role via `_factory.Services.CreateScope()`. Updated the RBAC test to use this helper with `OrgRole.Member` | Tests pass |
| 6 | Medium | Swagger test did not verify the `cron` field in the POST request body schema | Added `.Accepts<CreateScheduleRequest>("application/json")` to the POST endpoint registration. Updated the Swagger test to verify that the `requestBody` exists and the `cron` property is documented (handles both inline `properties` and `$ref` indirection) | Tests pass |
| 7 | Low | No unit test for `CreateAsync` with null `Cron` (distinct from malformed string) | Added `CreateAsync_returns_invalid_cron_for_null_cron_expression()` to `SchedulesServiceTests` | Tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj` | PASS |
| `dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | PASS |
| `golangci-lint run` | N/A (C# project) |
| Coverage (line rate) | 93.3% (up from 92.4%) |
| Total tests | 135 (130 original + 5 new) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 71ccf32 | fix(schedules): guard against null Cron field in CreateAsync | #1, #7 |
| 9b81083 | fix(tests): strengthen SchedulerHost log assertion to verify tick message | #4 |
| fffb1ad | fix(schedules): add missing endpoint tests and correct RBAC + Swagger assertions | #2, #3, #5, #6 |

## Summary

7/7 findings resolved. 0 deferred.

Key changes:
- **Critical correctness fix**: null `cron` field in POST body no longer causes unhandled `ArgumentNullException` (500) — now returns 400 with `invalid_cron` code.
- **Coverage improvement**: `ListSchedules` and `GetSchedule` handlers now have full integration test coverage. Line coverage improved from 92.4% to 93.3%.
- **Spec compliance**: Swagger now exposes the `CreateScheduleRequest` schema (including `cron` field) via `.Accepts<CreateScheduleRequest>`.
- **Test quality**: RBAC test exercises the correct scenario (Member role in org, not non-member), scheduler host test verifies the specific "scheduler tick" log message.
