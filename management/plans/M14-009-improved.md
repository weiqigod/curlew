# Improvement Report: M14-009

**Task:** Backend: Stripe billing portal session
**Date:** 2026-05-05
**Review:** management/reviews/M14-009-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `CreateBillingPortal` endpoint missing `.Produces<ProblemDetails>(StatusCodes.Status503ServiceUnavailable)` — Swagger did not document the rate-limit (503) response code | Added `.Produces<ProblemDetails>(StatusCodes.Status503ServiceUnavailable)` to the billing-portal fluent chain in `SubscriptionsEndpoints.cs` (line 59) | ✓ tests pass |
| 2 | Low | No endpoint-level integration test for the Stripe rate-limit → HTTP 503 path on `POST /api/v1/subscriptions/billing-portal` | Added `ThrowingRateLimitedPortalGateway` test double and `BillingPortal_with_stripe_rate_limit_returns_503` test in `SubscriptionsEndpointsTests.cs`; test seeds a subscription with `stripe_customer_id`, registers the throwing gateway via `WithWebHostBuilder`, and asserts HTTP 503 + `code: stripe_rate_limited` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `dotnet test` | PASS (752 passed, 6 skipped, 0 failed) |
| Coverage | Go gate: all packages cached/passing; .NET gate: 752 tests |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 75b1bbb7 | fix(subscriptions): add 503 Swagger declaration and rate-limit test for billing-portal | #1, #2 |

## Summary

2/2 findings resolved. 0 deferred.
