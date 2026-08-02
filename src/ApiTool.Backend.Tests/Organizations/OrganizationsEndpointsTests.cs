using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Organizations;

/// <summary>Full behavior-level tests for the /api/v1/organizations endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class OrganizationsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private readonly string _ownerToken;

    public OrganizationsEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var ownerEmail = $"owner-{_ownerId:N}@example.com";
        _ownerToken = TestTokens.Create(_ownerId, ownerEmail);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", _ownerToken);
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    // ── POST /api/v1/organizations ─────────────────────────────────────────

    [Fact]
    public async Task Post_creates_org_and_returns_201_with_owner_role_and_default_seat_counts()
    {
        var slug = $"acme-{Guid.NewGuid():N}"[..20];
        var body = new { name = "Acme", slug };

        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);

        response.StatusCode.Should().Be(HttpStatusCode.Created);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("role").GetString().Should().Be("owner");
        doc.RootElement.GetProperty("seat_count").GetInt32().Should().Be(1);
        doc.RootElement.GetProperty("seat_limit").GetInt32().Should().Be(1);
        doc.RootElement.GetProperty("slug").GetString().Should().Be(slug);
        doc.RootElement.GetProperty("tier").GetString().Should().Be("free");
    }

    [Fact]
    public async Task Post_with_duplicate_slug_returns_409_organization_slug_taken()
    {
        var slug = $"dup-{Guid.NewGuid():N}"[..20];
        var body = new { name = "Dup", slug };

        await _client.PostAsJsonAsync("/api/v1/organizations", body);
        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("organization_slug_taken");
    }

    [Fact]
    public async Task Post_with_uppercase_slug_returns_400_invalid_slug()
    {
        var body = new { name = "Bad", slug = "BadSlug" };

        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_slug");
    }

    [Fact]
    public async Task Post_with_too_short_slug_returns_400_invalid_slug()
    {
        var body = new { name = "X", slug = "x" };

        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_slug");
    }

    [Fact]
    public async Task Post_with_slug_containing_underscore_returns_400_invalid_slug()
    {
        var body = new { name = "Bad", slug = "bad_slug" };

        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_slug");
    }

    // ── GET /api/v1/organizations ──────────────────────────────────────────

    [Fact]
    public async Task Get_list_returns_empty_for_user_with_no_memberships()
    {
        // Use a fresh user with no orgs — unique email per run to avoid reuse conflicts
        var (token, _) = TestTokens.CreateNew($"fresh-{Guid.NewGuid():N}@example.com");
        var freshClient = _factory.CreateClient();
        freshClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);

        var response = await freshClient.GetAsync("/api/v1/organizations");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("organizations").GetArrayLength().Should().Be(0);
    }

    [Fact]
    public async Task Get_list_returns_orgs_for_member_user()
    {
        var slug = $"list-{Guid.NewGuid():N}"[..20];
        await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "ListOrg", slug });

        var response = await _client.GetAsync("/api/v1/organizations");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var orgs = doc.RootElement.GetProperty("organizations");
        orgs.GetArrayLength().Should().BeGreaterThan(0);
    }

    // ── GET /api/v1/organizations/{id} ─────────────────────────────────────

    [Fact]
    public async Task Get_detail_returns_200_with_role_and_seats_for_member()
    {
        var slug = $"detail-{Guid.NewGuid():N}"[..20];
        var createResponse = await _client.PostAsJsonAsync(
            "/api/v1/organizations", new { name = "Detail", slug });

        var createdJson = await createResponse.Content.ReadAsStringAsync();
        using var createdDoc = JsonDocument.Parse(createdJson);
        var id = createdDoc.RootElement.GetProperty("id").GetString()!;

        var response = await _client.GetAsync($"/api/v1/organizations/{id}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("role").GetString().Should().Be("owner");
        doc.RootElement.GetProperty("seat_count").GetInt32().Should().Be(1);
    }

    [Fact]
    public async Task Get_detail_returns_404_for_non_member_without_leaking_existence()
    {
        // Create org with owner user, try to fetch as different user
        var slug = $"noac-{Guid.NewGuid():N}"[..20];
        var createResponse = await _client.PostAsJsonAsync(
            "/api/v1/organizations", new { name = "NoAccess", slug });

        var createdJson = await createResponse.Content.ReadAsStringAsync();
        using var createdDoc = JsonDocument.Parse(createdJson);
        var id = createdDoc.RootElement.GetProperty("id").GetString()!;

        var (token, _) = TestTokens.CreateNew($"nobody-{Guid.NewGuid():N}@example.com");
        var otherClient = _factory.CreateClient();
        otherClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);

        var response = await otherClient.GetAsync($"/api/v1/organizations/{id}");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("organization_not_found");
    }

    [Fact]
    public async Task Get_detail_returns_404_for_malformed_id()
    {
        var response = await _client.GetAsync("/api/v1/organizations/not-an-org-id");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("organization_not_found");
    }

    // ── 401 paths ─────────────────────────────────────────────────────────

    [Fact]
    public async Task All_endpoints_return_401_when_no_bearer_header()
    {
        var anon = _factory.CreateClient();  // no auth header

        var getList = await anon.GetAsync("/api/v1/organizations");
        var post = await anon.PostAsync("/api/v1/organizations",
            new StringContent("{}", Encoding.UTF8, "application/json"));
        var getDetail = await anon.GetAsync("/api/v1/organizations/org_00000000000000000000000000000000");

        getList.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        post.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        getDetail.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Post_with_empty_name_returns_400_invalid_name()
    {
        var slug = $"name-{Guid.NewGuid():N}"[..20];
        var body = new { name = "", slug };

        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_name");
    }
}
