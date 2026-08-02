# Code Review: M16-001

**Task:** ITierGate generic abstraction with per-feature adapters
**Reviewer:** AI
**Date:** 2026-05-07
**Branch:** feature/M16-001-tiergate-abstraction
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Previous Findings — Verification

| # | Severity | Finding (Review 1) | Fixed? | Evidence |
|---|----------|--------------------|--------|---------|
| 1 | Critical | Locality regex `[^)]*?` could not match `.Select(s => (SubscriptionTier?)s.Tier)` — enforcement test was a no-op | FIXED | Regex replaced with `[\s\S]{0,400}?\.Select\([\s\S]{0,100}?\.Tier[\s\S]{0,10}?\)`. Self-test assertion `Regex_positively_matches_known_tier_projection_pattern` added and passing. Both locality tests pass. |
| 2 | Medium | Multi-row `OrderByDescending` edge case not tested with two actual rows | FIXED | `EnsureAsync_uses_most_recent_subscription_row_when_multiple_exist` drops `IX_subscriptions_OrgId` and inserts two rows via `ExecuteSqlRawAsync` with explicit `UpdatedAt` timestamps: older=Enterprise, newer=Free. Asserts `TierIneligible`. Test passes. |
| 3 | Medium | Behavior 8 (DI seam decoupling) not covered at endpoint level | FIXED | `vault_config_probe_returns_200_with_fake_ITierGate_without_seeding_database` test added: replaces `ITierGate` in DI with `AlwaysAllowedFakeTierGate`, calls probe with no org data seeded, asserts 200. Test passes. |
| 4 | Low | `featureCode` in `TierGateProblemFactory.AuthenticatedTierIneligible` had no null/empty guard | FIXED | `ArgumentException.ThrowIfNullOrEmpty(featureCode)` added at method entry. Two guard tests added (`throws_when_featureCode_is_null`, `throws_when_featureCode_is_empty`). Both pass. |

## Fresh Pass Findings

None.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All switch expressions over `TierGateResult` include `_ => throw new UnreachableException()` default arms. `featureCode` null/empty guard uses `ArgumentException.ThrowIfNullOrEmpty`. No swallowed errors. No panics on expected failures. `SsoTierGate` switch also has `UnreachableException` default arm. |
| Input Validation | PASS | `featureCode` guard added (Finding #4 fix). `ITierGate gate` and `HttpContext http` parameters are non-nullable; project enables `<Nullable>enable</Nullable>` which enforces this at call sites. |
| Naming | PASS | No stuttering. Short names in tight scopes, descriptive at package level. All exported types, enums, and methods have XML doc comments. `ITierGate` follows `-I` interface convention. `TierGateResult` enum is a discriminated result type with three well-named values. |
| Code Organization | PASS | All tier-check EF queries live under `Internal/TierGates/`. `SsoTierGate` correctly delegates via the new abstraction without modifying `SsoService.cs` or `OidcService.cs`. DI registration is in `Program.cs` alongside other `AddScoped` calls. `TierGate` is `internal sealed`. Probe endpoint is registered only in non-Production environments. Locality invariant is machine-enforced by `TierCheckLocalityTests`. |
| Correctness | PASS | All three `TierGateResult` variants handled in every switch. `OrderByDescending(s.UpdatedAt)` multi-row semantics tested with two actual rows. Missing subscription row treated as `Free` via `?? SubscriptionTier.Free`. Pre-cancelled token propagates `OperationCanceledException` via EF Core's async path. Production guard `if (env.IsProduction()) return` confirmed by dedicated test. |
| Test Quality | PASS | All 8 behaviors from the task YAML covered. Locality self-test prevents a broken regex from producing a false-passing enforcement test. Multi-row edge case uses actual two-row seed with explicit timestamps. Behavior 8 DI seam proven at endpoint level via `WebApplicationFactory` DI replacement. Table-driven tests (`Theory + MemberData`) used for Allowed/Ineligible matrices. All assertions specific (checking exact enum values, status codes, body fields, headers). |

## Test Coverage

- TierGate-filtered suite: **40 tests passing** (up from 35 pre-improve, 40 post-improve)
- `TierGate.cs`: 100% (8 test cases including multi-row ordering, cancellation, missing-subscription-row)
- `TierGateProblemFactory.cs`: 100% (8 test cases including tier serialisation, null/empty guards, header assertions)
- `InternalTierGateProbeEndpoint.cs`: 100% (8 test cases including Production guard, DI fake injection)
- `VaultConfigTierGate.cs`, `ScheduleExecutorTierGate.cs`, `DashboardTierGate.cs`: ~86% (unreachable `UnreachableException` arm — acceptable; all three result translations tested)
- `TierCheckLocalityTests`: 2 tests (self-test regex + file sweep — both pass)
- `SsoTierGate.cs`: 0% in TierGate-filtered run; exercised by full `SsoServiceTests`/`OidcServiceTests` suites (all pass in `ci-local.sh --go`)
- Missing coverage: none — all task behaviors covered

## Summary

All four findings from Review 1 are correctly fixed and verified. The critical locality-enforcement fix includes a self-test assertion that guards against future regex breakage. The multi-row edge case now uses two actual subscription rows with distinct `UpdatedAt` timestamps. Behavior 8 DI decoupling is proven at the endpoint level. The `featureCode` null/empty guard is in place with tests. The fresh review pass finds no new issues: architecture is sound, all behaviors covered, standards compliance across all categories.
