using System.Net;
using System.Text.Json;
using System.Threading.Channels;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;
using ApiTool.Backend.Data;

namespace ApiTool.Backend.Tests.Internal;

/// <summary>
/// Integration tests for the <c>GET /internal/test/email-audit</c> endpoint (M14-021).
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalEmailAuditEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    public InternalEmailAuditEndpointTests(BackendFactory factory) => _factory = factory;
    public Task InitializeAsync() => _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Get_in_Testing_returns_200_with_recent_list()
    {
        // The BackendFactory runs in Testing environment.
        var client = _factory.CreateClient();
        var res = await client.GetAsync("/internal/test/email-audit");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("entries").GetArrayLength().Should().BeGreaterThanOrEqualTo(0);
    }

    [Fact]
    public async Task Get_returns_billing_receipt_after_send()
    {
        // Arrange: directly record a billing_receipt entry into the log.
        var log = _factory.Services.GetRequiredService<IRecentlySentEmailLog>();
        var msg = new EmailMessage(
            "owner@acme.com",
            "billing_receipt",
            new Dictionary<string, string> { ["amount"] = "9.99", ["invoice_id"] = "inv_test_001" },
            DateTimeOffset.UtcNow);
        log.Record(msg, DateTimeOffset.UtcNow);

        // Act
        var client = _factory.CreateClient();
        var res = await client.GetAsync("/internal/test/email-audit?limit=50");

        // Assert
        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var entries = doc.RootElement.GetProperty("entries");
        var found = entries.EnumerateArray()
            .Any(e => e.GetProperty("template_slug").GetString() == "billing_receipt");
        found.Should().BeTrue("billing_receipt entry should be in the audit log");
    }

    /// <summary>
    /// Asserts that the email-audit endpoint is NOT registered in Production —
    /// exercising the primary security boundary that keeps this test-only surface
    /// out of the production binary.
    /// </summary>
    [Fact]
    public async Task Get_in_Production_returns_404()
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
                        ["Jwt:SigningKey"]              = "prod-test-signing-key-32-bytes-xxx",
                        ["Jwt:Issuer"]                 = "apitool-prod-test",
                        ["Jwt:Audience"]               = "apitool-prod-test",
                        ["ApiTool:App:WebAppUrl"]      = "https://app.apitool.test",
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
        var res = await client.GetAsync("/internal/test/email-audit");

        // Assert: route must not exist in Production.
        res.StatusCode.Should().Be(HttpStatusCode.NotFound,
            because: "GET /internal/test/email-audit must not be registered in Production");
    }
}
