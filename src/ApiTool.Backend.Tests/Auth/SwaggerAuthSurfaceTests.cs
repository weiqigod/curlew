using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>Verifies that Swagger correctly exposes the auth login, refresh, and JWKS endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class SwaggerAuthSurfaceTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public SwaggerAuthSurfaceTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Swagger_lists_auth_login_endpoint()
    {
        var client = _factory.CreateClient();

        var json = await client.GetStringAsync("/swagger/v1/swagger.json");
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        // Behavior 8: POST /api/v1/auth/login must be listed in Swagger
        paths.TryGetProperty("/api/v1/auth/login", out var loginPath)
            .Should().BeTrue(because: "POST /api/v1/auth/login must be documented in Swagger");

        loginPath.TryGetProperty("post", out _)
            .Should().BeTrue(because: "the auth login endpoint must use the POST verb");
    }

    [Fact]
    public async Task Swagger_lists_auth_refresh_endpoint_and_NOT_license_issue()
    {
        // Behavior #8 from M14-002: POST /api/v1/auth/refresh IS listed;
        // POST /api/v1/license/issue must NOT be registered.
        var client = _factory.CreateClient();
        var json = await client.GetStringAsync("/swagger/v1/swagger.json");
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        paths.TryGetProperty("/api/v1/auth/refresh", out var refreshPath)
            .Should().BeTrue(because: "POST /api/v1/auth/refresh must be documented in Swagger");
        refreshPath.TryGetProperty("post", out _)
            .Should().BeTrue(because: "auth/refresh must use the POST verb");

        paths.TryGetProperty("/api/v1/license/issue", out _)
            .Should().BeFalse(because: "there must be no /api/v1/license/issue route per M14-002 DoD");
    }

    [Fact]
    public async Task Swagger_lists_jwks_endpoint_and_NOT_v4_1_public_key()
    {
        // M14-003 Behavior #6 — GET /api/v1/.well-known/jwks.json IS listed;
        // /api/v1/public-key must NOT be registered.
        // DoD: endpoint is unauthenticated (no Bearer security block) and JSON-typed
        // (response lists application/jwk-set+json as content type).
        var client = _factory.CreateClient();
        var json = await client.GetStringAsync("/swagger/v1/swagger.json");
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        paths.TryGetProperty("/api/v1/.well-known/jwks.json", out var jwksPath)
            .Should().BeTrue(because: "GET /api/v1/.well-known/jwks.json must be documented in Swagger");
        jwksPath.TryGetProperty("get", out var jwksGet)
            .Should().BeTrue(because: "the JWKS endpoint must use the GET verb");

        paths.TryGetProperty("/api/v1/public-key", out _)
            .Should().BeFalse(because: "v4.1's bespoke /api/v1/public-key must not be registered as an alias");

        // DoD: unauthenticated — Swashbuckle strips the global Bearer security requirement
        // for AllowAnonymous endpoints; the JWKS get operation must have no security block.
        jwksGet.TryGetProperty("security", out _)
            .Should().BeFalse(because: "AllowAnonymous endpoints must not carry a Bearer security requirement in Swagger");

        // DoD: JSON-typed — the 200 response must list application/jwk-set+json as content type.
        jwksGet.TryGetProperty("responses", out var responses)
            .Should().BeTrue(because: "JWKS operation must have a responses block");
        responses.TryGetProperty("200", out var response200)
            .Should().BeTrue(because: "JWKS operation must document a 200 response");
        response200.TryGetProperty("content", out var content)
            .Should().BeTrue(because: "JWKS 200 response must declare content types");
        content.TryGetProperty("application/jwk-set+json", out _)
            .Should().BeTrue(because: "JWKS 200 response content type must be application/jwk-set+json per RFC 7517 §8.5.1");
    }
}
