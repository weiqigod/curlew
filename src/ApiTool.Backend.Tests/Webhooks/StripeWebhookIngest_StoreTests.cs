using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>Verifies StripeWebhookStore idempotency, state transitions, and cleanup.</summary>
public sealed class StripeWebhookIngest_StoreTests
{
    private static StripeWebhookStore CreateStore(
        TestDbScope scope, DateTimeOffset? now = null)
    {
        var clock = new FakeClock(now ?? new DateTimeOffset(2026, 5, 5, 12, 0, 0, TimeSpan.Zero));
        return new StripeWebhookStore(scope.Db, clock);
    }

    [Fact]
    public async Task TryInsertPending_returns_inserted_true_for_new_event()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        var result = await store.TryInsertPendingAsync("evt_1", "customer.subscription.created", "{}", CancellationToken.None);

        result.Inserted.Should().BeTrue();
        result.ExistingReceivedAt.Should().BeNull();
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(1);
    }

    [Fact]
    public async Task TryInsertPending_returns_inserted_false_for_duplicate()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        await store.TryInsertPendingAsync("evt_dup", "x", "{}", CancellationToken.None);
        var result = await store.TryInsertPendingAsync("evt_dup", "x", "{}", CancellationToken.None);

        result.Inserted.Should().BeFalse();
        result.ExistingReceivedAt.Should().NotBeNull();
        // Only one row should exist
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(1);
    }

    [Fact]
    public async Task MarkProcessed_sets_status_and_processed_at()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        await store.TryInsertPendingAsync("evt_2", "x", "{}", CancellationToken.None);
        await store.MarkProcessedAsync("evt_2", CancellationToken.None);

        // Use AsNoTracking to avoid reading from the EF change-tracker cache
        var row = await scope.Db.StripeWebhookEvents
            .AsNoTracking()
            .FirstAsync(x => x.EventId == "evt_2");
        row.Status.Should().Be("processed");
        row.ProcessedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task RecordFailure_increments_attempt_count_and_records_last_error()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        await store.TryInsertPendingAsync("evt_3", "x", "{}", CancellationToken.None);
        var count = await store.RecordFailureAsync("evt_3", "some error", CancellationToken.None);

        count.Should().Be(1);
        var row = await scope.Db.StripeWebhookEvents.FirstAsync(x => x.EventId == "evt_3");
        row.AttemptCount.Should().Be(1);
        row.LastError.Should().Be("some error");
        row.LastErrorAt.Should().NotBeNull();
        row.Status.Should().Be("pending"); // not yet quarantined
    }

    [Fact]
    public async Task RecordFailure_quarantines_at_fifth_attempt()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = CreateStore(scope);

        await store.TryInsertPendingAsync("evt_4", "x", "{}", CancellationToken.None);

        int finalCount = 0;
        for (var i = 0; i < StripeWebhookStore.QuarantineThreshold; i++)
            finalCount = await store.RecordFailureAsync("evt_4", $"err {i}", CancellationToken.None);

        finalCount.Should().Be(StripeWebhookStore.QuarantineThreshold);
        var row = await scope.Db.StripeWebhookEvents.FirstAsync(x => x.EventId == "evt_4");
        row.Status.Should().Be("quarantined");
        row.AttemptCount.Should().Be(StripeWebhookStore.QuarantineThreshold);
    }

    [Fact]
    public async Task DeleteOlderThan_only_removes_processed_rows_older_than_cutoff()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var oldTime = new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero);
        var store = CreateStore(scope, oldTime);

        // Insert old processed row
        await store.TryInsertPendingAsync("evt_old", "x", "{}", CancellationToken.None);
        await store.MarkProcessedAsync("evt_old", CancellationToken.None);

        // Insert recent processed row
        var newTime = new DateTimeOffset(2026, 5, 5, 12, 0, 0, TimeSpan.Zero);
        var storeNew = CreateStore(scope, newTime);
        await storeNew.TryInsertPendingAsync("evt_new", "x", "{}", CancellationToken.None);
        await storeNew.MarkProcessedAsync("evt_new", CancellationToken.None);

        var cutoff = new DateTime(2026, 3, 1, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(1);
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(1);
        var remaining = await scope.Db.StripeWebhookEvents.FirstAsync();
        remaining.EventId.Should().Be("evt_new");
    }

    [Fact]
    public async Task DeleteOlderThan_retains_quarantined_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var oldTime = new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero);
        var store = CreateStore(scope, oldTime);

        // Insert and quarantine an old event
        await store.TryInsertPendingAsync("evt_quarantine_old", "x", "{}", CancellationToken.None);
        for (var i = 0; i < StripeWebhookStore.QuarantineThreshold; i++)
            await store.RecordFailureAsync("evt_quarantine_old", "err", CancellationToken.None);

        var cutoff = new DateTime(2026, 12, 31, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0);
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(1);
        var row = await scope.Db.StripeWebhookEvents.FirstAsync();
        row.Status.Should().Be("quarantined");
    }
}
