using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Licensing.Trials;

/// <summary>Unit tests for <see cref="TrialsService"/>.</summary>
public sealed class TrialsServiceTests
{
    [Fact]
    public async Task ActivateOnDemandAsync_grants_when_no_prior_row()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        await SeedUser(scope.Db, userId);

        var sut = new TrialsService(scope.Db, TimeProvider.System,
            NullLogger<TrialsService>.Instance);

        var result = await sut.ActivateOnDemandAsync(userId, "vault_provider_profiles", default);

        result.Should().BeOfType<TrialActivationResult.Granted>();
        var granted = (TrialActivationResult.Granted)result;
        granted.Feature.Should().Be("vault_provider_profiles");
        granted.ExpiresAt.Should().BeCloseTo(granted.GrantedAt + TimeSpan.FromDays(7), precision: TimeSpan.FromSeconds(5));

        // Row should exist in DB with kind=OnDemand.
        var row = scope.Db.Trials.Single(t => t.UserId == userId && t.Feature == "vault_provider_profiles");
        row.Kind.Should().Be(TrialKind.OnDemand);
    }

    [Fact]
    public async Task ActivateOnDemandAsync_returns_AlreadyConsumed_when_full_initial_row_exists()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        await SeedUser(scope.Db, userId);
        await SeedTrialRow(scope.Db, userId, "vault_provider_profiles", TrialKind.FullInitial);

        var sut = new TrialsService(scope.Db, TimeProvider.System,
            NullLogger<TrialsService>.Instance);

        var result = await sut.ActivateOnDemandAsync(userId, "vault_provider_profiles", default);

        result.Should().BeOfType<TrialActivationResult.AlreadyConsumed>();
        var consumed = (TrialActivationResult.AlreadyConsumed)result;
        consumed.Kind.Should().Be(TrialKind.FullInitial);
    }

    [Fact]
    public async Task ActivateOnDemandAsync_returns_AlreadyConsumed_when_ondemand_row_exists()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        await SeedUser(scope.Db, userId);
        await SeedTrialRow(scope.Db, userId, "vault_provider_profiles", TrialKind.OnDemand);

        var sut = new TrialsService(scope.Db, TimeProvider.System,
            NullLogger<TrialsService>.Instance);

        var result = await sut.ActivateOnDemandAsync(userId, "vault_provider_profiles", default);

        result.Should().BeOfType<TrialActivationResult.AlreadyConsumed>();
        var consumed = (TrialActivationResult.AlreadyConsumed)result;
        consumed.Kind.Should().Be(TrialKind.OnDemand);
    }

    [Fact]
    public async Task ActivateOnDemandAsync_returns_UnknownFeature_when_feature_not_in_registry()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        await SeedUser(scope.Db, userId);

        var sut = new TrialsService(scope.Db, TimeProvider.System,
            NullLogger<TrialsService>.Instance);

        var result = await sut.ActivateOnDemandAsync(userId, "made_up_feature", default);

        result.Should().BeOfType<TrialActivationResult.UnknownFeature>();
    }

    [Fact]
    public async Task ActivateOnDemandAsync_does_not_insert_when_unknown_feature()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        await SeedUser(scope.Db, userId);

        var sut = new TrialsService(scope.Db, TimeProvider.System,
            NullLogger<TrialsService>.Instance);

        _ = await sut.ActivateOnDemandAsync(userId, "made_up_feature", default);

        scope.Db.Trials.Any(t => t.UserId == userId).Should().BeFalse();
    }

    // ── helpers ──────────────────────────────────────────────────────────────

    private static async Task SeedUser(ApiTool.Backend.Data.AppDbContext db, Guid userId)
    {
        db.Users.Add(new User
        {
            Id        = userId,
            Email     = $"user-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
    }

    private static async Task SeedTrialRow(
        ApiTool.Backend.Data.AppDbContext db, Guid userId, string feature, TrialKind kind)
    {
        var now = DateTime.UtcNow;
        db.Trials.Add(new Trial
        {
            Id        = Guid.NewGuid(),
            UserId    = userId,
            Feature   = feature,
            Kind      = kind,
            GrantedAt = now,
            ExpiresAt = now.AddDays(kind == TrialKind.FullInitial ? 14 : 7),
            CreatedAt = now,
            UpdatedAt = now,
        });
        await db.SaveChangesAsync();
    }
}
