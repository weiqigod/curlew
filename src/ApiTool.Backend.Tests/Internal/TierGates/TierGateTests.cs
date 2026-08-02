using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Internal.TierGates;

/// <summary>
/// Unit tests for <see cref="TierGate"/> — the canonical AppDbContext-backed ITierGate implementation.
/// </summary>
public class TierGateTests
{
    public static IEnumerable<object[]> AllowedMatrix => new List<object[]>
    {
        new object[] { SubscriptionTier.Enterprise, SubscriptionTier.Enterprise }, // exact match
        new object[] { SubscriptionTier.Team,       SubscriptionTier.Team       }, // exact match
        new object[] { SubscriptionTier.Enterprise, SubscriptionTier.Team       }, // above required
        new object[] { SubscriptionTier.Team,       SubscriptionTier.Free       }, // far above required
    };

    public static IEnumerable<object[]> IneligibleMatrix => new List<object[]>
    {
        new object[] { SubscriptionTier.Free,         SubscriptionTier.Team       },
        new object[] { SubscriptionTier.Professional, SubscriptionTier.Team       },
        new object[] { SubscriptionTier.Team,         SubscriptionTier.Enterprise },
        new object[] { SubscriptionTier.Free,         SubscriptionTier.Enterprise },
    };

    private static async Task<(Guid orgId, Guid userId)> SeedOrgAsync(
        DbContext db)
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        var user = new User { Id = userId, Email = $"owner-{userId:N}@test.com", CreatedAt = DateTime.UtcNow };
        var org = new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"test-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };

