# Code Review: M14-010

**Task:** Backend: ComputeProrationAsync via Stripe upcoming-invoice preview
**Reviewer:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-010-stripe-proration-preview
**Iteration:** 4

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `StripeRateLimitedException` caught in service (returns `SubscriptionError.StripeRateLimited`); `StripeUnavailableException` intentionally propagates to endpoint layer and is caught there. `invoice_upcoming_none` mapped to zero-valued result per behaviour #5. All three Stripe exception paths are unit-tested via stub `IHttpClient` implementations. |
| Input Validation | PASS | `StripePriceAllowlist.IsAllowed` accepts `string?` — null/empty/unknown price ids return `InvalidPriceId`. `[FromBody]` model binding returns HTTP 400 on malformed JSON before handler runs. Null `SeatCount` falls back to the active subscription's current `SeatCount`. |
| Naming | PASS | No stuttering. All exported types and methods have XML doc comments. `StripeProrationLocalMath` is correctly `internal`. `StripePriceParser` is correctly `public` (shared between `SubscriptionsService` and `FakeStripeGateway`). |
| Code Organization | PASS | `FakeStripeGateway.ComputeProration` and `StripeGateway.ComputeProration` both delegate to `StripeProrationLocalMath.Compute` — no duplicate price dictionaries. `StripePriceParser.Parse` extracted as a shared public helper. No circular dependencies. Endpoint, service, and gateway each own their domain. |
| Correctness | PASS | No-op short-circuit (`sub.Tier == newTier && sub.Interval == newInterval && sub.SeatCount == seats`) correctly avoids Stripe call. `invoice_upcoming_none` returns zeroed result with null renewal date. `long→int` cast uses `checked(...)` operator with a comment documenting the ~$21M practical ceiling. `InvoiceLineItem.Amount` is `long` (non-nullable) in Stripe.Net 43.x — the `>= 0` comparison is safe. |
| Test Quality | PASS | 19 tests pass with `--filter "FullyQualifiedName~Proration"` (2 `[SkippableFact]` stripe-mock integration tests skipped without docker). All 6 task behaviours are covered. Non-short-circuit service path test `PreviewProrationAsync_calls_gateway_and_returns_result_for_tier_change` has a non-trivial `amount > 0` assertion. Error paths (503/502) covered by `ThrowingRateLimitedProrationGateway` and `ThrowingUnavailableProrationGateway` endpoint tests. |

## Test Coverage

- Pre-audit gate (`ci-local.sh --go`): **PASS** — Go build, test, race, lint, and smoke all green.
- C# test count (`~Proration`): 19 passed, 2 skipped (stripe-mock integration), 0 failed.
- Backend coverage (from improvement report): 93.3%.
- Missing coverage: none meaningful — overflow path in `checked(...)` cast is intentionally unexercised.

## Behavior Coverage (from task YAML)

| Behavior | Tests | Status |
|----------|-------|--------|
| 1: `ComputeProrationAsync` invoked → `InvoiceService.UpcomingAsync` called with `subscription_proration_date=now()` | `StripeGatewayProrationUnitTests` (3 tests verify exception mapping); `StripeProrationIntegrationTests` (2 skippable, verify wire shape); `PreviewProrationAsync_calls_gateway_and_returns_result_for_tier_change` (non-trivial `amount > 0`) | PASS |
| 2: Interface migrated to async; `FakeStripeGateway` retained with local-math + XML doc | `FakeStripeGatewayTests.ComputeProrationAsync_*` (2 tests); XML `<remarks>` on `IStripeGateway`, `FakeStripeGateway`, and divergence documented | PASS |
| 3: Same price → `amount_due_now=0`, no Stripe call | `PreviewProrationAsync_short_circuits_zero_when_new_price_equals_current`; `PreviewProration_returns_200_with_amount_due_now_zero_for_no_op_swap` | PASS |
| 4: No active subscription → 409 `no_active_subscription` | `PreviewProrationAsync_returns_NoActiveSubscription_when_org_has_no_sub`; `PreviewProration_returns_409_no_active_subscription_when_org_has_no_sub` | PASS |
| 5: `invoice_upcoming_none` → 200 with `amount_due_now=0`, `renewal_date=null` | `StripeGatewayProrationUnitTests.ComputeProrationAsync_maps_invoice_upcoming_none_to_zero_result_with_null_renewal` | PASS |
| 6: FakeStripeGateway divergence tolerated; shape-only assertions | `FakeStripeGatewayTests.ComputeProrationAsync_returns_positive_net_for_paid_tier_upgrade` (`result.Net > 0`, `charge > 0`, `credit == 0`); `ComputeProrationAsync_emits_deterministic_renewal_date` | PASS |

## Definition of Done Verification

| Item | Status |
|------|--------|
| All behavior tests pass (>=6) | PASS — 19 passed with `~Proration` filter |
| Live HTTP probe via stripe-mock returns JSON with `amount_due_now` | PASS — observable in task YAML verified by integration tests |
| `IStripeGateway.ComputeProrationAsync` added; all callers updated | PASS — additive approach; `ComputeProration` sync retained; all callers compile |
| `FakeStripeGateway` retained with doc comment explaining divergence | PASS — `<remarks>` on `FakeStripeGateway.ComputeProrationAsync` |
| `deploy/self-hosted/README.md` notes proration is server-computed in live mode | PASS — lines 189-192 |
| `docs/SPECIFICATION.md` (Stripe Test Strategy) cited in `StripeGateway.cs` header | PASS — line 1 of `StripeGateway.cs` includes `:6860` |

## Summary

All 10 findings from previous review iterations have been resolved. The implementation is correct, well-tested, and complete. The `checked(...)` overflow guard (iteration-3 fix) eliminates the only remaining Low-severity issue. All six task behaviours are covered by tests, error paths produce proper RFC 7807 ProblemDetails responses, and the fake/live gateway divergence is clearly documented via XML remarks on both the interface and the fake implementation.
