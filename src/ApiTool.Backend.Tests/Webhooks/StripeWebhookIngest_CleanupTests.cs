using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>Verifies the 90-day cleanup retention policy via StripeWebhookStore.DeleteOlderThanAsync.</summary>
public sealed class StripeWebhookIngest_CleanupTests
{
    private static StripeWebhookStore CreateStore(TestDbScope scope, DateTimeOffset? now = null)
    {
        var clock = new FakeClock(now ?? new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero));
        return new StripeWebhookStore(scope.Db, clock);
    }

    [Fact]
    public async Task DeleteOlderThan_removes_processed_rows_older_than_cutoff()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        await store.TryInsertPendingAsync("evt_c1", "x", "{}", CancellationToken.None);
        await store.MarkProcessedAsync("evt_c1", CancellationToken.None);

        var cutoff = new DateTime(2026, 6, 1, 0, 0, 0, DateTimeKind.Utc); // future, older than Jan 1
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(1);
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(0);
    }

    [Fact]
    public async Task DeleteOlderThan_retains_quarantined_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        await store.TryInsertPendingAsync("evt_c2", "x", "{}", CancellationToken.None);
        for (var i = 0; i < StripeWebhookStore.QuarantineThreshold; i++)
            await store.RecordFailureAsync("evt_c2", "err", CancellationToken.None);

        var cutoff = new DateTime(2030, 12, 31, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0);
        var row = await scope.Db.StripeWebhookEvents.FirstAsync();
        row.Status.Should().Be("quarantined");
    }

    [Fact]
    public async Task DeleteOlderThan_retains_pending_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        await store.TryInsertPendingAsync("evt_c3", "x", "{}", CancellationToken.None);

        var cutoff = new DateTime(2030, 12, 31, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0);
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(1);
    }
}
