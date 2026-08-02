using System.Net;
using System.Text;
using ApiTool.Backend.Subscriptions;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using Stripe;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>
/// Unit tests for <see cref="StripeGateway.ComputeProrationAsync"/> exception-mapping behaviour.
/// Uses stub <see cref="IHttpClient"/> implementations — no network call or stripe-mock required.
/// Mirrors the <see cref="StripeGatewayPortalUnitTests"/> pattern.
/// </summary>
public sealed class StripeGatewayProrationUnitTests
{
    /// <summary>Behaviour #1: Stripe 429 on invoice.upcoming maps to StripeRateLimitedException preserving idempotency key.</summary>
    [Fact]
    public async Task ComputeProrationAsync_maps_429_to_StripeRateLimitedException()
    {
        var stub = new RateLimit429Stub();
        var stripeClient = new StripeClient("sk_test_unit", httpClient: stub);
        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "sk_test_unit" });
        var gateway = new StripeGateway(opts, NullLogger<StripeGateway>.Instance, stripeClient);

        var key = Guid.NewGuid().ToString();
        var act = () => gateway.ComputeProrationAsync(
            "sub_test_x", "price_test_team_monthly", 5,
            DateTimeOffset.UtcNow, idempotencyKey: key);

        var ex = await act.Should().ThrowAsync<StripeRateLimitedException>();
        ex.Which.IdempotencyKey.Should().Be(key);
    }

    /// <summary>Behaviour #2: invoice_upcoming_none from Stripe → zero-valued result with null renewal date.</summary>
    [Fact]
    public async Task ComputeProrationAsync_maps_invoice_upcoming_none_to_zero_result_with_null_renewal()
    {
        var stub = new InvoiceUpcomingNoneStub();
        var stripeClient = new StripeClient("sk_test_unit", httpClient: stub);
        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "sk_test_unit" });
        var gateway = new StripeGateway(opts, NullLogger<StripeGateway>.Instance, stripeClient);

        var (result, renewal) = await gateway.ComputeProrationAsync(
            "sub_test_x", "price_test_team_monthly", 5, DateTimeOffset.UtcNow);

        result.Net.Should().Be(0);
        result.Credit.Should().Be(0);
        result.Charge.Should().Be(0);
        renewal.Should().BeNull();
    }

    /// <summary>Behaviour #3: Stripe 5xx on invoice.upcoming maps to StripeUnavailableException.</summary>
    [Fact]
    public async Task ComputeProrationAsync_maps_500_to_StripeUnavailableException()
    {
        var stub = new ServerError500Stub();
        var stripeClient = new StripeClient("sk_test_unit", httpClient: stub);
        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "sk_test_unit" });
        var gateway = new StripeGateway(opts, NullLogger<StripeGateway>.Instance, stripeClient);

        var act = () => gateway.ComputeProrationAsync(
            "sub_test_x", "price_test_team_monthly", 5,
            DateTimeOffset.UtcNow, idempotencyKey: Guid.NewGuid().ToString());

        await act.Should().ThrowAsync<StripeUnavailableException>();
    }

    // ── HTTP stubs ────────────────────────────────────────────────────────────

    /// <summary>Always returns HTTP 429 with a Stripe-format rate-limit error body.</summary>
    private sealed class RateLimit429Stub : IHttpClient
    {
        public Task<StripeResponse> MakeRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            const string body = """{"error":{"type":"invalid_request_error","code":"rate_limit","message":"Too many requests."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.TooManyRequests).Headers;
            return Task.FromResult(new StripeResponse(HttpStatusCode.TooManyRequests, headers, body));
        }

        public Task<StripeStreamedResponse> MakeStreamingRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            const string body = """{"error":{"type":"invalid_request_error","code":"rate_limit","message":"Too many requests."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.TooManyRequests).Headers;
            var stream = new MemoryStream(Encoding.UTF8.GetBytes(body));
            return Task.FromResult(new StripeStreamedResponse(HttpStatusCode.TooManyRequests, headers, stream));
        }
    }

    /// <summary>Returns HTTP 404 with error code <c>invoice_upcoming_none</c>.</summary>
    private sealed class InvoiceUpcomingNoneStub : IHttpClient
    {
        public Task<StripeResponse> MakeRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            const string body = """{"error":{"type":"invalid_request_error","code":"invoice_upcoming_none","message":"No upcoming invoice."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.NotFound).Headers;
            return Task.FromResult(new StripeResponse(HttpStatusCode.NotFound, headers, body));
        }

        public Task<StripeStreamedResponse> MakeStreamingRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            const string body = """{"error":{"type":"invalid_request_error","code":"invoice_upcoming_none","message":"No upcoming invoice."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.NotFound).Headers;
            var stream = new MemoryStream(Encoding.UTF8.GetBytes(body));
            return Task.FromResult(new StripeStreamedResponse(HttpStatusCode.NotFound, headers, stream));
        }
    }

    /// <summary>Always returns HTTP 500 with a Stripe-format server error body.</summary>
    private sealed class ServerError500Stub : IHttpClient
    {
        public Task<StripeResponse> MakeRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            const string body = """{"error":{"type":"api_error","message":"Internal server error."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.InternalServerError).Headers;
            return Task.FromResult(new StripeResponse(HttpStatusCode.InternalServerError, headers, body));
        }

        public Task<StripeStreamedResponse> MakeStreamingRequestAsync(
            StripeRequest request, CancellationToken cancellationToken = default)
        {
            const string body = """{"error":{"type":"api_error","message":"Internal server error."}}""";
            var headers = new HttpResponseMessage(HttpStatusCode.InternalServerError).Headers;
            var stream = new MemoryStream(Encoding.UTF8.GetBytes(body));
            return Task.FromResult(new StripeStreamedResponse(HttpStatusCode.InternalServerError, headers, stream));
        }
    }
}
