using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Licensing.Trials;

public sealed class DatabaseTrialStateResolverTests
{
    [Fact]
    public async Task No_rows_returns_none()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var sut = new DatabaseTrialStateResolver(scope.Db, TimeProvider.System);
        var result = await sut.ResolveAsync(userId, tier: "free");

        result.TrialState.Should().Be("none");
        result.TrialExpiryUnixSeconds.Should().BeNull();
        result.TrialingFeatures.Should().BeEmpty();
    }

    [Fact]
    public async Task Active_full_initial_rows_return_active_with_earliest_expiry()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);
        var now = DateTime.UtcNow;

        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.FullInitial,
            grantedAt: now, expiresAt: now.AddDays(14)));
        scope.Db.Trials.Add(NewRow(userId, "schedules", TrialKind.FullInitial,
            grantedAt: now, expiresAt: now.AddDays(13)));
        await scope.Db.SaveChangesAsync();

        var sut = new DatabaseTrialStateResolver(scope.Db, TimeProvider.System);
        var result = await sut.ResolveAsync(userId, tier: "free");

        result.TrialState.Should().Be("active");
        var expected = new DateTimeOffset(now.AddDays(13), TimeSpan.Zero).ToUnixTimeSeconds();
        result.TrialExpiryUnixSeconds.Should().NotBeNull();
        result.TrialExpiryUnixSeconds!.Value.Should().BeInRange(expected - 2, expected + 2);
        result.TrialingFeatures.Should().BeEquivalentTo(["vault_provider_profiles", "schedules"]);
    }

    [Fact]
    public async Task All_full_initial_rows_expired_returns_expired_when_no_ondemand()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);
        var now = DateTime.UtcNow;

        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.FullInitial,
            grantedAt: now.AddDays(-30), expiresAt: now.AddDays(-16)));
        await scope.Db.SaveChangesAsync();

        var sut = new DatabaseTrialStateResolver(scope.Db, TimeProvider.System);
        var result = await sut.ResolveAsync(userId, tier: "free");

        result.TrialState.Should().Be("expired");
        result.TrialExpiryUnixSeconds.Should().BeNull();
        result.TrialingFeatures.Should().BeEmpty();
    }

    [Fact]
    public async Task Active_ondemand_row_returns_active_and_includes_feature()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);
        var now = DateTime.UtcNow;

        // Expired full_initial; live on-demand.
        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.FullInitial,
            grantedAt: now.AddDays(-30), expiresAt: now.AddDays(-16)));
        scope.Db.Trials.Add(NewRow(userId, "schedules", TrialKind.OnDemand,
            grantedAt: now, expiresAt: now.AddDays(7)));
        await scope.Db.SaveChangesAsync();

        var sut = new DatabaseTrialStateResolver(scope.Db, TimeProvider.System);
        var result = await sut.ResolveAsync(userId, tier: "free");

        result.TrialState.Should().Be("active");
        result.TrialingFeatures.Should().BeEquivalentTo(["schedules"]);
    }

    [Theory]
    [InlineData("professional")]
    [InlineData("team")]
    [InlineData("enterprise")]
    public async Task Active_subscription_overrides_to_none_regardless_of_rows(string tier)
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);
        var now = DateTime.UtcNow;

        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.FullInitial,
            grantedAt: now, expiresAt: now.AddDays(14)));
        await scope.Db.SaveChangesAsync();

        var sut = new DatabaseTrialStateResolver(scope.Db, TimeProvider.System);
        var result = await sut.ResolveAsync(userId, tier: tier);

        result.TrialState.Should().Be("none");
        result.TrialExpiryUnixSeconds.Should().BeNull();
        result.TrialingFeatures.Should().BeEmpty();
    }

    [Fact]
    public async Task Preempted_rows_are_not_counted_as_active()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);
        var now = DateTime.UtcNow;

        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.PreemptedBySubscription,
            grantedAt: now.AddDays(-3), expiresAt: now.AddDays(-1)));
        await scope.Db.SaveChangesAsync();

        var sut = new DatabaseTrialStateResolver(scope.Db, TimeProvider.System);
        var result = await sut.ResolveAsync(userId, tier: "free");

        result.TrialState.Should().Be("expired",
            because: "preempted rows still count as 'rows exist' but are not active");
    }

    // ── helpers ──────────────────────────────────────────────────────────────
    private static async Task<Guid> SeedUser(AppDbContext db)
    {
        var u = new User
        {
            Id = Guid.NewGuid(),
            Email = $"u-{Guid.NewGuid():N}@example.com",
            CreatedAt = DateTime.UtcNow,
        };
        db.Users.Add(u);
        await db.SaveChangesAsync();
        return u.Id;
    }

    private static Trial NewRow(Guid userId, string feature, TrialKind kind,
        DateTime grantedAt, DateTime expiresAt) => new()
    {
        Id = Guid.NewGuid(),
        UserId = userId,
        Feature = feature,
        Kind = kind,
        GrantedAt = grantedAt,
        ExpiresAt = expiresAt,
        CreatedAt = DateTime.UtcNow,
        UpdatedAt = DateTime.UtcNow,
    };
}
