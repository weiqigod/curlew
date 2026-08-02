// Unit tests for GitLabProjectLookup — URL encoding, header sending, status-code mapping.
// RED phase: these tests will fail until IGitLabProjectLookup and GitLabProjectLookup are created.
// Refs: M16-016
using System.Net;
using System.Net.Http;
using System.Text;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.GitLab.Installations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.GitLab;

/// <summary>Unit tests for <see cref="GitLabProjectLookup"/>.</summary>
public sealed class GitLabProjectLookupTests
{
    /// <summary>Builds a <see cref="GitLabProjectLookup"/> with a fake HTTP handler.</summary>
    private static (GitLabProjectLookup lookup, FakeHttpMessageHandler handler) Build(
        HttpStatusCode statusCode = HttpStatusCode.OK,
        string responseBody = """{"id":42,"path_with_namespace":"group/project"}""",
        bool allowHttp = false)
    {
        var handler = new FakeHttpMessageHandler(statusCode, responseBody);
        var options = Options.Create(new GitLabOptions
        {
            Poster = new GitLabOptions.PosterConfig { AllowHttp = allowHttp }
        });
        var factory = new FakeHttpClientFactory(handler, null);
        var lookup = new GitLabProjectLookup(factory, options);
        return (lookup, handler);
    }

    [Theory]
    [InlineData("https://gitlab.com", "group/project", "https://gitlab.com/api/v4/projects/group%2Fproject")]
    [InlineData("https://gitlab.com/", "group/project", "https://gitlab.com/api/v4/projects/group%2Fproject")]
    [InlineData("https://gl.example.com", "nested/group/project", "https://gl.example.com/api/v4/projects/nested%2Fgroup%2Fproject")]
    [InlineData("https://gitlab.com", "group with spaces/project", "https://gitlab.com/api/v4/projects/group%20with%20spaces%2Fproject")]
    public async Task Builds_encoded_url(string baseUrl, string path, string expectedUrl)
    {
        var handler = new CapturingHandler(HttpStatusCode.OK,
            """{"id":99,"path_with_namespace":"group/project"}""");
        var options = Options.Create(new GitLabOptions());
        var factory = new CapturingClientFactory(handler);
        var lookup = new GitLabProjectLookup(factory, options);

        await lookup.LookupAsync(baseUrl, path, "pat", null, CancellationToken.None);

        Assert.Equal(expectedUrl, handler.LastRequestUri?.AbsoluteUri);
    }

    [Fact]
    public async Task Sends_Private_Token_header()
    {
        var (lookup, handler) = Build();

        await lookup.LookupAsync("https://gitlab.com", "group/project", "mytoken", null, CancellationToken.None);

        Assert.Equal("mytoken", handler.LastRequestHeaders.GetValueOrDefault("Private-Token"));
    }

