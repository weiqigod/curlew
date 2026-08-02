using System.Net;
using System.Security.Cryptography;
using System.Text;
using System.Threading.RateLimiting;
using Microsoft.AspNetCore.Http;
using ApiTool.Backend.Data;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.Sso;
using ApiTool.Backend.Sso;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.Tests.Notifications;
using ApiTool.Backend.Tests.Licensing.Keys;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Notifications;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.AspNetCore.RateLimiting;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.Telemetry;

/// <summary>
/// Functional test for Behavior #4: per-<c>install_id</c> rate limiting (429).
/// <para>
/// This test class uses a dedicated <see cref="TelemetryRateLimitFactory"/> that
/// replaces the Testing-environment no-op <c>telemetry-ingest</c> policy with a
/// real fixed-window limiter (3 requests/min) so we can trigger a 429 without
/// sending 61 requests.  All other Testing-environment behaviour (in-memory DB,
/// no background hosts, etc.) is preserved.
/// </para>
/// </summary>
public sealed class TelemetryRateLimitTests : IClassFixture<TelemetryRateLimitFactory>
{
    private const string Path = "/api/v1/telemetry/events";

    private readonly TelemetryRateLimitFactory _factory;

    public TelemetryRateLimitTests(TelemetryRateLimitFactory factory)
    {
        _factory = factory;
    }

    private static HttpRequestMessage BuildRequest(string? installId = null, string? idempotencyKey = null)
    {
        var id = installId ?? Guid.NewGuid().ToString();
        var key = idempotencyKey ?? Guid.NewGuid().ToString();
        var body = $$"""
        {
          "install_id": "{{id}}",
          "event_type": "run.completed",
          "event_payload": { "collection_size": 1 }
        }
        """;
        var req = new HttpRequestMessage(HttpMethod.Post, Path)
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        req.Headers.Add("Idempotency-Key", key);
        return req;
    }

    [Fact]
    public async Task POST_4th_request_with_same_install_id_returns_429()
    {
        // The TelemetryRateLimitFactory registers a tight policy: 3 requests/min per install_id.
        // Send 3 successful requests, then assert the 4th is rate-limited.
        await _factory.InitializeAsync();

        var client = _factory.CreateClient();
        var installId = Guid.NewGuid().ToString();

        // Requests 1-3: should all succeed (202)
        for (var i = 1; i <= 3; i++)
        {
            using var req = BuildRequest(installId: installId);
            var resp = await client.SendAsync(req);
            resp.StatusCode.Should().Be(HttpStatusCode.Accepted,
                because: $"request #{i} should be accepted (within rate limit of 3/min)");
        }

        // Request 4: should be rate-limited (429)
        using var overLimitReq = BuildRequest(installId: installId);
        var overLimitResp = await client.SendAsync(overLimitReq);
        overLimitResp.StatusCode.Should().Be(HttpStatusCode.TooManyRequests,
            because: "the 4th request from the same install_id within the window must return 429 per behavior #4");
    }

    [Fact]
    public async Task POST_rate_limit_is_per_install_id_not_global()
    {
        // Different install_ids should have independent buckets.
        await _factory.InitializeAsync();

        var client = _factory.CreateClient();

        // Exhaust the bucket for install_id A (3 requests).
        var installIdA = Guid.NewGuid().ToString();
        for (var i = 0; i < 3; i++)
        {
            using var req = BuildRequest(installId: installIdA);
            var resp = await client.SendAsync(req);
            resp.StatusCode.Should().Be(HttpStatusCode.Accepted,
                because: $"install_id A request #{i + 1} should succeed");
        }

        // install_id A's bucket is exhausted — 4th request returns 429.
        using var aOverLimit = BuildRequest(installId: installIdA);
        var aResp = await client.SendAsync(aOverLimit);
        aResp.StatusCode.Should().Be(HttpStatusCode.TooManyRequests,
            because: "install_id A is now rate-limited");

        // install_id B's bucket is independent and should still accept requests.
        var installIdB = Guid.NewGuid().ToString();
        using var bReq = BuildRequest(installId: installIdB);
        var bResp = await client.SendAsync(bReq);
        bResp.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "install_id B is a separate partition and must not be affected by install_id A's limit");
    }
}

