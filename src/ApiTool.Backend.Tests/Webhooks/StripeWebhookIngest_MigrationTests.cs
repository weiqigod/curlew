using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>Verifies the stripe_webhook_events migration runs cleanly.</summary>
public sealed class StripeWebhookIngest_MigrationTests
{
    [Fact]
    public async Task Migration_creates_stripe_webhook_events_table()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        (await scope.Db.StripeWebhookEvents.CountAsync()).Should().Be(0);
    }

    [Fact]
    public async Task Migration_creates_status_received_index()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        // Smoke: query that exercises the status+received_at index path
        var rows = await scope.Db.StripeWebhookEvents
            .Where(x => x.Status == "pending")
            .OrderByDescending(x => x.ReceivedAt)
            .ToListAsync();
        rows.Should().BeEmpty();
    }
}
