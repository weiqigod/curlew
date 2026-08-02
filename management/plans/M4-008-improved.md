# Improvement Report: M4-008

**Task:** Backend: notification dispatcher (Slack + email)
**Date:** 2026-04-17
**Review:** management/reviews/M4-008-review.md (iteration 2)

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `NotificationsDispatcher` used `DateTime.UtcNow` directly instead of injecting `TimeProvider` | Injected `TimeProvider clock` into the primary constructor; changed `AttemptedAt` assignment to `clock.GetUtcNow().UtcDateTime`; updated all test constructor calls to pass `TimeProvider.System` | ✓ tests pass |
| 2 | Medium | `succeeded` boolean was dead code — all success paths returned early so `if (!succeeded)` guard was always reached on the failure path only | Removed the `succeeded` variable; the final `RecordDelivery` call is now unconditional with a comment explaining it is the fallthrough-failure path | ✓ tests pass |
| 3 | Medium | `NotifyAsync_payload_contains_result_id_collection_and_failure_counts` only asserted `collection_name`, not `result_id` or `fail_count` | Added assertions for `res_<guid>` prefix in result_id and numeric value of `fail_count` | ✓ tests pass |
| 4 | Low | `notification_rules` and `notification_deliveries` missing from `Migration_creates_expected_table` InlineData theory | Added `[InlineData("notification_rules")]` and `[InlineData("notification_deliveries")]` — theory now has 10 cases, all passing | ✓ tests pass |
| 5 | Low | `Post_notification_rules_returns_403_for_non_admin_member` sent an invitation but never accepted it, so it tested non-member returns 403 rather than member-without-admin-role returns 403 | Replaced the invitation flow with direct DB seeding of a `User` and `OrganizationMember` with `OrgRole.Member` via `_factory.Services.CreateScope()`, so the correct authorization code path is exercised | ✓ tests pass |
| 6 | Low | Two near-identical tests `NotifyAsync_retries_twice_on_5xx_then_records_failed` and `NotifyAsync_records_failed_when_all_retries_500` covered the same scenario | Kept the first (asserts `AttemptCount=3` and call count of 3); renamed and reworked the second to `NotifyAsync_records_error_message_when_all_retries_fail` (uses HTTP 503, asserts `ErrorMessage` content) | ✓ tests pass |
| 7 | Low | `NotificationEventHelper` methods were `public` despite the class being `internal static` | Changed `TryParse`, `Format`, `ParseStoredEvents`, and `FormatStoredEvents` from `public` to `internal` | ✓ build clean |

## Iteration 2 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 8 | Medium | `Ingesting_failing_run_triggers_slack_post_and_delivery_row` asserted `Calls.Should().NotBeEmpty()` on a shared singleton `FakeSlackWebhookPoster`, making the assertion trivially pass due to accumulated state from earlier tests | Snapshot `callsBefore = fakeSlack.Calls.Count` before the ingest call; assert `fakeSlack.Calls.Count.Should().BeGreaterThan(callsBefore)` after ingest to verify a new call was recorded by this specific test | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln -warnaserror` | PASS |
| `dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | PASS |
| Notifications filter (`FullyQualifiedName~Notifications`) | PASS — 24 passed |
| Full suite | PASS — 162 passed, 0 failed |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| e527587 | fix(notifications): inject TimeProvider and remove dead succeeded flag | #1, #2 |
| 961cc3e | fix(notifications): strengthen payload test assertions for result_id and fail_count | #3 |
| a85f033 | fix(notifications): add notification tables to Migration_creates_expected_table theory | #4 |
| a2955a7 | fix(notifications): seed Member row directly in 403 endpoint test | #5 |
| e4c11d1 | fix(notifications): differentiate duplicate retry tests to cover distinct scenarios | #6 |
| c80fee7 | fix(notifications): change NotificationEventHelper methods from public to internal | #7 |
| 4bb5560 | fix(tests): snapshot Slack call count before ingest to detect specific dispatch | #8 |

## Summary

8/8 findings resolved. 0 deferred.