    [Fact]
    public async Task Maps_200_with_numeric_id_to_Ok()
    {
        var (lookup, _) = Build(HttpStatusCode.OK, """{"id":42,"path_with_namespace":"group/project"}""");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Ok, result.Status);
        Assert.Equal(42L, result.ProjectId);
        Assert.Equal("group/project", result.ResolvedPath);
    }

    [Fact]
    public async Task Maps_200_with_missing_id_to_InvalidResponse()
    {
        var (lookup, _) = Build(HttpStatusCode.OK, """{"name":"project","missing_id_field":true}""");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.InvalidResponse, result.Status);
    }

    [Fact]
    public async Task Maps_401_to_Unauthorized()
    {
        var (lookup, _) = Build(HttpStatusCode.Unauthorized, """{"message":"401 Unauthorized"}""");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "bad-pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Unauthorized, result.Status);
    }

    [Fact]
    public async Task Maps_404_to_NotFound()
    {
        var (lookup, _) = Build(HttpStatusCode.NotFound, """{"message":"404 Project Not Found"}""");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.NotFound, result.Status);
    }

    [Fact]
    public async Task Maps_network_error_to_Unreachable()
    {
        var handler = new ThrowingHandler(new HttpRequestException("connection refused"));
        var options = Options.Create(new GitLabOptions());
        var factory = new CapturingClientFactory(handler);
        var lookup = new GitLabProjectLookup(factory, options);

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Unreachable, result.Status);
    }

    [Fact]
    public async Task Maps_200_with_string_id_to_InvalidResponse()
    {
        // GitLab returns 200 but with "id" as a string rather than a number.
        var (lookup, _) = Build(HttpStatusCode.OK, """{"id":"non-numeric","path_with_namespace":"group/project"}""");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.InvalidResponse, result.Status);
    }

    [Fact]
    public async Task Maps_200_with_malformed_json_to_InvalidResponse()
    {
        // GitLab returns 200 but the body is not valid JSON.
        var (lookup, _) = Build(HttpStatusCode.OK, "not-valid-json{{{{");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.InvalidResponse, result.Status);
    }

    [Fact]
    public async Task Http_base_url_with_AllowHttp_false_returns_InsecureBaseUrl()
    {
        var (lookup, _) = Build(allowHttp: false);

        var result = await lookup.LookupAsync("http://gitlab.internal", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.InsecureBaseUrl, result.Status);
    }

    [Fact]
    public async Task Http_base_url_with_AllowHttp_true_proceeds()
    {
        var (lookup, _) = Build(
            statusCode: HttpStatusCode.OK,
            responseBody: """{"id":99,"path_with_namespace":"group/project"}""",
            allowHttp: true);

        var result = await lookup.LookupAsync("http://gitlab.internal", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Ok, result.Status);
    }

    [Theory]
    [InlineData("not-a-url")]
    [InlineData("://broken")]
    [InlineData("just plain text with spaces")]
    public async Task Invalid_base_url_returns_InvalidBaseUrl_instead_of_throwing(string badBaseUrl)
    {
        // Finding #1: a syntactically invalid gitlab_base_url must never propagate as an unhandled
        // UriFormatException. The production code constructs a Uri with UriKind.Absolute, which
        // throws when the base URL is not a valid absolute URI. The fix must catch that exception
        // (or pre-validate) and return a typed status rather than blowing up as a 500.
        var (lookup, _) = Build();

        var result = await lookup.LookupAsync(badBaseUrl, "group/project", "pat", null, CancellationToken.None);

        // The UriFormatException must be caught; a typed InvalidBaseUrl result must be returned.
        Assert.Equal(GitLabProjectLookupStatus.InvalidBaseUrl, result.Status);
        Assert.Null(result.ProjectId);
    }

    [Fact]
    public async Task Maps_403_to_Unauthorized()
    {
        // GitLab returns 403 Forbidden (e.g. PAT lacks the required scope); treated as Unauthorized
        // so the caller sees the same user-facing error as a 401 (PAT invalid or insufficient scope).
        var (lookup, _) = Build(HttpStatusCode.Forbidden, """{"message":"403 Forbidden"}""");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "scoped-pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Unauthorized, result.Status);
    }

    [Fact]
    public async Task Maps_5xx_to_Unreachable()
    {
        // GitLab returns a 5xx HTTP response (server-side error); treated as Unreachable so the
        // caller surfaces "Could not reach GitLab" rather than exposing internal server errors.
        var (lookup, _) = Build(HttpStatusCode.ServiceUnavailable, """{"message":"503 Service Unavailable"}""");

        var result = await lookup.LookupAsync("https://gitlab.com", "group/project", "pat", null, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Unreachable, result.Status);
    }

    // ── CA-bundle code path ───────────────────────────────────────────────────

    [Fact]
    public async Task Ca_bundle_path_returns_Ok_with_numeric_id()
    {
        // Use the handlerFactory seam so we don't need a real X509 certificate.
        const string fakePem = "FAKE-PEM";
        var capturingHandler = new CapturingHandler(
            HttpStatusCode.OK,
            """{"id":77,"path_with_namespace":"group/ca-project"}""");

        var options = Options.Create(new GitLabOptions());
        var lookup = new GitLabProjectLookup(
            new CapturingClientFactory(capturingHandler),
            options,
            caBundleHandlerFactory: _ => capturingHandler);

        var result = await lookup.LookupAsync(
            "https://gitlab.example.com", "group/ca-project", "pat", fakePem, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Ok, result.Status);
        Assert.Equal(77L, result.ProjectId);
        Assert.Equal("group/ca-project", result.ResolvedPath);
    }

    [Fact]
    public async Task Ca_bundle_path_maps_network_error_to_Unreachable()
    {
        // Use the handlerFactory seam with a throwing handler.
        const string fakePem = "FAKE-PEM";
        var throwingHandler = new ThrowingHandler(new HttpRequestException("connection refused"));

        var options = Options.Create(new GitLabOptions());
        var lookup = new GitLabProjectLookup(
            new CapturingClientFactory(throwingHandler),
            options,
            caBundleHandlerFactory: _ => throwingHandler);

        var result = await lookup.LookupAsync(
            "https://gitlab.example.com", "group/ca-project", "pat", fakePem, CancellationToken.None);

        Assert.Equal(GitLabProjectLookupStatus.Unreachable, result.Status);
    }
}

// ── Test helpers ─────────────────────────────────────────────────────────────

/// <summary>An <see cref="HttpMessageHandler"/> that captures the request URI.</summary>
internal sealed class CapturingHandler(HttpStatusCode status, string body) : HttpMessageHandler
{
    public Uri? LastRequestUri { get; private set; }

    protected override Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken cancellationToken)
    {
        LastRequestUri = request.RequestUri;
        return Task.FromResult(new HttpResponseMessage(status)
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json")
        });
    }
}

/// <summary>An <see cref="IHttpClientFactory"/> that wraps any handler.</summary>
internal sealed class CapturingClientFactory(HttpMessageHandler handler) : IHttpClientFactory
{
    public HttpClient CreateClient(string name) =>
        new HttpClient(handler, disposeHandler: false);
}

/// <summary>An <see cref="HttpMessageHandler"/> that throws on every request.</summary>
internal sealed class ThrowingHandler(Exception ex) : HttpMessageHandler
{
    protected override Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken cancellationToken) =>
        Task.FromException<HttpResponseMessage>(ex);
}
