using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;


namespace ApiTool.Backend.Tests.Internal.TierGates;

/// <summary>
/// Integration tests for the Testing-only tier-gate probe endpoints.
/// Exercises the end-to-end RFC 7807 response shape via <c>VaultConfigTierGate</c>
/// and the public-flow 404 via the SSO-login probe.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalTierGateProbeEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public InternalTierGateProbeEndpointTests(BackendFactory factory) => _factory = factory;
    public Task InitializeAsync() => _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<Guid> SeedOrgWithTierAsync(SubscriptionTier tier)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"u-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "Probe Org",
            Slug = $"probe-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = tier,
            Status = SubscriptionStatus.Active,
            SeatCount = 1,
            SeatLimit = 25,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddDays(30),
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return orgId;
    }

    [Fact]
    public async Task vault_config_probe_returns_402_with_problem_detail_for_Free_org()
    {
        var orgId = await SeedOrgWithTierAsync(SubscriptionTier.Free);
        var client = _factory.CreateClient();

        var res = await client.PostAsync($"/internal/test/tier-gate-probe/{orgId}/vault-config", null);

        res.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
        res.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");

        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("type").GetString().Should().Contain("tier-ineligible");
        body.GetProperty("code").GetString().Should().Be("vault_config_tier_ineligible");
        body.GetProperty("current_tier").GetString().Should().Be("free");
        body.GetProperty("required_tier").GetString().Should().Be("team");
    }

    [Fact]
    public async Task vault_config_probe_returns_404_for_unknown_org()
    {
        var client = _factory.CreateClient();
        var unknownOrgId = Guid.NewGuid();

        var res = await client.PostAsync($"/internal/test/tier-gate-probe/{unknownOrgId}/vault-config", null);

        res.StatusCode.Should().Be(HttpStatusCode.NotFound);
        res.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");

        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("organization_not_found");
    }

    [Fact]
    public async Task vault_config_probe_returns_200_for_Team_org()
    {
        var orgId = await SeedOrgWithTierAsync(SubscriptionTier.Team);
        var client = _factory.CreateClient();

        var res = await client.PostAsync($"/internal/test/tier-gate-probe/{orgId}/vault-config", null);

        res.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task sso_login_probe_returns_404_with_cache_control_no_store_for_Free_org()
    {
        var orgId = await SeedOrgWithTierAsync(SubscriptionTier.Free);
        var client = _factory.CreateClient();

        var res = await client.GetAsync($"/internal/test/tier-gate-probe/{orgId}/sso-login");

        res.StatusCode.Should().Be(HttpStatusCode.NotFound);
        res.Headers.CacheControl?.NoStore.Should().BeTrue(
            because: "public-flow 404 must include Cache-Control: no-store to prevent caching");
        var body = await res.Content.ReadAsStringAsync();
        body.Should().BeEmpty(because: "public-flow 404 must not leak tier information in the body");
    }

    [Fact]
    public async Task sso_login_probe_returns_404_with_cache_control_no_store_for_unknown_org()
    {
        var unknownOrgId = Guid.NewGuid();
        var client = _factory.CreateClient();

        var res = await client.GetAsync($"/internal/test/tier-gate-probe/{unknownOrgId}/sso-login");

        res.StatusCode.Should().Be(HttpStatusCode.NotFound);
        res.Headers.CacheControl?.NoStore.Should().BeTrue();
        var body = await res.Content.ReadAsStringAsync();
        body.Should().BeEmpty();
    }

    [Fact]
    public async Task sso_login_probe_returns_200_for_Enterprise_org()
    {
        var orgId = await SeedOrgWithTierAsync(SubscriptionTier.Enterprise);
        var client = _factory.CreateClient();

        var res = await client.GetAsync($"/internal/test/tier-gate-probe/{orgId}/sso-login");

        res.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task vault_config_probe_in_Production_returns_404()
    {
        await using var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        var connCapture = conn;

        await using var prodFactory = new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseEnvironment("Production");
                builder.ConfigureAppConfiguration((_, cfg) =>
                    cfg.AddInMemoryCollection(new Dictionary<string, string?>
                    {
                        ["Jwt:SigningKey"]        = "prod-test-signing-key-32-bytes-xxx",
                        ["Jwt:Issuer"]            = "apitool-prod-test",
                        ["Jwt:Audience"]          = "apitool-prod-test",
                        ["ApiTool:App:WebAppUrl"] = "https://app.apitool.test",
                    }));
                builder.ConfigureServices(services =>
                {
                    services.RemoveAll<DbContextOptions<AppDbContext>>();
                    services.RemoveAll<AppDbContext>();
                    services.AddDbContext<AppDbContext>(o => o.UseSqlite(connCapture));
                });
            });

        var client = prodFactory.CreateClient();
        var res = await client.PostAsync(
            $"/internal/test/tier-gate-probe/{Guid.NewGuid()}/vault-config", null);

        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "probe endpoints must not be registered in Production");
    }

    /// <summary>
    /// Behavior 8: a unit test injects a fake <see cref="ITierGate"/> returning Allowed,
    /// when an endpoint test runs, then the test does not require a live <see cref="AppDbContext"/>
    /// (proving the seam decouples endpoints from EF).
    /// </summary>
    [Fact]
    public async Task vault_config_probe_returns_200_with_fake_ITierGate_without_seeding_database()
    {
        // Replace ITierGate in DI with a fake that always returns Allowed.
        // No org data is seeded — the endpoint must succeed using the fake gate alone.
        var client = _factory
            .WithWebHostBuilder(b => b.ConfigureServices(services =>
            {
                services.RemoveAll<ITierGate>();
                services.AddScoped<ITierGate>(_ => new AlwaysAllowedFakeTierGate());
            }))
            .CreateClient();

        var unknownOrgId = Guid.NewGuid(); // No org seeded — gate bypassed by fake.
        var res = await client.PostAsync(
            $"/internal/test/tier-gate-probe/{unknownOrgId}/vault-config", null);

        res.StatusCode.Should().Be(HttpStatusCode.OK,
            because: "the fake ITierGate returns Allowed so the endpoint must succeed " +
                     "without any database rows — proving the DI seam fully decouples the endpoint from EF");
    }

    /// <summary>
    /// Minimal fake <see cref="ITierGate"/> that always returns <see cref="TierGateResult.Allowed"/>.
    /// Used to prove the DI seam decouples endpoint tests from a live <see cref="AppDbContext"/>.
    /// </summary>
    private sealed class AlwaysAllowedFakeTierGate : ITierGate
    {
        public Task<TierGateResult> EnsureAsync(
            Guid orgId,
            SubscriptionTier requiredMinimumTier,
            CancellationToken ct) =>
            Task.FromResult(TierGateResult.Allowed);
    }
}
