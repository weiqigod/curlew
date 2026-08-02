# Code Review: M16-006

**Task:** ITrialStateResolver and LicenseTokenIssuer wiring with tier-upgrade preemption
**Reviewer:** AI
**Date:** 2026-05-10
**Branch:** feature/M16-006-trial-state-resolver
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All three findings from the previous review (iteration 1) have been resolved:

| Prior # | Severity | Fix Status |
|---------|----------|------------|
| 1 | Medium | RESOLVED — `log.LogDebug("trial_seed_race_discarded ...")` added inside the `DbUpdateException` catch block in `TrialSeederService.cs:63-71` |
| 2 | Low | RESOLVED — Belt-and-suspenders comment added to `DatabaseTrialStateResolver.cs:26-28` explaining that `ExpiresAt = now()` at preemption already excludes preempted rows and the explicit `Kind` guard makes intent unambiguous |
| 3 | Low | RESOLVED — `(await scope.Db.Trials.CountAsync()).Should().Be(TrialFeatures.All.Count)` assertion added to `RunAsync_empty_db_creates_admin_user_and_default_org` at line 79 |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Race-condition catch in `TrialSeederService` now logs at Debug before returning 0. `AdminBootstrap` catch-all re-throws after rollback. `CurrentUserAccessor` race catch is a pre-existing pattern outside this task's scope. No errors swallowed silently in M16-006 code. |
| Input Validation | PASS | `ResolveAsync` handles any tier string via `string.Equals(tier, "free", OrdinalIgnoreCase)`. Seeder uses `AnyAsync` guard before bulk-insert. Handler early-returns on null owner. `InternalRefreshSeedEndpoint` validates email with `IsNullOrWhiteSpace`. |
| Naming | PASS | No stuttering (`TrialFeatures`, not `TrialFeaturesConfig`; `ITrialStateResolver`, not `ITrialStateResolverInterface`). All exported types, methods, fields, and properties have XML doc comments. Single-method interface does not use `-er` suffix, which is correct — resolver semantics are not well-expressed as `ITrialStateResolving`. `TrialingFeatures` is unambiguous. |
| Code Organization | PASS | `internal/` project boundaries respected. Each class has single responsibility (`TrialSeederService` seeds only; `DatabaseTrialStateResolver` reads only; `LicenseTokenIssuer` mints only). DI registrations placed correctly in `Program.cs:392-394`. No circular dependencies introduced. |
| Correctness | PASS | UTC handling consistent: `ExpiresAt` stored as `DateTime` UTC, converted via `new DateTimeOffset(earliest, TimeSpan.Zero)` before `.ToUnixTimeSeconds()`. `anyRow` check correctly includes all row kinds (expired, preempted, consumed) — any row proves the user was registered. `tier != "free"` comparison is forward-compatible. Preemption idempotency verified: filter `Kind != PreemptedBySubscription && ExpiresAt > now` skips already-preempted rows cleanly. `Seeder` called after `FindAsync` confirms user exists in `CurrentUserAccessor`. |
| Test Quality | PASS | All 8 spec behaviors have at least one test. Table-driven tests used for tier-override and `MapStatus` branches. Edge cases covered: no owner, already-preempted rows, no trial rows, idempotent re-seed, concurrent seed. `TestDb.CreateOpen()` with real SQLite migrations used for constraint-sensitive tests. `FakeTrialStateResolver` enables unit testing without DB. `SeedRefreshTokenAsync` helper correctly seeds trial rows to match production path. |

## Test Coverage

- Go gate: 87.3% overall (unchanged — M16-006 is C# only)
- C# backend: all tests pass per improvement report (`dotnet test` 29/29)
- All 8 task behaviors covered:
  - Behavior #1 (no rows → none): `No_rows_returns_none`
  - Behavior #2 (active full_initial → active + soonest expiry): `Active_full_initial_rows_return_active_with_earliest_expiry`
  - Behavior #3 (all expired, no on-demand → expired): `All_full_initial_rows_expired_returns_expired_when_no_ondemand`
  - Behavior #4 (active on-demand → active + feature in features[]): `Active_ondemand_row_returns_active_and_includes_feature`
  - Behavior #5 (resolver consulted → JWT carries trial_state/trial_expiry): `IssueAsync_populates_trial_state_active_with_expiry_when_resolver_returns_active`, `IssueAsync_passes_input_tier_to_resolver`, `Trial_fields_reflect_seeded_full_initial_for_new_user`
  - Behavior #6 (subscription.created preempts owner's trials): `Created_event_for_paid_tier_preempts_owner_active_trials`
  - Behavior #7 (registration seeds one full_initial per feature): `Seeds_one_row_per_known_feature_for_new_user`, `RunAsync_seeds_full_initial_trial_rows_for_admin_user`, `RunAsync_empty_db_creates_admin_user_and_default_org` (now includes assertion)
  - Behavior #8 (active subscription → trial_state none): `Active_subscription_overrides_to_none_regardless_of_rows`
- Missing coverage: none identified

## Summary

The M16-006 implementation is architecturally sound and all prior review findings have been addressed correctly. The `ITrialStateResolver` / `DatabaseTrialStateResolver` / `TrialSeederService` chain is well-designed, DI wiring is correct, preemption logic handles idempotency and edge cases, and all 8 spec behaviors are covered by tests. The CHANGELOG has been updated. The code is ready for `/verify`.
