// Tests for POST /api/v1/internal/test-hooks/backdate-deletion-request (M18-012).
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
/// Integration tests for <c>POST /api/v1/internal/test-hooks/backdate-deletion-request</c>.
/// Uses <see cref="SqliteBackendFactory"/> because the endpoint writes to the Users table
/// and we want a real database for mutation assertions.
/// </summary>
[Collection(SqliteBackendCollection.Name)]
public sealed class InternalBackdateDeletionEndpointTests : IAsyncLifetime
{
    private readonly SqliteBackendFactory _factory;
    private readonly HttpClient _client;

    public InternalBackdateDeletionEndpointTests(SqliteBackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Backdates_pending_deletion_at_for_existing_request()
    {
        // Seed a user with a pending deletion set in the future (not yet eligible for finalizer).
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"backdate-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = DateTime.UtcNow.AddDays(28), // 28 days in future — not yet eligible
        });
        await db.SaveChangesAsync();

        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { user_id = userId, days_ago = 31 });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify pending_deletion_at is now in the past (>= 31 days ago).
        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var user = await verifyDb.Users.FindAsync(userId);
        user.Should().NotBeNull();
        user!.PendingDeletionAt.Should().NotBeNull();
        user.PendingDeletionAt!.Value.Should().BeBefore(DateTime.UtcNow.AddDays(-29),
            because: "pending_deletion_at must be backdated past the 30-day cooldown");
    }

    [Fact]
    public async Task Returns_404_when_user_has_no_pending_deletion()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"no-del-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = null, // no pending deletion
        });
        await db.SaveChangesAsync();

        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { user_id = userId, days_ago = 31 });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task Returns_400_when_user_id_missing()
    {
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { days_ago = 31 }); // user_id omitted

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Returns_400_for_empty_user_id()
    {
        // The implementation guards: body?.UserId is null || body.UserId == Guid.Empty → 400.
        // Mirrors the pattern in InternalSeedNearExpiryTrialEndpointTests.cs.
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { user_id = Guid.Empty, days_ago = 31 });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Default_days_ago_is_31()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"default-days-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = DateTime.UtcNow.AddDays(5),
        });
        await db.SaveChangesAsync();

        // Omit days_ago — should default to 31.
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { user_id = userId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var user = await verifyDb.Users.FindAsync(userId);
        user!.PendingDeletionAt.Should().BeBefore(DateTime.UtcNow.AddDays(-29),
            because: "default days_ago=31 must push pending_deletion_at past the 30-day cooldown");
    }

    [Fact]
    public async Task Endpoint_is_registered_in_Testing_environment()
    {
        // Post a missing-user_id body — the endpoint returns 400, not 405 (MethodNotAllowed)
        // or 404-from-routing (which would indicate the route is not registered). This proves
        // the route is registered: the 400 comes from the handler, not from the routing layer.
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { days_ago = 31 }); // user_id omitted

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest,
            because: "the endpoint must be registered and return 400 for a missing user_id");
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
        var res = await client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { user_id = Guid.NewGuid(), days_ago = 31 });
        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "POST /api/v1/internal/test-hooks/backdate-deletion-request must not be registered in Production");
    }

    [Fact]
    public async Task Response_includes_user_id_and_pending_deletion_at()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"response-shape-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = DateTime.UtcNow.AddDays(5),
        });
        await db.SaveChangesAsync();

        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/backdate-deletion-request",
            new { user_id = userId, days_ago = 31 });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.TryGetProperty("user_id", out _).Should().BeTrue();
        doc.RootElement.TryGetProperty("pending_deletion_at", out _).Should().BeTrue();
    }
}
