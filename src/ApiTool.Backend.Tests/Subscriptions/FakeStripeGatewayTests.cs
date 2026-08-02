using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Subscriptions;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Verifies FakeStripeGateway returns deterministic results suitable for tests.</summary>
public sealed class FakeStripeGatewayTests
{
    private static FakeStripeGateway CreateGateway() => new();

    [Fact]
    public async Task CreateCheckoutSession_returns_deterministic_url_shape()
    {
        var gateway = CreateGateway();
        var session = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "http://localhost/ok", "http://localhost/no");

        session.CheckoutUrl.Should().StartWith("https://checkout.stripe.test/cs_");
        session.SessionId.Should().StartWith("cs_");
    }

    [Fact]
    public async Task CreateCheckoutSession_returns_unique_session_ids_per_call()
    {
        var gateway = CreateGateway();
        var s1 = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "http://localhost/ok", "http://localhost/no");
        var s2 = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "http://localhost/ok", "http://localhost/no");

        s1.SessionId.Should().NotBe(s2.SessionId);
    }

    [Fact]
    public void ComputeProration_upgrade_5_to_8_team_seats_includes_positive_net()
    {
        var gateway = CreateGateway();
        var result = gateway.ComputeProration(
            SubscriptionTier.Team, 5, SubscriptionTier.Team, 8, "month");

        result.Net.Should().BeGreaterThan(0);
        result.Charge.Should().BeGreaterThan(0);
    }

    [Fact]
    public void ComputeProration_seat_decrease_returns_nonpositive_net()
    {
        var gateway = CreateGateway();
        var result = gateway.ComputeProration(
            SubscriptionTier.Team, 8, SubscriptionTier.Team, 5, "month");

        result.Net.Should().BeLessThanOrEqualTo(0);
    }

    [Fact]
    public async Task CreatePortalSession_returns_portal_url()
    {
        var gateway = CreateGateway();
        var session = await gateway.CreatePortalSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), "http://localhost/return");

        session.PortalUrl.Should().StartWith("https://billing.stripe.test/");
    }

    [Fact]
    public async Task CreateCheckoutSession_with_price_id_returns_cs_test_prefixed_session_id()
    {
        var gateway = CreateGateway();
        var session = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "http://localhost/ok", "http://localhost/no",
            priceId: "price_test_team_monthly");

        session.SessionId.Should().StartWith("cs_test_");
        session.CheckoutUrl.Should().Contain("cs_test_");
    }

    [Fact]
    public async Task CreateCheckoutSession_returns_null_customer_id_when_existing_customer_id_is_null()
    {
        var gateway = CreateGateway();
        var session = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "http://localhost/ok", "http://localhost/no");
        session.CustomerId.Should().BeNull();
    }

    [Fact]
    public async Task CreateCheckoutSession_passes_through_existing_customer_id_in_response()
    {
        var gateway = CreateGateway();
        var session = await gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "http://localhost/ok", "http://localhost/no",
            existingCustomerId: "cus_existing_123");
        session.CustomerId.Should().Be("cus_existing_123");
    }

    [Fact]
    public async Task ComputeProrationAsync_returns_positive_net_for_paid_tier_upgrade()
    {
        var gateway = CreateGateway();
        var (result, renewal) = await gateway.ComputeProrationAsync(
            subscriptionId: "sub_fake_active",
            newPriceId: "price_test_team_monthly",
            newQuantity: 8,
            prorationDate: DateTimeOffset.UtcNow);

        // From the Free/0 baseline: Team * 8 seats = 4900 * 8 = 39200 cents. Always positive.
        result.Net.Should().BeGreaterThan(0);
        result.Charge.Should().BeGreaterThan(0);
        result.Credit.Should().Be(0);
        renewal.Should().NotBeNull();
    }

    [Fact]
    public async Task ComputeProrationAsync_emits_deterministic_renewal_date()
    {
        var gateway = CreateGateway();
        var pinned = new DateTimeOffset(2026, 6, 1, 0, 0, 0, TimeSpan.Zero);
        var (_, renewal) = await gateway.ComputeProrationAsync(
            "sub_fake_active", "price_test_team_monthly", 5, pinned);
        renewal!.Value.Date.Should().Be(pinned.UtcDateTime.AddDays(30).Date);
    }

    // ── GetSubscriptionAsync ─────────────────────────────────────────────────

    [Fact]
    public async Task GetSubscriptionAsync_returns_scripted_subscription()
    {
        var fake = CreateGateway();
        fake.SetSubscriptionForTest(new Stripe.Subscription { Id = "sub_test_1", Status = "active" });
        var got = await fake.GetSubscriptionAsync("sub_test_1");
        got.Should().NotBeNull();
        got!.Id.Should().Be("sub_test_1");
    }

    [Fact]
    public async Task GetSubscriptionAsync_returns_null_when_missing()
    {
        var fake = CreateGateway();
        var got = await fake.GetSubscriptionAsync("sub_missing");
        got.Should().BeNull();
    }

    // ── GetCustomerAsync ─────────────────────────────────────────────────────

    [Fact]
    public async Task GetCustomerAsync_returns_scripted_customer()
    {
        var fake = CreateGateway();
        fake.SetCustomerForTest(new Stripe.Customer { Id = "cus_test_1", Email = "owner@example.com" });
        var got = await fake.GetCustomerAsync("cus_test_1");
        got.Should().NotBeNull();
        got!.Id.Should().Be("cus_test_1");
        got.Email.Should().Be("owner@example.com");
    }

    [Fact]
    public async Task GetCustomerAsync_returns_null_when_missing()
    {
        var fake = CreateGateway();
        var got = await fake.GetCustomerAsync("cus_missing");
        got.Should().BeNull();
    }

    // ── GetInvoiceAsync ──────────────────────────────────────────────────────

    [Fact]
    public async Task GetInvoiceAsync_returns_scripted_invoice()
    {
        var fake = CreateGateway();
        fake.SetInvoiceForTest(new Stripe.Invoice { Id = "inv_test_1", Status = "paid" });
        var got = await fake.GetInvoiceAsync("inv_test_1");
        got.Should().NotBeNull();
        got!.Id.Should().Be("inv_test_1");
        got.Status.Should().Be("paid");
    }

    [Fact]
    public async Task GetInvoiceAsync_returns_null_when_missing()
    {
        var fake = CreateGateway();
        var got = await fake.GetInvoiceAsync("inv_missing");
        got.Should().BeNull();
    }

    // ── GetPaymentMethodAsync ────────────────────────────────────────────────

    [Fact]
    public async Task GetPaymentMethodAsync_returns_scripted_payment_method()
    {
        var fake = CreateGateway();
        fake.SetPaymentMethodForTest(new Stripe.PaymentMethod
        {
            Id = "pm_test_1",
            Card = new Stripe.PaymentMethodCard { Brand = "visa", Last4 = "4242" },
        });
        var got = await fake.GetPaymentMethodAsync("pm_test_1");
        got.Should().NotBeNull();
        got!.Id.Should().Be("pm_test_1");
        got.Card!.Brand.Should().Be("visa");
    }

    [Fact]
    public async Task GetPaymentMethodAsync_returns_null_when_missing()
    {
        var fake = CreateGateway();
        var got = await fake.GetPaymentMethodAsync("pm_missing");
        got.Should().BeNull();
    }
}
