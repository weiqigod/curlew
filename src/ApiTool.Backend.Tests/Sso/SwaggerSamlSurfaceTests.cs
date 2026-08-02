using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Verifies that Swagger exposes all three SAML SSO endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class SwaggerSamlSurfaceTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public SwaggerSamlSurfaceTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Swagger_lists_saml_endpoints()
    {
        var client = _factory.CreateClient();

        var json = await client.GetStringAsync("/swagger/v1/swagger.json");
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        // B8: three SAML endpoints must appear in Swagger
        paths.TryGetProperty("/api/v1/organizations/{id}/sso/saml", out _)
            .Should().BeTrue(because: "PUT /organizations/{id}/sso/saml must be in Swagger");
        paths.TryGetProperty("/api/v1/sso/saml/{orgId}/login", out _)
            .Should().BeTrue(because: "GET /sso/saml/{orgId}/login must be in Swagger");
        paths.TryGetProperty("/api/v1/sso/saml/{orgId}/acs", out _)
            .Should().BeTrue(because: "POST /sso/saml/{orgId}/acs must be in Swagger");
    }
}
