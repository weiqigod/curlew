// Refs docs/SPECIFICATION.md:8595 (90-day retention; processed only).
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>Verifies the 90-day cleanup retention policy via GithubWebhookStore.DeleteOlderThanAsync.</summary>
public sealed class GithubWebhookIngest_CleanupTests
{
    private static GithubWebhookStore CreateStore(TestDbScope scope, DateTimeOffset? now = null)
    {
        var clock = new FakeClock(now ?? new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero));
        return new GithubWebhookStore(scope.Db, clock);
    }

    [Fact]
    public async Task DeleteOlderThan_removes_processed_rows_older_than_cutoff()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        var id = Guid.NewGuid();
        await store.TryInsertPendingAsync(id, "installation", "created", "{}", CancellationToken.None);
        await store.MarkProcessedAsync(id, CancellationToken.None);

        var cutoff = new DateTime(2026, 6, 1, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(1);
        scope.Db.ChangeTracker.Clear();
        (await scope.Db.GithubWebhookEvents.CountAsync()).Should().Be(0);
    }

    [Fact]
    public async Task DeleteOlderThan_retains_quarantined_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        var id = Guid.NewGuid();
        await store.TryInsertPendingAsync(id, "installation", "created", "{}", CancellationToken.None);
        for (var i = 0; i < GithubWebhookStore.QuarantineThreshold; i++)
            await store.RecordFailureAsync(id, "err", CancellationToken.None);

        var cutoff = new DateTime(2030, 12, 31, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0);
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GithubWebhookEvents.FirstAsync();
        row.Status.Should().Be("quarantined");
    }

    [Fact]
    public async Task DeleteOlderThan_retains_pending_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        var id = Guid.NewGuid();
        await store.TryInsertPendingAsync(id, "installation", "created", "{}", CancellationToken.None);

        var cutoff = new DateTime(2030, 12, 31, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0);
        scope.Db.ChangeTracker.Clear();
        (await scope.Db.GithubWebhookEvents.CountAsync()).Should().Be(1);
    }
}
