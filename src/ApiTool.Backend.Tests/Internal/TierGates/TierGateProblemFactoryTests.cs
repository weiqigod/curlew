using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.Features;
using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Internal.TierGates;

/// <summary>
/// Unit tests for <see cref="TierGateProblemFactory"/> — verifies RFC 7807 problem detail shape
/// and response codes for both authenticated org-scoped denials and public-flow 404s.
/// </summary>
public class TierGateProblemFactoryTests
{
    /// <summary>
    /// Build a <see cref="DefaultHttpContext"/> with a minimal service provider so
    /// <c>Results.Json</c> can resolve <see cref="IOptions{JsonOptions}"/>.
    /// </summary>
    private static DefaultHttpContext BuildHttpContext(string traceId = "test-trace-id")
    {
        var services = new ServiceCollection();
        services.AddLogging();
        services.AddSingleton<IOptions<JsonOptions>>(Options.Create(new JsonOptions()));
        var sp = services.BuildServiceProvider();

        var ctx = new DefaultHttpContext { RequestServices = sp };
        ctx.Response.Body = new System.IO.MemoryStream();
        var traceIdFeature = new HttpRequestIdentifierFeature { TraceIdentifier = traceId };
        ctx.Features.Set<IHttpRequestIdentifierFeature>(traceIdFeature);
        return ctx;
    }

    private static async Task<(int statusCode, string body, IHeaderDictionary headers)> ExecuteResultAsync(
        IResult result, HttpContext ctx)
    {
        await result.ExecuteAsync(ctx);
        ctx.Response.Body.Seek(0, System.IO.SeekOrigin.Begin);
        var body = await new System.IO.StreamReader(ctx.Response.Body).ReadToEndAsync();
        return (ctx.Response.StatusCode, body, ctx.Response.Headers);
    }

    [Fact]
    public async Task AuthenticatedTierIneligible_emits_402_problem_detail_with_tier_extensions()
    {
        var ctx = BuildHttpContext("req-123");
        var result = TierGateProblemFactory.AuthenticatedTierIneligible(
            ctx, SubscriptionTier.Free, SubscriptionTier.Team, "vault_config");

        var (status, body, headers) = await ExecuteResultAsync(result, ctx);

        status.Should().Be(StatusCodes.Status402PaymentRequired);
        headers.ContentType.ToString().Should().Contain("application/problem+json");

        var doc = JsonDocument.Parse(body).RootElement;
        doc.GetProperty("type").GetString().Should().Contain("tier-ineligible");
        doc.GetProperty("title").GetString().Should().Be("Tier ineligible");
        doc.GetProperty("status").GetInt32().Should().Be(402);
        doc.GetProperty("code").GetString().Should().Be("vault_config_tier_ineligible");
        doc.GetProperty("current_tier").GetString().Should().Be("free");
        doc.GetProperty("required_tier").GetString().Should().Be("team");
        doc.GetProperty("request_id").GetString().Should().Be("req-123");
    }

    [Fact]
    public async Task AuthenticatedOrgNotFound_emits_404_problem_detail()
    {
        var ctx = BuildHttpContext("req-456");
        var result = TierGateProblemFactory.AuthenticatedOrgNotFound(ctx);

        var (status, body, _) = await ExecuteResultAsync(result, ctx);

        status.Should().Be(StatusCodes.Status404NotFound);
        var doc = JsonDocument.Parse(body).RootElement;
        doc.GetProperty("type").GetString().Should().Contain("organization-not-found");
        doc.GetProperty("title").GetString().Should().Be("Organization not found");
        doc.GetProperty("status").GetInt32().Should().Be(404);
        doc.GetProperty("code").GetString().Should().Be("organization_not_found");
        doc.GetProperty("request_id").GetString().Should().Be("req-456");
    }

    [Fact]
    public async Task PublicFlowNotFound_emits_404_with_cache_control_no_store_and_zero_body()
    {
        var ctx = BuildHttpContext();
        var result = TierGateProblemFactory.PublicFlowNotFound();

        var (status, body, headers) = await ExecuteResultAsync(result, ctx);

        status.Should().Be(StatusCodes.Status404NotFound);
        headers.CacheControl.ToString().Should().Be("no-store");
        body.Should().BeEmpty(because: "public-flow 404 must not leak tier metadata in the body");
    }

    [Theory]
    [InlineData(SubscriptionTier.Free, SubscriptionTier.Team, "free", "team")]
    [InlineData(SubscriptionTier.Professional, SubscriptionTier.Enterprise, "professional", "enterprise")]
    public async Task AuthenticatedTierIneligible_serialises_tier_names_lower_invariant(
        SubscriptionTier current, SubscriptionTier required,
        string expectedCurrent, string expectedRequired)
    {
        var ctx = BuildHttpContext();
        var result = TierGateProblemFactory.AuthenticatedTierIneligible(
            ctx, current, required, "test_feature");

        var (_, body, _) = await ExecuteResultAsync(result, ctx);

        var doc = JsonDocument.Parse(body).RootElement;
        doc.GetProperty("current_tier").GetString().Should().Be(expectedCurrent);
        doc.GetProperty("required_tier").GetString().Should().Be(expectedRequired);
    }

    [Fact]
    public async Task AuthenticatedTierIneligible_sets_X_Request_Id_response_header()
    {
        var ctx = BuildHttpContext("req-789");
        var result = TierGateProblemFactory.AuthenticatedTierIneligible(
            ctx, SubscriptionTier.Free, SubscriptionTier.Enterprise, "sso");

        await result.ExecuteAsync(ctx);

        ctx.Response.Headers["X-Request-Id"].ToString().Should().Be("req-789");
    }

    [Fact]
    public void AuthenticatedTierIneligible_throws_when_featureCode_is_null()
    {
        var ctx = BuildHttpContext();

        var act = () => TierGateProblemFactory.AuthenticatedTierIneligible(
            ctx, SubscriptionTier.Free, SubscriptionTier.Team, null!);

        act.Should().Throw<ArgumentException>(
            because: "a null featureCode would silently produce 'null_tier_ineligible' in the error code");
    }

    [Fact]
    public void AuthenticatedTierIneligible_throws_when_featureCode_is_empty()
    {
        var ctx = BuildHttpContext();

        var act = () => TierGateProblemFactory.AuthenticatedTierIneligible(
            ctx, SubscriptionTier.Free, SubscriptionTier.Team, string.Empty);

        act.Should().Throw<ArgumentException>(
            because: "an empty featureCode would silently produce '_tier_ineligible' in the error code");
    }
}