        db.Add(user);
        db.Add(org);
        await db.SaveChangesAsync();
        return (orgId, userId);
    }

    private static async Task SeedSubscriptionAsync(
        DbContext db,
        Guid orgId,
        SubscriptionTier tier,
        DateTime? updatedAt = null)
    {
        db.Add(new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = tier,
            Status = SubscriptionStatus.Active,
            SeatCount = 1,
            SeatLimit = 25,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddDays(30),
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = updatedAt ?? DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
    }

    [Theory, MemberData(nameof(AllowedMatrix))]
    public async Task EnsureAsync_returns_Allowed_when_current_meets_or_exceeds_required(
        SubscriptionTier current, SubscriptionTier required)
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        var (orgId, _) = await SeedOrgAsync(scope.Db);
        await SeedSubscriptionAsync(scope.Db, orgId, current);

        var gate = new TierGate(scope.Db);
        var result = await gate.EnsureAsync(orgId, required, CancellationToken.None);

        result.Should().Be(TierGateResult.Allowed);
    }

    [Theory, MemberData(nameof(IneligibleMatrix))]
    public async Task EnsureAsync_returns_TierIneligible_when_current_below_required(
        SubscriptionTier current, SubscriptionTier required)
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        var (orgId, _) = await SeedOrgAsync(scope.Db);
        await SeedSubscriptionAsync(scope.Db, orgId, current);

        var gate = new TierGate(scope.Db);
        var result = await gate.EnsureAsync(orgId, required, CancellationToken.None);

        result.Should().Be(TierGateResult.TierIneligible);
    }

    [Fact]
    public async Task GetCurrentTierAsync_returns_latest_subscription_or_free()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        var (paidOrgId, _) = await SeedOrgAsync(scope.Db);
        var (freeOrgId, _) = await SeedOrgAsync(scope.Db);
		await SeedSubscriptionAsync(scope.Db, paidOrgId, SubscriptionTier.Team, DateTime.UtcNow);

        var gate = new TierGate(scope.Db);

        (await gate.GetCurrentTierAsync(paidOrgId, default)).Should().Be(SubscriptionTier.Team);
        (await gate.GetCurrentTierAsync(freeOrgId, default)).Should().Be(SubscriptionTier.Free);
    }

    [Fact]
    public async Task EnsureAsync_returns_OrgNotFound_for_unknown_orgId()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();

        var gate = new TierGate(scope.Db);
        var result = await gate.EnsureAsync(Guid.NewGuid(), SubscriptionTier.Team, CancellationToken.None);

        result.Should().Be(TierGateResult.OrgNotFound);
    }

    [Fact]
    public async Task EnsureAsync_treats_missing_subscription_row_as_Free()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        var (orgId, _) = await SeedOrgAsync(scope.Db);
        // No subscription row seeded — should be treated as Free.

        var gate = new TierGate(scope.Db);

        // Requiring Team → ineligible when treated as Free.
        var result = await gate.EnsureAsync(orgId, SubscriptionTier.Team, CancellationToken.None);
        result.Should().Be(TierGateResult.TierIneligible);

        // Requiring Free → allowed when treated as Free.
        var resultFree = await gate.EnsureAsync(orgId, SubscriptionTier.Free, CancellationToken.None);
        resultFree.Should().Be(TierGateResult.Allowed);
    }

    [Fact]
    public async Task EnsureAsync_uses_updated_subscription_tier_after_downgrade()
    {
        // Seed Enterprise subscription, then update to Free; required=Team → TierIneligible.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        var (orgId, _) = await SeedOrgAsync(scope.Db);

        await SeedSubscriptionAsync(scope.Db, orgId, SubscriptionTier.Enterprise);

        // Downgrade the subscription in-place.
        var sub = await scope.Db.Subscriptions.FirstAsync(s => s.OrgId == orgId);
        sub.Tier = SubscriptionTier.Free;
        sub.UpdatedAt = DateTime.UtcNow;
        await scope.Db.SaveChangesAsync();

        var gate = new TierGate(scope.Db);
        var result = await gate.EnsureAsync(orgId, SubscriptionTier.Team, CancellationToken.None);

        result.Should().Be(TierGateResult.TierIneligible,
            because: "the downgraded Free subscription makes the org ineligible for Team");
    }

    [Fact]
    public async Task EnsureAsync_uses_most_recent_subscription_row_when_multiple_exist()
    {
        // Two distinct subscription rows: older=Enterprise, newer=Free.
        // The production schema enforces a unique OrgId index, but history / migration scenarios
        // (e.g. a data-fix or index dropped during migration) could allow two rows.
        // OrderByDescending(s => s.UpdatedAt) must pick the newer Free row → TierIneligible.
        // We drop the unique index and use ExecuteSqlRawAsync with safe inline literals (test data only).
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        var (orgId, _) = await SeedOrgAsync(scope.Db);

        var orgIdStr       = orgId.ToString();
        var olderUpdatedAt = DateTime.UtcNow.AddDays(-2).ToString("yyyy-MM-dd HH:mm:ss");
        var newerUpdatedAt = DateTime.UtcNow.AddDays(-1).ToString("yyyy-MM-dd HH:mm:ss");
        var periodEnd      = DateTime.UtcNow.AddDays(30).ToString("yyyy-MM-dd HH:mm:ss");
        var olderId        = Guid.NewGuid().ToString();
        var newerId        = Guid.NewGuid().ToString();

        // Drop the unique index so two subscription rows for the same org are permitted.
        // Temporarily disable FK checks so raw-SQL inserts aren't blocked.
#pragma warning disable EF1002 // Safe: all values are test-controlled literals, never user input.
        await scope.Db.Database.ExecuteSqlRawAsync("PRAGMA foreign_keys = OFF");
        await scope.Db.Database.ExecuteSqlRawAsync(
            "DROP INDEX IF EXISTS \"IX_subscriptions_OrgId\"");

        // Column names are PascalCase (EF default — no HasColumnName overrides on Subscription).
        // Older row: Enterprise tier.
        await scope.Db.Database.ExecuteSqlRawAsync(
            $"""
            INSERT INTO subscriptions (
                Id, OrgId, Tier, Status, SeatCount, SeatLimit,
                CurrentPeriodStart, CurrentPeriodEnd, CancelAtPeriodEnd,
                CreatedAt, UpdatedAt, Interval
            ) VALUES (
                '{olderId}', '{orgIdStr}', 'Enterprise', 'Active', 1, 25,
                '{olderUpdatedAt}', '{periodEnd}', 0,
                '{olderUpdatedAt}', '{olderUpdatedAt}',
                'monthly'
            )
            """);

        // Newer row: Free tier (higher UpdatedAt — should win).
        await scope.Db.Database.ExecuteSqlRawAsync(
            $"""
            INSERT INTO subscriptions (
                Id, OrgId, Tier, Status, SeatCount, SeatLimit,
                CurrentPeriodStart, CurrentPeriodEnd, CancelAtPeriodEnd,
                CreatedAt, UpdatedAt, Interval
            ) VALUES (
                '{newerId}', '{orgIdStr}', 'Free', 'Active', 1, 25,
                '{newerUpdatedAt}', '{periodEnd}', 0,
                '{newerUpdatedAt}', '{newerUpdatedAt}',
                'monthly'
            )
            """);

        await scope.Db.Database.ExecuteSqlRawAsync("PRAGMA foreign_keys = ON");
#pragma warning restore EF1002

        var gate = new TierGate(scope.Db);
        var result = await gate.EnsureAsync(orgId, SubscriptionTier.Team, CancellationToken.None);

        result.Should().Be(TierGateResult.TierIneligible,
            because: "the newer Free row should win over the older Enterprise row via OrderByDescending(s.UpdatedAt)");
    }

    [Fact]
    public async Task EnsureAsync_propagates_cancellation()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        var (orgId, _) = await SeedOrgAsync(scope.Db);

        var gate = new TierGate(scope.Db);
        using var cts = new CancellationTokenSource();
        cts.Cancel();

        await Assert.ThrowsAnyAsync<OperationCanceledException>(
            () => gate.EnsureAsync(orgId, SubscriptionTier.Team, cts.Token));
    }
}
