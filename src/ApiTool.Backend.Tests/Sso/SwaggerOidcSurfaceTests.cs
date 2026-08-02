using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Verifies that Swagger exposes all three OIDC SSO endpoints (B8).</summary>
[Collection(BackendCollection.Name)]
public sealed class SwaggerOidcSurfaceTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public SwaggerOidcSurfaceTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Swagger_lists_oidc_endpoints()
    {
        var client = _factory.CreateClient();
        var json = await client.GetStringAsync("/swagger/v1/swagger.json");
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        paths.TryGetProperty("/api/v1/organizations/{id}/sso/oidc", out _)
            .Should().BeTrue(because: "PUT /organizations/{id}/sso/oidc must be in Swagger");
        paths.TryGetProperty("/api/v1/sso/oidc/{orgId}/login", out _)
            .Should().BeTrue(because: "GET /sso/oidc/{orgId}/login must be in Swagger");
        paths.TryGetProperty("/api/v1/sso/oidc/{orgId}/callback", out _)
            .Should().BeTrue(because: "GET /sso/oidc/{orgId}/callback must be in Swagger");
    }
}
