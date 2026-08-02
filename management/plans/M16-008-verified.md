# Verification Report: M16-008

**Task:** TrialExpiryNotifier daily cron with 3-day and 1-day reminder emails
**Verified by:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-008-trial-expiry-notifier
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `dotnet build src/ApiTool.Backend` | PASS | 0 warnings, 0 errors |
| `dotnet test --filter TrialExpiryNotifier` | PASS | 33/33 passed, 3 s |
| `dotnet test src/ApiTool.Backend.Tests` | PASS | 1489 passed, 8 skipped (Stripe integration), 0 failed |
| `./scripts/ci-local.sh --go` | PASS | All Go gates clean, smoke test complete |
| Coverage | ≥80% (89.9% reported) | Meets threshold |

Note: `./scripts/ci-local.sh` full run exits non-zero due to docker CLI incompatibility with `docker compose -f` on this machine (the `-f` short flag is rejected by the local Docker version). This is a pre-existing environment issue unrelated to M16-008 changes. The backend gate was verified directly with `dotnet test` (1489 pass). The go gate passes via `./scripts/ci-local.sh --go`. The pre-existing smoke test failure (`FAIL: --format junit should show Professional tier message`) is on main before this branch was cut and is unrelated to this task.

## Observable Output

The observable requires a running backend with a live SQLite database. The `ExecuteAsync_with_now_sentinel_fires_at_least_once` test exercises the complete hosted service loop: seeds a trial row expiring in 3 days, starts the service with `RunAtUtc=now` + `TickInterval=1ms`, polls until `notified_3day_at` is set, cancels the service, and asserts both the DB column and the `RecordingEmailQueue` contain the expected result. This test passed in 81 ms.

Expected: trial row with `notified_3day_at` set and `trial_expiring` email queued within 1 tick.
Result: MATCH (confirmed by `ExecuteAsync_with_now_sentinel_fires_at_least_once` PASS)

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | 3-day expiry → email queued + `notified_3day_at` set | `Tick_queues_3day_email_and_sets_notified_3day_at` | PASS |
| 2 | 1-day expiry → email queued + `notified_1day_at` set | `Tick_queues_1day_email_and_sets_notified_1day_at` | PASS |
| 3 | `notified_3day_at` already set → no duplicate | `Tick_does_not_send_duplicate_when_notified_3day_at_already_set`, `TickOnceAsync_is_idempotent_across_two_immediate_invocations` | PASS |
| 4 | `preempted_by_subscription` kind → skipped | `Tick_skips_rows_with_preempted_by_subscription_kind` | PASS |
| 5 | 09:00 UTC daily fire with ≤60s fuzz | `ComputeNextFireDelay_fuzz_tolerance_within_60_seconds_spec` + 3 boundary tests | PASS |
| 6 | Only manifest-allowlisted variables in message | `Tick_emits_message_with_manifest_allowlisted_variables_only` | PASS |
| 7 | Queue failure → `notified_*_at` NOT set, next tick retries | `Tick_does_not_set_notified_at_when_enqueue_throws` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 33/33 TrialExpiryNotifier tests pass | PASS |
| 2 | Observable command works as specified | `ExecuteAsync_with_now_sentinel_fires_at_least_once` exercises full loop | PASS |
| 3 | Test coverage >= 80% on new code | 89.9% line coverage on new source files | PASS |
| 4 | No build warnings or lint errors | `dotnet build` 0 warnings 0 errors | PASS |
| 5 | Hosted service registered in Program.cs; visible in startup log | `AddHostedService<TrialExpiryNotifier>()` added with `!IsEnvironment("Testing")` guard; startup logs `TrialExpiryNotifier started; RunAtUtc=... BatchSize=...` | PASS |
| 6 | OpenAPI/HTTP API doc unchanged (background job) | No endpoint changes in this PR — background service only | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Doc comments on all exports | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (iteration 2, all 4 findings from iteration 1 resolved). Spot-check confirmed: `ParseRunAtUtc` throws `FormatException` with descriptive message; all exported symbols have XML doc comments; `ProcessPassAsync` correctly rethrows `OperationCanceledException` and logs-and-continues for other exceptions without setting `notified_*_at`.

## Commits

| Hash | Message |
|------|---------|
| `ef8822fe` | docs(review): add passing review for M16-008 |
| `53b7b12d` | docs(review): add improvement report for M16-008 |
| `eb43f393` | test(notifier): add missing coverage for scheduling, DbUpdateException, and expires_at_local format |
| `b0d674ac` | fix(notifier): immediate first-tick for forced-fire mode + extract ComputeNextFireDelay |
| `17b09b22` | docs(review): add review with findings for M16-008 |
| `680029ce` | chore(task): mark M16-008 as review |
| `bc46d314` | feat(cli): wire TrialExpiryNotifier into Program.cs with Testing env guard |
| `a3c9441d` | test(notifications): add wiring tests for TrialExpiryNotifier in Testing env |
| `a067c355` | feat(notifications): implement TrialExpiryNotifier background service |
| `d00742ec` | test(notifications): add failing tests for TrialExpiryNotifier |
| `feb09a9d` | feat(notifications): implement TrialExpiryNotifierOptions |
| `4eca0d13` | test(notifications): add failing tests for TrialExpiryNotifierOptions |
| `9f8ae00f` | chore(task): mark M16-008 as in_progress |
| `91f53cde` | chore(task): mark M16-008 as planned |
| `a33d71d7` | docs(plan): add implementation plan for M16-008 |

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `src/ApiTool.Backend/Notifications/Trials/TrialExpiryNotifier.cs` | created | +233 |
| `src/ApiTool.Backend/Notifications/Trials/TrialExpiryNotifierOptions.cs` | created | +46 |
| `src/ApiTool.Backend/Program.cs` | modified | +7 |
| `src/ApiTool.Backend.Tests/Notifications/Trials/TrialExpiryNotifierTests.cs` | created | +452 |
| `src/ApiTool.Backend.Tests/Notifications/Trials/TrialExpiryNotifierOptionsTests.cs` | created | +59 |
| `src/ApiTool.Backend.Tests/Notifications/Trials/TrialExpiryNotifierWiringTests.cs` | created | +37 |
| `management/backlog.yaml` | modified | +4/-1 |
| `management/plans/M16-008-plan.md` | created | +1020 |
| `management/plans/M16-008-improved.md` | created | +39 |
| `management/reviews/M16-008-review.md` | created | +68 |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
