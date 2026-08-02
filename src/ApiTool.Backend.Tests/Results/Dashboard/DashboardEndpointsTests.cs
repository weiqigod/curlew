using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Results.Dashboard;

/// <summary>HTTP integration tests for GET /results/stats and GET /results/failures (M16-019).</summary>
[Collection(BackendCollection.Name)]
public sealed class DashboardEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;
    private Guid _orgGuid;

    public DashboardEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"dash-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        var slug = $"dash-{_ownerId:N}"[..20];
        var body = new { name = "DashEndpointOrg", slug };
        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
        var hexPart = _orgId.StartsWith("org_", StringComparison.Ordinal)
            ? _orgId["org_".Length..] : _orgId;
        _orgGuid = Guid.ParseExact(hexPart, "N");

        // Upgrade to Team tier so tier-gate passes
        await _factory.SeedSubscriptionAsync(_orgGuid, SubscriptionTier.Team);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    /// <summary>
    /// Creates an HTTP client for a user who is a member of <c>_orgGuid</c> but is
    /// assigned a custom role that omits <c>dashboard.view</c>. Used by RBAC 403 tests.
    /// </summary>
    private async Task<HttpClient> CreateMemberWithoutDashboardViewAsync()
    {
        var memberId = Guid.NewGuid();
        var memberEmail = $"nodash-{memberId:N}@example.com";
        var memberToken = TestTokens.Create(memberId, memberEmail);

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        // Trigger user-row upsert before we insert the member record.
        await memberClient.GetAsync("/api/v1/organizations");

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        // Create a custom role that explicitly excludes dashboard.view.
        var customRoleId = Guid.NewGuid();
        db.OrganizationCustomRoles.Add(new CustomRole
        {
            Id = customRoleId,
            OrgId = _orgGuid,
            Name = $"no-dash-{memberId:N}"[..20],
            PermissionsJson = """["results.view"]""",
            CreatedAt = DateTime.UtcNow,
            CreatedBy = _ownerId,
        });

        // Add the user as a member with the custom role.
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = _orgGuid,
            UserId = memberId,
            Role = OrgRole.Member,
            RoleId = customRoleId,
            JoinedAt = DateTime.UtcNow,
        });

        await db.SaveChangesAsync();

        return memberClient;
    }

    private string StatsUrl(string? window = null) =>
        window is null
            ? $"/api/v1/organizations/{_orgId}/results/stats"
            : $"/api/v1/organizations/{_orgId}/results/stats?window={window}";

    private string FailuresUrl(string? window = null, int? limit = null)
    {
        var qs = new List<string>();
        if (window is not null) qs.Add($"window={window}");
        if (limit is not null)  qs.Add($"limit={limit}");
        var suffix = qs.Count > 0 ? "?" + string.Join("&", qs) : string.Empty;
        return $"/api/v1/organizations/{_orgId}/results/failures{suffix}";
    }

    private async Task SeedResultWithItemAsync(
        DateTime createdAt,
        int passCount = 1,
        int failCount = 0,
        string? method = null,
        string? pathTemplate = null)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var resultId = Guid.NewGuid();
        db.Results.Add(new Result
        {
            Id = resultId,
            OrgId = _orgGuid,
            UploadedBy = _ownerId,
            CollectionName = "dash-test",
            RunAt = createdAt,
            DurationMs = 100,
            PassCount = passCount,
            FailCount = failCount,
            SkippedCount = 0,
            CreatedAt = createdAt,
        });

        if (method is not null && pathTemplate is not null)
        {
            db.ResultItems.Add(new ResultItem
            {
                Id = Guid.NewGuid(),
                ResultId = resultId,
                Ordinal = 0,
                Name = "fail-item",
                Status = ResultStatus.Failed,
                DurationMs = 50,
                Method = method,
                PathTemplate = pathTemplate,
            });
        }

        await db.SaveChangesAsync();
    }

    // ── STATS happy path ──────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_returns_200_with_full_schema_for_team_org()
    {
        await SeedResultWithItemAsync(DateTime.UtcNow.AddDays(-1), passCount: 3, failCount: 1);

        var response = await _client.GetAsync(StatsUrl("30d"));
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var root = doc.RootElement;

        root.TryGetProperty("window", out _).Should().BeTrue();
        root.TryGetProperty("window_start", out _).Should().BeTrue();
        root.TryGetProperty("window_end", out _).Should().BeTrue();
        root.TryGetProperty("totals", out var totals).Should().BeTrue();
        root.TryGetProperty("trend", out _).Should().BeTrue();

        totals.TryGetProperty("runs", out _).Should().BeTrue();
        totals.TryGetProperty("pass_count", out _).Should().BeTrue();
        totals.TryGetProperty("fail_count", out _).Should().BeTrue();
        totals.TryGetProperty("skipped_count", out _).Should().BeTrue();
        totals.TryGetProperty("pass_rate", out _).Should().BeTrue();
        totals.TryGetProperty("avg_duration_ms", out _).Should().BeTrue();
        totals.TryGetProperty("p50_duration_ms", out _).Should().BeTrue();
        totals.TryGetProperty("p95_duration_ms", out _).Should().BeTrue();
    }

    // ── tier gate ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_returns_402_for_free_tier_org()
    {
        // Create a separate free-tier org
        var freeUserId = Guid.NewGuid();
        var freeToken = TestTokens.Create(freeUserId, $"free-{freeUserId:N}@test.com");
        var freeClient = _factory.CreateClient();
        freeClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", freeToken);

        var createResp = await freeClient.PostAsJsonAsync("/api/v1/organizations",
            new { name = "FreeDashOrg", slug = $"freedash-{freeUserId:N}"[..20] });
        createResp.EnsureSuccessStatusCode();
        var idJson = await createResp.Content.ReadAsStringAsync();
        using var idDoc = JsonDocument.Parse(idJson);
        var freeOrgId = idDoc.RootElement.GetProperty("id").GetString()!;

        // No subscription → Free tier
        var response = await freeClient.GetAsync($"/api/v1/organizations/{freeOrgId}/results/stats");
        response.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    // ── window parsing ────────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_omitted_window_uses_30d_default()
    {
        var response = await _client.GetAsync(StatsUrl());
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("window").GetString().Should().Be("30d");
    }

    [Theory]
    [InlineData("14d")]
    [InlineData("1d")]
    [InlineData("seven")]
    public async Task Stats_unsupported_window_returns_400(string w)
    {
        var response = await _client.GetAsync(StatsUrl(w));
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("type").GetString().Should()
            .Contain("unsupported-window");
    }

    // ── zero data ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_returns_200_with_zero_totals_for_empty_org()
    {
        // Create a fresh org with no results
        var userId2 = Guid.NewGuid();
        var token2 = TestTokens.Create(userId2, $"empty-{userId2:N}@test.com");
        var client2 = _factory.CreateClient();
        client2.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token2);

        var createResp = await client2.PostAsJsonAsync("/api/v1/organizations",
            new { name = "EmptyDashOrg", slug = $"emptydash-{userId2:N}"[..20] });
        createResp.EnsureSuccessStatusCode();
        var idJson = await createResp.Content.ReadAsStringAsync();
        using var idDoc = JsonDocument.Parse(idJson);
        var emptyOrgId = idDoc.RootElement.GetProperty("id").GetString()!;
        var hexPart = emptyOrgId.StartsWith("org_", StringComparison.Ordinal)
            ? emptyOrgId["org_".Length..] : emptyOrgId;
        var emptyOrgGuid = Guid.ParseExact(hexPart, "N");
        await _factory.SeedSubscriptionAsync(emptyOrgGuid, SubscriptionTier.Team);

        var response = await client2.GetAsync($"/api/v1/organizations/{emptyOrgId}/results/stats");
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var totals = doc.RootElement.GetProperty("totals");
        totals.GetProperty("runs").GetInt32().Should().Be(0);
        doc.RootElement.GetProperty("trend").GetArrayLength().Should().Be(0);
    }

    // ── unauthenticated ───────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_returns_401_when_unauthenticated()
    {
        var anonClient = _factory.CreateClient();
        var response = await anonClient.GetAsync(StatsUrl());
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    // ── RBAC ──────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_returns_403_for_member_without_dashboard_view_permission()
    {
        var memberClient = await CreateMemberWithoutDashboardViewAsync();
        var response = await memberClient.GetAsync(StatsUrl("30d"));
        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── slug routing ──────────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_accepts_org_slug_in_path()
    {
        // Re-resolve org slug from id by reading the org
        var orgResp = await _client.GetAsync($"/api/v1/organizations/{_orgId}");
        orgResp.EnsureSuccessStatusCode();
        var orgJson = await orgResp.Content.ReadAsStringAsync();
        using var orgDoc = JsonDocument.Parse(orgJson);
        var slug = orgDoc.RootElement.GetProperty("slug").GetString()!;

        var response = await _client.GetAsync($"/api/v1/organizations/{slug}/results/stats");
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.TryGetProperty("window", out _).Should().BeTrue();
    }

    // ── FAILURES happy path ───────────────────────────────────────────────────

    [Fact]
    public async Task Failures_returns_groups_sorted_by_count_desc()
    {
        var t = DateTime.UtcNow.AddDays(-1);
        await SeedResultWithItemAsync(t, failCount: 1, method: "GET", pathTemplate: "/users/{id}");
        await SeedResultWithItemAsync(t, failCount: 1, method: "GET", pathTemplate: "/users/{id}");
        await SeedResultWithItemAsync(t, failCount: 1, method: "POST", pathTemplate: "/orders");

        var response = await _client.GetAsync(FailuresUrl("7d"));
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var root = doc.RootElement;

        // Verify response envelope fields (window, limit)
        root.GetProperty("window").GetString().Should().Be("7d");
        root.GetProperty("limit").GetInt32().Should().BeGreaterThan(0);

        var items = root.GetProperty("items").EnumerateArray().ToList();
        items.Should().NotBeEmpty();

        // First item should have the highest failure_count
        var first = items[0];
        first.GetProperty("method").GetString().Should().Be("GET");
        first.GetProperty("path_template").GetString().Should().Be("/users/{id}");
        first.GetProperty("failure_count").GetInt32().Should().BeGreaterThan(1);

        // Verify all required FailureGroup fields are serialised in the HTTP response (behavior #5)
        first.TryGetProperty("first_seen_at", out _).Should().BeTrue(
            "first_seen_at must be present in the HTTP response per behavior #5");
        first.TryGetProperty("last_seen_at", out _).Should().BeTrue(
            "last_seen_at must be present in the HTTP response per behavior #5");
        first.TryGetProperty("sample_run_ids", out var sampleRunIds).Should().BeTrue(
            "sample_run_ids must be present in the HTTP response per behavior #5");
        sampleRunIds.GetArrayLength().Should().BeGreaterThan(0,
            "at least one sample_run_id should be returned for a group with failures");
    }

    [Fact]
    public async Task Failures_limit_above_50_clamps_and_emits_warning_header()
    {
        var response = await _client.GetAsync(FailuresUrl("7d", limit: 200));
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        // Warning header should be present
        response.Headers.TryGetValues("Warning", out var warnValues).Should().BeTrue();
        warnValues!.Should().Contain(s => s.Contains("limit_clamped"));

        // Body should have limit=50 and limit_clamped=true
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("limit").GetInt32().Should().Be(50);
        doc.RootElement.GetProperty("limit_clamped").GetBoolean().Should().BeTrue();
    }

    [Fact]
    public async Task Failures_returns_402_for_free_tier()
    {
        var freeUserId = Guid.NewGuid();
        var freeToken = TestTokens.Create(freeUserId, $"free2-{freeUserId:N}@test.com");
        var freeClient = _factory.CreateClient();
        freeClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", freeToken);

        var createResp = await freeClient.PostAsJsonAsync("/api/v1/organizations",
            new { name = "FreeFailOrg", slug = $"freefail-{freeUserId:N}"[..20] });
        createResp.EnsureSuccessStatusCode();
        var idJson = await createResp.Content.ReadAsStringAsync();
        using var idDoc = JsonDocument.Parse(idJson);
        var freeOrgId = idDoc.RootElement.GetProperty("id").GetString()!;

        var response = await freeClient.GetAsync($"/api/v1/organizations/{freeOrgId}/results/failures");
        response.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    [Theory]
    [InlineData("14d")]
    [InlineData("seven")]
    public async Task Failures_unsupported_window_returns_400(string w)
    {
        var response = await _client.GetAsync(FailuresUrl(w));
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("type").GetString().Should()
            .Contain("unsupported-window");
    }

    // ── slug routing ──────────────────────────────────────────────────────────

    [Fact]
    public async Task Failures_accepts_org_slug_in_path()
    {
        // Re-resolve org slug from id by reading the org
        var orgResp = await _client.GetAsync($"/api/v1/organizations/{_orgId}");
        orgResp.EnsureSuccessStatusCode();
        var orgJson = await orgResp.Content.ReadAsStringAsync();
        using var orgDoc = JsonDocument.Parse(orgJson);
        var slug = orgDoc.RootElement.GetProperty("slug").GetString()!;

        var response = await _client.GetAsync($"/api/v1/organizations/{slug}/results/failures");
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.TryGetProperty("items", out _).Should().BeTrue();
    }

    [Fact]
    public async Task Failures_returns_401_when_unauthenticated()
    {
        var anonClient = _factory.CreateClient();
        var response = await anonClient.GetAsync(FailuresUrl("7d"));
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Failures_returns_403_for_member_without_dashboard_view_permission()
    {
        var memberClient = await CreateMemberWithoutDashboardViewAsync();
        var response = await memberClient.GetAsync(FailuresUrl("7d"));
        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }
}
