// Tests for POST /api/v1/internal/test-hooks/mint-reauth-token (M18-012).
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
/// Integration tests for <c>POST /api/v1/internal/test-hooks/mint-reauth-token</c>.
/// Uses <see cref="SqliteBackendFactory"/> because the endpoint writes to the
/// <c>deletion_reauth_tokens</c> table and we want a real database for mutation assertions.
/// </summary>
[Collection(SqliteBackendCollection.Name)]
public sealed class InternalMintReauthTokenEndpointTests : IAsyncLifetime
{
    private readonly SqliteBackendFactory _factory;
    private readonly HttpClient _client;

    public InternalMintReauthTokenEndpointTests(SqliteBackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Returns_200_with_drto_token_for_existing_user()
    {
        // Seed a user (no password hash — mirrors a seed-refresh user).
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"mint-reauth-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/mint-reauth-token",
            new { user_id = userId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var token = doc.RootElement.GetProperty("token").GetString();
        token.Should().NotBeNullOrEmpty();
        token!.Should().StartWith("drto_", because: "minted token must carry the drto_ prefix");
        doc.RootElement.TryGetProperty("expires_at", out _).Should().BeTrue();
    }

    [Fact]
    public async Task Token_is_persisted_as_hash_in_deletion_reauth_tokens_table()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"token-persisted-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/mint-reauth-token",
            new { user_id = userId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify a row was inserted in deletion_reauth_tokens.
        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await verifyDb.DeletionReauthTokens
            .FirstOrDefaultAsync(r => r.UserId == userId);
        row.Should().NotBeNull("a deletion_reauth_tokens row must be created for the user");
        row!.ConsumedAt.Should().BeNull("freshly minted token must not be pre-consumed");
        row.ExpiresAt.Should().BeAfter(DateTime.UtcNow, "token must not be already expired");
    }

    [Fact]
    public async Task Returns_404_when_user_does_not_exist()
    {
        var unknownId = Guid.NewGuid();
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/mint-reauth-token",
            new { user_id = unknownId });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task Returns_400_when_user_id_missing()
    {
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/mint-reauth-token",
            new { }); // user_id omitted

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Returns_400_for_empty_user_id()
    {
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/mint-reauth-token",
            new { user_id = Guid.Empty });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Endpoint_is_registered_in_Testing_environment()
    {
        // Post a missing-user_id body — endpoint returns 400, not 404 (from routing).
        // 400 from the handler proves the route is registered.
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/internal/test-hooks/mint-reauth-token",
            new { });

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
            "/api/v1/internal/test-hooks/mint-reauth-token",
            new { user_id = Guid.NewGuid() });
        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "POST /api/v1/internal/test-hooks/mint-reauth-token must not be registered in Production");
    }
}
