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
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// <see cref="WebApplicationFactory{TEntryPoint}"/> that replaces the SQLite file-based database
/// with an EF Core in-memory database and injects deterministic JWT options.
/// <para>
/// The EF Core InMemory provider is used here for test-host isolation. It does not enforce
/// unique-index constraints; however, the database-level slug uniqueness constraint is verified
/// separately by <c>AppDbContextSchemaTests.Organizations_slug_is_unique</c> which uses a real
/// SQLite in-memory database via <see cref="TestDb"/>. The slug endpoint test covers the endpoint
/// behaviour (status 409 via the <c>AnyAsync</c> pre-check in <see cref="OrganizationService"/>)
/// while the schema test guards against removal of the DB-level uniqueness constraint.
/// </para>
/// </summary>
public sealed class BackendFactory : WebApplicationFactory<Program>
{
    // Unique database name per factory instance — prevents test collection cross-contamination.
    private readonly string _dbName = $"TestDb_{Guid.NewGuid():N}";

    /// <summary>The deterministic signing key used for test tokens.</summary>
    public const string TestSigningKey = "test-signing-key-for-integration-tests-32bytes!";

    /// <summary>The deterministic issuer used for test tokens.</summary>
    public const string TestIssuer = "apitool-test";

    /// <summary>The deterministic audience used for test tokens.</summary>
    public const string TestAudience = "apitool-test";

    private int _initialized;

    // Temp directory for FileKeyProvider PEM files during integration tests.
    private readonly string _keyDir = Path.Combine(Path.GetTempPath(), $"apitool_test_keys_{Guid.NewGuid():N}");

    // Temp PEM path for FileGitHubAppKeyProvider in integration tests.
    private readonly string _ghAppPemPath;

    /// <summary>
    /// Initialises the factory, pre-generating the GitHub App PEM in the key directory.
    /// </summary>
    public BackendFactory()
    {
        Directory.CreateDirectory(_keyDir);
        _ghAppPemPath = Path.Combine(_keyDir, "github-app-test.pem");
        using var rsa = RSA.Create(2048);
        File.WriteAllText(_ghAppPemPath, rsa.ExportRSAPrivateKeyPem());
    }

