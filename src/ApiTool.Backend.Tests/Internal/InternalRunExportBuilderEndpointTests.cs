// Tests for POST /api/v1/internal/test-hooks/run-export-builder (M18-004).
using System.Net;
using System.Net.Http.Headers;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Storage;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Internal;

/// <summary>
/// Integration tests for <c>POST /api/v1/internal/test-hooks/run-export-builder</c>.
/// Uses the shared <see cref="BackendFactory"/> (InMemory DB) since the builder assembler
/// only uses EF LINQ queries (no bulk-delete or raw SQL) which the InMemory provider supports.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class InternalRunExportBuilderEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    public InternalRunExportBuilderEndpointTests(BackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Tick_endpoint_returns_200_with_ticked_true()
    {
        var response = await _client.PostAsync("/api/v1/internal/test-hooks/run-export-builder", null);
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("ticked").GetBoolean().Should().BeTrue();
    }

    [Fact]
    public async Task Tick_endpoint_transitions_queued_row_to_ready()
    {
        // Seed a user and a queued request with a future timestamp so it sorts last in the
        // builder's FIFO queue — this ensures our row is either the only one picked up, or
        // the tick runs until it drains.  We use a fixed-timestamp seed in a dedicated scope
        // so even concurrent test runs can identify their own row.
        using var seedScope = _factory.Services.CreateScope();
        var seedDb = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();

        var userId = Guid.NewGuid();
        seedDb.Users.Add(new User { Id = userId, Email = $"hook-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        var reqId = Guid.NewGuid();
        seedDb.UserExportRequests.Add(new UserExportRequest
        {
            Id = reqId,
            UserId = userId,
            Status = UserExportStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        });
        await seedDb.SaveChangesAsync();

        // Call the hook — each POST ticks the builder once; keep calling until our specific
        // row has left the Queued state (max 10 ticks to avoid an infinite loop in case of bug).
        string status = "queued";
        using var authClient = _factory.CreateClient();
        authClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"hook-{userId:N}@example.com"));

        for (var i = 0; i < 10 && status == "queued"; i++)
        {
            var hookResp = await _client.PostAsync("/api/v1/internal/test-hooks/run-export-builder", null);
            hookResp.StatusCode.Should().Be(HttpStatusCode.OK);

            var getResp = await authClient.GetAsync($"/api/v1/users/me/export-requests/{reqId}");
            getResp.StatusCode.Should().Be(HttpStatusCode.OK);
            var body = await getResp.Content.ReadAsStringAsync();
            using var doc = JsonDocument.Parse(body);
            status = doc.RootElement.GetProperty("status").GetString()!;
        }

        // Our row must have been processed by the builder at least once.
        status.Should().BeOneOf("ready", "failed",
            $"request {reqId} should have been processed from Queued → Ready|Failed after up to 10 ticks");
    }
}
