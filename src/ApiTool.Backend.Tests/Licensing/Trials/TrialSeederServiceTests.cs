using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Licensing.Trials;

public sealed class TrialSeederServiceTests
{
    [Fact]
    public async Task Seeds_one_row_per_known_feature_for_new_user()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);
        var grantedAt = DateTime.UtcNow;

        var sut = new TrialSeederService(scope.Db, TimeProvider.System,
            NullLogger<TrialSeederService>.Instance);
        var count = await sut.SeedFullInitialAsync(userId, grantedAt);

        count.Should().Be(TrialFeatures.All.Count);
        var rows = await scope.Db.Trials.Where(t => t.UserId == userId).ToListAsync();
        rows.Should().HaveCount(TrialFeatures.All.Count);
        rows.Select(r => r.Feature).Should().BeEquivalentTo(TrialFeatures.All);
        rows.Should().OnlyContain(r => r.Kind == TrialKind.FullInitial);
        rows.Should().OnlyContain(r => r.GrantedAt == grantedAt);
        rows.Should().OnlyContain(r => r.ExpiresAt == grantedAt.AddDays(14));
    }

    [Fact]
    public async Task Re_invocation_on_seeded_user_is_a_noop()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var sut = new TrialSeederService(scope.Db, TimeProvider.System,
            NullLogger<TrialSeederService>.Instance);
        await sut.SeedFullInitialAsync(userId, DateTime.UtcNow);
        var second = await sut.SeedFullInitialAsync(userId, DateTime.UtcNow);

        second.Should().Be(0);
        (await scope.Db.Trials.CountAsync(t => t.UserId == userId))
            .Should().Be(TrialFeatures.All.Count);
    }

    [Fact]
    public async Task Concurrent_seeds_for_same_user_yield_one_seeded_set()
    {
        // The UNIQUE(user_id, feature) constraint guards against double-insertion
        // in the race window. The catch+clear pattern keeps the call safe.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var sut = new TrialSeederService(scope.Db, TimeProvider.System,
            NullLogger<TrialSeederService>.Instance);

        // Pre-insert one row to simulate the race-loser arriving second.
        scope.Db.Trials.Add(new Trial
        {
            Id = Guid.NewGuid(), UserId = userId, Feature = TrialFeatures.All[0],
            Kind = TrialKind.FullInitial, GrantedAt = DateTime.UtcNow, ExpiresAt = DateTime.UtcNow.AddDays(14),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        var count = await sut.SeedFullInitialAsync(userId, DateTime.UtcNow);
        count.Should().Be(0, because: "AnyAsync sees the pre-existing row and short-circuits");
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
}
