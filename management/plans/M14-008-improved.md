# Improvement Report: M14-008

**Task:** Backend: live Stripe SDK wiring + CreateCheckoutAsync
**Date:** 2026-05-05
**Review:** management/reviews/M14-008-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 2 | High | Wrong env var naming: `StripeOptions.Section = "Stripe"` maps to `STRIPE__*`, not the documented `APITOOL__STRIPE__*` convention | Changed `Section` to `"ApiTool:Stripe"`; updated `Program.cs` config lookup from `"Stripe:Mode"` to `"ApiTool:Stripe:Mode"`; updated error messages and `StripeOptionsTests` key names | ✓ tests pass |
| 1 | High | Dead code: `existingCustomerId` query unreachable because `AlreadySubscribed` guard returns before it | Removed the dead query; replaced with `string? existingCustomerId = null`; documented future-slice reuse pattern as an inline comment | ✓ tests pass |
| 5 | Low | `StripeRateLimitedException` defined in `StripeGateway.cs` alongside the class that throws it | Moved to dedicated file `StripeRateLimitedException.cs` in the same namespace | ✓ tests pass |
| 3 | Medium | Test-injection constructor (`StripeGateway(IOptions, ILogger, StripeClient)`) was `public` | Changed to `internal`; added `<InternalsVisibleTo Include="ApiTool.Backend.Tests" />` to `ApiTool.Backend.csproj` | ✓ tests pass |
| 4 | Medium | Behaviours #4, #5, #6 lacked HTTP endpoint-level tests | Added three tests to `SubscriptionsEndpointsTests.cs`: 400 for invalid_price_id, 503 for StripeRateLimited (via `ThrowingFakeStripeGateway` injected with `WithWebHostBuilder`), and org_id-in-body ignored on price_id path | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build /p:TreatWarningsAsErrors=true` | PASS |
| `dotnet test` (738 tests, excluding stripe-integration which needs docker) | PASS |
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `2ce56a17` | fix(subscriptions): align StripeOptions.Section with APITOOL__STRIPE__* convention | #2 |
| `0d9684e5` | fix(subscriptions): remove dead existingCustomerId query in CreateCheckoutByPriceAsync | #1 |
| `ac62e306` | refactor(subscriptions): move StripeRateLimitedException to its own file | #5 |
| `74c0b8d0` | fix(subscriptions): make StripeGateway test-injection constructor internal | #3 |
| `d9637acd` | test(subscriptions): add HTTP endpoint tests for behaviours #4 #5 #6 | #4 |

## Summary
5/5 findings resolved. 0 deferred.
