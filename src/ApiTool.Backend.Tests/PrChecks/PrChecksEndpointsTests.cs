using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>HTTP integration tests for the pr-checks endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class PrChecksEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;

    public PrChecksEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"owner-prc-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        // Create an org for this test instance.
        var slug = $"prc-{_ownerId:N}"[..20];
        var body = new { name = "PrCheckTestOrg", slug };
        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private object ValidBody(string state = "success") => new
    {
        repo = "acme/api",
        pr = 7,
        state,
        result_id = (string?)null,
    };

    // ── POST happy path ───────────────────────────────────────────────────────

    [Fact]
    public async Task Post_ValidBody_Returns200WithRowPersisted()
    {
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/pr-checks", ValidBody());

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("id").GetString().Should().StartWith("prc_");
        doc.RootElement.GetProperty("state").GetString().Should().Be("success");
    }

    [Fact]
    public async Task Post_FailureState_Returns200()
    {
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/pr-checks", ValidBody("failure"));

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("state").GetString().Should().Be("failure");
    }

    // ── POST validation ───────────────────────────────────────────────────────

    [Fact]
    public async Task Post_InvalidState_Returns400()
    {
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/pr-checks", ValidBody("pending"));

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_request");
    }

    // ── POST authentication / authorization ───────────────────────────────────

    [Fact]
    public async Task Post_NoToken_Returns401()
    {
        using var anonClient = _factory.CreateClient();
        var response = await anonClient.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/pr-checks", ValidBody());

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Post_AsNonMember_Returns403()
    {
        var (strangerToken, _) = TestTokens.CreateNew("stranger@example.com");
        using var strangerClient = _factory.CreateClient();
        strangerClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", strangerToken);

        var response = await strangerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/pr-checks", ValidBody());

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── GET list ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task List_ReturnsPrChecksNewestFirst()
    {
        // Post two checks with different states.
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/pr-checks", ValidBody("success"));
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/pr-checks", ValidBody("failure"));

        var response = await _client.GetAsync($"/api/v1/organizations/{_orgId}/pr-checks?limit=10");

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var arr = doc.RootElement.GetProperty("pr_checks");
        arr.GetArrayLength().Should().BeGreaterThanOrEqualTo(2);
        // Newest first: failure was posted second.
        arr[0].GetProperty("state").GetString().Should().Be("failure");
    }

    [Fact]
    public async Task List_AsNonMember_Returns403()
    {
        var (strangerToken, _) = TestTokens.CreateNew("stranger2@example.com");
        using var strangerClient = _factory.CreateClient();
        strangerClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", strangerToken);

        var response = await strangerClient.GetAsync(
            $"/api/v1/organizations/{_orgId}/pr-checks");

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }
}
