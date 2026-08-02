using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>Verifies that Swagger exposes the audit-log endpoint and its filter parameters.</summary>
[Collection(BackendCollection.Name)]
public sealed class AuditLogSwaggerSurfaceTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    public AuditLogSwaggerSurfaceTests(BackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Swagger_lists_audit_log_endpoint_with_filter_params()
    {
        var resp = await _client.GetAsync("/swagger/v1/swagger.json");
        resp.EnsureSuccessStatusCode();

        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        // Verify the path exists
        var auditLogPath = paths.EnumerateObject()
            .FirstOrDefault(p => p.Name.Contains("audit-log"));
        auditLogPath.Name.Should().NotBeNullOrEmpty();

        // Find the GET operation
        var getOp = auditLogPath.Value.GetProperty("get");
        var paramNames = getOp.GetProperty("parameters")
            .EnumerateArray()
            .Select(p => p.GetProperty("name").GetString())
            .ToList();

        paramNames.Should().Contain("event_type");
        paramNames.Should().Contain("user_id");
        paramNames.Should().Contain("from");
        paramNames.Should().Contain("to");
        paramNames.Should().Contain("format");

        // M18-001: verify the Swagger GET operation for audit-log contains
        // at least one 200 response (the streaming export shapes are declared
        // via Produces() metadata; ASP.NET Minimal API exposes them in the
        // responses object keyed by status code).
        var responses = getOp.GetProperty("responses");
        responses.TryGetProperty("200", out var ok200).Should().BeTrue(
            "the audit-log GET operation should declare a 200 response for export content types");

        // The operation should also declare a 402 for tier-gate denials (M18-001).
        responses.TryGetProperty("402", out _).Should().BeTrue(
            "the audit-log GET operation should declare a 402 response (Enterprise tier gate)");

        // M18-001 finding #1: verify the 200 response content object contains both
        // application/x-ndjson (JSONL streaming) and text/csv (CSV streaming) entries.
        // These are declared via .Produces(StatusCodes.Status200OK, contentType: "...") in
        // AuditLogEndpoints.cs and must appear in the OpenAPI spec so API clients can
        // discover the export content types without reading source code.
        ok200.TryGetProperty("content", out var content200).Should().BeTrue(
            "the 200 response should have a 'content' object listing all declared media types");

        var contentKeys = content200.EnumerateObject().Select(p => p.Name).ToList();
        contentKeys.Should().Contain("application/x-ndjson",
            "the JSONL streaming export shape must be declared in the OpenAPI 200 response content");
        contentKeys.Should().Contain("text/csv",
            "the CSV streaming export shape must be declared in the OpenAPI 200 response content");
    }
}
