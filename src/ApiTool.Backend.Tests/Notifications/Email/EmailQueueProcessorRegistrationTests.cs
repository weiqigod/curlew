// Tests that EmailQueueProcessor is registered in Development+fake mode but not in Testing.
// Refs M16-021 plan Step 1.
using System.Security.Cryptography;
using ApiTool.Backend.Data;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;
using Microsoft.Extensions.Hosting;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>
/// Wiring tests for <see cref="EmailQueueProcessor"/> registration policy.
/// Verifies that the processor runs in Development+fake mode (so the email audit log
/// receives entries during e2e runs) but is excluded in Testing and Production.
/// </summary>
public sealed class EmailQueueProcessorRegistrationTests
{
    [Fact]
    public void Processor_is_not_registered_in_Testing()
    {
        // BackendFactory uses Environment=Testing; the processor must be absent so
        // RecordingEmailQueue (wired separately) drives test assertions instead.
        using var factory = new BackendFactory();

        var hosted = factory.Services
            .GetServices<IHostedService>()
            .OfType<EmailQueueProcessor>()
            .ToList();

        hosted.Should().BeEmpty(
            because: "EmailQueueProcessor must not run in Testing; RecordingEmailQueue handles assertions");
    }

    [Fact]
    public async Task Processor_is_registered_in_Development_with_fake_mode()
    {
        // Spin up a WebApplicationFactory in Development with sendGridMode == "fake" (the default).
        // The processor should be registered so the audit log fills during docker-stack e2e runs.
        await using var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        var connCapture = conn;

        var tempKeyDir = Path.Combine(Path.GetTempPath(), $"apitool_test_keys_{Guid.NewGuid():N}");
        Directory.CreateDirectory(tempKeyDir);
        var ghPemPath = Path.Combine(tempKeyDir, "github-app-test.pem");
        using (var rsa = RSA.Create(2048))
            File.WriteAllText(ghPemPath, rsa.ExportRSAPrivateKeyPem());

        await using var devFactory = new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseEnvironment("Development");

                builder.ConfigureAppConfiguration((_, cfg) =>
                    cfg.AddInMemoryCollection(new Dictionary<string, string?>
                    {
                        ["Jwt:SigningKey"]                        = "dev-test-signing-key-32-bytes-xxx",
                        ["Jwt:Issuer"]                           = "apitool-dev-test",
                        ["Jwt:Audience"]                         = "apitool-dev-test",
                        ["ApiTool:App:WebAppUrl"]                = "http://localhost:3000",
                        ["ApiTool:GitHub:AppId"]                 = "12345",
                        ["ApiTool:GitHub:AppPemPath"]            = ghPemPath,
                        ["ApiTool:Keys:KeysDirectory"]           = tempKeyDir,
                        ["ApiTool:SendGrid:Mode"]                = "fake",
                    }));

                builder.ConfigureServices(services =>
                {
                    services.RemoveAll<DbContextOptions<AppDbContext>>();
                    services.RemoveAll<AppDbContext>();
                    services.AddDbContext<AppDbContext>(o => o.UseSqlite(connCapture));
                });
            });

        var hosted = devFactory.Services
            .GetServices<IHostedService>()
            .OfType<EmailQueueProcessor>()
            .ToList();

        hosted.Should().HaveCount(1,
            because: "EmailQueueProcessor must run in Development+fake mode so the audit log fills during e2e runs");
    }

    [Fact]
    public async Task Processor_is_not_registered_in_Production_with_fake_mode()
    {
        // In Production with sendGridMode != "live", the processor must NOT be registered
        // to prevent live emails going out without a real sender configured.
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
                        ["ApiTool:SendGrid:Mode"] = "fake",
                    }));

                builder.ConfigureServices(services =>
                {
                    services.RemoveAll<DbContextOptions<AppDbContext>>();
                    services.RemoveAll<AppDbContext>();
                    services.AddDbContext<AppDbContext>(o => o.UseSqlite(connCapture));
                });
            });

        var hosted = prodFactory.Services
            .GetServices<IHostedService>()
            .OfType<EmailQueueProcessor>()
            .ToList();

        hosted.Should().BeEmpty(
            because: "EmailQueueProcessor must not run in Production with fake sendgrid mode");
    }
}
