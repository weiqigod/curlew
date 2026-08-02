using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using FluentAssertions;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>
/// Verifies the EmailVerificationToken entity round-trips against SQLite and that
/// the partial active-token index is created.
/// Filter: FullyQualifiedName~EmailVerificationTokenEntity
/// </summary>
public sealed class EmailVerificationTokenEntityTests
{
    [Fact]
    public async Task Entity_persists_and_round_trips()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var token = new EmailVerificationToken
        {
            Id = Guid.NewGuid(),
            UserId = userId,
            TokenHash = new byte[] { 5, 6, 7, 8 },
            IssuedAt = DateTime.UtcNow,
            ExpiresAt = DateTime.UtcNow.AddHours(24),
        };
        scope.Db.EmailVerificationTokens.Add(token);
        await scope.Db.SaveChangesAsync();

        (await scope.Db.EmailVerificationTokens.CountAsync()).Should().Be(1);
        var loaded = await scope.Db.EmailVerificationTokens.SingleAsync();
        loaded.UserId.Should().Be(userId);
        loaded.TokenHash.Should().Equal(token.TokenHash);
        loaded.ConsumedAt.Should().BeNull();
        loaded.RevokedAt.Should().BeNull();
    }

    [Fact]
    public async Task Token_hash_unique_index_prevents_duplicate()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var userId = await SeedUser(scope.Db);

        var hash = new byte[] { 11, 22, 33, 44 };
        scope.Db.EmailVerificationTokens.Add(NewRow(userId, hash));
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();

        scope.Db.EmailVerificationTokens.Add(NewRow(userId, hash));
        await Assert.ThrowsAsync<DbUpdateException>(
            () => scope.Db.SaveChangesAsync());
    }

    [Fact]
    public async Task Active_partial_index_exists_on_sqlite()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var sql = await ReadIndexSqlAsync(scope.Db,
            "idx_email_verification_tokens_active");
        sql.Should().Contain("consumed_at").And.Contain("revoked_at");
    }

    private static async Task<Guid> SeedUser(AppDbContext db)
    {
        var u = new User { Id = Guid.NewGuid(), Email = $"v-{Guid.NewGuid():N}@example.com", CreatedAt = DateTime.UtcNow };
        db.Users.Add(u);
        await db.SaveChangesAsync();
        return u.Id;
    }

    private static EmailVerificationToken NewRow(Guid userId, byte[] hash) => new()
    {
        Id = Guid.NewGuid(),
        UserId = userId,
        TokenHash = hash,
        IssuedAt = DateTime.UtcNow,
        ExpiresAt = DateTime.UtcNow.AddHours(24),
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