/// <summary>
/// <see cref="WebApplicationFactory{TEntryPoint}"/> variant that configures the
/// Testing environment (in-memory DB, fake services, no background hosts) but
/// replaces the no-op <c>telemetry-ingest</c> rate-limit policy with a real
/// fixed-window limiter (3 req/min) so <see cref="TelemetryRateLimitTests"/> can
/// trigger a 429 with only 4 HTTP requests.
/// </summary>
public sealed class TelemetryRateLimitFactory : WebApplicationFactory<Program>
{
    private readonly string _dbName = $"TelemetryRateLimitDb_{Guid.NewGuid():N}";
    private readonly string _keyDir = System.IO.Path.Combine(System.IO.Path.GetTempPath(), $"apitool_ratelimit_keys_{Guid.NewGuid():N}");
    private readonly string _ghAppPemPath;
    private int _initialized;

    /// <summary>Initialises the factory, pre-generating the GitHub App PEM.</summary>
    public TelemetryRateLimitFactory()
    {
        System.IO.Directory.CreateDirectory(_keyDir);
        _ghAppPemPath = System.IO.Path.Combine(_keyDir, "github-app-test.pem");
        using var rsa = RSA.Create(2048);
        System.IO.File.WriteAllText(_ghAppPemPath, rsa.ExportRSAPrivateKeyPem());
    }

    /// <inheritdoc/>
    protected override void ConfigureWebHost(IWebHostBuilder builder)
    {
        builder.UseEnvironment("Testing");
        System.IO.Directory.CreateDirectory(_keyDir);

        builder.ConfigureAppConfiguration((_, cfg) =>
        {
            cfg.AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["Jwt:SigningKey"] = BackendFactory.TestSigningKey,
                ["Jwt:Issuer"] = BackendFactory.TestIssuer,
                ["Jwt:Audience"] = BackendFactory.TestAudience,
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
            // In-memory DB.
            services.AddDbContext<AppDbContext>(options =>
                options.UseInMemoryDatabase(_dbName));

            // Fake services required by Program.cs to not blow up in Testing mode.
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

            // Replace the no-op telemetry-ingest policy with a real tight limiter (3/min).
            // RateLimiterOptions.AddPolicy throws if a policy with the same name already exists.
            // The internal PolicyMap is not publicly accessible in .NET 9, so we use reflection
            // to remove the no-op entry before re-registering with the real policy.
            services.PostConfigure<RateLimiterOptions>(options =>
            {
                // RateLimiterOptions stores policies in an internal Dictionary<string, ...>.
                // Reflection is acceptable here because this is test-only infrastructure.
                var policyMapProp = options.GetType()
                    .GetProperty("PolicyMap",
                        System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance);
                if (policyMapProp?.GetValue(options) is System.Collections.IDictionary policyMap)
                    policyMap.Remove("telemetry-ingest");

                // Ensure rejection returns 429, not the default 503.
                options.RejectionStatusCode = StatusCodes.Status429TooManyRequests;

                options.AddPolicy("telemetry-ingest", httpContext =>
                    RateLimitPartition.GetFixedWindowLimiter(
                        partitionKey: httpContext.Items.TryGetValue("telemetry.install_id", out var id)
                            ? id?.ToString() ?? "unknown"
                            : "unknown",
                        factory: _ => new FixedWindowRateLimiterOptions
                        {
                            PermitLimit = 3,
                            Window = TimeSpan.FromMinutes(1),
                            QueueLimit = 0,
                        }));
            });
        });
    }

    /// <summary>Ensures the in-memory DB schema is created. Idempotent.</summary>
    public async Task InitializeAsync()
    {
        if (System.Threading.Interlocked.CompareExchange(ref _initialized, 1, 0) != 0)
            return;

        using var scope = Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        await db.Database.EnsureCreatedAsync();
    }

    /// <inheritdoc/>
    protected override void Dispose(bool disposing)
    {
        base.Dispose(disposing);
        if (disposing && System.IO.Directory.Exists(_keyDir))
        {
            try { System.IO.Directory.Delete(_keyDir, recursive: true); } catch { /* best effort */ }
        }
    }
}
