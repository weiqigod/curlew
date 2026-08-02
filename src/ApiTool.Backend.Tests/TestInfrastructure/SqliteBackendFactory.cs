using System.Security.Cryptography;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Schedules.Keys;
using ApiTool.Backend.Sso;
using ApiTool.Backend.Tests.Licensing.Keys;
using ApiTool.Backend.Tests.Notifications;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.Sso;
using ApiTool.Backend.VaultConfig.Keys;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// Variant of <see cref="BackendFactory"/> that uses a real SQLite in-memory database
/// (via a kept-open <see cref="SqliteConnection"/>) rather than the EF Core InMemory
/// provider. Required for tests that exercise code paths using
/// <c>ExecuteUpdateAsync</c> / <c>ExecuteDeleteAsync</c> (e.g.
/// <see cref="ApiTool.Backend.Compliance.Gdpr.UserAnonymiser"/>), which are not supported
/// by the EF Core InMemory provider.
/// </summary>
public sealed class SqliteBackendFactory : WebApplicationFactory<Program>
{
    // Same constants as BackendFactory for token compatibility.
    public const string TestSigningKey = BackendFactory.TestSigningKey;
    public const string TestIssuer = BackendFactory.TestIssuer;
    public const string TestAudience = BackendFactory.TestAudience;

    // Kept open for the lifetime of the factory so the in-memory database persists.
    private readonly SqliteConnection _conn;
    private readonly string _keyDir = Path.Combine(Path.GetTempPath(), $"apitool_sqlitetest_keys_{Guid.NewGuid():N}");
    private readonly string _ghAppPemPath;
    private int _initialized;

    /// <summary>Initialises the factory with a fresh SQLite in-memory connection.</summary>
    public SqliteBackendFactory()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        Directory.CreateDirectory(_keyDir);
        _ghAppPemPath = Path.Combine(_keyDir, "github-app-test.pem");
        using var rsa = RSA.Create(2048);
        File.WriteAllText(_ghAppPemPath, rsa.ExportRSAPrivateKeyPem());
    }

    /// <inheritdoc/>
    protected override void ConfigureWebHost(IWebHostBuilder builder)
    {
        builder.UseEnvironment("Testing");

        builder.ConfigureAppConfiguration((_, cfg) =>
        {
            cfg.AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["Jwt:SigningKey"] = TestSigningKey,
                ["Jwt:Issuer"] = TestIssuer,
                ["Jwt:Audience"] = TestAudience,
                ["ApiTool:KeyProvider:Mode"] = "file",
                ["ApiTool:KeyProvider:Env"] = "test",
                ["ApiTool:KeyProvider:File:Dir"] = _keyDir,
                ["ApiTool:App:WebAppUrl"] = "http://web.test",
                ["ApiTool:GitHubApp:AppId"] = "12345",
                ["ApiTool:GitHubApp:Slug"] = "apitool-checks-test",
                ["ApiTool:GitHubApp:KeyProvider:Mode"] = "file",
                ["ApiTool:GitHubApp:KeyProvider:File:Path"] = _ghAppPemPath,
                ["ApiTool:GitHubApp:StateSigningKey"] = "test-state-signing-key-32chars!!",
                ["ApiTool:GitLab:KeyProvider:Mode"] = "file",
            });
        });

        builder.ConfigureServices(services =>
        {
            // Use the shared SQLite in-memory connection.
            services.AddDbContext<AppDbContext>(options =>
                options.UseSqlite(_conn));

            services.RemoveAll<ISlackWebhookPoster>();
            services.AddSingleton<FakeSlackWebhookPoster>();
            services.AddSingleton<ISlackWebhookPoster>(sp => sp.GetRequiredService<FakeSlackWebhookPoster>());

            services.RemoveAll<ISmtpSender>();
            services.AddSingleton<FakeSmtpSender>();
            services.AddSingleton<ISmtpSender>(sp => sp.GetRequiredService<FakeSmtpSender>());

            services.Configure<NotificationsDispatcherOptions>(o =>
                o.RetryDelays = [TimeSpan.Zero, TimeSpan.Zero]);

            services.AddSingleton<FakeSamlHandler>();
            services.AddSingleton<ISamlHandler>(sp => sp.GetRequiredService<FakeSamlHandler>());

            services.AddSingleton<FakeOidcDiscoveryClient>();
            services.AddSingleton<IOidcDiscoveryClient>(sp => sp.GetRequiredService<FakeOidcDiscoveryClient>());

            services.AddSingleton<FakeOidcHandler>();
            services.AddSingleton<IOidcHandler>(sp => sp.GetRequiredService<FakeOidcHandler>());

            services.RemoveAll<IEmailQueue>();
            services.AddSingleton<RecordingEmailQueue>();
            services.AddSingleton<IEmailQueue>(sp => sp.GetRequiredService<RecordingEmailQueue>());

            services.RemoveAll<IKeyProvider>();
            services.AddSingleton<FakeKeyProvider>();
            services.AddSingleton<IKeyProvider>(sp => sp.GetRequiredService<FakeKeyProvider>());

            services.AddSingleton<FakeGitLabKeyProvider>();
            services.AddSingleton<IGitLabKeyProvider>(sp => sp.GetRequiredService<FakeGitLabKeyProvider>());

            // M18-009: register fake key providers for vault config and schedule env_vars.
            services.AddSingleton<FakeTeamVaultKeyProvider>();
            services.AddSingleton<ITeamVaultKeyProvider>(sp => sp.GetRequiredService<FakeTeamVaultKeyProvider>());
            services.AddSingleton<FakeScheduleEnvKeyProvider>();
            services.AddSingleton<IScheduleEnvKeyProvider>(sp => sp.GetRequiredService<FakeScheduleEnvKeyProvider>());
        });
    }

    /// <summary>Ensures the SQLite database schema is initialised — idempotent.</summary>
    public async Task InitializeAsync()
    {
        if (Interlocked.CompareExchange(ref _initialized, 1, 0) != 0)
            return;

        using var scope = Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        await db.Database.EnsureCreatedAsync();
    }

    /// <summary>Returns the <see cref="RecordingEmailQueue"/> for asserting on email messages.</summary>
    public RecordingEmailQueue GetEmailQueue() =>
        Services.GetRequiredService<RecordingEmailQueue>();

    /// <inheritdoc/>
    protected override void Dispose(bool disposing)
    {
        base.Dispose(disposing);
        if (disposing)
        {
            _conn.Dispose();
            if (Directory.Exists(_keyDir))
            {
                try { Directory.Delete(_keyDir, recursive: true); } catch { /* best effort */ }
            }
        }
    }
}

/// <summary>
/// xUnit collection fixture for tests that require a SQLite-backed
/// <see cref="SqliteBackendFactory"/>.
/// </summary>
[CollectionDefinition(Name)]
public sealed class SqliteBackendCollection : ICollectionFixture<SqliteBackendFactory>
{
    /// <summary>Collection name used in <see cref="CollectionAttribute"/>.</summary>
    public const string Name = "SqliteBackend";
}
