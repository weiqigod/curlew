using System.Net;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.GitHub;

/// <summary>
/// Integration tests for the /internal/github-app/jwt-self-test endpoint.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalGitHubAppEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    public InternalGitHubAppEndpointsTests(BackendFactory factory) => _factory = factory;
    public Task InitializeAsync() => _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task GET_jwt_self_test_returns_alg_RS256_typ_JWT_iss_appid_offsets_and_verified()
    {
        var client = _factory.CreateClient();
        var res = await client.GetAsync("/internal/github-app/jwt-self-test");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("alg").GetString().Should().Be("RS256");
        doc.RootElement.GetProperty("typ").GetString().Should().Be("JWT");
        doc.RootElement.GetProperty("iss").GetInt64().Should().Be(12345);  // AppId configured in BackendFactory
        doc.RootElement.GetProperty("iat_offset_seconds").GetInt32().Should().Be(-60);
        doc.RootElement.GetProperty("exp_offset_seconds").GetInt32().Should().Be(540);
        doc.RootElement.GetProperty("verified").GetBoolean().Should().BeTrue();
    }
}
