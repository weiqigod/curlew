using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>
/// Integration tests for <see cref="StripeGateway"/> against stripe-mock.
/// Requires stripe-mock running at APITOOL__STRIPE__APIBASE (default http://localhost:12111).
/// Skipped automatically when stripe-mock is not reachable.
/// </summary>
[Trait("Category", "stripe-integration")]
public sealed class StripeCheckoutIntegrationTests : IClassFixture<StripeMockFixture>
{
    private readonly StripeMockFixture _mock;

    public StripeCheckoutIntegrationTests(StripeMockFixture mock) => _mock = mock;

    private StripeGateway BuildGateway() => new(
        Options.Create(new StripeOptions
        {
            Mode = "live",
            ApiKey = Environment.GetEnvironmentVariable("APITOOL__STRIPE__APIKEY")
                     ?? "sk_test_123",
            ApiBase = _mock.BaseUrl,
        }),
        NullLogger<StripeGateway>.Instance);

    /// <summary>Behaviour #1: CreateCheckoutSessionAsync carries a per-call IdempotencyKey and returns a session URL.</summary>
    [SkippableFact]
    public async Task CreateCheckoutSessionAsync_passes_idempotency_key_to_stripe()
    {
        Skip.IfNot(_mock.IsAvailable);
        var gateway = BuildGateway();
        var idempotencyKey = Guid.NewGuid().ToString();

        var session = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "https://app.example/ok", "https://app.example/no",
            priceId: "price_test_team_monthly",
            existingCustomerId: null,
            idempotencyKey: idempotencyKey);

        session.SessionId.Should().NotBeEmpty();
        session.CheckoutUrl.Should().StartWith("https://");
        session.CustomerId.Should().NotBeNullOrEmpty();
    }

    /// <summary>
    /// Replaying with the same idempotency key remains wire-compatible. stripe-mock is stateless
    /// and does not emulate Stripe's idempotent response cache, so identity equality belongs in
    /// the pre-release Stripe test-mode smoke pass rather than this protocol-shape test.
    /// </summary>
    [SkippableFact]
    public async Task Replay_with_same_idempotency_key_returns_valid_sessions()
    {
        Skip.IfNot(_mock.IsAvailable);
        var gateway = BuildGateway();
        var idempotencyKey = Guid.NewGuid().ToString();
        var orgId = Guid.NewGuid();
        var userId = Guid.NewGuid();

        var s1 = await gateway.CreateCheckoutSessionAsync(
            userId, orgId, SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            idempotencyKey: idempotencyKey);

        var s2 = await gateway.CreateCheckoutSessionAsync(
            userId, orgId, SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            idempotencyKey: idempotencyKey);

        s1.SessionId.Should().NotBeNullOrWhiteSpace();
        s2.SessionId.Should().NotBeNullOrWhiteSpace();
        s1.CheckoutUrl.Should().StartWith("https://");
        s2.CheckoutUrl.Should().StartWith("https://");
    }

    /// <summary>Behaviour #3: First call for an org creates a Stripe customer.</summary>
    [SkippableFact]
    public async Task First_call_for_an_org_creates_a_stripe_customer()
    {
        Skip.IfNot(_mock.IsAvailable);
        var gateway = BuildGateway();

        var session = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            idempotencyKey: Guid.NewGuid().ToString());

        session.CustomerId.Should().StartWith("cus_");
    }

    /// <summary>Behaviour #3b: Existing customer id is passed through; no new customer is created.</summary>
    [SkippableFact]
    public async Task Existing_customer_id_is_reused_no_new_customer_created()
    {
        Skip.IfNot(_mock.IsAvailable);
        var gateway = BuildGateway();

        // Step A: create a real customer id via the first call.
        var first = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            idempotencyKey: Guid.NewGuid().ToString());

        var customerId = first.CustomerId;
        customerId.Should().NotBeNullOrEmpty();

        // Step B: second call reuses that customer id.
        var second = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            existingCustomerId: customerId,
            idempotencyKey: Guid.NewGuid().ToString());

        second.CustomerId.Should().Be(customerId);
    }
}
