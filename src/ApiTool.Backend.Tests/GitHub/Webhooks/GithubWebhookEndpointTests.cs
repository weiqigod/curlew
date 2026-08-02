// Refs docs/SPECIFICATION.md:8551-8597 (signature verification, idempotency, quarantine).
using System.Net;
using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.Data;
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// HTTP-level tests for POST /webhooks/github covering signature verification,
/// idempotency, quarantine, and duplicate handling.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class GithubWebhookEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public GithubWebhookEndpointTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private const string Secret = "whsec_test";

    private HttpClient CreateClient(
        string secrets = Secret,
        Action<IServiceCollection>? configureServices = null)
    {
        return _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureAppConfiguration((_, cfg) =>
            {
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["ApiTool:GitHub:Webhook:Secrets"] = secrets,
                });
            });
            b.ConfigureServices(services =>
            {
                // Default: replace the real dispatcher with the noop so ingest tests
                // don't depend on installation DB state. Tests that want a custom dispatcher
                // pass configureServices which overrides this.
                services.RemoveAll<IGithubWebhookDispatcher>();
                services.AddSingleton<IGithubWebhookDispatcher, NoopGithubWebhookDispatcher>();
                configureServices?.Invoke(services);
            });
        }).CreateClient();
    }

    private static (Guid deliveryId, string body, string sigHeader) BuildSignedDelivery(
        string? body = null, string secret = Secret)
    {
        var deliveryId = Guid.NewGuid();
        body ??= """{"action":"created","installation":{"id":12345,"app_id":111},"account":{"login":"acme","type":"Organization"},"repository_selection":"selected","repositories":[]}""";
        var rawBody = Encoding.UTF8.GetBytes(body);
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(secret));
        var hash = hmac.ComputeHash(rawBody);
        var sigHeader = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";
        return (deliveryId, body, sigHeader);
    }

    [Fact]
    public async Task Valid_signature_returns_200()
    {
        var client = CreateClient();
        var (deliveryId, body, sig) = BuildSignedDelivery();

        var request = BuildRequest(deliveryId, body, sig, "installation");
        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Invalid_signature_returns_401_no_row_inserted()
    {
        var client = CreateClient();
        var body = """{"action":"created"}""";
        var deliveryId = Guid.NewGuid();

        var request = BuildRequest(deliveryId, body, "sha256=" + new string('a', 64), "installation");
        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GithubWebhookEvents.FirstOrDefaultAsync(x => x.DeliveryId == deliveryId);
        row.Should().BeNull("signature rejection must not insert a row");
    }

    [Fact]
    public async Task Missing_signature_header_returns_401()
    {
        var client = CreateClient();
        var body = """{"action":"created"}""";
        var deliveryId = Guid.NewGuid();

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/github")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-GitHub-Delivery", deliveryId.ToString());
        request.Headers.Add("X-GitHub-Event", "installation");
        // No X-Hub-Signature-256

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Sha1_only_legacy_header_returns_401()
    {
        var client = CreateClient();
        var body = """{"action":"created"}""";
        var deliveryId = Guid.NewGuid();

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/github")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-GitHub-Delivery", deliveryId.ToString());
        request.Headers.Add("X-GitHub-Event", "installation");
        // Only legacy SHA-1 header, no SHA-256
        request.Headers.Add("X-Hub-Signature", "sha1=deadbeef");

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Multi_secret_rotation_old_and_new_both_accepted()
    {
        const string secrets = "whsec_old,whsec_new";
        var clientOld = CreateClient(secrets);
        var clientNew = CreateClient(secrets);

        var (deliveryOld, bodyOld, sigOld) = BuildSignedDelivery(secret: "whsec_old");
        var reqOld = BuildRequest(deliveryOld, bodyOld, sigOld, "installation");
        var respOld = await clientOld.SendAsync(reqOld);
        respOld.StatusCode.Should().Be(HttpStatusCode.OK);

        var (deliveryNew, bodyNew, sigNew) = BuildSignedDelivery(secret: "whsec_new");
        var reqNew = BuildRequest(deliveryNew, bodyNew, sigNew, "installation");
        var respNew = await clientNew.SendAsync(reqNew);
        respNew.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Duplicate_delivery_id_returns_200_no_second_row()
    {
        var client = CreateClient();
        var (deliveryId, body, sig) = BuildSignedDelivery();

        // First delivery
        var req1 = BuildRequest(deliveryId, body, sig, "installation");
        var resp1 = await client.SendAsync(req1);
        resp1.StatusCode.Should().Be(HttpStatusCode.OK);

        // Second delivery (same delivery_id — idempotent)
        var req2 = BuildRequest(deliveryId, body, sig, "installation");
        var resp2 = await client.SendAsync(req2);
        resp2.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Handler_exception_records_failure_returns_500()
    {
        var throwingDispatcher = new ThrowingGithubWebhookDispatcher(throwUntilAttempt: 1);
        var client = CreateClient(configureServices: services =>
        {
            services.RemoveAll<IGithubWebhookDispatcher>();
            services.AddSingleton<IGithubWebhookDispatcher>(throwingDispatcher);
        });

        var (deliveryId, body, sig) = BuildSignedDelivery();
        var request = BuildRequest(deliveryId, body, sig, "installation");
        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.InternalServerError);
    }

    [Fact]
    public async Task Fifth_handler_failure_quarantines_returns_200()
    {
        var throwingDispatcher = new ThrowingGithubWebhookDispatcher(throwUntilAttempt: int.MaxValue);
        var quarantineClient = _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureAppConfiguration((_, cfg) =>
            {
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["ApiTool:GitHub:Webhook:Secrets"] = Secret,
                });
            });
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IGithubWebhookDispatcher>();
                services.AddSingleton<IGithubWebhookDispatcher>(throwingDispatcher);
            });
        }).CreateClient();

        var (deliveryId, body, sig) = BuildSignedDelivery();
        HttpStatusCode? lastStatus = null;
        for (var attempt = 0; attempt < GithubWebhookStore.QuarantineThreshold; attempt++)
        {
            var request = BuildRequest(deliveryId, body, sig, "installation");
            var response = await quarantineClient.SendAsync(request);
            lastStatus = response.StatusCode;
        }

        // 5th attempt should quarantine and return 200 (break retry storm)
        lastStatus.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GithubWebhookEvents.FirstOrDefaultAsync(x => x.DeliveryId == deliveryId);
        row.Should().NotBeNull();
        row!.Status.Should().Be("quarantined");
        row.AttemptCount.Should().Be(GithubWebhookStore.QuarantineThreshold);
    }

    [Fact]
    public async Task Successful_processing_marks_status_processed()
    {
        var client = CreateClient();
        var (deliveryId, body, sig) = BuildSignedDelivery();
        var request = BuildRequest(deliveryId, body, sig, "installation");

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GithubWebhookEvents.FirstOrDefaultAsync(x => x.DeliveryId == deliveryId);
        row.Should().NotBeNull();
        row!.Status.Should().Be("processed");
        row.ProcessedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task Malformed_delivery_id_returns_400()
    {
        var client = CreateClient();
        var body = """{"action":"created"}""";
        var rawBody = Encoding.UTF8.GetBytes(body);
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(Secret));
        var hash = hmac.ComputeHash(rawBody);
        var sig = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/github")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Hub-Signature-256", sig);
        request.Headers.Add("X-GitHub-Delivery", "not-a-guid");
        request.Headers.Add("X-GitHub-Event", "installation");

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task No_secrets_configured_returns_401()
    {
        var client = CreateClient(secrets: "");
        var (deliveryId, body, sig) = BuildSignedDelivery();
        var request = BuildRequest(deliveryId, body, sig, "installation");

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Invalid_json_after_valid_signature_returns_400()
    {
        var client = CreateClient();
        var body = "not-json";
        var rawBody = Encoding.UTF8.GetBytes(body);
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(Secret));
        var hash = hmac.ComputeHash(rawBody);
        var sig = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        var deliveryId = Guid.NewGuid();
        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/github")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Hub-Signature-256", sig);
        request.Headers.Add("X-GitHub-Delivery", deliveryId.ToString());
        request.Headers.Add("X-GitHub-Event", "installation");

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private static HttpRequestMessage BuildRequest(
        Guid deliveryId, string body, string sigHeader, string eventType)
    {
        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/github")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Hub-Signature-256", sigHeader);
        request.Headers.Add("X-GitHub-Delivery", deliveryId.ToString());
        request.Headers.Add("X-GitHub-Event", eventType);
        return request;
    }
}

/// <summary>No-op dispatcher for ingest pipeline tests.</summary>
internal sealed class NoopGithubWebhookDispatcher : IGithubWebhookDispatcher
{
    public Task DispatchAsync(GithubWebhookEnvelope envelope, CancellationToken ct) => Task.CompletedTask;
}

/// <summary>Throwing dispatcher for quarantine/failure tests.</summary>
internal sealed class ThrowingGithubWebhookDispatcher(int throwUntilAttempt) : IGithubWebhookDispatcher
{
    private int _callCount;

    public Task DispatchAsync(GithubWebhookEnvelope envelope, CancellationToken ct)
    {
        var n = Interlocked.Increment(ref _callCount);
        if (n <= throwUntilAttempt)
            throw new InvalidOperationException($"Simulated handler failure #{n}");
        return Task.CompletedTask;
    }
}
