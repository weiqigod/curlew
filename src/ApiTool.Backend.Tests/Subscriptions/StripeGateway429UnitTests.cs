using System.Net;
using System.Net.Http.Headers;
using System.Text;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Subscriptions;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using Stripe;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>
/// Unit tests for the Stripe 429 rate-limit → <see cref="StripeRateLimitedException"/> mapping.
/// Uses a stub <see cref="IHttpClient"/> so no real network call or stripe-mock is required.
/// </summary>
public sealed class StripeGateway429UnitTests
{
    /// <summary>Behaviour #5: StripeException with 429 is mapped to StripeRateLimitedException preserving idempotency key.</summary>
    [Fact]
    public async Task Stripe_429_on_customer_create_is_mapped_to_StripeRateLimitedException_with_idempotency_key()
    {
        var stub = new RateLimitingHttpClientStub();
        var stripeClient = new StripeClient(
            "sk_test_unit",
            httpClient: stub);

        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "sk_test_unit" });
        var gateway = new StripeGateway(opts, NullLogger<StripeGateway>.Instance, stripeClient);

        var idempotencyKey = Guid.NewGuid().ToString();
        var act = () => gateway.CreateCheckoutSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), SubscriptionTier.Team, "month", 5,
            "https://app/ok", "https://app/no",
            priceId: "price_test_team_monthly",
            idempotencyKey: idempotencyKey);

        var ex = await act.Should().ThrowAsync<StripeRateLimitedException>();
        ex.Which.IdempotencyKey.Should().Be(idempotencyKey);
    }

    // Stub that always returns 429 with a Stripe-format error body.
    private sealed class RateLimitingHttpClientStub : IHttpClient
    {
        public Task<StripeResponse> MakeRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            var body = """{"error":{"type":"invalid_request_error","code":"rate_limit","message":"Too many requests."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.TooManyRequests).Headers;
            return Task.FromResult(
                new StripeResponse(HttpStatusCode.TooManyRequests, headers, body));
        }

        public Task<StripeStreamedResponse> MakeStreamingRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            var body = """{"error":{"type":"invalid_request_error","code":"rate_limit","message":"Too many requests."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.TooManyRequests).Headers;
            var stream = new MemoryStream(Encoding.UTF8.GetBytes(body));
            return Task.FromResult(
                new StripeStreamedResponse(HttpStatusCode.TooManyRequests, headers, stream));
        }
    }
}
