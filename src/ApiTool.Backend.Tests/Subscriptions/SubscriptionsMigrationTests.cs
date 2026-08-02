using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Verifies the billing and invitations migration runs successfully.</summary>
public sealed class SubscriptionsMigrationTests
{
    [Fact]
    public async Task Migration_creates_subscriptions_table()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        (await scope.Db.Subscriptions.CountAsync()).Should().Be(0);
    }

    [Fact]
    public async Task Migration_creates_organization_invitations_with_token_hash_column()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        (await scope.Db.OrganizationInvitations.CountAsync()).Should().Be(0);
    }
}
