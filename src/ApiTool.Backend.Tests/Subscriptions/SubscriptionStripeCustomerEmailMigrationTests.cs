// Refs docs/SPECIFICATION.md:6796–6848 (subscription event handlers, re-fetch pattern).
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>
/// Verifies that the AddSubscriptionStripeCustomerEmail migration adds the
/// StripeCustomerEmail column and that the Quarantined enum value persists correctly.
/// </summary>
public sealed class SubscriptionStripeCustomerEmailMigrationTests
{
    [Fact]
    public async Task Migration_adds_StripeCustomerEmail_column()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var columns = await scope.Db.Database
            .SqlQueryRaw<string>("SELECT name FROM pragma_table_info('subscriptions')")
            .ToListAsync();

        columns.Should().Contain("StripeCustomerEmail");
    }

    [Fact]
    public async Task SubscriptionStatus_Quarantined_round_trips_through_db()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = "q@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Org", Slug = $"org-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        var subId = Guid.NewGuid();
        db.Subscriptions.Add(new Subscription
        {
            Id = subId, OrgId = orgId,
            Tier = SubscriptionTier.Team,
            Status = SubscriptionStatus.Quarantined,
            Interval = "month", SeatCount = 3, SeatLimit = 3,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        db.ChangeTracker.Clear();
        var loaded = await db.Subscriptions.SingleAsync(s => s.Id == subId);
        loaded.Status.Should().Be(SubscriptionStatus.Quarantined);
    }

    [Fact]
    public async Task StripeCustomerEmail_persists_and_loads_correctly()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = "email@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Acme", Slug = $"acme-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        var subId = Guid.NewGuid();
        db.Subscriptions.Add(new Subscription
        {
            Id = subId, OrgId = orgId,
            Tier = SubscriptionTier.Team,
            Status = SubscriptionStatus.Active,
            Interval = "month", SeatCount = 3, SeatLimit = 3,
            StripeCustomerId = "cus_test_123",
            StripeCustomerEmail = "billing@acme.com",
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        db.ChangeTracker.Clear();
        var loaded = await db.Subscriptions.SingleAsync(s => s.Id == subId);
        loaded.StripeCustomerEmail.Should().Be("billing@acme.com");
    }
}
