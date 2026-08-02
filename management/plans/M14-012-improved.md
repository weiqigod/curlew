# Improvement Report: M14-012

**Task:** Backend: Stripe subscription + customer event handlers
**Date:** 2026-05-06
**Review:** management/reviews/M14-012-review.md

## Iteration 1 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `MapStatus` default branch silently resolves unknown Stripe status strings to `Incomplete` with no log entry | Converted `MapStatus` from `static` to instance method; added `log.LogWarning` in the default branch for unknown statuses | ✓ tests pass, MapStatus 100% branch |
| 2 | High | `HandleDeletedAsync` `owner is null` branch untested; silent skip with no log when no org owner found | Added `log.LogWarning` in the else branch; added test `Deleted_event_with_no_owner_skips_email_and_does_not_throw` | ✓ tests pass |
| 3 | Medium | Dead code secondary lookup in `HandleCreatedOrUpdatedAsync` (StripeCustomerId query was a strict subset of the first OR clause) | Removed the dead secondary query; replaced with direct log.LogWarning + return when first query returns null | ✓ tests pass |
| 4 | Medium | Integration test used hardcoded event id `evt_integ_del_1`; idempotency store deduplication would cause flakiness on repeated runs | Generated a unique event id per run using `Guid.NewGuid()` | ✓ tests pass |
| 5 | Low | `MapStatus` branch coverage 18.42% — only 7 of 19 branches exercised | Added theory test covering all 9 documented status strings plus unknown default | ✓ MapStatus 100% branch, 100% line |

## Iteration 2 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `HandleCreatedOrUpdatedAsync` "Stripe returns valid sub but no local row matches" path (lines 59–67) was at 0 hits — no test covered this scenario | Added test `Created_event_for_valid_subscription_with_no_local_row_is_noop`: seeds no subscription row, primes gateway with a valid Stripe.Subscription for a different customer id, asserts handler returns without throwing and no row is created | ✓ tests pass, path now covered |
| 2 | Medium | `HandleDeletedAsync` "no local row found" early-return path (lines 102–107) was at 0 hits | Added test `Deleted_event_with_no_local_row_is_noop`: seeds no subscription row, calls `HandleDeletedAsync`, asserts no throw, no email enqueued, no rows created | ✓ tests pass, path now covered |
| 3 | Medium | `StripeCustomerUpdatedHandler.HandleAsync` customer-404 path (lines 25–27) was at 0 hits; existing Test 10 scripted a non-null customer | Added test `Customer_updated_returns_when_stripe_customer_is_404`: primes FakeStripeGateway with no customer registered so GetCustomerAsync returns null, seeds a subscription row, asserts no throw and row.StripeCustomerEmail remains null | ✓ tests pass, path now covered |

## Out of Scope (Deferred)

No findings deferred. All findings from both iterations resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (943 passed, 8 skipped — stripe-mock integration tests) |
| Overall line coverage | 93.6% |
| `StripeCustomerUpdatedHandler.HandleAsync` | 100% line, 100% branch |
| `StripeSubscriptionHandler.HandleCreatedOrUpdatedAsync` | 100% line |
| `StripeSubscriptionHandler.HandleDeletedAsync` | 100% line |
| `StripeSubscriptionHandler.MapStatus` | 100% line, 100% branch |
| `SubscriptionsService.GetForOrgAsync` new branches | 100% (canceled + quarantined + active tested) |

## Fix Commits

### Iteration 1

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| e0ff9b30 | fix(webhooks): add log warnings for unknown Stripe status and missing owner | #1, #2, #3 |
| 4bccf767 | test(webhooks): add tests for owner-not-found branch and MapStatus all branches | #2, #5 |
| df863385 | fix(webhooks): use unique event id in integration test to prevent flakiness | #4 |

### Iteration 2

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 1fc9e051 | test(webhooks): add tests for 3 untested early-return paths in handlers | #1, #2, #3 (iteration 2) |

## Summary

Iteration 1: 5/5 findings resolved.
Iteration 2: 3/3 findings resolved.
Total: 8/8 findings resolved across both iterations. 0 deferred.
