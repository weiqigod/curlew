// Tests for POST /internal/test/seed-near-expiry-trial (M16-021 Step 2).
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
/// Integration tests for the <c>POST /internal/test/seed-near-expiry-trial</c> endpoint (M16-021).
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalSeedNearExpiryTrialEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    public InternalSeedNearExpiryTrialEndpointTests(BackendFactory factory) => _factory = factory;
    public Task InitializeAsync() => _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<Guid> SeedUserAsync()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"near-expiry-{userId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return userId;
    }

    [Fact]
    public async Task Inserts_a_trial_row_with_expires_at_in_two_days()
    {
        var userId = await SeedUserAsync();
        var client = _factory.CreateClient();

        var res = await client.PostAsJsonAsync("/internal/test/seed-near-expiry-trial", new
        {
            user_id = userId,
            feature = "shared_vault_templates",
            days_until_expiry = 2,
        });

        res.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify DB row has expires_at approximately 2 days from now.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var trial = await db.Trials.FirstAsync(t => t.UserId == userId && t.Feature == "shared_vault_templates");
        trial.ExpiresAt.Should().BeCloseTo(DateTime.UtcNow.AddDays(2), precision: TimeSpan.FromSeconds(5));
        trial.Notified3DayAt.Should().BeNull(because: "notified_3day_at must be null so TrialExpiryNotifier will process it");
        trial.Kind.Should().Be(TrialKind.FullInitial);
    }

    [Fact]
    public async Task Is_idempotent_for_same_user_feature_pair()
    {
        var userId = await SeedUserAsync();
        var client = _factory.CreateClient();

        var body = new { user_id = userId, feature = "analytics_dashboard", days_until_expiry = 2 };
        // First call inserts.
        await client.PostAsJsonAsync("/internal/test/seed-near-expiry-trial", body);
        // Second call re-seeds (delete + insert).
        var res2 = await client.PostAsJsonAsync("/internal/test/seed-near-expiry-trial", body);
        res2.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var count = await db.Trials.CountAsync(t => t.UserId == userId && t.Feature == "analytics_dashboard");
        count.Should().Be(1, because: "re-seeding must replace the existing row, not duplicate it");
    }

    [Fact]
    public async Task Returns_400_for_empty_user_id()
    {
        var client = _factory.CreateClient();
        var res = await client.PostAsJsonAsync("/internal/test/seed-near-expiry-trial", new
        {
            user_id = Guid.Empty,
            feature = "some_feature",
        });
        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Returns_400_for_empty_feature()
    {
        var userId = await SeedUserAsync();
        var client = _factory.CreateClient();
        var res = await client.PostAsJsonAsync("/internal/test/seed-near-expiry-trial", new
        {
            user_id = userId,
            feature = "",
        });
        res.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Returns_404_in_Production()
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
        var res = await client.PostAsJsonAsync("/internal/test/seed-near-expiry-trial", new
        {
            user_id = Guid.NewGuid(),
            feature = "shared_vault_templates",
        });
        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "POST /internal/test/seed-near-expiry-trial must not be registered in Production");
    }

    [Fact]
    public async Task Response_body_contains_trial_id_and_expires_at()
    {
        var userId = await SeedUserAsync();
        var client = _factory.CreateClient();

        var res = await client.PostAsJsonAsync("/internal/test/seed-near-expiry-trial", new
        {
            user_id = userId,
            feature = "gitlab_integration",
            days_until_expiry = 3,
        });

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.TryGetProperty("trial_id", out _).Should().BeTrue(
            because: "response must contain trial_id");
        doc.RootElement.TryGetProperty("expires_at", out _).Should().BeTrue(
            because: "response must contain expires_at");
    }
}
