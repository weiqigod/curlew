// Tests for POST /internal/test/trial-expiry-tick (M16-021 Step 2).
// The endpoint resolves TrialExpiryNotifier from DI and fires one tick.
using System.Net;
using System.Net.Http.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;
using Microsoft.Extensions.Hosting;

namespace ApiTool.Backend.Tests.Internal;

/// <summary>
/// Integration tests for the <c>POST /internal/test/trial-expiry-tick</c> endpoint (M16-021).
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalTrialTickEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    public InternalTrialTickEndpointTests(BackendFactory factory) => _factory = factory;
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
            Email = $"trial-tick-{userId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return userId;
    }

    [Fact]
    public async Task Returns_not_404_when_registered_in_Testing()
    {
        // The endpoint is registered in both Development and Testing environments.
        // In Testing, TrialExpiryNotifier is NOT wired (per existing wiring tests), so
        // the endpoint returns 503 rather than 200 — but it must be reachable (not 404).
        var client = _factory.CreateClient();
        var res = await client.PostAsync("/internal/test/trial-expiry-tick", null);

        res.StatusCode.Should().NotBe(HttpStatusCode.NotFound,
            because: "the trial-expiry-tick endpoint must be registered in Testing environment");
    }

    [Fact]
    public async Task Returns_503_when_notifier_not_registered_in_Testing()
    {
        // TrialExpiryNotifier is explicitly excluded from Testing by Program.cs.
        // Endpoint must gracefully return 503 rather than throw.
        var client = _factory.CreateClient();
        var res = await client.PostAsync("/internal/test/trial-expiry-tick", null);
        res.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable,
            because: "TrialExpiryNotifier is not registered in Testing environment");
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
        var res = await client.PostAsync("/internal/test/trial-expiry-tick", null);
        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "POST /internal/test/trial-expiry-tick must not be registered in Production");
    }
}
