// Refs docs/SPECIFICATION.md:8429-8550 (POST /api/v1/pr-checks endpoint behaviours).
// M16-014: adds provider dispatch tests.
using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.PrChecks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// HTTP integration tests for <c>POST /api/v1/pr-checks</c> (v4.2.1 upload endpoint).
/// Uses <see cref="BackendFactory"/> with a fake <see cref="ICheckRunPoster"/> so the
/// GitHub side-effect is deterministic and isolated from real HTTP calls.
/// Named so the filter FullyQualifiedName~PrChecksExpansion does NOT conflict with
/// the existing <c>PrChecksEndpointsTests</c> class (which covers the v4.2 legacy path).
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class PrChecksUploadEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly Guid _ownerId;
    private readonly string _ownerEmail;
    private string? _orgId;

    // Default: poster returns Posted with check_run_id=42
    private FakeCheckRunPoster _fakePoster = FakeCheckRunPoster.Posted(42L);
    // Default GitLab fake: posted
    private FakeGitLabCheckPoster _fakeGitLabPoster = FakeGitLabCheckPoster.Posted();

    public PrChecksUploadEndpointTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        _ownerEmail = $"upload-ep-{_ownerId:N}@example.com";
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        // Create an org via the legacy endpoint (needs a fresh org per test class instance)
        var token = TestTokens.Create(_ownerId, _ownerEmail);
        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        var slug = $"up{_ownerId:N}"[..20];
        var resp = await client.PostAsJsonAsync("/api/v1/organizations",
            new { name = "UploadTestOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    // ── Helpers ──────────────────────────────────────────────────────────────

    /// <summary>
    /// Creates an <see cref="HttpClient"/> using a custom factory where the
    /// <see cref="ICheckRunPoster"/> DI registration is replaced with <see cref="_fakePoster"/>.
    /// A valid bearer token is pre-attached for the owner.
    /// </summary>
    private HttpClient CreateAuthorizedClient()
    {
        var poster = _fakePoster; // capture current fake for this client
        var glPoster = _fakeGitLabPoster; // capture current GitLab fake
        var client = _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureServices(services =>
            {
                services.RemoveAll<ICheckRunPoster>();
                services.AddScoped<ICheckRunPoster>(_ => poster);
                services.RemoveAll<IGitLabCheckPoster>();
                services.AddScoped<IGitLabCheckPoster>(_ => glPoster);
            });
        }).CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(_ownerId, _ownerEmail));
        return client;
    }

    private static object ValidUploadBody(string state = "success") => new
    {
        repo = "acme/api",
        pr = 7,
        state,
        head_sha = new string('a', 40),
    };

    private static StringContent JsonContent(object body) =>
        new(JsonSerializer.Serialize(body, new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        }), Encoding.UTF8, "application/json");

    // ── Happy path ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_HappyPath_Returns200WithPostedAndCheckRunId()
    {
        _fakePoster = FakeCheckRunPoster.Posted(checkRunId: 99L);
        using var client = CreateAuthorizedClient();

        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody()));

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("state").GetString().Should().Be("posted");
        doc.RootElement.GetProperty("github_check_run_id").GetInt64().Should().Be(99L);
        doc.RootElement.GetProperty("pr_check_id").GetString().Should().StartWith("prc_");
    }

    // ── Authentication ──────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_NoBearer_Returns401()
    {
        _fakePoster = FakeCheckRunPoster.Posted();
        var anonClient = _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureServices(services =>
            {
                services.RemoveAll<ICheckRunPoster>();
                services.AddScoped<ICheckRunPoster>(_ => _fakePoster);
            });
        }).CreateClient();

        var resp = await anonClient.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody()));

        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    // ── Token-leak detection ────────────────────────────────────────────────────

    [Fact]
    public async Task Post_BodyContainsGhsToken_Returns400_PRCHECK_TOKEN_LEAK_DETECTED()
    {
        _fakePoster = FakeCheckRunPoster.Posted();
        using var client = CreateAuthorizedClient();

        // Embed a fake ghs_ token in the body (40 chars = 3 prefix chars + 37 alphanums)
        var leakyBody = new
        {
            repo = "acme/api",
            pr = 7,
            state = "success",
            head_sha = new string('a', 40),
            details_url = "https://example.com/ghs_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        };

        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(leakyBody));

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_TOKEN_LEAK_DETECTED");
    }

    // ── State validation ────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_InvalidState_Returns400_PRCHECK_INVALID_STATE()
    {
        _fakePoster = FakeCheckRunPoster.Posted();
        using var client = CreateAuthorizedClient();

        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody("pending")));

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_INVALID_STATE");
    }

    [Fact]
    public async Task Post_ActionRequiredState_Returns400()
    {
        _fakePoster = FakeCheckRunPoster.Posted();
        using var client = CreateAuthorizedClient();

        var resp = await client.PostAsync("/api/v1/pr-checks",
            JsonContent(ValidUploadBody("action_required")));

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Post_AllSixValidStates_Accepted()
    {
        string[] states = ["success", "failure", "cancelled", "timed_out", "neutral", "skipped"];
        foreach (var state in states)
        {
            _fakePoster = FakeCheckRunPoster.Posted();
            using var client = CreateAuthorizedClient();
            var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody(state)));
            resp.StatusCode.Should().Be(HttpStatusCode.OK, $"state '{state}' should be accepted");
        }
    }

    // ── head_sha validation ─────────────────────────────────────────────────────

    [Fact]
    public async Task Post_MissingHeadSha_Returns400()
    {
        _fakePoster = FakeCheckRunPoster.Posted();
        using var client = CreateAuthorizedClient();

        var body = new { repo = "acme/api", pr = 7, state = "success" };
        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(body));

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Post_HeadShaTooShort_Returns400()
    {
        _fakePoster = FakeCheckRunPoster.Posted();
        using var client = CreateAuthorizedClient();

        var body = new { repo = "acme/api", pr = 7, state = "success", head_sha = "abc" };
        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(body));

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── GitHub poster error mapping ─────────────────────────────────────────────

    [Fact]
    public async Task Post_NoInstallation_Returns404_PRCHECK_NO_INSTALLATION()
    {
        _fakePoster = FakeCheckRunPoster.NoInstallation();
        using var client = CreateAuthorizedClient();

        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody()));

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_NO_INSTALLATION");
    }

    [Fact]
    public async Task Post_RepoNotCovered_Returns403_PRCHECK_REPO_NOT_COVERED()
    {
        _fakePoster = FakeCheckRunPoster.RepoNotCovered();
        using var client = CreateAuthorizedClient();

        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody()));

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_REPO_NOT_COVERED");
    }

    [Fact]
    public async Task Post_RateLimited_Returns429_PRCHECK_GITHUB_RATE_LIMITED()
    {
        _fakePoster = FakeCheckRunPoster.RateLimited();
        using var client = CreateAuthorizedClient();

        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody()));

        resp.StatusCode.Should().Be(HttpStatusCode.TooManyRequests);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_GITHUB_RATE_LIMITED");
    }

    // ── M16-014: Provider dispatch tests ───────────────────────────────────────

    [Fact]
    public async Task Post_ProviderGithub_RoutesToGithubPoster()
    {
        _fakePoster = FakeCheckRunPoster.Posted(77L);
        _fakeGitLabPoster = FakeGitLabCheckPoster.Posted();
        using var client = CreateAuthorizedClient();

        var body = new { repo = "acme/api", pr = 7, state = "success", head_sha = new string('a', 40), provider = "github" };
        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(body));

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        _fakePoster.CallCount.Should().Be(1, "github poster should be called");
        _fakeGitLabPoster.CallCount.Should().Be(0, "gitlab poster should NOT be called");
    }

    [Fact]
    public async Task Post_ProviderMissing_DefaultsToGithub()
    {
        _fakePoster = FakeCheckRunPoster.Posted(77L);
        _fakeGitLabPoster = FakeGitLabCheckPoster.Posted();
        using var client = CreateAuthorizedClient();

        // No provider field
        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(ValidUploadBody()));

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        _fakePoster.CallCount.Should().Be(1, "github poster should be the default");
        _fakeGitLabPoster.CallCount.Should().Be(0);
    }

    [Fact]
    public async Task Post_ProviderInvalid_Returns400()
    {
        _fakePoster = FakeCheckRunPoster.Posted();
        using var client = CreateAuthorizedClient();

        var body = new { repo = "acme/api", pr = 7, state = "success", head_sha = new string('a', 40), provider = "bitbucket" };
        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(body));

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("provider");
    }

    [Fact]
    public async Task Post_ProviderGitlab_NoInstallationInDb_Returns404_PRCHECK_GITLAB_NO_INSTALLATION()
    {
        // provider=gitlab but no gitlab_installations row in DB → 404 from endpoint (before poster is called)
        _fakeGitLabPoster = FakeGitLabCheckPoster.Posted();
        using var client = CreateAuthorizedClient();

        var body = new { repo = "acme/api", pr = 7, state = "success", head_sha = new string('a', 40), provider = "gitlab" };
        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(body));

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("PRCHECK_GITLAB_NO_INSTALLATION");
        _fakeGitLabPoster.CallCount.Should().Be(0, "poster should NOT be called if no installation exists");
    }

    [Fact]
    public async Task Post_ProviderGitlab_PersistsRowWithProviderGitlab_AndInstallationFk()
    {
        // Seed a GitLabInstallation for this org so the endpoint can resolve it
        OrgId.TryParse(_orgId, out var orgGuid).Should().BeTrue();
        var glInstallId = Guid.NewGuid();

        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.GitLabInstallations.Add(new GitLabInstallation
            {
                Id = glInstallId,
                OrgId = orgGuid,
                ProjectId = 42L,
                ProjectPath = "group/project",
                GitLabBaseUrl = "https://gitlab.com",
                AccessTokenCiphertext = Array.Empty<byte>(),
                AccessTokenKid = "test-kid",
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        _fakeGitLabPoster = FakeGitLabCheckPoster.Posted();
        using var client = CreateAuthorizedClient();

        var body = new { repo = "group/project", pr = 3, state = "success", head_sha = new string('b', 40), provider = "gitlab" };
        var resp = await client.PostAsync("/api/v1/pr-checks", JsonContent(body));

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        _fakeGitLabPoster.CallCount.Should().Be(1, "GitLab poster should be called");

        // Verify the PrCheck row was persisted with Provider="gitlab" and GitLabInstallationId set
        using var readScope = _factory.Services.CreateScope();
        var readDb = readScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await readDb.PrChecks
            .Where(r => r.OrgId == orgGuid && r.Provider == "gitlab")
            .OrderByDescending(r => r.CreatedAt)
            .FirstOrDefaultAsync();

        row.Should().NotBeNull("a pr_checks row with provider=gitlab should be persisted");
        row!.Provider.Should().Be("gitlab");
        row.GitLabInstallationId.Should().Be(glInstallId,
            "the row should reference the seeded gitlab_installations row");
    }
}
