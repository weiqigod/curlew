using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>
/// Integration tests for <see cref="StripeGateway.CreatePortalSessionAsync"/> against stripe-mock.
/// Requires stripe-mock running at APITOOL__STRIPE__APIBASE (default http://localhost:12111).
/// Skipped automatically when stripe-mock is not reachable.
///
/// NOTE: stripe-mock replays idempotency keys for billing-portal, which differs from real Stripe
/// behaviour (real Stripe does not deduplicate billing-portal sessions by idempotency key).
/// The test asserts stripe-mock semantics only; a code comment flags the divergence.
/// </summary>
[Trait("Category", "stripe-integration")]
public sealed class StripeBillingPortalIntegrationTests : IClassFixture<StripeMockFixture>
{
    private readonly StripeMockFixture _mock;

    public StripeBillingPortalIntegrationTests(StripeMockFixture mock) => _mock = mock;

    private StripeGateway BuildGateway() => new(
        Options.Create(new StripeOptions
        {
            Mode = "live",
            ApiKey = Environment.GetEnvironmentVariable("APITOOL__STRIPE__APIKEY")
                     ?? "sk_test_123",
            ApiBase = _mock.BaseUrl,
        }),
        NullLogger<StripeGateway>.Instance);

    /// <summary>Behaviour: live portal call with a real customer id returns a URL.</summary>
    [SkippableFact]
    public async Task CreatePortalSessionAsync_returns_billing_portal_url()
    {
        Skip.IfNot(_mock.IsAvailable);
        var gateway = BuildGateway();

        // Mint a customer via checkout first so we have a real cus_* id from stripe-mock.
        var checkout = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            idempotencyKey: Guid.NewGuid().ToString());
        var customerId = checkout.CustomerId!;

        var portal = await gateway.CreatePortalSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), "https://app/billing",
            customerId: customerId,
            idempotencyKey: Guid.NewGuid().ToString());

        portal.PortalUrl.Should().StartWith("https://");
    }

    /// <summary>
    /// Behaviour #3: replay with same idempotency key returns equivalent session (stripe-mock semantic).
    /// NOTE: Real Stripe does NOT deduplicate billing-portal sessions by idempotency key — this test
    /// asserts stripe-mock behaviour only. The assertion would fail against the live Stripe API.
    /// </summary>
    [SkippableFact]
    public async Task Replay_with_same_idempotency_key_returns_equivalent_session()
    {
        Skip.IfNot(_mock.IsAvailable);
        var gateway = BuildGateway();

        var checkout = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            idempotencyKey: Guid.NewGuid().ToString());
        var customerId = checkout.CustomerId!;
        var key = Guid.NewGuid().ToString();

        var p1 = await gateway.CreatePortalSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), "https://app/billing",
            customerId: customerId, idempotencyKey: key);
        var p2 = await gateway.CreatePortalSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), "https://app/billing",
            customerId: customerId, idempotencyKey: key);

        // stripe-mock replays the same response for matching idempotency keys.
        p1.PortalUrl.Should().Be(p2.PortalUrl);
    }
}
