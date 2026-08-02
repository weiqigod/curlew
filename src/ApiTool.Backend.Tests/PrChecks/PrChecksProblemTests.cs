// Tests for PrChecksProblem GitLab-specific error factories.
// Refs M16-014 plan step 2.
using System.Text.Json;
using ApiTool.Backend.PrChecks;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.Features;
using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// Tests for the GitLab-specific factory methods on <see cref="PrChecksProblem"/>.
/// Verifies status codes, <c>code</c> extensions, and RFC-7807 <c>type</c> fields.
/// Named so the filter FullyQualifiedName~PrChecksProblem matches.
/// </summary>
public sealed class PrChecksProblemTests
{
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

    private static async Task<(int statusCode, JsonDocument doc)> ExecuteAsync(IResult result, HttpContext ctx)
    {
        await result.ExecuteAsync(ctx);
        ctx.Response.Body.Seek(0, System.IO.SeekOrigin.Begin);
        var body = await new System.IO.StreamReader(ctx.Response.Body).ReadToEndAsync();
        return (ctx.Response.StatusCode, JsonDocument.Parse(body));
    }

    [Fact]
    public async Task GitLabNoInstallation_Returns404_WithCode()
    {
        var ctx = BuildHttpContext();
        var result = PrChecksProblem.GitLabNoInstallation(ctx);
        var (status, doc) = await ExecuteAsync(result, ctx);

        status.Should().Be(404);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_GITLAB_NO_INSTALLATION");
        doc.RootElement.GetProperty("type").GetString().Should().Contain("prcheck-gitlab-no-installation");
    }

    [Fact]
    public async Task GitLabTokenRevoked_Returns423_WithCode()
    {
        var ctx = BuildHttpContext();
        var result = PrChecksProblem.GitLabTokenRevoked(ctx);
        var (status, doc) = await ExecuteAsync(result, ctx);

        status.Should().Be(423);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_GITLAB_TOKEN_REVOKED");
        doc.RootElement.GetProperty("type").GetString().Should().Contain("prcheck-gitlab-token-revoked");
    }

    [Fact]
    public async Task GitLabUnreachable_Returns502_WithCode()
    {
        var ctx = BuildHttpContext();
        var result = PrChecksProblem.GitLabUnreachable(ctx);
        var (status, doc) = await ExecuteAsync(result, ctx);

        status.Should().Be(502);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_GITLAB_UNREACHABLE");
        doc.RootElement.GetProperty("type").GetString().Should().Contain("prcheck-gitlab-unreachable");
    }

    [Fact]
    public async Task GitLabHttpInsecure_Returns400_DetailContainsBaseUrl()
    {
        var ctx = BuildHttpContext();
        var baseUrl = "http://my-gitlab.internal";
        var result = PrChecksProblem.GitLabHttpInsecure(ctx, baseUrl);
        var (status, doc) = await ExecuteAsync(result, ctx);

        status.Should().Be(400);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_GITLAB_HTTP_INSECURE");
        doc.RootElement.GetProperty("detail").GetString().Should().Contain(baseUrl);
        doc.RootElement.GetProperty("type").GetString().Should().Contain("prcheck-gitlab-http-insecure");
    }

    [Fact]
    public async Task GitLabRateLimited_Returns429_WithCode()
    {
        var ctx = BuildHttpContext();
        var result = PrChecksProblem.GitLabRateLimited(ctx);
        var (status, doc) = await ExecuteAsync(result, ctx);

        status.Should().Be(429);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_GITLAB_RATE_LIMITED");
        doc.RootElement.GetProperty("type").GetString().Should().Contain("prcheck-gitlab-rate-limited");
    }
}
