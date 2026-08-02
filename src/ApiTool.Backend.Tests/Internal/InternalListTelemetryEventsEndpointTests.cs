// Tests for GET /api/v1/internal/test-hooks/list-telemetry-events (M18-012).
using System.Net;
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
/// Integration tests for <c>GET /api/v1/internal/test-hooks/list-telemetry-events</c>.
/// Uses the shared <see cref="BackendFactory"/> (InMemory DB) — the endpoint only uses
/// simple EF LINQ queries which the InMemory provider supports.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalListTelemetryEventsEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    public InternalListTelemetryEventsEndpointTests(BackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Returns_200_with_rows_for_known_install_id()
    {
        var installId = Guid.NewGuid();
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Set<TelemetryEvent>().Add(new TelemetryEvent
        {
            Id = Guid.NewGuid(),
            InstallId = installId,
            EventType = "run.completed",
            ReceivedAt = DateTime.UtcNow,
            IdempotencyKey = Guid.NewGuid().ToString(),
        });
        await db.SaveChangesAsync();

        var resp = await _client.GetAsync(
            $"/api/v1/internal/test-hooks/list-telemetry-events?install_id={installId}");

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("count").GetInt32().Should().BeGreaterThanOrEqualTo(1);
        var rows = doc.RootElement.GetProperty("rows");
        rows.GetArrayLength().Should().BeGreaterThanOrEqualTo(1);
        rows[0].GetProperty("event_type").GetString().Should().Be("run.completed");
    }

    [Fact]
    public async Task Returns_count_zero_when_no_rows_for_install_id()
    {
        var unknownId = Guid.NewGuid();
        var resp = await _client.GetAsync(
            $"/api/v1/internal/test-hooks/list-telemetry-events?install_id={unknownId}");

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("count").GetInt32().Should().Be(0);
        doc.RootElement.GetProperty("rows").GetArrayLength().Should().Be(0);
    }

    [Fact]
    public async Task Returns_400_when_install_id_missing()
    {
        var resp = await _client.GetAsync(
            "/api/v1/internal/test-hooks/list-telemetry-events");

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Returns_400_when_install_id_malformed()
    {
        var resp = await _client.GetAsync(
            "/api/v1/internal/test-hooks/list-telemetry-events?install_id=not-a-guid");

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Endpoint_is_registered_in_Testing_environment()
    {
        // The endpoint must be reachable in the Testing environment (not 404).
        var resp = await _client.GetAsync(
            $"/api/v1/internal/test-hooks/list-telemetry-events?install_id={Guid.NewGuid()}");

        resp.StatusCode.Should().NotBe(HttpStatusCode.NotFound,
            because: "list-telemetry-events hook must be registered in Testing environment");
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
        var res = await client.GetAsync(
            $"/api/v1/internal/test-hooks/list-telemetry-events?install_id={Guid.NewGuid()}");
        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "GET /api/v1/internal/test-hooks/list-telemetry-events must not be registered in Production");
    }

    [Fact]
    public async Task Response_rows_carry_install_id_event_type_and_received_at_fields()
    {
        var installId = Guid.NewGuid();
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Set<TelemetryEvent>().Add(new TelemetryEvent
        {
            Id = Guid.NewGuid(),
            InstallId = installId,
            EventType = "telemetry.delete_request",
            ReceivedAt = DateTime.UtcNow,
            IdempotencyKey = Guid.NewGuid().ToString(),
        });
        await db.SaveChangesAsync();

        var resp = await _client.GetAsync(
            $"/api/v1/internal/test-hooks/list-telemetry-events?install_id={installId}");

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var row = doc.RootElement.GetProperty("rows")[0];
        row.GetProperty("install_id").GetString().Should().Be(installId.ToString());
        row.TryGetProperty("event_type", out _).Should().BeTrue();
        row.TryGetProperty("received_at", out _).Should().BeTrue();
    }
}