    /// <inheritdoc/>
    protected override void ConfigureWebHost(IWebHostBuilder builder)
    {
        builder.UseEnvironment("Testing");
        Directory.CreateDirectory(_keyDir);

        builder.ConfigureAppConfiguration((_, cfg) =>
        {
            // Override JWT settings so BackendFactory uses our deterministic test key.
            cfg.AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["Jwt:SigningKey"] = TestSigningKey,
                ["Jwt:Issuer"] = TestIssuer,
                ["Jwt:Audience"] = TestAudience,
                // Key provider — file mode using a temp dir so FileKeyProvider works with InMemory DB.
                ["ApiTool:KeyProvider:Mode"] = "file",
                ["ApiTool:KeyProvider:Env"] = "test",
                ["ApiTool:KeyProvider:File:Dir"] = _keyDir,
                // App options — required to satisfy ValidateOnStart for AppOptions.
                ["ApiTool:App:WebAppUrl"] = "http://web.test",
                // GitHub App options — required for FileGitHubAppKeyProvider in integration tests.
                ["ApiTool:GitHubApp:AppId"] = "12345",
                ["ApiTool:GitHubApp:Slug"] = "apitool-checks-test",
                ["ApiTool:GitHubApp:KeyProvider:Mode"] = "file",
                ["ApiTool:GitHubApp:KeyProvider:File:Path"] = _ghAppPemPath,
                // State signing key for install-url tests (M14-017).
                ["ApiTool:GitHubApp:StateSigningKey"] = "test-state-signing-key-32chars!!",
                // GitLab key provider — FakeGitLabKeyProvider registered below in ConfigureServices
                // so no real KEK file is needed. No GitLab config needed here; options validation
                // is deferred to provider construction time (not ValidateOnStart).
                ["ApiTool:GitLab:KeyProvider:Mode"] = "file",
            });
        });

        builder.ConfigureServices(services =>
        {
            // Program.cs skips AddDbContext when env == "Testing", so we register
            // the in-memory provider here without any conflict.
            services.AddDbContext<AppDbContext>(options =>
                options.UseInMemoryDatabase(_dbName));

            // Replace production Slack/SMTP senders with in-memory fakes.
            services.RemoveAll<ISlackWebhookPoster>();
            services.AddSingleton<FakeSlackWebhookPoster>();
            services.AddSingleton<ISlackWebhookPoster>(sp => sp.GetRequiredService<FakeSlackWebhookPoster>());

            services.RemoveAll<ISmtpSender>();
            services.AddSingleton<FakeSmtpSender>();
            services.AddSingleton<ISmtpSender>(sp => sp.GetRequiredService<FakeSmtpSender>());

            // Zero-delay retries so dispatcher tests complete instantly.
            services.Configure<NotificationsDispatcherOptions>(o =>
                o.RetryDelays = [TimeSpan.Zero, TimeSpan.Zero]);

            // Replace production SamlHandler with a deterministic fake.
            services.AddSingleton<FakeSamlHandler>();
            services.AddSingleton<ISamlHandler>(sp => sp.GetRequiredService<FakeSamlHandler>());

            // Replace production OIDC handler and discovery client with deterministic fakes.
            services.AddSingleton<FakeOidcDiscoveryClient>();
            services.AddSingleton<IOidcDiscoveryClient>(sp => sp.GetRequiredService<FakeOidcDiscoveryClient>());

            services.AddSingleton<FakeOidcHandler>();
            services.AddSingleton<IOidcHandler>(sp => sp.GetRequiredService<FakeOidcHandler>());

            // Replace production ChannelEmailQueue with RecordingEmailQueue so integration
            // tests can assert on enqueued email messages (e.g., account_security_alert).
            services.RemoveAll<IEmailQueue>();
            services.AddSingleton<RecordingEmailQueue>();
            services.AddSingleton<IEmailQueue>(sp => sp.GetRequiredService<RecordingEmailQueue>());

            // Replace FileKeyProvider with FakeKeyProvider so token-issuance tests are
            // isolated from file-system I/O and DB key-rotation state left by other tests
            // (e.g. InternalKeysEndpointsTests seeds a 'next' key that becomes 'current'
            // but has no PEM file, causing FileKeyProvider.SignAsync to fail).
            services.RemoveAll<IKeyProvider>();
            services.AddSingleton<FakeKeyProvider>();
            services.AddSingleton<IKeyProvider>(sp => sp.GetRequiredService<FakeKeyProvider>());

            // Register FakeGitLabKeyProvider so endpoint-level tests don't need a real KEK file.
            services.AddSingleton<FakeGitLabKeyProvider>();
            services.AddSingleton<IGitLabKeyProvider>(sp => sp.GetRequiredService<FakeGitLabKeyProvider>());

            // M18-009: register fake key providers for vault config and schedule env_vars.
            services.AddSingleton<FakeTeamVaultKeyProvider>();
            services.AddSingleton<ITeamVaultKeyProvider>(sp => sp.GetRequiredService<FakeTeamVaultKeyProvider>());
            services.AddSingleton<FakeScheduleEnvKeyProvider>();
            services.AddSingleton<IScheduleEnvKeyProvider>(sp => sp.GetRequiredService<FakeScheduleEnvKeyProvider>());
        });
    }

    /// <summary>
    /// Ensures the in-memory database schema is initialized — idempotent.
    /// Call from <see cref="Xunit.IAsyncLifetime.InitializeAsync"/> in each test class.
    /// </summary>
    public async Task InitializeAsync()
    {
        if (Interlocked.CompareExchange(ref _initialized, 1, 0) != 0)
            return;

        using var scope = Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        // EF Core InMemory does not support migrations; EnsureCreated creates the schema.
        await db.Database.EnsureCreatedAsync();
    }

    /// <summary>
    /// Returns the <see cref="FakeSlackWebhookPoster"/> registered for this factory so
    /// integration tests can assert on recorded Slack calls.
    /// </summary>
    public FakeSlackWebhookPoster? GetFakeSlackPoster() =>
        Services.GetService<FakeSlackWebhookPoster>();

    /// <summary>
    /// Returns the <see cref="FakeSamlHandler"/> registered for this factory so
    /// integration tests can control the SAML validation outcome.
    /// </summary>
    public FakeSamlHandler? GetFakeSamlHandler() =>
        Services.GetService<FakeSamlHandler>();

    /// <summary>
    /// Returns the <see cref="FakeOidcHandler"/> registered for this factory so
    /// integration tests can control the OIDC validation outcome.
    /// </summary>
    public FakeOidcHandler? GetFakeOidcHandler() =>
        Services.GetService<FakeOidcHandler>();

    /// <summary>
    /// Returns the <see cref="FakeOidcDiscoveryClient"/> registered for this factory so
    /// integration tests can script discovery responses or failure modes.
    /// </summary>
    public FakeOidcDiscoveryClient? GetFakeOidcDiscovery() =>
        Services.GetService<FakeOidcDiscoveryClient>();

    /// <summary>
    /// Returns the <see cref="FakeKeyProvider"/> registered for this factory so
    /// integration tests can simulate key rotation mid-flight (Behavior #7).
    /// </summary>
    public FakeKeyProvider? GetFakeKeyProvider() =>
        Services.GetService<FakeKeyProvider>();

    /// <summary>
    /// Ensures the <see cref="User"/> row for the given user id has
    /// <c>EmailVerified = true</c>. Call from <c>InitializeAsync</c> in test
    /// classes that hit checkout or invitation-accept endpoints (M16-003: those
    /// endpoints now gate on email_verified).
    /// A no-op if the user row does not yet exist — in that case the first
    /// authenticated request will upsert it; call this method AFTER the first
    /// request that triggers the upsert.
    /// </summary>
    public async Task SetEmailVerifiedAsync(Guid userId)
    {
        using var scope = Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var user = await db.Users.FindAsync(userId);
        if (user is null) return;
        user.EmailVerified = true;
        await db.SaveChangesAsync();
    }

    /// <summary>
    /// Seeds an active <see cref="Subscription"/> for the given org. Used by SSO
    /// integration tests after they create their fixture org via the public
    /// organisations endpoint (M15-002).
    /// </summary>
    public async Task SeedSubscriptionAsync(Guid orgId, SubscriptionTier tier)
    {
        using var scope = Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = tier,
            Status = SubscriptionStatus.Active,
            SeatCount = 1,
            SeatLimit = 25,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddDays(30),
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
    }

    /// <inheritdoc/>
    protected override void Dispose(bool disposing)
    {
        base.Dispose(disposing);
        if (disposing && Directory.Exists(_keyDir))
        {
            try { Directory.Delete(_keyDir, recursive: true); } catch { /* best effort */ }
        }
    }
}
