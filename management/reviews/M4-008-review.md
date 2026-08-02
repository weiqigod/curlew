# Code Review: M4-008

**Task:** Backend: notification dispatcher (Slack + email)
**Reviewer:** AI
**Date:** 2026-04-17
**Branch:** feature/M4-008-backend-notifications
**Iteration:** 3 (post-improve iteration 2)

## Verdict: PASS

## Prior-Review Findings (all resolved)

All 8 findings from iterations 1 and 2 have been verified fixed in the current code:

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 | Medium | `DateTime.UtcNow` used in dispatcher instead of `TimeProvider` | Fixed |
| 2 | Medium | Dead `succeeded` boolean | Fixed |
| 3 | Medium | Payload test only asserted `collection_name` | Fixed |
| 4 | Low | `notification_rules`/`notification_deliveries` missing from schema theory | Fixed |
| 5 | Low | 403 test exercised non-member path, not member-without-admin path | Fixed |
| 6 | Low | Two near-identical retry tests | Fixed |
| 7 | Low | `NotificationEventHelper` methods `public` despite `internal` class | Fixed |
| 8 | Medium | `Ingesting_failing_run_triggers_slack_post_and_delivery_row` asserted `Calls.Should().NotBeEmpty()` on shared singleton | Fixed: snapshot `callsBefore = fakeSlack.Calls.Count` before ingest; asserts `fakeSlack.Calls.Count.Should().BeGreaterThan(callsBefore)` |

## Findings

No new findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All expected failures return structured error tuples; dispatcher swallows notification failures after catch+log; no panics; exceptions never propagate from `NotifyAsync`. |
| Input Validation | PASS | Channel, target (HTTPS for Slack, contains `@` for email), and events all validated; null/empty request body handled; invalid JSON caught with 400. |
| Naming | PASS | No stuttering; all exported symbols have doc comments; `NotificationEventHelper` methods are `internal` matching the `internal` class; interface names (`ISlackWebhookPoster`, `ISmtpSender`) are clear and meaningful. |
| Code Organization | PASS | Clean service/dispatcher/endpoints/entities layer separation; DI lifetimes correct (Scoped dispatcher, Singleton senders); `IServiceScopeFactory` child-scope pattern used correctly to avoid captive dependencies. |
| Correctness | PASS | `TimeProvider` injected and used for `AttemptedAt`; retry delay indexing verified correct; `lastResponseCode` tracking across mixed success/exception/non-2xx attempts is sound; `FindMatchingRulesAsync` does exact token matching to avoid substring false-positives. |
| Test Quality | PASS | 24 notification tests; snapshot pattern fixes the shared-singleton assertion; all 8 task behaviors covered; dispatcher unit tests use `FakeScopeFactory` for isolation; integration tests use `BackendFactory` with fake senders; error-message test differentiates from retry-count test. |

## Test Coverage

- Notifications filter: **24 passed, 0 failed** (exceeds DoD requirement of ≥10)
- Full suite: **162 passed, 0 failed**
- Build: clean (`0 Warning(s), 0 Error(s)` with `-warnaserror`)
- All 8 task behaviors covered by at least one test

## Summary

The implementation is complete and correct. All 8 findings from prior iterations are verified fixed. The Slack snapshot assertion now correctly detects whether the specific test's ingest triggered a new dispatch call, removing the false-positive risk. No new findings were identified in this iteration.
