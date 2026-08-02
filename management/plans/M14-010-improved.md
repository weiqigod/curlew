# Improvement Report: M14-010

**Task:** Backend: ComputeProrationAsync via Stripe upcoming-invoice preview
**Date:** 2026-05-05
**Review:** management/reviews/M14-010-review.md

## Resolved Findings (Iteration 1 — Review 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Dead `StripeRateLimited` switch case in `PreviewProration` endpoint | Moved `StripeRateLimitedException` catch into service; switch arm is now reachable. | ✓ tests pass |
| 2 | Medium | Weak `renewal_date` assertion (`NotBe(JsonValueKind.Undefined)`) | Changed to `NotBe(JsonValueKind.Null)` — verifies actual date value. | ✓ tests pass |
| 3 | Medium | No service-level test for non-short-circuit path | Added `PreviewProrationAsync_calls_gateway_and_returns_result_for_tier_change`. | ✓ tests pass |
| 4 | Low | No endpoint tests for 502/503 on preview-proration | Added `PreviewProration_with_stripe_rate_limit_returns_503` and `PreviewProration_with_stripe_5xx_returns_502_STRIPE_UNAVAILABLE`. | ✓ tests pass |
| 5 | Low | Redundant `DerivePlanFromPriceId` wrapper in `SubscriptionsService` | Removed wrapper; call `StripePriceParser.Parse` directly. | ✓ tests pass |

## Resolved Findings (Iteration 2 — Review 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `FakeStripeGateway.ComputeProrationAsync` always returned zero — called `ComputeProration(toTier, newQuantity, toTier, newQuantity, "month")` with the same tier/seats for both from and to | Changed baseline "from" to `SubscriptionTier.Free` / 0 seats; used `StripePriceParser.Parse` for correct interval. Any paid target now returns a positive charge (e.g. Team/3 monthly = 14 700 cents). | ✓ tests pass |
| 2 | Medium | Service test `PreviewProrationAsync_calls_gateway_and_returns_result_for_tier_change` had trivially-true assertion `amount.Should().Be(charge - credit)` (0 == 0 − 0) | Strengthened to `amount.Should().BeGreaterThan(0)`, `charge.Should().BeGreaterThan(0)`, `credit.Should().Be(0)`. | ✓ tests pass non-trivially |
| 3 | Low | `FakeStripeGateway.ComputeProration` (sync) had own `s_monthlyCentsPerSeat` dictionary — diverged from `StripeProrationLocalMath` for yearly billing (missing ×12 multiplier) | Removed duplicate dictionary; delegated to `StripeProrationLocalMath.Compute(fromTier, fromSeats, toTier, toSeats, interval)`. | ✓ yearly billing now consistent between fake and live gateway |

## Resolved Findings (Iteration 3 — Review 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Task observable filter `~StripeProration` only matched 2 skippable stripe-mock tests, making `>=6` threshold unreachable | Updated `management/tasks/M14-010.yaml` observable to use `"FullyQualifiedName~Proration"` — captures all 21 proration tests (19 passed, 2 skipped). | ✓ `dotnet test --filter "FullyQualifiedName~Proration"` → 19 passed, 2 skipped |
| 2 | Low | Unchecked `long→int` cast in `StripeGateway.ComputeProrationAsync` lines 249-251 — silently wraps for invoices exceeding ~$21M in cents | Added `checked(...)` wrapper on all three casts; added comment documenting the accepted practical ceiling. | ✓ tests pass; build clean |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS — 768 passed, 8 skipped (stripe-mock integration), 0 failed |
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS — 0 issues |
| Coverage | 93.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `59e07b77` | fix(subscriptions): strengthen renewal_date assertion | Review-1 #2 |
| `22a8be08` | fix(subscriptions): resolve dead StripeRateLimited switch arm | Review-1 #1 |
| `b2cb5608` | refactor(subscriptions): remove redundant DerivePlanFromPriceId wrapper | Review-1 #5 |
| `333d369d` | test(subscriptions): add service-level non-short-circuit test | Review-1 #3 |
| `ed25850b` | test(subscriptions): add 502/503 endpoint tests for preview-proration | Review-1 #4 |
| `394eb546` | fix(subscriptions): correct FakeStripeGateway.ComputeProrationAsync and consolidate sync proration | Review-2 #1, #2, #3 |
| `74377816` | fix(subscriptions): resolve iter-3 review findings for M14-010 | Review-3 #1, #2 |

## Summary

10/10 findings resolved across three review iterations. 0 deferred.
