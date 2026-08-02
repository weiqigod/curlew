using System.Net;
using System.Net.Http.Json;
using System.Text;
using ApiTool.Backend.Data;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Telemetry;

/// <summary>
/// Integration tests for <c>POST /api/v1/telemetry/events</c> exercised through
/// the full ASP.NET pipeline via <see cref="BackendFactory"/>.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class TelemetryIngestEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    private const string Path = "/api/v1/telemetry/events";
    private static readonly Guid TestInstallId = new("22222222-2222-2222-2222-222222222222");

    public TelemetryIngestEndpointTests(BackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private static HttpRequestMessage BuildRequest(
        string body,
        string? idempotencyKey = null,
        string contentType = "application/json")
    {
        var key = idempotencyKey ?? Guid.NewGuid().ToString();
        var req = new HttpRequestMessage(HttpMethod.Post, Path)
        {
            Content = new StringContent(body, Encoding.UTF8, contentType),
        };
        req.Headers.Add("Idempotency-Key", key);
        return req;
    }

    private static string ValidBody(Guid? installId = null) =>
        $$"""
        {
          "install_id": "{{installId ?? TestInstallId}}",
          "event_type": "run.completed",
          "event_payload": { "collection_size": 12, "duration_ms": 420 }
        }
        """;

    [Fact]
    public async Task POST_with_fresh_idempotency_key_returns_202_and_inserts_one_row()
    {
        var key = Guid.NewGuid().ToString();
        using var req = BuildRequest(ValidBody(), idempotencyKey: key);

        var resp = await _client.SendAsync(req);

        resp.StatusCode.Should().Be(HttpStatusCode.Accepted);

        // Verify row was inserted.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var count = await db.TelemetryEvents.CountAsync(e => e.IdempotencyKey == key);
        count.Should().Be(1, because: "exactly one row should be inserted for a fresh idempotency key");
    }

    [Fact]
    public async Task POST_replayed_idempotency_key_returns_202_and_does_not_insert()
    {
        var key = Guid.NewGuid().ToString();

        // First request — inserts
        using var req1 = BuildRequest(ValidBody(), idempotencyKey: key);
        var resp1 = await _client.SendAsync(req1);
        resp1.StatusCode.Should().Be(HttpStatusCode.Accepted);

        // Replay — must not insert a second row
        using var req2 = BuildRequest(ValidBody(), idempotencyKey: key);
        var resp2 = await _client.SendAsync(req2);
        resp2.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "idempotent replay should still return 202");

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var count = await db.TelemetryEvents.CountAsync(e => e.IdempotencyKey == key);
        count.Should().Be(1, because: "replay must not insert a second row");
    }

    [Fact]
    public async Task POST_body_over_64KB_returns_413()
    {
        var largePayload = new string('x', 66 * 1024);
        var body = $$"""
        {
          "install_id": "{{TestInstallId}}",
          "event_type": "run.completed",
          "event_payload": { "big": "{{largePayload}}" }
        }
        """;

        using var req = BuildRequest(body);
        var resp = await _client.SendAsync(req);

        resp.StatusCode.Should().Be(HttpStatusCode.RequestEntityTooLarge,
            because: "bodies exceeding 64KB must be rejected with 413");
    }

    [Fact]
    public async Task POST_missing_install_id_returns_400()
    {
        var body = """{ "event_type": "run.completed", "event_payload": {} }""";
        using var req = BuildRequest(body);
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task POST_invalid_install_id_uuid_returns_400()
    {
        var body = """
        {
          "install_id": "not-a-guid",
          "event_type": "run.completed",
          "event_payload": {}
        }
        """;
        using var req = BuildRequest(body);
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task POST_missing_idempotency_key_header_returns_400()
    {
        var req = new HttpRequestMessage(HttpMethod.Post, Path)
        {
            Content = new StringContent(ValidBody(), Encoding.UTF8, "application/json"),
        };
        // Deliberately no Idempotency-Key header

        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest,
            because: "missing Idempotency-Key header must return 400");
    }

    [Fact]
    public async Task POST_accepts_unknown_event_type_forward_compat()
    {
        var key = Guid.NewGuid().ToString();
        var body = $$"""
        {
          "install_id": "{{TestInstallId}}",
          "event_type": "future.event.type.v2",
          "event_payload": { "new_field": 42 }
        }
        """;
        using var req = BuildRequest(body, idempotencyKey: key);
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "unknown event types should be accepted for forward-compat");
    }

    [Fact]
    public async Task POST_does_not_require_authentication()
    {
        // Use a fresh client with no auth header
        var anonClient = _factory.CreateClient();
        // Explicitly omit any Bearer token
        var key = Guid.NewGuid().ToString();
        using var req = BuildRequest(ValidBody(), idempotencyKey: key);
        // No auth header added.

        var resp = await anonClient.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "telemetry ingest must be anonymous — no authentication required");
    }

    [Fact]
    public async Task POST_empty_event_payload_is_accepted()
    {
        var key = Guid.NewGuid().ToString();
        var body = $$"""
        {
          "install_id": "{{TestInstallId}}",
          "event_type": "run.completed",
          "event_payload": {}
        }
        """;
        using var req = BuildRequest(body, idempotencyKey: key);
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.Accepted);
    }

    [Fact]
    public async Task POST_event_type_over_64_chars_returns_400()
    {
        var longType = new string('a', 65);
        var body = $$"""
        {
          "install_id": "{{TestInstallId}}",
          "event_type": "{{longType}}",
          "event_payload": {}
        }
        """;
        using var req = BuildRequest(body);
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest,
            because: "event_type must be at most 64 characters");
    }

    [Fact]
    public async Task POST_idempotency_key_over_64_chars_returns_400()
    {
        var longKey = new string('k', 65);
        using var req = BuildRequest(ValidBody(), idempotencyKey: longKey);
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest,
            because: "Idempotency-Key must be at most 64 characters");
    }

    [Fact]
    public async Task POST_non_json_body_returns_400()
    {
        using var req = BuildRequest("not-json-at-all");
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest,
            because: "non-JSON body must return 400");
    }

    [Fact]
    public async Task POST_null_json_body_returns_400()
    {
        // "null" is valid JSON but deserializes to null TelemetryIngestRequest.
        using var req = BuildRequest("null");
        var resp = await _client.SendAsync(req);
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest,
            because: "null JSON body must return 400");
    }

    [Fact]
    public async Task POST_stores_server_received_at_timestamp()
    {
        var key = Guid.NewGuid().ToString();
        var before = DateTime.UtcNow.AddSeconds(-1);

        using var req = BuildRequest(ValidBody(), idempotencyKey: key);
        await _client.SendAsync(req);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var ev = await db.TelemetryEvents.SingleAsync(e => e.IdempotencyKey == key);
        ev.ReceivedAt.Should().BeOnOrAfter(before,
            because: "received_at must be server-stamped at ingest time");
        ev.InstallId.Should().Be(TestInstallId,
            because: "install_id must be stored from the request body");
    }
}
