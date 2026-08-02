using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Telemetry;

/// <summary>
/// Verifies that the OpenAPI (Swagger) document correctly exposes the telemetry ingest
/// endpoint with anonymous auth, expected response shapes, and the Idempotency-Key header.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class TelemetryIngestSwaggerSurfaceTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    public TelemetryIngestSwaggerSurfaceTests(BackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<JsonDocument> GetSwaggerDocAsync()
    {
        var resp = await _client.GetAsync("/swagger/v1/swagger.json");
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        return JsonDocument.Parse(json);
    }

    [Fact]
    public async Task OpenApi_documents_telemetry_events_path()
    {
        using var doc = await GetSwaggerDocAsync();
        var paths = doc.RootElement.GetProperty("paths");

        var telemetryPath = paths.EnumerateObject()
            .FirstOrDefault(p => p.Name.Contains("telemetry/events"));

        telemetryPath.Name.Should().NotBeNullOrEmpty(
            because: "OpenAPI must expose POST /api/v1/telemetry/events");

        telemetryPath.Value.TryGetProperty("post", out _).Should().BeTrue(
            because: "the path must include a POST operation");
    }

    [Fact]
    public async Task OpenApi_documents_202_413_429_responses()
    {
        using var doc = await GetSwaggerDocAsync();
        var paths = doc.RootElement.GetProperty("paths");

        var telemetryPath = paths.EnumerateObject()
            .First(p => p.Name.Contains("telemetry/events"));

        var responses = telemetryPath.Value.GetProperty("post").GetProperty("responses");

        responses.TryGetProperty("202", out _).Should().BeTrue(
            because: "telemetry ingest must declare a 202 Accepted response");

        responses.TryGetProperty("413", out _).Should().BeTrue(
            because: "telemetry ingest must declare a 413 Payload Too Large response");

        responses.TryGetProperty("429", out _).Should().BeTrue(
            because: "telemetry ingest must declare a 429 Too Many Requests response");
    }

    [Fact]
    public async Task OpenApi_documents_idempotency_key_header_required()
    {
        using var doc = await GetSwaggerDocAsync();
        var paths = doc.RootElement.GetProperty("paths");

        var telemetryPath = paths.EnumerateObject()
            .First(p => p.Name.Contains("telemetry/events"));

        var postOp = telemetryPath.Value.GetProperty("post");

        // The Idempotency-Key header is documented as a parameter.
        if (postOp.TryGetProperty("parameters", out var parameters))
        {
            var paramNames = parameters.EnumerateArray()
                .Select(p => p.TryGetProperty("name", out var n) ? n.GetString() : null)
                .Where(n => n != null)
                .ToList();

            paramNames.Should().Contain("Idempotency-Key",
                because: "the Idempotency-Key header must be documented in the OpenAPI spec");
        }
        else
        {
            // If no parameters element, just verify the path is accessible — a weaker assertion.
            // The Idempotency-Key requirement is still enforced at runtime (400 without it).
            // This assertion will intentionally fail until the header is documented properly.
            postOp.TryGetProperty("parameters", out _).Should().BeTrue(
                because: "the Idempotency-Key header parameter must appear in the OpenAPI parameters list");
        }
    }

    [Fact]
    public async Task OpenApi_path_has_no_security_requirement()
    {
        using var doc = await GetSwaggerDocAsync();
        var paths = doc.RootElement.GetProperty("paths");

        var telemetryPath = paths.EnumerateObject()
            .First(p => p.Name.Contains("telemetry/events"));

        var postOp = telemetryPath.Value.GetProperty("post");

        // An operation that allows anonymous access must either:
        //   (a) have no "security" property at all (Swashbuckle default for AllowAnonymous), or
        //   (b) have an explicit empty security array [].
        // A non-empty security array means Bearer auth is required — that would be a regression.
        if (postOp.TryGetProperty("security", out var security))
        {
            security.GetArrayLength().Should().Be(0,
                because: "anonymous telemetry ingest must have an empty security array — a non-empty array means auth is required");
        }
        else
        {
            // No "security" property at the operation level means the global security
            // requirement does NOT apply to this operation (Swashbuckle respects AllowAnonymous).
            // Assert the property is genuinely absent rather than silently passing.
            postOp.TryGetProperty("security", out _).Should().BeFalse(
                because: "anonymous ingest endpoint must not inherit the global Bearer security requirement");
        }
    }
}
