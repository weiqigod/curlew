using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.VaultConfig;
using Microsoft.AspNetCore.Hosting;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.VaultConfig;

/// <summary>HTTP integration tests for the vault-config endpoints (M16-017).</summary>
[Collection(BackendCollection.Name)]
public sealed class VaultConfigEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;
    private Guid _orgGuid;

    private const string ValidYaml = """
        team_secrets:
          provider: aws-secrets-manager
          keys:
            api_key: prod/api-key
        """;

    private const string SuspiciousYaml = """
        team_secrets:
          password: abcdefghij1234567890
        """;

    public VaultConfigEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"vault-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        var slug = $"vault-{_ownerId:N}"[..20];
        var body = new { name = "VaultTestOrg", slug };
        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;

        var hexPart = _orgId.StartsWith("org_", StringComparison.Ordinal)
            ? _orgId["org_".Length..]
            : _orgId;
        _orgGuid = Guid.ParseExact(hexPart, "N");

        // Upgrade to Team tier so tier-gate passes
        await _factory.SeedSubscriptionAsync(_orgGuid, SubscriptionTier.Team);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private string VaultConfigUrl => $"/api/v1/organizations/{_orgId}/vault-config";

    private HttpContent YamlContent(string yaml) =>
        new StringContent(yaml, Encoding.UTF8, "application/yaml");

    private async Task<(HttpClient client, Guid userId)> CreateMemberClientAsync(OrgRole role)
    {
        var userId = Guid.NewGuid();
        var email = $"member-{userId:N}@example.com";
        var token = TestTokens.Create(userId, email);

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);

        // Trigger user upsert by hitting an authenticated endpoint
        await memberClient.GetAsync("/api/v1/organizations");

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = _orgGuid,
            UserId = userId,
            Role = role,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        return (memberClient, userId);
    }

    // ── GET tests ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task Get_returns_404_when_no_row()
    {
        var response = await _client.GetAsync(VaultConfigUrl);
        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task Get_returns_200_with_etag_when_row_exists()
    {
        // First PUT to create a row
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));

        var response = await _client.GetAsync(VaultConfigUrl);
        response.StatusCode.Should().Be(HttpStatusCode.OK);
        response.Headers.ETag.Should().NotBeNull();

        var body = await response.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("version").GetInt64().Should().Be(1);
    }

    [Fact]
    public async Task Get_returns_304_when_if_none_match_matches_version()
    {
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));

        // Get to fetch ETag
        var getResp = await _client.GetAsync(VaultConfigUrl);
        var etag = getResp.Headers.ETag!.Tag;

        // Conditional GET
        var request = new HttpRequestMessage(HttpMethod.Get, VaultConfigUrl);
        request.Headers.IfNoneMatch.ParseAdd(etag);
        var condResp = await _client.SendAsync(request);
        condResp.StatusCode.Should().Be(HttpStatusCode.NotModified);
    }

    [Fact]
    public async Task Get_returns_200_when_if_none_match_does_not_match()
    {
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));

        var request = new HttpRequestMessage(HttpMethod.Get, VaultConfigUrl);
        request.Headers.IfNoneMatch.ParseAdd("\"999\"");
        var response = await _client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Get_returns_402_for_free_tier_org()
    {
        // Create a free-tier org (no subscription upgrade)
        var freeOwnerId = Guid.NewGuid();
        var freeEmail = $"free-{freeOwnerId:N}@example.com";
        var freeToken = TestTokens.Create(freeOwnerId, freeEmail);
        var freeClient = _factory.CreateClient();
        freeClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", freeToken);

        var slug = $"free-{freeOwnerId:N}"[..20];
        var createResp = await freeClient.PostAsJsonAsync("/api/v1/organizations",
            new { name = "FreeOrg", slug });
        createResp.EnsureSuccessStatusCode();
        var json = await createResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var freeOrgId = doc.RootElement.GetProperty("id").GetString()!;

        var response = await freeClient.GetAsync($"/api/v1/organizations/{freeOrgId}/vault-config");
        response.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    [Fact]
    public async Task Get_returns_403_for_non_member()
    {
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));

        var nonMemberId = Guid.NewGuid();
        var nonMemberClient = _factory.CreateClient();
        nonMemberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(nonMemberId, $"{nonMemberId:N}@test.com"));
        await nonMemberClient.GetAsync("/api/v1/organizations"); // trigger upsert

        var response = await nonMemberClient.GetAsync(VaultConfigUrl);
        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── PUT tests ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task Put_with_valid_yaml_returns_200_and_version_1_on_first_save()
    {
        var response = await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await response.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("version").GetInt64().Should().Be(1);
        response.Headers.ETag.Should().NotBeNull();
    }

    [Fact]
    public async Task Put_increments_version_on_subsequent_saves()
    {
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));
        var response = await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));
        var body = await response.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("version").GetInt64().Should().Be(2);
    }

    [Fact]
    public async Task Put_returns_422_problem_detail_with_offending_paths_in_reject_mode()
    {
        var response = await _client.PutAsync(VaultConfigUrl, YamlContent(SuspiciousYaml));
        response.StatusCode.Should().Be(HttpStatusCode.UnprocessableEntity);

        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("vault-template-suspicious-value");
    }

    [Fact]
    public async Task Put_returns_400_for_malformed_yaml()
    {
        var response = await _client.PutAsync(VaultConfigUrl, YamlContent(": invalid {{ yaml"));
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Put_returns_402_for_free_tier_org()
    {
        var freeOwnerId = Guid.NewGuid();
        var freeEmail = $"free-put-{freeOwnerId:N}@example.com";
        var freeClient = _factory.CreateClient();
        freeClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(freeOwnerId, freeEmail));

        var slug = $"freeput-{freeOwnerId:N}"[..20];
        var createResp = await freeClient.PostAsJsonAsync("/api/v1/organizations", new { name = "FreeOrg2", slug });
        createResp.EnsureSuccessStatusCode();
        var json = await createResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var freeOrgId = doc.RootElement.GetProperty("id").GetString()!;

        var response = await freeClient.PutAsync(
            $"/api/v1/organizations/{freeOrgId}/vault-config",
            YamlContent(ValidYaml));
        response.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    [Fact]
    public async Task Put_returns_403_for_member_without_vault_config_manage()
    {
        // OrgRole.Member has vault_config.view but NOT vault_config.manage
        var (memberClient, _) = await CreateMemberClientAsync(OrgRole.Member);
        var response = await memberClient.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));
        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    /// <summary>
    /// Behavior #6: in warn mode, a template containing a literal-secret field is saved
    /// (200 OK) with a non-empty warnings array rather than rejected with 422.
    /// Exercises the endpoint + ValidatorMode=warn configuration together.
    /// Review finding #2.
    /// </summary>
    [Fact]
    public async Task Put_returns_200_with_warnings_in_warn_mode()
    {
        // Create a factory variant that overrides ValidatorMode to "warn".
        await using var warnFactory = _factory.WithWebHostBuilder(builder =>
        {
            builder.ConfigureAppConfiguration((_, cfg) =>
            {
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    [$"{VaultConfigOptions.Section}:ValidatorMode"] = "warn",
                });
            });
        });

        // Initialize the warn-factory's in-memory DB.
        using (var scope = warnFactory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            await db.Database.EnsureCreatedAsync();
        }

        var warnOwnerId = Guid.NewGuid();
        var warnEmail = $"warn-{warnOwnerId:N}@example.com";
        var warnToken = TestTokens.Create(warnOwnerId, warnEmail);
        var warnClient = warnFactory.CreateClient();
        warnClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", warnToken);

        // Create an org in the warn-factory.
        var slug = $"warn-{warnOwnerId:N}"[..20];
        var createResp = await warnClient.PostAsJsonAsync("/api/v1/organizations", new { name = "WarnOrg", slug });
        createResp.EnsureSuccessStatusCode();
        var json = await createResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var warnOrgIdStr = doc.RootElement.GetProperty("id").GetString()!;
        var hexPart = warnOrgIdStr.StartsWith("org_", StringComparison.Ordinal)
            ? warnOrgIdStr["org_".Length..] : warnOrgIdStr;
        var warnOrgGuid = Guid.ParseExact(hexPart, "N");

        // Seed a Team subscription directly.
        using (var scope = warnFactory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(),
                OrgId = warnOrgGuid,
                Tier = SubscriptionTier.Team,
                Status = SubscriptionStatus.Active,
                SeatCount = 1,
                SeatLimit = 25,
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddDays(30),
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        // PUT the suspicious YAML — warn mode should return 200 with warnings.
        var response = await warnClient.PutAsync(
            $"/api/v1/organizations/{warnOrgIdStr}/vault-config",
            YamlContent(SuspiciousYaml));

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await response.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("version").GetInt64().Should().Be(1);
        body.TryGetProperty("warnings", out var warningsEl).Should().BeTrue();
        warningsEl.GetArrayLength().Should().BeGreaterThan(0);
    }

    [Fact]
    public async Task Put_emits_audit_log_entry()
    {
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var entry = db.OrganizationAuditLog
            .FirstOrDefault(e => e.EventType == "vault_config.upserted" && e.OrgId == _orgGuid);
        entry.Should().NotBeNull();
    }

    // ── DELETE tests ──────────────────────────────────────────────────────────

    [Fact]
    public async Task Delete_returns_204_on_success()
    {
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));
        var response = await _client.DeleteAsync(VaultConfigUrl);
        response.StatusCode.Should().Be(HttpStatusCode.NoContent);
    }

    [Fact]
    public async Task Delete_returns_404_when_no_row()
    {
        var response = await _client.DeleteAsync(VaultConfigUrl);
        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task Delete_returns_402_for_free_tier_org()
    {
        var freeOwnerId = Guid.NewGuid();
        var freeClient = _factory.CreateClient();
        freeClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer",
                TestTokens.Create(freeOwnerId, $"freedel-{freeOwnerId:N}@example.com"));

        var slug = $"freedel-{freeOwnerId:N}"[..20];
        var createResp = await freeClient.PostAsJsonAsync("/api/v1/organizations", new { name = "FreeOrg3", slug });
        createResp.EnsureSuccessStatusCode();
        var json = await createResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var freeOrgId = doc.RootElement.GetProperty("id").GetString()!;

        var response = await freeClient.DeleteAsync($"/api/v1/organizations/{freeOrgId}/vault-config");
        response.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    [Fact]
    public async Task Delete_emits_audit_log_entry()
    {
        await _client.PutAsync(VaultConfigUrl, YamlContent(ValidYaml));
        await _client.DeleteAsync(VaultConfigUrl);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var entry = db.OrganizationAuditLog
            .FirstOrDefault(e => e.EventType == "vault_config.deleted" && e.OrgId == _orgGuid);
        entry.Should().NotBeNull();
    }
}
