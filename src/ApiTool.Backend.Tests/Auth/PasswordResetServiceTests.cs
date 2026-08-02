// Unit tests for PasswordResetService using SQLite-in-memory + FakeClock.
using ApiTool.Backend.Auth;
using ApiTool.Backend.Auth.Refresh;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Auth;

public sealed class PasswordResetServiceTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly RecordingEmailQueue _emailQueue;
    private readonly FakeClock _clock;
    private readonly PasswordResetService _sut;

    public PasswordResetServiceTests()
    {
        _scope      = TestDb.CreateOpen();
        _emailQueue = new RecordingEmailQueue();
        _clock      = new FakeClock(new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero));

        var hasher    = new PasswordHasher();
        var checker   = new ZxcvbnPasswordStrengthChecker();
        var refreshSvc = new RefreshTokenService(
            _scope.Db, _emailQueue, _clock,
            NullLogger<RefreshTokenService>.Instance);
        var appOptions = Options.Create(new AppOptions { WebAppUrl = "https://app.example.com" });

        _sut = new PasswordResetService(
            _scope.Db, hasher, checker, refreshSvc,
            _emailQueue, appOptions, _clock,
            NullLogger<PasswordResetService>.Instance);
    }

    public async ValueTask DisposeAsync() => await _scope.DisposeAsync();

    // ── Setup helpers ─────────────────────────────────────────────────────────

    private async Task<ApiTool.Backend.Data.Entities.User> SeedUserAsync(string email = "user@example.com")
    {
        await _scope.Db.Database.MigrateAsync();
        var hasher = new PasswordHasher();
        var user = new ApiTool.Backend.Data.Entities.User
        {
            Id = Guid.NewGuid(),
            Email = email,
            CreatedAt = _clock.GetUtcNow().UtcDateTime,
            PasswordHash = hasher.Hash("OriginalPass1!"),
        };
        _scope.Db.Users.Add(user);
        await _scope.Db.SaveChangesAsync();
        return user;
    }

    // ── RequestAsync ──────────────────────────────────────────────────────────

    [Fact]
    public async Task RequestAsync_unknown_email_does_not_create_a_row_or_enqueue_email()
    {
        await _scope.Db.Database.MigrateAsync();

        await _sut.RequestAsync("nobody@example.com", null, null, CancellationToken.None);

        _scope.Db.PasswordResetTokens.Should().BeEmpty();
        _emailQueue.Messages.Should().BeEmpty();
    }

    [Fact]
    public async Task RequestAsync_known_email_creates_token_row_with_30min_expiry_and_enqueues_password_reset_email()
    {
        var user = await SeedUserAsync();

        await _sut.RequestAsync(user.Email, null, null, CancellationToken.None);

        var token = _scope.Db.PasswordResetTokens.Single();
        token.UserId.Should().Be(user.Id);
        token.IssuedAt.Should().Be(_clock.GetUtcNow().UtcDateTime);
        token.ExpiresAt.Should().Be(_clock.GetUtcNow().UtcDateTime.AddMinutes(30));
        token.ConsumedAt.Should().BeNull();
        token.RevokedAt.Should().BeNull();

        _emailQueue.Messages.Should().ContainSingle(m => m.TemplateSlug == "password_reset");
        var email = _emailQueue.Messages.Single();
        email.To.Should().Be(user.Email);
        email.Variables["reset_url"].Should().Contain("prst_");
        email.Variables["reset_url"].Should().StartWith("https://app.example.com");
    }

    [Fact]
    public async Task RequestAsync_token_row_carries_requester_ip_and_ua()
    {
        var user = await SeedUserAsync();

        await _sut.RequestAsync(user.Email, "1.2.3.4", "TestAgent/1.0", CancellationToken.None);

        var token = _scope.Db.PasswordResetTokens.Single();
        token.RequesterIp.Should().Be("1.2.3.4");
        token.RequesterUa.Should().Be("TestAgent/1.0");
    }

    [Fact]
    public async Task RequestAsync_fourth_request_within_24h_is_silently_rate_limited()
    {
        var user = await SeedUserAsync();

        // Make 3 successful requests
        for (var i = 0; i < 3; i++)
        {
            _emailQueue.Clear();
            await _sut.RequestAsync(user.Email, null, null, CancellationToken.None);
        }

        _emailQueue.Clear();
        // 4th request — should be silently rate limited
        await _sut.RequestAsync(user.Email, null, null, CancellationToken.None);

        _emailQueue.Messages.Should().BeEmpty();
        _scope.Db.PasswordResetTokens.Count().Should().Be(3);
    }

    [Fact]
    public async Task RequestAsync_rate_limit_resets_after_24h_window_passes()
    {
        var user = await SeedUserAsync();

        // Make 3 requests exhausting the rate limit
        for (var i = 0; i < 3; i++)
            await _sut.RequestAsync(user.Email, null, null, CancellationToken.None);

        _emailQueue.Clear();

        // Advance clock 25 hours so the window resets
        _clock.Advance(TimeSpan.FromHours(25));

        await _sut.RequestAsync(user.Email, null, null, CancellationToken.None);

        _emailQueue.Messages.Should().ContainSingle(m => m.TemplateSlug == "password_reset");
    }

    // ── ConfirmAsync ──────────────────────────────────────────────────────────

    private async Task<string> SeedResetTokenAsync(ApiTool.Backend.Data.Entities.User user)
    {
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix);
        var token = new PasswordResetToken
        {
            Id        = Guid.NewGuid(),
            UserId    = user.Id,
            TokenHash = hash,
            IssuedAt  = _clock.GetUtcNow().UtcDateTime,
            ExpiresAt = _clock.GetUtcNow().UtcDateTime.AddMinutes(30),
        };
        _scope.Db.PasswordResetTokens.Add(token);
        await _scope.Db.SaveChangesAsync();
        return plaintext;
    }

    [Fact]
    public async Task ConfirmAsync_with_valid_token_and_strong_password_returns_None_and_updates_user_password_hash()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedResetTokenAsync(user);
        var oldHash   = user.PasswordHash;

        var (err, score) = await _sut.ConfirmAsync(plaintext, "correct horse battery staple", CancellationToken.None);

        err.Should().Be(PasswordResetError.None);
        score.Should().BeNull();

        _scope.Db.ChangeTracker.Clear();
        var updated = await _scope.Db.Users.FindAsync(user.Id);
        updated!.PasswordHash.Should().NotBe(oldHash);

        var tokenRow = _scope.Db.PasswordResetTokens.Single();
        tokenRow.ConsumedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task ConfirmAsync_revokes_every_refresh_family_for_user_with_reason_password_reset()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedResetTokenAsync(user);

        // Seed a refresh token
        var rt = new RefreshToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, DeviceId = Guid.NewGuid(), FamilyId = Guid.NewGuid(),
            TokenHash = [10, 20, 30, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32],
            IssuedAt = _clock.GetUtcNow().UtcDateTime.AddDays(-1),
            ExpiresAt = _clock.GetUtcNow().UtcDateTime.AddDays(89),
        };
        _scope.Db.RefreshTokens.Add(rt);
        await _scope.Db.SaveChangesAsync();

        await _sut.ConfirmAsync(plaintext, "correct horse battery staple", CancellationToken.None);

        _scope.Db.ChangeTracker.Clear();
        var revoked = await _scope.Db.RefreshTokens.FindAsync(rt.Id);
        revoked!.RevokedAt.Should().NotBeNull();
        revoked.RevokeReason.Should().Be("password_reset");
    }

    [Fact]
    public async Task ConfirmAsync_with_weak_password_returns_PasswordTooWeak_with_score()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedResetTokenAsync(user);

        var (err, score) = await _sut.ConfirmAsync(plaintext, "password", CancellationToken.None);

        err.Should().Be(PasswordResetError.PasswordTooWeak);
        score.Should().NotBeNull();
        score!.Value.Should().BeLessThan(3);
    }

    [Fact]
    public async Task ConfirmAsync_does_not_mutate_password_when_score_below_three()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedResetTokenAsync(user);
        var originalHash = user.PasswordHash;

        await _sut.ConfirmAsync(plaintext, "password", CancellationToken.None);

        _scope.Db.ChangeTracker.Clear();
        var unchanged = await _scope.Db.Users.FindAsync(user.Id);
        unchanged!.PasswordHash.Should().Be(originalHash);
    }

    [Fact]
    public async Task ConfirmAsync_with_unknown_token_returns_TokenInvalid()
    {
        await _scope.Db.Database.MigrateAsync();

        var fakeToken = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix).Plaintext;
        var (err, _) = await _sut.ConfirmAsync(fakeToken, "correct horse battery staple", CancellationToken.None);

        err.Should().Be(PasswordResetError.TokenInvalid);
    }

    [Fact]
    public async Task ConfirmAsync_with_expired_token_returns_TokenInvalid()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedResetTokenAsync(user);

        // Advance clock past 30-minute lifetime
        _clock.Advance(TimeSpan.FromMinutes(31));

        var (err, _) = await _sut.ConfirmAsync(plaintext, "correct horse battery staple", CancellationToken.None);

        err.Should().Be(PasswordResetError.TokenInvalid);
    }

    [Fact]
    public async Task ConfirmAsync_with_already_consumed_token_returns_TokenInvalid()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedResetTokenAsync(user);

        // Consume the token
        await _sut.ConfirmAsync(plaintext, "correct horse battery staple", CancellationToken.None);

        // Try to use it again
        var (err, _) = await _sut.ConfirmAsync(plaintext, "correct horse battery staple", CancellationToken.None);

        err.Should().Be(PasswordResetError.TokenInvalid);
    }

    [Fact]
    public async Task ConfirmAsync_with_revoked_token_returns_TokenInvalid()
    {
        var user = await SeedUserAsync();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix);
        var token = new PasswordResetToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
            IssuedAt = _clock.GetUtcNow().UtcDateTime,
            ExpiresAt = _clock.GetUtcNow().UtcDateTime.AddMinutes(30),
            RevokedAt = _clock.GetUtcNow().UtcDateTime.AddMinutes(-1),
        };
        _scope.Db.PasswordResetTokens.Add(token);
        await _scope.Db.SaveChangesAsync();

        var (err, _) = await _sut.ConfirmAsync(plaintext, "correct horse battery staple", CancellationToken.None);

        err.Should().Be(PasswordResetError.TokenInvalid);
    }

    [Fact]
    public async Task ConfirmAsync_token_without_prst_prefix_returns_TokenInvalid()
    {
        await _scope.Db.Database.MigrateAsync();

        var (err, _) = await _sut.ConfirmAsync("evtk_somethingsomething", "correct horse battery staple", CancellationToken.None);

        err.Should().Be(PasswordResetError.TokenInvalid);
    }
}
