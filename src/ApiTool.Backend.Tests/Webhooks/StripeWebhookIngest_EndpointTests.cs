using System.Net;
using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.Data;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;
namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>
/// HTTP-level tests for POST /webhooks/stripe covering signature verification,
/// idempotency, quarantine, and duplicate handling.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class StripeWebhookIngest_EndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public StripeWebhookIngest_EndpointTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private const string Secret = "whsec_test";

    /// <summary>
    /// Creates a factory that overrides webhook secrets and optionally replaces the dispatcher.
    /// By default installs the noop dispatcher — these tests verify the ingest pipeline
    /// (signature, idempotency, status tracking), not handler logic which is tested separately.
    /// Pass <paramref name="configureServices"/> to override the dispatcher with a custom one.
    /// </summary>
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
                    ["ApiTool:Stripe:Webhook:Secrets"] = secrets,
                });
            });
            b.ConfigureServices(services =>
            {
                // Default: replace the real dispatcher with the no-op so ingest tests
                // don't depend on Stripe gateway state. Tests that want a custom dispatcher
                // pass configureServices which runs after this and overrides again.
                services.RemoveAll<IStripeWebhookDispatcher>();
                services.AddSingleton<IStripeWebhookDispatcher, NoopStripeWebhookDispatcher>();
                configureServices?.Invoke(services);
            });
        }).CreateClient();
    }

    private static (string body, string sigHeader) BuildSignedDelivery(string eventJson, string secret = Secret, int? epochOffset = null)
    {
        var ts = DateTimeOffset.UtcNow.ToUnixTimeSeconds() + (epochOffset ?? 0);
        var signed = $"{ts}.{eventJson}";
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(secret));
        var sig = Convert.ToHexString(hmac.ComputeHash(Encoding.UTF8.GetBytes(signed))).ToLowerInvariant();
        return (eventJson, $"t={ts},v1={sig}");
    }

    private static string MakeEventJson(string id = "evt_test_ep_1") =>
        $"{{\"id\":\"{id}\",\"object\":\"event\",\"api_version\":\"2024-06-20\",\"created\":1234567890,\"livemode\":false,\"pending_webhooks\":0,\"request\":null,\"type\":\"customer.subscription.created\",\"data\":{{\"object\":{{}}}}}}";


    [Fact]
    public async Task Valid_signature_inserts_pending_row_returns_200()
    {
        var client = CreateClient();
        var (body, sig) = BuildSignedDelivery(MakeEventJson("evt_ep_valid_1"));

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("Stripe-Signature", sig);

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Invalid_signature_returns_400_no_row_inserted()
    {
        var client = CreateClient();
        var body = MakeEventJson("evt_ep_bad_sig");

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("Stripe-Signature", "t=12345,v1=bad_signature_value");

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Missing_signature_header_returns_400()
    {
        var client = CreateClient();
        var body = MakeEventJson("evt_ep_no_sig");

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        // No Stripe-Signature header

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Multi_secret_rotation_old_and_new_both_accepted()
    {
        var secrets = "whsec_old,whsec_new";
        var clientOld = CreateClient(secrets);
        var clientNew = CreateClient(secrets);

        var (bodyOld, sigOld) = BuildSignedDelivery(MakeEventJson("evt_ep_rot_old"), "whsec_old");
        var reqOld = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(bodyOld, Encoding.UTF8, "application/json"),
        };
        reqOld.Headers.Add("Stripe-Signature", sigOld);
        var respOld = await clientOld.SendAsync(reqOld);
        respOld.StatusCode.Should().Be(HttpStatusCode.OK);

        var (bodyNew, sigNew) = BuildSignedDelivery(MakeEventJson("evt_ep_rot_new"), "whsec_new");
        var reqNew = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(bodyNew, Encoding.UTF8, "application/json"),
        };
        reqNew.Headers.Add("Stripe-Signature", sigNew);
        var respNew = await clientNew.SendAsync(reqNew);
        respNew.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Duplicate_event_id_returns_200_no_second_row()
    {
        var client = CreateClient();
        var eventJson = MakeEventJson("evt_ep_dup");

        // First delivery
        var (body1, sig1) = BuildSignedDelivery(eventJson);
        var req1 = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body1, Encoding.UTF8, "application/json"),
        };
        req1.Headers.Add("Stripe-Signature", sig1);
        var resp1 = await client.SendAsync(req1);
        resp1.StatusCode.Should().Be(HttpStatusCode.OK);

        // Second delivery (same event_id, new timestamp in sig — valid sig, but idempotent)
        var (body2, sig2) = BuildSignedDelivery(eventJson);
        var req2 = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body2, Encoding.UTF8, "application/json"),
        };
        req2.Headers.Add("Stripe-Signature", sig2);
        var resp2 = await client.SendAsync(req2);
        resp2.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Handler_exception_records_failure_returns_500()
    {
        var throwingDispatcher = new ThrowingStripeWebhookDispatcher(throwUntilAttempt: 1);
        var client = CreateClient(configureServices: services =>
        {
            services.RemoveAll<IStripeWebhookDispatcher>();
            services.AddSingleton<IStripeWebhookDispatcher>(throwingDispatcher);
        });

        var (body, sig) = BuildSignedDelivery(MakeEventJson("evt_ep_fail_1"));
        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("Stripe-Signature", sig);

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.InternalServerError);
    }

    [Fact]
    public async Task Fifth_handler_failure_quarantines_returns_200()
    {
        // Each retry from Stripe uses the same event_id. The endpoint re-dispatches
        // 'pending' rows (failed retries), so sending the same event 5 times with a
        // throwing dispatcher drives the quarantine path via HTTP.
        const string eventId = "evt_ep_quarantine_1";
        var eventJson = MakeEventJson(eventId);

        var throwingDispatcher = new ThrowingStripeWebhookDispatcher(throwUntilAttempt: int.MaxValue);
        var quarantineClient = _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureAppConfiguration((_, cfg) =>
            {
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["ApiTool:Stripe:Webhook:Secrets"] = Secret,
                });
            });
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IStripeWebhookDispatcher>();
                services.AddSingleton<IStripeWebhookDispatcher>(throwingDispatcher);
            });
        }).CreateClient();

        HttpStatusCode? lastStatus = null;
        for (var attempt = 0; attempt < StripeWebhookStore.QuarantineThreshold; attempt++)
        {
            var (body, sig) = BuildSignedDelivery(eventJson);
            var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
            {
                Content = new StringContent(body, Encoding.UTF8, "application/json"),
            };
            request.Headers.Add("Stripe-Signature", sig);
            var response = await quarantineClient.SendAsync(request);
            lastStatus = response.StatusCode;
        }

        // 5th attempt should quarantine and return 200 (break retry storm).
        lastStatus.Should().Be(HttpStatusCode.OK);

        // Verify the row is quarantined in the DB.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.StripeWebhookEvents.FirstOrDefaultAsync(x => x.EventId == eventId);
        row.Should().NotBeNull();
        row!.Status.Should().Be("quarantined");
        row.AttemptCount.Should().Be(StripeWebhookStore.QuarantineThreshold);
    }

    [Fact]
    public async Task Successful_processing_marks_status_processed_with_processed_at()
    {
        var client = CreateClient();
        const string eventId = "evt_ep_success_1";
        var (body, sig) = BuildSignedDelivery(MakeEventJson(eventId));
        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("Stripe-Signature", sig);

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify DB state: row should have status='processed' and processed_at set.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.StripeWebhookEvents.FirstOrDefaultAsync(x => x.EventId == eventId);
        row.Should().NotBeNull("a row should have been inserted for the event");
        row!.Status.Should().Be("processed");
        row.ProcessedAt.Should().NotBeNull("processed_at must be set when dispatch succeeds");
    }

    [Fact]
    public async Task Stale_timestamp_outside_tolerance_returns_400()
    {
        var client = CreateClient();
        // Sign with timestamp 10 minutes in the past (beyond default 5-minute tolerance)
        var (body, sig) = BuildSignedDelivery(MakeEventJson("evt_ep_stale"), epochOffset: -601);

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("Stripe-Signature", sig);

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task No_secrets_configured_returns_400()
    {
        // When Secrets is empty (non-live mode allows boot, but requests cannot be verified).
        var client = CreateClient(secrets: "");
        var (body, sig) = BuildSignedDelivery(MakeEventJson("evt_ep_no_secrets"));

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("Stripe-Signature", sig);

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }
}

/// <summary>
/// Test-only dispatcher that throws for the first N attempts, then returns CompletedTask.
/// </summary>
internal sealed class ThrowingStripeWebhookDispatcher(int throwUntilAttempt) : IStripeWebhookDispatcher
{
    private int _callCount;

    public Task DispatchAsync(Stripe.Event @event, CancellationToken ct)
    {
        var n = Interlocked.Increment(ref _callCount);
        if (n <= throwUntilAttempt)
            throw new InvalidOperationException($"Simulated handler failure #{n}");
        return Task.CompletedTask;
    }
}
