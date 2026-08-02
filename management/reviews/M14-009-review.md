# Code Review: M14-009

**Task:** Backend: Stripe billing portal session
**Reviewer:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-009-stripe-billing-portal
**Iteration:** 2 (re-review after /improve)

## Verdict: PASS

## Findings

No findings. All issues from review iteration 1 have been resolved.

## Resolved Findings from Iteration 1

| # | Severity | Category | Finding | Resolution |
|---|----------|----------|---------|------------|
| 1 | Medium | Correctness / Swagger | `CreateBillingPortal` endpoint missing `.Produces<ProblemDetails>(StatusCodes.Status503ServiceUnavailable)` | Fixed: `.Produces<ProblemDetails>(StatusCodes.Status503ServiceUnavailable)` added to the fluent chain at `SubscriptionsEndpoints.cs:60` |
| 2 | Low | Test Quality | No endpoint-level test for the rate-limit (503) path on `POST /api/v1/subscriptions/billing-portal` | Fixed: `BillingPortal_with_stripe_rate_limit_returns_503` added to `SubscriptionsEndpointsTests.cs` (lines 505–556), using `ThrowingRateLimitedPortalGateway` — mirrors the checkout rate-limit test pattern exactly |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All `StripeException` catch clauses use `when` guards. `StripeUnavailableException` carries `RequestId` from `ex.StripeResponse?.RequestId` (correct — HTTP-header path, not `StripeError` which has no `RequestId` property). `StripeUnavailableException` intentionally escapes the service layer to preserve `RequestId` at the endpoint. No swallowed errors. |
| Input Validation | PASS | `customerId` null-check in `StripeGateway.CreatePortalSessionAsync` provides a clear fail-fast message. `BillingPortalRequest` is nullable; handler defaults to `App:WebAppUrl + "/billing"` when absent. `AppOptions.WebAppUrl` validated unconditionally at startup via `ValidateOnStart`. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. `SubscriptionsProblem`, `BillingPortalRequest`, `BillingPortalResponse`, `StripeUnavailableException` follow project conventions. |
| Code Organization | PASS | `StripeUnavailableException` is parallel to `StripeRateLimitedException`. `SubscriptionsProblem` mirrors `RefreshProblem`. Service layer isolates business logic; gateway layer isolates Stripe SDK calls. |
| Correctness | PASS | `StripeResponse.RequestId` is the correct property (HTTP header path). Idempotency key truncated to 255 chars. `OrderBy(m => m.JoinedAt)` provides deterministic org resolution for multi-org users. All HTTP response codes documented in Swagger. |
| Test Quality | PASS | All 6 task behaviors covered. Rate-limit 503 path for `/billing-portal` now covered by endpoint-level integration test. Unit stubs cover null-customerId guard and 5xx mapping without network calls. |

## Test Coverage

- Coverage: Go gate passes (C# backend). All dotnet tests pass per `./scripts/ci-local.sh --go` output.
- Missing coverage: None.

## Behavior Coverage

| Behavior | Test | Status |
|----------|------|--------|
| #1: CreateBillingPortalAsync with customer_id calls SDK with return_url from WebAppUrl | `BillingPortal_with_empty_body_returns_url_using_default_return_url` + `CreatePortalSessionAsync_returns_billing_portal_url` | PASS |
| #2: No stripe_customer_id → 409 no_billing_setup | `BillingPortal_with_no_stripe_customer_id_returns_409_no_billing_setup` + `CreateBillingPortalAsync_returns_NoBillingSetup_when_org_has_no_stripe_customer_id` | PASS |
| #3: Idempotency replay returns equivalent session (stripe-mock) | `Replay_with_same_idempotency_key_returns_equivalent_session` (SkippableFact) | PASS (conditional on stripe-mock) |
| #4: Stripe 5xx → RFC 7807 STRIPE_UNAVAILABLE 502 with request_id | `BillingPortal_with_stripe_5xx_returns_502_STRIPE_UNAVAILABLE_with_request_id` + `StripeGatewayPortalUnitTests` | PASS |
| #5: Member role → 403 permission_denied | `BillingPortal_with_member_role_returns_403_permission_denied` + `CreateBillingPortalAsync_returns_PermissionDenied_for_member_role` | PASS |
| #6: App:WebAppUrl unset → boot-time fail-fast | `AppOptionsTests` + `BackendFactory` config seed + `SchemaGuardMiddlewareTests` | PASS |

## Summary

The implementation is complete and correct. Both findings from iteration 1 have been resolved: the Swagger contract for `billing-portal` now documents the 503 response code, and an endpoint-level integration test covers the rate-limit → 503 path using the same `WithWebHostBuilder` pattern as the checkout endpoint. All six task behaviors are covered by tests, error propagation is correct throughout the gateway → service → endpoint chain, and the `AppOptions` validation prevents misconfigured deployments from serving broken portal `return_url` values.
