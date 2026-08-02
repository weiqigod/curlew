// Tests for POST /api/v1/internal/test-hooks/run-audit-cleanup (M18-002).
// Uses a SQLite-backed custom factory because ExecuteDeleteAsync requires a real SQL provider.
using System.Net;
using System.Security.Cryptography;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.Licensing.Keys;
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
/// Integration test for <c>POST /api/v1/internal/test-hooks/run-audit-cleanup</c>.
/// Seeds an org with 100 rows where 40 are expired, POSTs the hook, and asserts
/// 60 rows remain.
/// </summary>
/// <remarks>
/// Uses a custom SQLite-backed <see cref="WebApplicationFactory{TProgram}"/> because
/// <c>ExecuteDeleteAsync</c> is not supported by the EF Core InMemory provider used
/// by the shared <see cref="BackendFactory"/>. SQLite supports bulk-delete operations.
/// </remarks>
public sealed class InternalAuditCleanupTickEndpointTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly WebApplicationFactory<Program> _factory;
    private readonly HttpClient _client;

    private readonly string _keyDir;
    private readonly string _ghAppPemPath;

    public InternalAuditCleanupTickEndpointTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        var connCapture = _conn;

        _keyDir = Path.Combine(Path.GetTempPath(), $"apitool_audit_test_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_keyDir);
        _ghAppPemPath = Path.Combine(_keyDir, "github-app-test.pem");
        using var rsa = RSA.Create(2048);
        File.WriteAllText(_ghAppPemPath, rsa.ExportRSAPrivateKeyPem());

        _factory = new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseEnvironment("Testing");
                builder.ConfigureAppConfiguration((_, cfg) =>
                    cfg.AddInMemoryCollection(new Dictionary<string, string?>
                    {
                        ["Jwt:SigningKey"]        = BackendFactory.TestSigningKey,
                        ["Jwt:Issuer"]            = BackendFactory.TestIssuer,
                        ["Jwt:Audience"]          = BackendFactory.TestAudience,
                        ["ApiTool:App:WebAppUrl"] = "http://web.test",
                        ["ApiTool:KeyProvider:Mode"] = "file",
                        ["ApiTool:KeyProvider:Env"] = "test",
                        ["ApiTool:KeyProvider:File:Dir"] = _keyDir,
                        ["ApiTool:GitHubApp:AppId"] = "12345",
                        ["ApiTool:GitHubApp:Slug"] = "apitool-checks-test",
                        ["ApiTool:GitHubApp:KeyProvider:Mode"] = "file",
                        ["ApiTool:GitHubApp:KeyProvider:File:Path"] = _ghAppPemPath,
                        ["ApiTool:GitHubApp:StateSigningKey"] = "test-state-signing-key-32chars!!",
                        ["ApiTool:GitLab:KeyProvider:Mode"] = "file",
                    }));
                builder.ConfigureServices(services =>
                {
                    // Replace InMemory with SQLite so ExecuteDeleteAsync works.
                    services.AddDbContext<AppDbContext>(o => o.UseSqlite(connCapture));

                    // Replace key providers with fakes to avoid file-system I/O.
                    services.RemoveAll<IKeyProvider>();
                    services.AddSingleton<FakeKeyProvider>();
                    services.AddSingleton<IKeyProvider>(sp => sp.GetRequiredService<FakeKeyProvider>());

                    services.AddSingleton<FakeGitLabKeyProvider>();
                    services.AddSingleton<IGitLabKeyProvider>(sp => sp.GetRequiredService<FakeGitLabKeyProvider>());
                });
            });

        _client = _factory.CreateClient();

        // Ensure schema
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Database.EnsureCreated();
    }

    [Fact]
    public async Task Internal_hook_runs_cleanup_and_deletes_expired_rows()
    {
        var ownerId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        const int retentionDays = 30;

        using var seedScope = _factory.Services.CreateScope();
        var db = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();

        db.Users.Add(new User
        {
            Id = ownerId,
            Email = $"cleanup-hook-{ownerId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "CleanupHookOrg",
            Slug = $"chk-{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
            AuditLogRetentionDays = retentionDays,
        });

        // 60 rows within the 30-day window
        var cutoff = DateTime.UtcNow.AddDays(-retentionDays);
        for (var i = 0; i < 60; i++)
            db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                ActorId = ownerId,
                EventType = "within.window",
                CreatedAt = cutoff.AddSeconds(i + 1),
                Success = true,
            });

        // 40 rows older than the cutoff
        for (var i = 0; i < 40; i++)
            db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                ActorId = ownerId,
                EventType = "expired",
                CreatedAt = cutoff.AddDays(-1).AddSeconds(-i),
                Success = true,
            });

        await db.SaveChangesAsync();

        // POST to the test hook
        var resp = await _client.PostAsync(
            "/api/v1/internal/test-hooks/run-audit-cleanup", null);
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify only 60 rows remain
        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var remaining = await verifyDb.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgId);
        remaining.Should().Be(60);
    }

    [Fact]
    public async Task Internal_hook_is_registered_in_Testing_environment()
    {
        // Endpoint must be reachable in Testing (not 404).
        var resp = await _client.PostAsync(
            "/api/v1/internal/test-hooks/run-audit-cleanup", null);
        resp.StatusCode.Should().NotBe(HttpStatusCode.NotFound,
            because: "the cleanup hook must be registered in Testing environment");
    }

    public async ValueTask DisposeAsync()
    {
        await _factory.DisposeAsync();
        await _conn.DisposeAsync();
        if (Directory.Exists(_keyDir))
        {
            try { Directory.Delete(_keyDir, recursive: true); } catch { /* best effort */ }
        }
    }
}
