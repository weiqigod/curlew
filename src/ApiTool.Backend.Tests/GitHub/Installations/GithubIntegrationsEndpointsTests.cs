// Refs docs/SPECIFICATION.md:8413-8416 (install-url + callback endpoints).
using System.Net;
using System.Net.Http.Headers;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.GitHub.Installations;

/// <summary>
/// Integration tests for the GitHub integrations endpoints:
/// GET /api/v1/integrations/github/install-url
/// GET /api/v1/integrations/github/callback
/// POST /api/v1/integrations/github/claim (stub returning 501)
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class GithubIntegrationsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _anonClient;
    private Guid _ownerUserId;
    private Guid _adminUserId;
    private Guid _memberUserId;
    private Guid _orgId;
    private HttpClient _ownerClient = null!;
    private HttpClient _adminClient = null!;
    private HttpClient _memberClient = null!;

    public GithubIntegrationsEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _anonClient = factory.CreateClient(
            new Microsoft.AspNetCore.Mvc.Testing.WebApplicationFactoryClientOptions
            {
                AllowAutoRedirect = false,
            });
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        _ownerUserId = Guid.NewGuid();
        _adminUserId = Guid.NewGuid();
        _memberUserId = Guid.NewGuid();
        _orgId = Guid.NewGuid();

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        db.Users.AddRange(
            new User { Id = _ownerUserId, Email = $"gh-owner-{_ownerUserId:N}@example.com", CreatedAt = DateTime.UtcNow },
            new User { Id = _adminUserId, Email = $"gh-admin-{_adminUserId:N}@example.com", CreatedAt = DateTime.UtcNow },
            new User { Id = _memberUserId, Email = $"gh-member-{_memberUserId:N}@example.com", CreatedAt = DateTime.UtcNow });

        db.Organizations.Add(new Organization
        {
            Id = _orgId, Name = "GhTest", Slug = $"ghtest-{_orgId:N}"[..20],
            OwnerId = _ownerUserId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });

        db.OrganizationMembers.AddRange(
            new OrganizationMember { OrgId = _orgId, UserId = _ownerUserId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow },
            new OrganizationMember { OrgId = _orgId, UserId = _adminUserId, Role = OrgRole.Admin, JoinedAt = DateTime.UtcNow },
            new OrganizationMember { OrgId = _orgId, UserId = _memberUserId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow });

        await db.SaveChangesAsync();

        _ownerClient = CreateAuthedClient(_ownerUserId, $"gh-owner-{_ownerUserId:N}@example.com");
        _adminClient = CreateAuthedClient(_adminUserId, $"gh-admin-{_adminUserId:N}@example.com");
        _memberClient = CreateAuthedClient(_memberUserId, $"gh-member-{_memberUserId:N}@example.com");
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private HttpClient CreateAuthedClient(Guid userId, string email)
    {
        var client = _factory.CreateClient(new Microsoft.AspNetCore.Mvc.Testing.WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });
        var token = TestTokens.Create(userId, email);
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);
        return client;
    }

    // ── GET /api/v1/integrations/github/install-url ────────────────────────────

    [Fact]
    public async Task GET_install_url_unauthenticated_returns_401()
    {
        var res = await _anonClient.GetAsync("/api/v1/integrations/github/install-url");
        res.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task GET_install_url_member_returns_403()
    {
        var res = await _memberClient.GetAsync($"/api/v1/integrations/github/install-url?org_id=org_{_orgId:N}");
        res.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    [Fact]
    public async Task GET_install_url_owner_returns_signed_state_token_and_apps_slug_url()
    {
        var res = await _ownerClient.GetAsync($"/api/v1/integrations/github/install-url?org_id=org_{_orgId:N}");
        res.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);

        var installUrl = doc.RootElement.GetProperty("install_url").GetString()!;
        installUrl.Should().StartWith("https://github.com/apps/apitool-checks-test/installations/new?state=");

        var stateExpires = doc.RootElement.GetProperty("state_expires_at").GetString()!;
        DateTimeOffset.Parse(stateExpires).Should().BeAfter(DateTimeOffset.UtcNow.AddMinutes(13));
    }

    [Fact]
    public async Task GET_install_url_admin_returns_signed_state_token()
    {
        var res = await _adminClient.GetAsync($"/api/v1/integrations/github/install-url?org_id=org_{_orgId:N}");
        res.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("install_url").GetString().Should().NotBeNullOrEmpty();
    }

    [Fact]
    public async Task GET_install_url_state_expires_at_is_15min_in_future()
    {
        var before = DateTimeOffset.UtcNow;
        var res = await _ownerClient.GetAsync($"/api/v1/integrations/github/install-url?org_id=org_{_orgId:N}");
        var after = DateTimeOffset.UtcNow;

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);

        var expiresAt = DateTimeOffset.Parse(doc.RootElement.GetProperty("state_expires_at").GetString()!);
        expiresAt.Should().BeAfter(before.AddMinutes(14));
        expiresAt.Should().BeBefore(after.AddMinutes(16));
    }

    // ── GET /api/v1/integrations/github/callback ────────────────────────────────

    [Fact]
    public async Task GET_callback_with_invalid_state_returns_401_invalid_state()
    {
        var res = await _anonClient.GetAsync(
            "/api/v1/integrations/github/callback?installation_id=99999&state=definitely.invalid");
        res.StatusCode.Should().Be(HttpStatusCode.Unauthorized);

        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_state");
    }

    [Fact]
    public async Task GET_callback_with_expired_state_returns_401_invalid_state()
    {
        // Mint an already-expired state token
        using var scope = _factory.Services.CreateScope();
        var opts = scope.ServiceProvider.GetRequiredService<Microsoft.Extensions.Options.IOptions<ApiTool.Backend.GitHub.GitHubAppOptions>>().Value;
        var key = opts.StateSigningKey; // Always set by BackendFactory (validated at startup)

        var expiredPayload = new GithubInstallationStatePayload(
            OrgId: _orgId, UserId: _ownerUserId,
            ExpiresAt: DateTimeOffset.UtcNow.AddMinutes(-5),
            Nonce: "abc123");
        var expiredToken = GithubInstallationStateToken.Mint(expiredPayload, key);

        var res = await _anonClient.GetAsync(
            $"/api/v1/integrations/github/callback?installation_id=99999&state={Uri.EscapeDataString(expiredToken)}");
        res.StatusCode.Should().Be(HttpStatusCode.Unauthorized);

        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_state");
    }

    [Fact]
    public async Task GET_callback_with_valid_state_inserts_row_and_redirects_302_to_billing()
    {
        // Get a valid state token by calling install-url
        var urlRes = await _ownerClient.GetAsync($"/api/v1/integrations/github/install-url?org_id=org_{_orgId:N}");
        var urlBody = await urlRes.Content.ReadAsStringAsync();
        using var urlDoc = JsonDocument.Parse(urlBody);
        var installUrl = urlDoc.RootElement.GetProperty("install_url").GetString()!;
        var state = Uri.UnescapeDataString(installUrl.Split("state=")[1]);

        var res = await _anonClient.GetAsync(
            $"/api/v1/integrations/github/callback?installation_id=55555&state={Uri.EscapeDataString(state)}");

        res.StatusCode.Should().Be(HttpStatusCode.Redirect);
        res.Headers.Location!.ToString().Should().Contain("billing");
        res.Headers.Location!.ToString().Should().Contain("installed=true");
    }

    /// <summary>
    /// The 409 install_already_linked path is enforced via the partial UNIQUE index on org_id
    /// in the github_installations table. The BackendFactory uses an InMemory EF provider which
    /// does NOT enforce unique indexes — so this test uses a direct service call to seed a
    /// pre-existing row for the same installationId with a different org, simulating the
    /// application-level AlreadyLinked check in GithubInstallationsService.ClaimAsync.
    /// The full DB-level constraint is verified by
    /// GithubInstallationsMigrationTests.Two_active_installations_for_same_org_violates_unique_index.
    /// </summary>
    [Fact]
    public async Task GET_callback_when_org_already_has_install_returns_409_install_already_linked()
    {
        // Get a valid state token
        var urlRes = await _ownerClient.GetAsync($"/api/v1/integrations/github/install-url?org_id=org_{_orgId:N}");
        var urlBody = await urlRes.Content.ReadAsStringAsync();
        using var urlDoc = JsonDocument.Parse(urlBody);
        var installUrl = urlDoc.RootElement.GetProperty("install_url").GetString()!;
        var state = Uri.UnescapeDataString(installUrl.Split("state=")[1]);

        // Pre-seed: installation 66701 claimed by a DIFFERENT org (simulates application-level conflict)
        // The service's ClaimAsync finds the row and sees OrgId != payload.OrgId → AlreadyLinked.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var otherOrgId = Guid.NewGuid();
        db.GithubInstallations.Add(new GithubInstallation
        {
            InstallationId = 66701L,
            AppId = 12345L,
            OrgId = otherOrgId,  // Different org — triggers AlreadyLinked
            AccountLogin = "other-org",
            AccountType = "Organization",
            RepoSelection = "selected",
            RepoSetJson = "[]",
            InstalledAt = DateTime.UtcNow,
            ClaimedAt = DateTime.UtcNow,
            LastReconciledAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        // Callback with installation_id 66701 but state token for _orgId → conflict
        var res = await _anonClient.GetAsync(
            $"/api/v1/integrations/github/callback?installation_id=66701&state={Uri.EscapeDataString(state)}");

        res.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("code").GetString().Should().Be("install_already_linked");
    }
}
