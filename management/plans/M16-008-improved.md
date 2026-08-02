# Improvement Report: M16-008

**Task:** TrialExpiryNotifier daily cron with 3-day and 1-day reminder emails
**Date:** 2026-05-11
**Review:** management/reviews/M16-008-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Behavior #5 (wall-clock scheduling accuracy, ≤60s fuzz) had no test; `DelayUntilNextFireAsync` delay logic was untestable (private method, no indirect coverage). | Extracted `ComputeNextFireDelay(opts, now)` as an `internal static` method. Added four unit tests: exact-fire-time boundary, 30 min before, 22 h after, and the 59-second fuzz tolerance assertion. | ✓ tests pass |
| 2 | Medium | `DbUpdateException` catch branch (lines 204–213 of `TrialExpiryNotifier.cs`) was completely untested. | Added `Tick_does_not_set_notified_at_when_save_throws_DbUpdateException` using an EF Core `SaveChangesInterceptor` (`DbUpdateExceptionInterceptor`) that throws `DbUpdateException` from `SavingChangesAsync`. Asserts email was enqueued but `notified_3day_at` remains null so the next tick retries. | ✓ tests pass |
| 3 | Medium | Doc comment on `DelayUntilNextFireAsync` incorrectly stated "returns immediately on the first call" but code always slept 1 minute in forced-fire mode, leaving only ~0 s margin for the observable. | Added `_firstFireDone` flag: forced-fire mode now skips the delay on the very first loop iteration and only sleeps 1 minute on subsequent ticks. Doc comment corrected. | ✓ tests pass |
| 4 | Low | `Tick_emits_message_with_manifest_allowlisted_variables_only` only checked key presence, not the `expires_at_local` value format. | Added assertion `Should().MatchRegex(@"^\d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC$")` on `Variables["expires_at_local"]`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| `golangci-lint run` | N/A (Go code unchanged) |
| Coverage (`TrialExpiryNotifier*`) | 89.9% (266/296 lines) |
| Total tests | 1489 passed, 0 failed, 8 skipped |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `b0d674ac` | fix(notifier): immediate first-tick for forced-fire mode + extract ComputeNextFireDelay | #3, #1 (extraction) |
| `eb43f393` | test(notifier): add missing coverage for scheduling, DbUpdateException, and expires_at_local format | #1, #2, #4 |

## Summary

4/4 findings resolved. 0 deferred.
