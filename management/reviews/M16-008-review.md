# Code Review: M16-008

**Task:** TrialExpiryNotifier daily cron with 3-day and 1-day reminder emails
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-008-trial-expiry-notifier
**Iteration:** 2 (after /improve)

## Verdict: PASS

## Findings

No findings. All four findings from the iteration-1 review were resolved in the /improve pass.

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| — | — | — | — | — | No findings | — |

## Pre-audit Gate

The Go CI gate (`./scripts/ci-local.sh --go`) reports `go=1` (failure), but investigation confirms the failing smoke test (`FAIL: --format junit should show Professional tier message`) is pre-existing on `main` before this branch was cut. M16-008 touches no Go files (all changes are C# backend). The failure is unrelated to this task. The backend gate was verified separately:

- `dotnet build src/ApiTool.Backend` → **PASS** (0 warnings, 0 errors)
- `dotnet test src/ApiTool.Backend.Tests --filter "FullyQualifiedName~TrialExpiryNotifier"` → **PASS** (33/33 passed)
- `dotnet test src/ApiTool.Backend.Tests` → **PASS** (1489 passed, 8 skipped, 0 failed)

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `EnqueueAsync` failure correctly rethrows `OperationCanceledException` and logs/continues for all other exceptions without setting `notified_*_at`. `DbUpdateException` is caught, logged, and the change tracker is cleared — next tick retries. `ExecuteAsync` outer loop catches and logs unexpected exceptions with a 1-minute back-off delay. |
| Input Validation | PASS | `ParseRunAtUtc()` validates all input with a clear `FormatException` message. `IsForcedFire` is case-insensitive. `BatchSize` and `RunAtUtc` have sensible defaults. |
| Naming | PASS | No stuttering. Doc comments on all exported and `internal` symbols. `TickOnceAsync` and `ComputeNextFireDelay` marked `internal` for test access. `_firstFireDone` clearly documents its purpose via comment. |
| Code Organization | PASS | `Notifications/Trials/` namespace fits the existing tree. Single responsibility. `await using` for DI scope cleanup. No circular dependencies. Minimal exported surface. |
| Correctness | PASS | Per-row write-after-enqueue pattern is correct for the retry contract. Kind filter (`FullInitial`/`OnDemand` only) is explicit and defensive. Both passes are independent — a row expiring within 1 day correctly sends both 3-day and 1-day emails on the same tick. `_firstFireDone` flag makes forced-fire mode fire immediately on first tick, then sleep 1 min. `ComputeNextFireDelay` correctly handles the at/before/after fire-time boundaries. |
| Test Quality | PASS | All 7 behaviors have test coverage. `ComputeNextFireDelay` is extracted as `internal static` and tested at four clock positions including the spec's 60-second fuzz boundary. `DbUpdateException` branch exercised via `SaveChangesInterceptor`. `ThrowingEmailQueue` covers the enqueue-failure retry path. `ExecuteAsync_with_now_sentinel_fires_at_least_once` covers the hosted service loop. `expires_at_local` value format is asserted via regex. |

## Behavior Coverage (from M16-008.yaml)

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | 3-day expiry → email queued + `notified_3day_at` set | `Tick_queues_3day_email_and_sets_notified_3day_at` |
| 2 | 1-day expiry → email queued + `notified_1day_at` set | `Tick_queues_1day_email_and_sets_notified_1day_at` |
| 3 | `notified_3day_at` already set → no duplicate | `Tick_does_not_send_duplicate_when_notified_3day_at_already_set`, `TickOnceAsync_is_idempotent_across_two_immediate_invocations` |
| 4 | `preempted_by_subscription` kind → skipped | `Tick_skips_rows_with_preempted_by_subscription_kind` |
| 5 | 09:00 UTC daily fire with ≤60s fuzz | `ComputeNextFireDelay_fuzz_tolerance_within_60_seconds_spec` + three other `ComputeNextFireDelay_*` tests |
| 6 | Only manifest-allowlisted variables in message | `Tick_emits_message_with_manifest_allowlisted_variables_only` (keys + `expires_at_local` value format) |
| 7 | Queue failure → `notified_*_at` NOT set, next tick retries | `Tick_does_not_set_notified_at_when_enqueue_throws` |

## Test Coverage

- `TrialExpiryNotifierOptions.cs`: ~92% line coverage (all branches except one edge of the `TimeOnly` parse path)
- `TrialExpiryNotifier.cs`:
  - `ComputeNextFireDelay`: 100%
  - `TickOnceAsync`: 100%
  - `ProcessPassAsync`: ~95%
  - `ExecuteAsync` state machine: ~73% (real-timer paths not exercised — acceptable for `BackgroundService` infrastructure)
  - `DelayUntilNextFireAsync` state machine: ~43% (real `Task.Delay` paths not testable; the extractable calculation is at 100% via `ComputeNextFireDelay`)
- Overall: improvement report confirms 89.9% line coverage across the two new source files
- Requirement: ≥80% — **MET**

## Program.cs Wiring

`AddHostedService<TrialExpiryNotifier>()` is gated by `!IsEnvironment("Testing")`, correctly preventing the background service from starting under `BackendFactory`. `Configure<TrialExpiryNotifierOptions>` is registered unconditionally so the wiring tests can resolve `IOptions<TrialExpiryNotifierOptions>`. Both verified by `TrialExpiryNotifierWiringTests`.

## Summary

All four findings from the iteration-1 review are fully resolved. The implementation correctly handles the 3-day/1-day dual-pass pattern, per-row idempotency via `notified_*_at` columns, enqueue-failure retry, and `DbUpdateException` recovery. The scheduling accuracy is now directly tested via the extracted `ComputeNextFireDelay` static method with four boundary cases including the spec's ≤60s fuzz tolerance. Build is clean with zero warnings and all 1489 backend tests pass.
