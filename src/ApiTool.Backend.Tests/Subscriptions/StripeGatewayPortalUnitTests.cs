using System.Net;
using System.Text;
using ApiTool.Backend.Subscriptions;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using Stripe;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>
/// Unit tests for <see cref="StripeGateway.CreatePortalSessionAsync"/> error-mapping behaviour.
/// Uses stub <see cref="IHttpClient"/> implementations so no network call or stripe-mock is required.
/// </summary>
public sealed class StripeGatewayPortalUnitTests
{
    /// <summary>Behaviour: portal call requires a customer id; missing → InvalidOperationException.</summary>
    [Fact]
    public async Task CreatePortalSessionAsync_throws_when_customer_id_missing()
    {
        var stub = new NeverCalledHttpClientStub();
        var stripeClient = new StripeClient("sk_test_unit", httpClient: stub);
        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "sk_test_unit" });
        var gateway = new StripeGateway(opts, NullLogger<StripeGateway>.Instance, stripeClient);

        var act = () => gateway.CreatePortalSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), "https://app/billing",
            customerId: null, idempotencyKey: Guid.NewGuid().ToString());

        await act.Should().ThrowAsync<InvalidOperationException>()
            .WithMessage("*customerId*");
    }

    /// <summary>Behaviour #4: Stripe 5xx on portal create maps to StripeUnavailableException with request_id.</summary>
    [Fact]
    public async Task Stripe_503_on_portal_create_is_mapped_to_StripeUnavailableException_with_request_id()
    {
        var stub = new FixedStatusHttpClientStub(HttpStatusCode.ServiceUnavailable, requestId: "req_test_5xx_abc");
        var stripeClient = new StripeClient("sk_test_unit", httpClient: stub);
        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "sk_test_unit" });
        var gateway = new StripeGateway(opts, NullLogger<StripeGateway>.Instance, stripeClient);

        var idempotencyKey = Guid.NewGuid().ToString();
        var act = () => gateway.CreatePortalSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), "https://app/billing",
            customerId: "cus_test_123", idempotencyKey: idempotencyKey);

        var ex = await act.Should().ThrowAsync<StripeUnavailableException>();
        ex.Which.IdempotencyKey.Should().Be(idempotencyKey);
        ex.Which.RequestId.Should().Be("req_test_5xx_abc");
    }

    /// <summary>Behaviour: 429 on portal create maps to StripeRateLimitedException (consistency with checkout).</summary>
    [Fact]
    public async Task Stripe_429_on_portal_create_is_mapped_to_StripeRateLimitedException()
    {
        var stub = new FixedStatusHttpClientStub(HttpStatusCode.TooManyRequests, requestId: null);
        var stripeClient = new StripeClient("sk_test_unit", httpClient: stub);
        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "sk_test_unit" });
        var gateway = new StripeGateway(opts, NullLogger<StripeGateway>.Instance, stripeClient);

        var idempotencyKey = Guid.NewGuid().ToString();
        var act = () => gateway.CreatePortalSessionAsync(
            Guid.NewGuid(), Guid.NewGuid(), "https://app/billing",
            customerId: "cus_test_123", idempotencyKey: idempotencyKey);

        var ex = await act.Should().ThrowAsync<StripeRateLimitedException>();
        ex.Which.IdempotencyKey.Should().Be(idempotencyKey);
    }

    /// <summary>Returns a fixed HTTP status for all requests, with an optional Stripe-Request-Id header.</summary>
    private sealed class FixedStatusHttpClientStub : IHttpClient
    {
        private readonly HttpStatusCode _status;
        private readonly string? _requestId;

        public FixedStatusHttpClientStub(HttpStatusCode status, string? requestId)
        {
            _status = status;
            _requestId = requestId;
        }

        public Task<StripeResponse> MakeRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            var body = """{"error":{"type":"api_error","message":"Service unavailable."}}""";
            var msg = new HttpResponseMessage(_status);
            // Stripe SDK parses RequestId from the "Request-Id" header (see StripeResponseBase).
            if (_requestId is not null)
                msg.Headers.TryAddWithoutValidation("Request-Id", _requestId);
            return Task.FromResult(new StripeResponse(_status, msg.Headers, body));
        }

        public Task<StripeStreamedResponse> MakeStreamingRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            var body = """{"error":{"type":"api_error","message":"Service unavailable."}}""";
            var msg = new HttpResponseMessage(_status);
            if (_requestId is not null)
                msg.Headers.TryAddWithoutValidation("Request-Id", _requestId);
            var stream = new MemoryStream(Encoding.UTF8.GetBytes(body));
            return Task.FromResult(new StripeStreamedResponse(_status, msg.Headers, stream));
        }
    }

    /// <summary>Throws if called — used to assert the gateway never reaches Stripe when customerId is missing.</summary>
    private sealed class NeverCalledHttpClientStub : IHttpClient
    {
        public Task<StripeResponse> MakeRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
            => throw new InvalidOperationException("Stripe HTTP client should not have been called.");

        public Task<StripeStreamedResponse> MakeStreamingRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
            => throw new InvalidOperationException("Stripe HTTP client should not have been called.");
    }
}
