using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.Internal;

/// <summary>
/// Integration tests for the <c>POST /internal/test/seed-m14</c> endpoint (M14-021).
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalSeedM14EndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    public InternalSeedM14EndpointTests(BackendFactory factory) => _factory = factory;
    public Task InitializeAsync() => _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<(Guid orgId, Guid userId)> SeedOrgAsync()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"owner-{userId:N}@acme.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "Acme",
            Slug = $"acme-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = SubscriptionTier.Free,
            Status = SubscriptionStatus.Active,
            SeatCount = 1,
            SeatLimit = 1,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddDays(30),
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (orgId, userId);
    }

    [Fact]
    public async Task SeedM14_first_call_upgrades_to_team()
    {
        var (orgId, userId) = await SeedOrgAsync();
        var client = _factory.CreateClient();

        var res = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = orgId,
            installation_id = 12345L,
            app_id = 111L,
            repos = new[] { "acme/api" },
            stripe_customer_id = "cus_test_owner",
        });
        res.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var sub = await db.Subscriptions.FirstAsync(s => s.OrgId == orgId);
        sub.Tier.Should().Be(SubscriptionTier.Team);
        sub.StripeCustomerId.Should().Be("cus_test_owner");
        var owner = await db.Users.SingleAsync(u => u.Id == userId);
        owner.EmailVerified.Should().BeTrue();
    }

    [Fact]
    public async Task SeedM14_without_subscription_creates_team_subscription()
    {
        var (orgId, _) = await SeedOrgAsync();
        await using (var setupScope = _factory.Services.CreateAsyncScope())
        {
            var setupDb = setupScope.ServiceProvider.GetRequiredService<AppDbContext>();
            var existing = await setupDb.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            setupDb.Subscriptions.Remove(existing);
            await setupDb.SaveChangesAsync();
        }

        var client = _factory.CreateClient();
        var res = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = orgId,
            installation_id = 12346L,
            app_id = 111L,
            repos = new[] { "acme/api" },
            stripe_customer_id = "cus_test_created",
        });

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        await using var assertScope = _factory.Services.CreateAsyncScope();
        var db = assertScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var sub = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
        sub.Tier.Should().Be(SubscriptionTier.Team);
        sub.Status.Should().Be(SubscriptionStatus.Active);
        sub.SeatLimit.Should().Be(3);
    }

    [Fact]
    public async Task SeedM14_explicit_enterprise_tier_promotes_subscription()
    {
        var (orgId, _) = await SeedOrgAsync();
        var verifiedUserId = Guid.NewGuid();
        var client = _factory.CreateClient();

        var res = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = orgId,
            installation_id = 12347L,
            app_id = 111L,
            repos = new[] { "acme/api" },
            tier = "enterprise",
            verified_user_id = verifiedUserId,
            verified_user_email = $"qa-{verifiedUserId:N}@acme.com",
        });

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var teamReseed = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = orgId,
            installation_id = 99998L,
            app_id = 111L,
            repos = Array.Empty<string>(),
        });
        teamReseed.StatusCode.Should().Be(HttpStatusCode.OK);

        await using var scope = _factory.Services.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var sub = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
        sub.Tier.Should().Be(SubscriptionTier.Enterprise);
        sub.SeatLimit.Should().Be(5);
        var verifiedUser = await db.Users.SingleAsync(u => u.Id == verifiedUserId);
        verifiedUser.EmailVerified.Should().BeTrue();
    }

    [Fact]
    public async Task SeedM14_second_call_is_noop()
    {
        var (orgId, _) = await SeedOrgAsync();
        var client = _factory.CreateClient();

        var body = new
        {
            org_id = orgId,
            installation_id = 22222L,
            app_id = 111L,
            repos = new[] { "acme/api" },
            stripe_customer_id = "cus_test_noop",
        };

        await client.PostAsJsonAsync("/internal/test/seed-m14", body);
        var res2 = await client.PostAsJsonAsync("/internal/test/seed-m14", body);
        res2.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var rowCount = await db.GithubInstallations
            .CountAsync(gi => gi.InstallationId == 22222L && gi.OrgId == orgId);
        rowCount.Should().Be(1);
    }

    [Fact]
    public async Task SeedM14_reseed_with_different_installation_id_keeps_one_org_row()
    {
        var (orgId, _) = await SeedOrgAsync();
        var client = _factory.CreateClient();

        var first = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = orgId,
            installation_id = 22223L,
            app_id = 111L,
            repos = new[] { "acme/api" },
        });
        first.StatusCode.Should().Be(HttpStatusCode.OK);

        var second = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = orgId,
            installation_id = 99999L,
            app_id = 222L,
            repos = new[] { "acme/web" },
        });
        second.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var rows = await db.GithubInstallations.Where(gi => gi.OrgId == orgId).ToListAsync();
        rows.Should().ContainSingle();
        rows[0].InstallationId.Should().Be(22223L);
        rows[0].AppId.Should().Be(222L);
        rows[0].RepoSetJson.Should().Contain("acme/web");
    }

    [Fact]
    public async Task SeedM14_inserts_github_installations_row()
    {
        var (orgId, _) = await SeedOrgAsync();
        var client = _factory.CreateClient();

        var res = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = orgId,
            installation_id = 33333L,
            app_id = 111L,
            repos = new[] { "acme/api", "acme/web" },
            stripe_customer_id = "cus_test_install",
        });
        res.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var install = await db.GithubInstallations
            .FirstAsync(gi => gi.InstallationId == 33333L);
        install.OrgId.Should().Be(orgId);
        install.AppId.Should().Be(111L);
        install.RepoSetJson.Should().Contain("acme/api");
        install.ClaimedAt.Should().NotBeNull();
    }

    /// <summary>
    /// Asserts that the seed-m14 endpoint is NOT registered in Production —
    /// exercising the primary security boundary that keeps this test-only surface
    /// out of the production binary.
    /// </summary>
    [Fact]
    public async Task SeedM14_in_Production_returns_404()
    {
        // Arrange: spin up an isolated factory in Production environment.
        // Use an in-memory SQLite connection so no real database is needed;
        // migrations are skipped when BACKEND_RUN_MIGRATIONS is not set.
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
                        ["Jwt:SigningKey"]         = "prod-test-signing-key-32-bytes-xxx",
                        ["Jwt:Issuer"]             = "apitool-prod-test",
                        ["Jwt:Audience"]           = "apitool-prod-test",
                        ["ApiTool:App:WebAppUrl"]  = "https://app.apitool.test",
                    }));

                builder.ConfigureServices(services =>
                {
                    // Override DB with in-memory SQLite — avoids needing PostgreSQL.
                    services.RemoveAll<DbContextOptions<AppDbContext>>();
                    services.RemoveAll<AppDbContext>();
                    services.AddDbContext<AppDbContext>(o => o.UseSqlite(connCapture));
                });
            });

        var client = prodFactory.CreateClient();

        // Act
        var res = await client.PostAsJsonAsync("/internal/test/seed-m14", new
        {
            org_id = Guid.NewGuid(),
            installation_id = 99999L,
            app_id = 111L,
            repos = new[] { "acme/api" },
        });

        // Assert: route must not exist in Production.
        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "POST /internal/test/seed-m14 must not be registered in Production");
    }
}
