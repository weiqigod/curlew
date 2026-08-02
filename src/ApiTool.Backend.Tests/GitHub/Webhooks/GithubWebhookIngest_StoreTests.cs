// Refs docs/SPECIFICATION.md:8586-8595 (INSERT … ON CONFLICT DO NOTHING),
// :8597 (5-failure quarantine budget), :8595 (90-day retention).
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubWebhookStore using SQLite in-memory DB.
/// </summary>
public sealed class GithubWebhookIngest_StoreTests
{
    private const string EventType = "installation";
    private const string Action = "created";
    private const string Payload = """{"action":"created"}""";

    private static GithubWebhookStore CreateStore(ApiTool.Backend.Data.AppDbContext db)
        => new(db, TimeProvider.System);

    [Fact]
    public async Task TryInsertPending_returns_inserted_true_for_new_delivery()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var store = CreateStore(db);

        var deliveryId = Guid.NewGuid();
        var result = await store.TryInsertPendingAsync(deliveryId, EventType, Action, Payload, CancellationToken.None);

        result.Inserted.Should().BeTrue();
        result.ExistingReceivedAt.Should().BeNull();
        result.ExistingStatus.Should().BeNull();
        var row = await db.GithubWebhookEvents.FindAsync(deliveryId);
        row.Should().NotBeNull();
        row!.Status.Should().Be("pending");
        row.EventType.Should().Be(EventType);
        row.Action.Should().Be(Action);
    }

    [Fact]
    public async Task TryInsertPending_returns_inserted_false_for_duplicate()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var store = CreateStore(db);

        var deliveryId = Guid.NewGuid();
        await store.TryInsertPendingAsync(deliveryId, EventType, Action, Payload, CancellationToken.None);
        var result = await store.TryInsertPendingAsync(deliveryId, EventType, Action, Payload, CancellationToken.None);

        result.Inserted.Should().BeFalse();
        result.ExistingReceivedAt.Should().NotBeNull();
        result.ExistingStatus.Should().Be("pending");
    }

    [Fact]
    public async Task MarkProcessed_sets_status_and_processed_at()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var store = CreateStore(db);

        var deliveryId = Guid.NewGuid();
        await store.TryInsertPendingAsync(deliveryId, EventType, Action, Payload, CancellationToken.None);
        await store.MarkProcessedAsync(deliveryId, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubWebhookEvents.FindAsync(deliveryId);
        row!.Status.Should().Be("processed");
        row.ProcessedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task RecordFailure_increments_attempt_count()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var store = CreateStore(db);

        var deliveryId = Guid.NewGuid();
        await store.TryInsertPendingAsync(deliveryId, EventType, Action, Payload, CancellationToken.None);
        var attempts = await store.RecordFailureAsync(deliveryId, "Something failed", CancellationToken.None);

        attempts.Should().Be(1);
        db.ChangeTracker.Clear();
        var row = await db.GithubWebhookEvents.FindAsync(deliveryId);
        row!.AttemptCount.Should().Be(1);
        row.LastError.Should().Be("Something failed");
        row.LastErrorAt.Should().NotBeNull();
        row.Status.Should().Be("pending"); // not quarantined yet
    }

    [Fact]
    public async Task RecordFailure_quarantines_at_fifth_attempt()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var store = CreateStore(db);

        var deliveryId = Guid.NewGuid();
        await store.TryInsertPendingAsync(deliveryId, EventType, Action, Payload, CancellationToken.None);

        int attempts = 0;
        for (var i = 0; i < GithubWebhookStore.QuarantineThreshold; i++)
            attempts = await store.RecordFailureAsync(deliveryId, $"Error {i + 1}", CancellationToken.None);

        attempts.Should().Be(GithubWebhookStore.QuarantineThreshold);
        db.ChangeTracker.Clear();
        var row = await db.GithubWebhookEvents.FindAsync(deliveryId);
        row!.Status.Should().Be("quarantined");
        row.AttemptCount.Should().Be(GithubWebhookStore.QuarantineThreshold);
    }

    [Fact]
    public async Task DeleteOlderThan_only_removes_processed_rows()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var store = CreateStore(db);

        var oldDelivery = Guid.NewGuid();
        await store.TryInsertPendingAsync(oldDelivery, EventType, Action, Payload, CancellationToken.None);
        await store.MarkProcessedAsync(oldDelivery, CancellationToken.None);

        // Manually set the received_at to an old date
        db.ChangeTracker.Clear();
        var row = await db.GithubWebhookEvents.FindAsync(oldDelivery);
        row!.ReceivedAt = DateTime.UtcNow.AddDays(-100);
        await db.SaveChangesAsync();

        var cutoff = DateTime.UtcNow.AddDays(-90);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(1);
        db.ChangeTracker.Clear();
        var found = await db.GithubWebhookEvents.FindAsync(oldDelivery);
        found.Should().BeNull();
    }

    [Fact]
    public async Task DeleteOlderThan_retains_quarantined_rows()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var store = CreateStore(db);

        var quarantinedId = Guid.NewGuid();
        await store.TryInsertPendingAsync(quarantinedId, EventType, Action, Payload, CancellationToken.None);

        // Quarantine by recording 5 failures
        for (var i = 0; i < GithubWebhookStore.QuarantineThreshold; i++)
            await store.RecordFailureAsync(quarantinedId, "Error", CancellationToken.None);

        // Set old received_at
        db.ChangeTracker.Clear();
        var row = await db.GithubWebhookEvents.FindAsync(quarantinedId);
        row!.ReceivedAt = DateTime.UtcNow.AddDays(-100);
        await db.SaveChangesAsync();

        var cutoff = DateTime.UtcNow.AddDays(-90);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0); // quarantined rows must be retained
        db.ChangeTracker.Clear();
        var stillThere = await db.GithubWebhookEvents.FindAsync(quarantinedId);
        stillThere.Should().NotBeNull();
    }
}
