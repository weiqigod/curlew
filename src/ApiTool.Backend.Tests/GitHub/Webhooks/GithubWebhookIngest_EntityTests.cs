// Refs docs/SPECIFICATION.md:10031-10049 (github_webhook_events schema).
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests verifying that GithubWebhookEvent entity and DbSet registration work correctly.
/// </summary>
public sealed class GithubWebhookIngest_EntityTests
{
    [Fact]
    public async Task DbSet_is_registered()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var count = await db.GithubWebhookEvents.CountAsync();
        count.Should().Be(0);
    }

    [Fact]
    public void Defaults_are_pending_status_and_empty_payload()
    {
        var entity = new GithubWebhookEvent();
        entity.Status.Should().Be("pending");
        entity.PayloadJson.Should().Be("{}");
        entity.AttemptCount.Should().Be(0);
        entity.LastError.Should().BeNull();
        entity.LastErrorAt.Should().BeNull();
        entity.ProcessedAt.Should().BeNull();
        entity.Action.Should().BeNull();
    }

    [Theory]
    [InlineData("pending")]
    [InlineData("processed")]
    [InlineData("quarantined")]
    public async Task Status_accepts_three_legal_values(string status)
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var deliveryId = Guid.NewGuid();
        db.GithubWebhookEvents.Add(new GithubWebhookEvent
        {
            DeliveryId = deliveryId,
            EventType = "installation",
            ReceivedAt = DateTime.UtcNow,
            Status = status,
        });
        await db.SaveChangesAsync();

        db.ChangeTracker.Clear();
        var row = await db.GithubWebhookEvents.FindAsync(deliveryId);
        row.Should().NotBeNull();
        row!.Status.Should().Be(status);
    }
}
