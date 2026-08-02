using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using FluentAssertions;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>
/// Verifies the Trial entity round-trips against SQLite, that UNIQUE (user_id,
/// feature) prevents a duplicate, that the kind CHECK constraint rejects bad
/// values, and that the partial cron indexes are present in the DDL.
/// Filter: FullyQualifiedName~TrialEntity
/// </summary>
public sealed class TrialEntityTests
{
    [Fact]
    public async Task Entity_persists_and_round_trips()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var trial = new Trial
        {
            Id = Guid.NewGuid(),
            UserId = userId,
            Feature = "vault_provider_profiles",
            Kind = TrialKind.OnDemand,
            GrantedAt = DateTime.UtcNow,
            ExpiresAt = DateTime.UtcNow.AddDays(7),
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };
        scope.Db.Trials.Add(trial);
        await scope.Db.SaveChangesAsync();

        scope.Db.ChangeTracker.Clear();
        var loaded = await scope.Db.Trials.SingleAsync();
        loaded.UserId.Should().Be(userId);
        loaded.Feature.Should().Be("vault_provider_profiles");
        loaded.Kind.Should().Be(TrialKind.OnDemand);
        loaded.ConsumedAt.Should().BeNull();
        loaded.Notified3DayAt.Should().BeNull();
        loaded.Notified1DayAt.Should().BeNull();
    }

    [Theory]
    [InlineData(TrialKind.FullInitial,             "full_initial")]
    [InlineData(TrialKind.OnDemand,                "ondemand")]
    [InlineData(TrialKind.PreemptedBySubscription, "preempted_by_subscription")]
    public async Task Kind_round_trips_via_string_converter(TrialKind kind, string expectedDb)
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        scope.Db.Trials.Add(NewRow(userId, $"feat_{kind}", kind));
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();

        // Round-trip via EF
        var loaded = await scope.Db.Trials.SingleAsync();
        loaded.Kind.Should().Be(kind);

        // Raw column value matches the spec literal
        var raw = await ReadKindRawAsync(scope.Db);
        raw.Should().Be(expectedDb);
    }

    [Fact]
    public async Task Duplicate_user_feature_pair_is_rejected()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.FullInitial));
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();

        // Same (user_id, feature) pair must fail.
        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.OnDemand));
        await Assert.ThrowsAsync<DbUpdateException>(
            () => scope.Db.SaveChangesAsync());
    }

    [Fact]
    public async Task Same_feature_for_different_users_is_allowed()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userA = await SeedUser(scope.Db);
        var userB = await SeedUser(scope.Db);

        scope.Db.Trials.Add(NewRow(userA, "schedules", TrialKind.FullInitial));
        scope.Db.Trials.Add(NewRow(userB, "schedules", TrialKind.FullInitial));
        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().NotThrowAsync();
    }

    [Fact]
    public async Task Different_features_for_same_user_are_allowed()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        scope.Db.Trials.Add(NewRow(userId, "schedules", TrialKind.FullInitial));
        scope.Db.Trials.Add(NewRow(userId, "vault_provider_profiles", TrialKind.FullInitial));
        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().NotThrowAsync();
    }

    [Fact]
    public async Task Kind_check_constraint_rejects_unknown_string()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        // Bypass EF to insert a raw bad value — only way to hit the DDL CHECK.
        await using var cmd = scope.Db.Database.GetDbConnection().CreateCommand();
        cmd.CommandText = """
            INSERT INTO trials (Id, user_id, feature, kind, granted_at, expires_at, created_at, updated_at)
            VALUES (@id, @uid, 'feat', 'bogus', @t, @t, @t, @t)
            """;
        AddParam(cmd, "@id", Guid.NewGuid().ToString());
        AddParam(cmd, "@uid", userId.ToString());
        AddParam(cmd, "@t", DateTime.UtcNow.ToString("O"));

        var act = () => cmd.ExecuteNonQueryAsync();
        await act.Should().ThrowAsync<Microsoft.Data.Sqlite.SqliteException>(
            because: "ck_trials_kind blocks values outside the spec enum");
    }

    [Fact]
    public async Task Notify_3day_partial_index_filters_on_notified_3day_null()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var sql = await ReadIndexSqlAsync(scope.Db, "idx_trials_notify_3day");
        sql.Should().Contain("notified_3day_at").And.Contain("consumed_at");
    }

    [Fact]
    public async Task Notify_1day_partial_index_filters_on_notified_1day_null()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var sql = await ReadIndexSqlAsync(scope.Db, "idx_trials_notify_1day");
        sql.Should().Contain("notified_1day_at").And.Contain("consumed_at");
    }

    [Fact]
    public async Task User_index_exists_for_per_user_lookups()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var sql = await ReadIndexSqlAsync(scope.Db, "idx_trials_user");
        sql.Should().NotBeNullOrEmpty();
    }

    [Fact]
    public async Task Down_migration_drops_table_cleanly()
    {
        // Apply, verify exists, then revert and verify gone.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        (await TableExistsAsync(scope.Db, "trials")).Should().BeTrue();

        // EF Core's MigrateAsync(targetMigration:) reverts to the named migration.
        // Pick the migration immediately preceding AddTrials.
        var migrator = scope.Db.GetInfrastructure().GetRequiredService<IMigrator>();
        await migrator.MigrateAsync("AddGitlabInstallationsAndEvents");

        (await TableExistsAsync(scope.Db, "trials")).Should().BeFalse();
    }

    // ── helpers ──────────────────────────────────────────────────────────

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

    private static Trial NewRow(Guid userId, string feature, TrialKind kind) => new()
    {
        Id = Guid.NewGuid(),
        UserId = userId,
        Feature = feature,
        Kind = kind,
        GrantedAt = DateTime.UtcNow,
        ExpiresAt = DateTime.UtcNow.AddDays(7),
        CreatedAt = DateTime.UtcNow,
        UpdatedAt = DateTime.UtcNow,
    };

    private static async Task<string> ReadIndexSqlAsync(AppDbContext db, string name)
    {
        await using var cmd = db.Database.GetDbConnection().CreateCommand();
        cmd.CommandText = "SELECT sql FROM sqlite_master WHERE type='index' AND name=@n";
        AddParam(cmd, "@n", name);
        return (await cmd.ExecuteScalarAsync()) as string ?? string.Empty;
    }

    private static async Task<string> ReadKindRawAsync(AppDbContext db)
    {
        await using var cmd = db.Database.GetDbConnection().CreateCommand();
        cmd.CommandText = "SELECT kind FROM trials";
        return (await cmd.ExecuteScalarAsync()) as string ?? string.Empty;
    }

    private static async Task<bool> TableExistsAsync(DbContext db, string name)
    {
        await using var cmd = db.Database.GetDbConnection().CreateCommand();
        cmd.CommandText = "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=@n";
        AddParam(cmd, "@n", name);
        return Convert.ToInt64(await cmd.ExecuteScalarAsync()) > 0;
    }

    private static void AddParam(System.Data.Common.DbCommand cmd, string name, object value)
    {
        var p = cmd.CreateParameter();
        p.ParameterName = name;
        p.Value = value;
        cmd.Parameters.Add(p);
    }
}
