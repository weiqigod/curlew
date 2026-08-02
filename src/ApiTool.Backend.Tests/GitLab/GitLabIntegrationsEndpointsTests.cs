// Behavioural integration tests for GET, POST, DELETE /api/v1/integrations/gitlab (M16-016).
// Replaces the M16-013 stub tests.
// Refs docs/SPECIFICATION.md:9177-9195 (Per-Org GitLab Configuration).
using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab.Installations;
using ApiTool.Backend.Tests.GitLab.Installations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Hosting;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.GitLab;

/// <summary>
/// Behavioural integration tests for the GitLab integrations endpoints (M16-016).
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class GitLabIntegrationsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private Guid _userId;
    private Guid _orgId;
    private HttpClient _authedClient = null!;
    private HttpClient _anonClient;
    private FakeGitLabProjectLookup _fakeLookup = null!;

    public GitLabIntegrationsEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _anonClient = factory.CreateClient();
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        _userId = Guid.NewGuid();
        _orgId = Guid.NewGuid();

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        db.Users.Add(new User { Id = _userId, Email = $"gl-ep-{_userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        var org = new Organization { Id = _orgId, Name = "GitLab Test Org", Slug = $"gl-ep-{_orgId:N}", CreatedAt = DateTime.UtcNow };
        db.Organizations.Add(org);
        db.OrganizationMembers.Add(new OrganizationMember
        {
            UserId = _userId,
            OrgId = _orgId,
            Role = OrgRole.Owner,
            JoinedAt = DateTime.UtcNow
        });
        await db.SaveChangesAsync();

        // Seed a Team-tier subscription so tier-gate tests can verify the 402 path
        // via a separate client (see CreateFreeTierClient()).
        await _factory.SeedSubscriptionAsync(_orgId, SubscriptionTier.Team);

        // Wire the fake lookup into a custom factory for this test instance.
        _fakeLookup = new FakeGitLabProjectLookup();
        var derived = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(s =>
            {
                s.RemoveAll<IGitLabProjectLookup>();
                s.AddSingleton<IGitLabProjectLookup>(_fakeLookup);
            }));

        _authedClient = derived.CreateClient();
        var token = TestTokens.Create(_userId, $"gl-ep-{_userId:N}@example.com");
        _authedClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    // ── GET ───────────────────────────────────────────────────────────────────

    [Fact]
    public async Task GET_without_bearer_returns_401()
    {
        var res = await _anonClient.GetAsync("/api/v1/integrations/gitlab");
        res.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task GET_multi_org_user_without_org_id_returns_400_ambiguous_org_id()
    {
        // Use a dedicated user so mutations do not bleed into other tests.
        var multiUserId = Guid.NewGuid();
        var org1Id = Guid.NewGuid();
        var org2Id = Guid.NewGuid();

        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.Users.Add(new User { Id = multiUserId, Email = $"multi-{multiUserId:N}@example.com", CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization { Id = org1Id, Name = "Multi Org 1", Slug = $"multi1-{org1Id:N}", CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization { Id = org2Id, Name = "Multi Org 2", Slug = $"multi2-{org2Id:N}", CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember { UserId = multiUserId, OrgId = org1Id, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember { UserId = multiUserId, OrgId = org2Id, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow.AddSeconds(1) });
            await db.SaveChangesAsync();
        }
        // Seed Team-tier for both orgs so the tier gate is not triggered before the ambiguous-org check.
        await _factory.SeedSubscriptionAsync(org1Id, SubscriptionTier.Team);
        await _factory.SeedSubscriptionAsync(org2Id, SubscriptionTier.Team);

        var multiClient = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(s =>
            {
                s.RemoveAll<IGitLabProjectLookup>();
                s.AddSingleton<IGitLabProjectLookup>(_fakeLookup);
            })).CreateClient();
        multiClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue(
            "Bearer", TestTokens.Create(multiUserId, $"multi-{multiUserId:N}@example.com"));

        var res = await multiClient.GetAsync("/api/v1/integrations/gitlab");
        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("ambiguous_org_id");
    }

    [Fact]
    public async Task GET_with_explicit_org_id_returns_200()
    {
        var res = await _authedClient.GetAsync($"/api/v1/integrations/gitlab?org_id={_orgId}");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task GET_free_tier_returns_402_problem_with_gitlab_integrations_code()
    {
        // Create a user+org with NO subscription (free tier).
        var (client, _) = await CreateFreeTierClientAsync();
        var res = await client.GetAsync("/api/v1/integrations/gitlab");
        res.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
        var raw = await res.Content.ReadAsStringAsync();
        raw.Should().Contain("gitlab_integrations");
    }

    [Fact]
    public async Task GET_team_tier_empty_returns_200_with_empty_list()
    {
        var res = await _authedClient.GetAsync("/api/v1/integrations/gitlab");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("installations").GetArrayLength().Should().Be(0);
    }

    [Fact]
    public async Task GET_team_tier_returns_only_undeleted_installations_for_this_org()
    {
        // Seed one live and one soft-deleted installation.
        var liveId = Guid.NewGuid();
        var deletedId = Guid.NewGuid();
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = liveId,
                OrgId = _orgId,
                ProjectId = 101L,
                ProjectPath = "g/live",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [1, 2, 3],
                AccessTokenKid = "k1",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow
            });
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = deletedId,
                OrgId = _orgId,
                ProjectId = 102L,
                ProjectPath = "g/deleted",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [4, 5, 6],
                AccessTokenKid = "k2",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
                DeletedAt = DateTime.UtcNow
            });
            await db.SaveChangesAsync();
        }

        var res = await _authedClient.GetAsync("/api/v1/integrations/gitlab");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        var arr = body.GetProperty("installations");
        arr.GetArrayLength().Should().BeGreaterThanOrEqualTo(1);
        var ids = Enumerable.Range(0, arr.GetArrayLength())
            .Select(i => arr[i].GetProperty("id").GetString())
            .ToList();
        ids.Should().Contain(liveId.ToString());
        ids.Should().NotContain(deletedId.ToString());
    }

    [Fact]
    public async Task GET_returns_access_token_revoked_at_when_set()
    {
        var instId = Guid.NewGuid();
        var revokedAt = DateTime.UtcNow.AddDays(-2);

        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = instId,
                OrgId = _orgId,
                ProjectId = 999L,
                ProjectPath = "g/revoked-token",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [20, 21],
                AccessTokenKid = "k-revoked",
                AccessTokenRevokedAt = revokedAt,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow
            });
            await db.SaveChangesAsync();
        }

        var res = await _authedClient.GetAsync("/api/v1/integrations/gitlab");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        var arr = body.GetProperty("installations");
        var found = Enumerable.Range(0, arr.GetArrayLength())
            .Select(i => arr[i])
            .FirstOrDefault(el => el.GetProperty("id").GetString() == instId.ToString());
        found.ValueKind.Should().NotBe(JsonValueKind.Undefined);
        // Verify access_token_revoked_at is serialized as a non-null timestamp in the JSON response.
        found.GetProperty("access_token_revoked_at").ValueKind.Should().NotBe(JsonValueKind.Null);
        found.GetProperty("access_token_revoked_at").ValueKind.Should().NotBe(JsonValueKind.Undefined);
    }

    [Fact]
    public async Task GET_returns_last_status_post_timestamp_when_pr_checks_present()
    {
        var instId = Guid.NewGuid();
        var postedAt = DateTime.UtcNow.AddHours(-1);

        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = instId,
                OrgId = _orgId,
                ProjectId = 201L,
                ProjectPath = "g/ts-test",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [7, 8],
                AccessTokenKid = "k3",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow
            });
            db.PrChecks.Add(new PrCheck
            {
                Id = Guid.NewGuid(),
                OrgId = _orgId,
                Repo = "g/ts-test",
                Pr = 1,
                State = "success",
                HeadSha = new string('a', 40),
                Provider = "gitlab",
                GitLabInstallationId = instId,
                CreatedAt = DateTime.UtcNow,
                PostedAt = postedAt
            });
            await db.SaveChangesAsync();
        }

        var res = await _authedClient.GetAsync("/api/v1/integrations/gitlab");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        var arr = body.GetProperty("installations");
        var found = Enumerable.Range(0, arr.GetArrayLength())
            .Select(i => arr[i])
            .FirstOrDefault(el => el.GetProperty("id").GetString() == instId.ToString());
        found.ValueKind.Should().NotBe(JsonValueKind.Undefined);
        found.GetProperty("last_status_post_at").ValueKind.Should().NotBe(JsonValueKind.Null);
    }

    // ── POST ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task POST_happy_path_creates_row_with_encrypted_pat_and_returns_201()
    {
        _fakeLookup.ReturnsOk(99L, "group/project");

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project", access_token = "mytoken" }));

        res.StatusCode.Should().Be(HttpStatusCode.Created);

        // Verify row persisted with ciphertext.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabInstallations.FindAsync(
            Guid.Parse((await res.Content.ReadFromJsonAsync<JsonElement>())
                .GetProperty("id").GetString()!));
        row.Should().NotBeNull();
        row!.AccessTokenCiphertext.Should().NotBeEmpty();
    }

    [Fact]
    public async Task POST_response_body_never_echoes_the_pat()
    {
        _fakeLookup.ReturnsOk(88L, "group/secret");

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/secret", access_token = "super-secret-token" }));

        res.StatusCode.Should().Be(HttpStatusCode.Created);
        var raw = await res.Content.ReadAsStringAsync();
        raw.Should().NotContain("super-secret-token");
    }

    [Fact]
    public async Task POST_defaults_gitlab_base_url_to_https_gitlab_com_when_missing()
    {
        _fakeLookup.ReturnsOk(77L, "group/default-url");

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/default-url", access_token = "tok" }));

        res.StatusCode.Should().Be(HttpStatusCode.Created);
        var raw = await res.Content.ReadAsStringAsync();
        // GitLabBaseUrl serialized as "gitlab_base_url" via JsonPropertyName attribute.
        raw.Should().Contain("https://gitlab.com");
        raw.Should().Contain("gitlab_base_url");
    }

    [Fact]
    public async Task POST_missing_project_path_returns_400_invalid_request()
    {
        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { access_token = "tok" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task POST_missing_access_token_returns_400_invalid_request()
    {
        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task POST_lookup_NotFound_returns_400_project_not_found()
    {
        _fakeLookup.Returns(new GitLabProjectLookupResult(GitLabProjectLookupStatus.NotFound, null, null));

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/missing", access_token = "tok" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("project_not_found");
    }

    [Fact]
    public async Task POST_lookup_Unauthorized_returns_400_pat_unauthorized()
    {
        _fakeLookup.Returns(new GitLabProjectLookupResult(GitLabProjectLookupStatus.Unauthorized, null, null));

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project", access_token = "bad-token" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("pat_unauthorized");
    }

    [Fact]
    public async Task POST_lookup_InsecureBaseUrl_returns_400_insecure_base_url()
    {
        _fakeLookup.Returns(new GitLabProjectLookupResult(GitLabProjectLookupStatus.InsecureBaseUrl, null, null));

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project", access_token = "tok", gitlab_base_url = "http://insecure.internal" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("insecure_base_url");
    }

    [Fact]
    public async Task POST_lookup_InvalidBaseUrl_returns_400_invalid_base_url()
    {
        // Finding #1: when GitLabProjectLookup returns InvalidBaseUrl (e.g. the caller submitted
        // a syntactically invalid gitlab_base_url), the endpoint must return 400 with
        // code = "invalid_base_url" rather than propagating an exception as a 500.
        _fakeLookup.Returns(new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidBaseUrl, null, null));

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project", access_token = "tok", gitlab_base_url = "not-a-url" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("invalid_base_url");
    }

    [Fact]
    public async Task POST_lookup_Unreachable_returns_502_gitlab_unreachable()
    {
        _fakeLookup.Returns(new GitLabProjectLookupResult(GitLabProjectLookupStatus.Unreachable, null, null));

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project", access_token = "tok" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadGateway);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("gitlab_unreachable");
    }

    [Fact]
    public async Task POST_lookup_InvalidResponse_returns_502_gitlab_invalid_response()
    {
        _fakeLookup.Returns(new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidResponse, null, null));

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project", access_token = "tok" }));

        res.StatusCode.Should().Be(HttpStatusCode.BadGateway);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("gitlab_invalid_response");
    }

    [Fact]
    public async Task POST_with_ca_bundle_persists_bundle_on_row()
    {
        const string pemBundle = "---BEGIN CERTIFICATE---\nfakedata\n---END CERTIFICATE---";
        _fakeLookup.ReturnsOk(456L, "group/ca-project");

        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new
            {
                project_path = "group/ca-project",
                access_token = "tok",
                gitlab_ca_bundle = pemBundle
            }));

        res.StatusCode.Should().Be(HttpStatusCode.Created);

        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        var createdId = Guid.Parse(body.GetProperty("id").GetString()!);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabInstallations.FindAsync(createdId);
        row.Should().NotBeNull();
        row!.GitLabCaBundle.Should().Be(pemBundle);
    }

    [Fact]
    public async Task POST_duplicate_org_project_returns_409()
    {
        _fakeLookup.ReturnsOk(55L, "group/dup");

        // First create succeeds.
        await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/dup", access_token = "tok1" }));

        // Second create with same project_path + base_url should conflict.
        var res = await _authedClient.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/dup", access_token = "tok2" }));

        res.StatusCode.Should().Be(HttpStatusCode.Conflict);
    }

    [Fact]
    public async Task POST_free_tier_returns_402_problem()
    {
        var (client, _) = await CreateFreeTierClientAsync();

        var res = await client.PostAsync(
            "/api/v1/integrations/gitlab",
            JsonContent.Create(new { project_path = "group/project", access_token = "tok" }));

        res.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    // ── DELETE ────────────────────────────────────────────────────────────────

    [Fact]
    public async Task DELETE_existing_row_soft_deletes_and_returns_204()
    {
        var instId = Guid.NewGuid();
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = instId,
                OrgId = _orgId,
                ProjectId = 300L,
                ProjectPath = "g/to-delete",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [10, 11],
                AccessTokenKid = "k",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow
            });
            await db.SaveChangesAsync();
        }

        var res = await _authedClient.DeleteAsync($"/api/v1/integrations/gitlab/{instId}");
        res.StatusCode.Should().Be(HttpStatusCode.NoContent);

        // Row should be soft-deleted.
        using var scope2 = _factory.Services.CreateScope();
        var db2 = scope2.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db2.GitLabInstallations.FindAsync(instId);
        row!.DeletedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task DELETE_unknown_id_returns_404()
    {
        var res = await _authedClient.DeleteAsync($"/api/v1/integrations/gitlab/{Guid.NewGuid()}");
        res.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task DELETE_already_soft_deleted_returns_404()
    {
        var instId = Guid.NewGuid();
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = instId,
                OrgId = _orgId,
                ProjectId = 301L,
                ProjectPath = "g/already-deleted",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [12, 13],
                AccessTokenKid = "k",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
                DeletedAt = DateTime.UtcNow
            });
            await db.SaveChangesAsync();
        }

        var res = await _authedClient.DeleteAsync($"/api/v1/integrations/gitlab/{instId}");
        res.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task DELETE_other_orgs_installation_returns_404()
    {
        // Create another org and seed an installation belonging to it.
        var otherId = Guid.NewGuid();
        var otherInstId = Guid.NewGuid();
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.Organizations.Add(new Organization { Id = otherId, Name = "Other Org", Slug = $"other-{otherId:N}", CreatedAt = DateTime.UtcNow });
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = otherInstId,
                OrgId = otherId,
                ProjectId = 302L,
                ProjectPath = "g/other",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [14, 15],
                AccessTokenKid = "k",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow
            });
            await db.SaveChangesAsync();
        }

        // Our authed user belongs to _orgId, not otherId.
        var res = await _authedClient.DeleteAsync($"/api/v1/integrations/gitlab/{otherInstId}");
        res.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task DELETE_free_tier_returns_402_problem()
    {
        var (client, freeOrgId) = await CreateFreeTierClientAsync();

        // Seed an installation for the free org.
        var instId = Guid.NewGuid();
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = instId,
                OrgId = freeOrgId,
                ProjectId = 303L,
                ProjectPath = "g/free",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = [16, 17],
                AccessTokenKid = "k",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow
            });
            await db.SaveChangesAsync();
        }

        var res = await client.DeleteAsync($"/api/v1/integrations/gitlab/{instId}");
        res.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    // ── helpers ───────────────────────────────────────────────────────────────

    /// <summary>Creates a new user+org pair with no subscription (free tier) and returns a client for it.</summary>
    private async Task<(HttpClient client, Guid orgId)> CreateFreeTierClientAsync()
    {
        var freeUserId = Guid.NewGuid();
        var freeOrgId = Guid.NewGuid();

        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.Users.Add(new User { Id = freeUserId, Email = $"free-{freeUserId:N}@example.com", CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization { Id = freeOrgId, Name = "Free Org", Slug = $"free-{freeOrgId:N}", CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                UserId = freeUserId,
                OrgId = freeOrgId,
                Role = OrgRole.Owner,
                JoinedAt = DateTime.UtcNow
            });
            await db.SaveChangesAsync();
            // No subscription seeded → free tier.
        }

        var client = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(s =>
            {
                s.RemoveAll<IGitLabProjectLookup>();
                s.AddSingleton<IGitLabProjectLookup>(_fakeLookup);
            })).CreateClient();
        var token = TestTokens.Create(freeUserId, $"free-{freeUserId:N}@example.com");
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);
        return (client, freeOrgId);
    }
}
