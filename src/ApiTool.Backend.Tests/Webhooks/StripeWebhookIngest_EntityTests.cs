using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>Verifies the StripeWebhookEvent entity round-trips against SQLite.</summary>
public sealed class StripeWebhookIngest_EntityTests
{
    [Fact]
    public async Task Entity_persists_and_round_trips()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        scope.Db.StripeWebhookEvents.Add(new StripeWebhookEvent
        {
            EventId = "evt_test_1",
            EventType = "customer.subscription.created",
            ReceivedAt = DateTime.UtcNow,
            Status = "pending",
            PayloadJson = "{\"id\":\"evt_test_1\"}",
        });
        await scope.Db.SaveChangesAsync();
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(1);
    }

    [Fact]
    public async Task Inserting_duplicate_event_id_throws_unique_violation()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        scope.Db.StripeWebhookEvents.Add(new StripeWebhookEvent
        {
            EventId = "evt_dup",
            EventType = "x",
            ReceivedAt = DateTime.UtcNow,
            Status = "pending",
            PayloadJson = "{}",
        });
        await scope.Db.SaveChangesAsync();

        // Detach tracked entities so the second Add triggers a DB-level unique constraint error
        // rather than an EF change-tracker identity conflict.
        scope.Db.ChangeTracker.Clear();

        scope.Db.StripeWebhookEvents.Add(new StripeWebhookEvent
        {
            EventId = "evt_dup",
            EventType = "x",
            ReceivedAt = DateTime.UtcNow,
            Status = "pending",
            PayloadJson = "{}",
        });
        await Assert.ThrowsAsync<DbUpdateException>(() => scope.Db.SaveChangesAsync());
    }
}
