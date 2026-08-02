using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>
/// Unit tests for <see cref="DeletionReauthService"/>.
/// Uses SQLite in-memory for realistic EF Core behaviour.
/// </summary>
public sealed class DeletionReauthServiceTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly FakeClock _clock;

    private static readonly DateTimeOffset Now = new DateTimeOffset(2026, 5, 18, 12, 0, 0, TimeSpan.Zero);
    private const string ValidPassword = "Hunter2IsNotAPassword!";

    public DeletionReauthServiceTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        _clock = new FakeClock(Now);

        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    public async ValueTask DisposeAsync()
    {
        await _conn.DisposeAsync();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    private DeletionReauthService BuildService(AppDbContext db)
    {
        var hasher = new PasswordHasher();
        return new DeletionReauthService(db, hasher, _clock);
    }

    private async Task<User> SeedUserAsync(AppDbContext db, string? password = ValidPassword)
    {
        var hasher = new PasswordHasher();
        var user = new User
        {
            Id = Guid.NewGuid(),
            Email = $"drto-{Guid.NewGuid():N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PasswordHash = password is null ? null : hasher.Hash(password),
        };
        db.Users.Add(user);
        await db.SaveChangesAsync();
        return user;
    }

    [Fact]
    public async Task IssueAsync_with_correct_password_returns_token_starting_with_drto()
    {
        using var scope = OpenScope();
        var user = await SeedUserAsync(scope.Db);
        var svc = BuildService(scope.Db);

        var (error, token, expiresAt) = await svc.IssueAsync(user.Id, ValidPassword, default);

        error.Should().Be(ReauthError.None);
        token.Should().NotBeNull();
        token!.Should().StartWith("drto_");
        expiresAt.Should().NotBeNull();
        expiresAt!.Value.Should().BeCloseTo(Now.UtcDateTime.AddMinutes(5), TimeSpan.FromSeconds(5));
    }

    [Fact]
    public async Task IssueAsync_with_wrong_password_returns_WrongPassword()
    {
        using var scope = OpenScope();
        var user = await SeedUserAsync(scope.Db);
        var svc = BuildService(scope.Db);

        var (error, token, _) = await svc.IssueAsync(user.Id, "WrongPass!!", default);

        error.Should().Be(ReauthError.WrongPassword);
        token.Should().BeNull();
    }

    [Fact]
    public async Task IssueAsync_user_without_password_returns_WrongPassword()
    {
        using var scope = OpenScope();
        var user = await SeedUserAsync(scope.Db, password: null);
        var svc = BuildService(scope.Db);

        var (error, token, _) = await svc.IssueAsync(user.Id, ValidPassword, default);

        error.Should().Be(ReauthError.WrongPassword);
        token.Should().BeNull();
    }

    [Fact]
    public async Task ConsumeAsync_with_fresh_token_returns_None_and_marks_consumed()
    {
        using var scope = OpenScope();
        var user = await SeedUserAsync(scope.Db);
        var svc = BuildService(scope.Db);

        var (_, rawToken, _) = await svc.IssueAsync(user.Id, ValidPassword, default);
        var error = await svc.ConsumeAsync(user.Id, rawToken!, default);

        error.Should().Be(ReauthError.None);

        // Verify row is consumed
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.DeletionReauthTokens
            .FirstOrDefaultAsync(r => r.UserId == user.Id);
        row.Should().NotBeNull();
        row!.ConsumedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task ConsumeAsync_with_already_consumed_token_returns_TokenConsumed()
    {
        using var scope = OpenScope();
        var user = await SeedUserAsync(scope.Db);
        var svc = BuildService(scope.Db);

        var (_, rawToken, _) = await svc.IssueAsync(user.Id, ValidPassword, default);
        await svc.ConsumeAsync(user.Id, rawToken!, default); // first consume

        scope.Db.ChangeTracker.Clear();
        var error = await svc.ConsumeAsync(user.Id, rawToken!, default); // second consume

        error.Should().Be(ReauthError.TokenConsumed);
    }

    [Fact]
    public async Task ConsumeAsync_with_expired_token_returns_TokenExpired()
    {
        using var scope = OpenScope();
        var user = await SeedUserAsync(scope.Db);
        var svc = BuildService(scope.Db);

        var (_, rawToken, _) = await svc.IssueAsync(user.Id, ValidPassword, default);

        // Advance clock past 5-minute TTL
        _clock.Advance(TimeSpan.FromMinutes(6));

        scope.Db.ChangeTracker.Clear();
        var error = await svc.ConsumeAsync(user.Id, rawToken!, default);

        error.Should().Be(ReauthError.TokenExpired);
    }

    [Fact]
    public async Task ConsumeAsync_with_bogus_token_returns_TokenInvalid()
    {
        using var scope = OpenScope();
        var user = await SeedUserAsync(scope.Db);
        var svc = BuildService(scope.Db);

        var error = await svc.ConsumeAsync(user.Id, "drto_notarealtoken", default);

        error.Should().Be(ReauthError.TokenInvalid);
    }

    [Fact]
    public async Task ConsumeAsync_with_other_users_token_returns_TokenInvalid()
    {
        using var scope = OpenScope();
        var userA = await SeedUserAsync(scope.Db);
        var userB = await SeedUserAsync(scope.Db);
        var svc = BuildService(scope.Db);

        var (_, tokenForA, _) = await svc.IssueAsync(userA.Id, ValidPassword, default);

        // userB tries to use userA's token
        scope.Db.ChangeTracker.Clear();
        var error = await svc.ConsumeAsync(userB.Id, tokenForA!, default);

        error.Should().Be(ReauthError.TokenInvalid);
    }
}
