// Tests for POST /api/v1/internal/test-hooks/run-deletion-finalizer (M18-005 / M18-006).
using System.Net;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Internal;

/// <summary>
/// Integration tests for <c>POST /api/v1/internal/test-hooks/run-deletion-finalizer</c>.
/// Uses <see cref="SqliteBackendFactory"/> (real SQLite in-memory) because
/// <see cref="ApiTool.Backend.Compliance.Gdpr.UserAnonymiser"/> uses
/// <c>ExecuteUpdateAsync</c>/<c>ExecuteDeleteAsync</c> which are not supported by the
/// EF Core InMemory provider.
/// </summary>
[Collection(SqliteBackendCollection.Name)]
public sealed class InternalRunDeletionFinalizerEndpointTests : IAsyncLifetime
{
    private readonly SqliteBackendFactory _factory;
    private readonly HttpClient _client;

    public InternalRunDeletionFinalizerEndpointTests(SqliteBackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private RecordingEmailQueue GetEmailQueue() =>
        _factory.Services.GetRequiredService<RecordingEmailQueue>();

    [Fact]
    public async Task Endpoint_returns_200_with_ticked_true()
    {
        var response = await _client.PostAsync(
            "/api/v1/internal/test-hooks/run-deletion-finalizer", null);

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("ticked").GetBoolean().Should().BeTrue();
    }

    [Fact]
    public async Task Endpoint_sets_anonymised_at_for_users_past_30_day_cooldown()
    {
        // Seed a user with pending_deletion_at set 31 days ago.
        using var seedScope = _factory.Services.CreateScope();
        var db = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();

        var userId = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = userId,
            Email = $"finalizer-hook-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = DateTime.UtcNow.AddDays(-31),
        });
        await db.SaveChangesAsync();

        // Trigger the finalizer via the test hook.
        var resp = await _client.PostAsync(
            "/api/v1/internal/test-hooks/run-deletion-finalizer", null);
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify anonymised_at is now set on the user row.
        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var user = await verifyDb.Users.FindAsync(userId);
        user.Should().NotBeNull();
        user!.AnonymisedAt.Should().NotBeNull(
            because: "UserAnonymiser must set anonymised_at when cooldown has elapsed");
        user.PendingDeletionAt.Should().BeNull(
            because: "UserAnonymiser clears pending_deletion_at after anonymisation");
    }

    [Fact]
    public async Task Endpoint_enqueues_account_deletion_completed_email()
    {
        var emailQueue = GetEmailQueue();
        emailQueue.Clear();

        // Seed a user with pending_deletion_at set 31 days ago.
        using var seedScope = _factory.Services.CreateScope();
        var db = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();

        var userId = Guid.NewGuid();
        var email = $"finalizer-email-{userId:N}@example.com";
        db.Users.Add(new User
        {
            Id = userId,
            Email = email,
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = DateTime.UtcNow.AddDays(-31),
        });
        await db.SaveChangesAsync();

        // Trigger the finalizer.
        var resp = await _client.PostAsync(
            "/api/v1/internal/test-hooks/run-deletion-finalizer", null);
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        // The finalizer must have enqueued an account_deletion_completed email.
        emailQueue.Messages
            .Where(m => m.TemplateSlug == "account_deletion_completed" && m.To == email)
            .Should().ContainSingle(
                because: "finalizer must enqueue account_deletion_completed email for the processed user");
    }

    [Fact]
    public async Task Endpoint_is_registered_in_Testing_environment()
    {
        // The endpoint must be reachable in the Testing environment (not 404).
        var resp = await _client.PostAsync(
            "/api/v1/internal/test-hooks/run-deletion-finalizer", null);
        resp.StatusCode.Should().NotBe(HttpStatusCode.NotFound,
            because: "the deletion-finalizer hook must be registered in Testing environment");
    }
}
