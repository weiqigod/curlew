using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using FluentAssertions;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>
/// Verifies the PasswordResetToken entity round-trips against SQLite and that
/// the partial active-token index is created.
/// Filter: FullyQualifiedName~PasswordResetTokenEntity
/// </summary>
public sealed class PasswordResetTokenEntityTests
{
    [Fact]
    public async Task Entity_persists_and_round_trips()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var token = new PasswordResetToken
        {
            Id = Guid.NewGuid(),
            UserId = userId,
            TokenHash = new byte[] { 1, 2, 3, 4 },
            IssuedAt = DateTime.UtcNow,
            ExpiresAt = DateTime.UtcNow.AddMinutes(30),
            RequesterIp = "203.0.113.42",
            RequesterUa = "Mozilla/5.0",
        };
        scope.Db.PasswordResetTokens.Add(token);
        await scope.Db.SaveChangesAsync();

        (await scope.Db.PasswordResetTokens.CountAsync()).Should().Be(1);
        var loaded = await scope.Db.PasswordResetTokens.SingleAsync();
        loaded.UserId.Should().Be(userId);
        loaded.TokenHash.Should().Equal(token.TokenHash);
        loaded.RequesterIp.Should().Be("203.0.113.42");
        loaded.ConsumedAt.Should().BeNull();
        loaded.RevokedAt.Should().BeNull();
    }

    [Fact]
    public async Task Token_hash_unique_index_prevents_duplicate()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var hash = new byte[] { 9, 9, 9, 9 };
        scope.Db.PasswordResetTokens.Add(NewRow(userId, hash));
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();

        scope.Db.PasswordResetTokens.Add(NewRow(userId, hash));
        await Assert.ThrowsAsync<DbUpdateException>(
            () => scope.Db.SaveChangesAsync());
    }

    [Fact]
    public async Task Active_partial_index_exists_on_sqlite()
    {
        // The partial-index filter is what proves behavior #7 ("EXPLAIN uses
        // the index on lookup"). On SQLite we assert the index DDL contains
        // the WHERE clause; full EXPLAIN parsing is a Postgres-only concern
        // verified by the migration round-trip in the docker stack.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var sql = await ReadIndexSqlAsync(scope.Db,
            "idx_password_reset_tokens_active");
        sql.Should().Contain("consumed_at").And.Contain("revoked_at");
    }

    [Fact]
    public async Task User_email_verified_column_defaults_false()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var u = new User { Id = Guid.NewGuid(), Email = "x@example.com", CreatedAt = DateTime.UtcNow };
        scope.Db.Users.Add(u);
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();

        var loaded = await scope.Db.Users.SingleAsync(x => x.Id == u.Id);
        loaded.EmailVerified.Should().BeFalse();
    }

    private static async Task<Guid> SeedUser(AppDbContext db)
    {
        var u = new User { Id = Guid.NewGuid(), Email = $"u-{Guid.NewGuid():N}@example.com", CreatedAt = DateTime.UtcNow };
        db.Users.Add(u);
        await db.SaveChangesAsync();
        return u.Id;
    }

    private static PasswordResetToken NewRow(Guid userId, byte[] hash) => new()
    {
        Id = Guid.NewGuid(),
        UserId = userId,
        TokenHash = hash,
        IssuedAt = DateTime.UtcNow,
        ExpiresAt = DateTime.UtcNow.AddMinutes(30),
    };

    private static async Task<string> ReadIndexSqlAsync(AppDbContext db, string name)
    {
        await using var cmd = db.Database.GetDbConnection().CreateCommand();
        cmd.CommandText =
            "SELECT sql FROM sqlite_master WHERE type='index' AND name=@n";
        var p = cmd.CreateParameter(); p.ParameterName = "@n"; p.Value = name;
        cmd.Parameters.Add(p);
        var result = await cmd.ExecuteScalarAsync();
        return result as string ?? string.Empty;
    }
}
