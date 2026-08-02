# Improvement Report: M16-001

**Task:** ITierGate generic abstraction with per-feature adapters
**Date:** 2026-05-07
**Review:** management/reviews/M16-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | Locality regex `[^)]*?` forbids `)` chars — pattern cannot match `.Select(s => (SubscriptionTier?)s.Tier)` because the cast introduces `)` before `.Tier`. Test was a non-functional enforcement mechanism. | Replaced `[^)]*?` with `[\s\S]{0,100}?\.Tier[\s\S]{0,10}?\)` requiring `.Tier` to appear *inside* the `.Select(...)` parentheses. Added a self-test assertion that positively matches the known query shape before scanning files. Fixed a secondary false-positive against `SubscriptionsService.cs` where `.Tier` appeared after a non-tier `.Select(s => s.StripeCustomerId)` close paren. | ✓ 2 locality tests pass |
| 2 | Medium | Multi-row `OrderByDescending` edge case tested with single-row in-place modification, not two actual subscription rows. | Added `EnsureAsync_uses_most_recent_subscription_row_when_multiple_exist` that drops the `IX_subscriptions_OrgId` unique index and inserts two rows via `ExecuteSqlRawAsync` (FK checks disabled with `PRAGMA foreign_keys = OFF`): older=Enterprise, newer=Free. Asserts `TierIneligible` for Team requirement. | ✓ 13 TierGateTests pass |
| 3 | Medium | Behavior 8 (DI seam decoupling) not demonstrated at endpoint level — only at adapter unit level. | Added `vault_config_probe_returns_200_with_fake_ITierGate_without_seeding_database` endpoint test that replaces `ITierGate` in DI with `AlwaysAllowedFakeTierGate` via `WithWebHostBuilder`, then verifies the probe returns 200 with no org data seeded, proving the endpoint layer is decoupled from EF via the DI seam. | ✓ 8 InternalTierGateProbeEndpointTests pass |
| 4 | Low | `featureCode` parameter in `TierGateProblemFactory.AuthenticatedTierIneligible` lacked a null/empty guard; null produced `"null_tier_ineligible"`. | Added `ArgumentException.ThrowIfNullOrEmpty(featureCode)` at the top of the method. Added two guard tests: one for null input and one for empty string. | ✓ 8 TierGateProblemFactoryTests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `dotnet build src/ApiTool.Backend.Tests` | PASS (0 warnings) |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1300 passed, 8 skipped — Stripe integration, expected) |
| TierGate-filtered suite | PASS (40 tests, up from 35) |
| Coverage (overall line rate) | 94.5% |
| Coverage (TierGates new files, line rate) | TierGate.cs 100%, TierGateProblemFactory.cs 100%, InternalTierGateProbeEndpoint.cs 100%, adapters ~86% (unreachable `UnreachableException` arm — acceptable per review) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `3aeaf894` | fix(tiergate): correct locality regex to match actual tier-projection pattern | #1 |
| `09e3c294` | test(tiergate): add two-row OrderByDescending edge case test | #2 |
| `a1717f6b` | test(tiergate): add DI seam decoupling endpoint test (Behavior 8) | #3 |
| `b93170ae` | fix(tiergate): add null/empty guard for featureCode in TierGateProblemFactory | #4 |

## Summary

4/4 findings resolved. 0 deferred.

The critical finding (#1) required careful analysis: the first regex fix (`[\s\S]*?`) introduced a false-positive match against `SubscriptionsService.cs` where `.Tier` appeared as a property assignment after a non-tier `.Select()`. The final regex `[\s\S]{0,100}?\.Tier[\s\S]{0,10}?\)` correctly anchors `.Tier` to appear *inside* the `.Select(...)` parentheses by requiring a closing `)` within 10 chars of `.Tier`, eliminating both the original `[^)]*?` failure and the false-positive.
