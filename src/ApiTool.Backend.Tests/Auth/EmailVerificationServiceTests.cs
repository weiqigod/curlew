// Unit tests for EmailVerificationService using SQLite-in-memory + FakeClock.
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Auth;

public sealed class EmailVerificationServiceTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly RecordingEmailQueue _emailQueue;
    private readonly FakeClock _clock;
    private readonly EmailVerificationService _sut;

    public EmailVerificationServiceTests()
    {
        _scope      = TestDb.CreateOpen();
        _emailQueue = new RecordingEmailQueue();
        _clock      = new FakeClock(new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero));
        var appOptions = Options.Create(new AppOptions { WebAppUrl = "https://app.example.com" });

        _sut = new EmailVerificationService(
            _scope.Db, _emailQueue, appOptions, _clock,
            NullLogger<EmailVerificationService>.Instance);
    }

    public async ValueTask DisposeAsync() => await _scope.DisposeAsync();

    private async Task<User> SeedUserAsync(string? email = null, bool emailVerified = false)
    {
        await _scope.Db.Database.MigrateAsync();
        var user = new User
        {
            Id = Guid.NewGuid(),
            Email = email ?? $"user-{Guid.NewGuid():N}@example.com",
            CreatedAt = _clock.GetUtcNow().UtcDateTime,
            EmailVerified = emailVerified,
        };
        _scope.Db.Users.Add(user);
        await _scope.Db.SaveChangesAsync();
        return user;
    }

    // ── ResendAsync ───────────────────────────────────────────────────────────

    [Fact]
    public async Task ResendAsync_unknown_email_no_op()
    {
        await _scope.Db.Database.MigrateAsync();

        await _sut.ResendAsync("nobody@example.com", CancellationToken.None);

        _scope.Db.EmailVerificationTokens.Should().BeEmpty();
        _emailQueue.Messages.Should().BeEmpty();
    }

    [Fact]
    public async Task ResendAsync_first_resend_inserts_row_and_enqueues_email()
    {
        var user = await SeedUserAsync();

        await _sut.ResendAsync(user.Email, CancellationToken.None);

        var token = _scope.Db.EmailVerificationTokens.Single();
        token.UserId.Should().Be(user.Id);
        token.IssuedAt.Should().Be(_clock.GetUtcNow().UtcDateTime);
        token.ExpiresAt.Should().Be(_clock.GetUtcNow().UtcDateTime.AddHours(24));
        token.ConsumedAt.Should().BeNull();
        token.RevokedAt.Should().BeNull();

        _emailQueue.Messages.Should().ContainSingle(m =>
            m.To == user.Email && m.TemplateSlug == "email_verification");
        _emailQueue.Messages[0].Variables["verification_url"].Should().Contain("evtk_");
    }

    [Fact]
    public async Task ResendAsync_subsequent_resend_revokes_prior_row_and_inserts_new()
    {
        var user = await SeedUserAsync();
        await _sut.ResendAsync(user.Email, CancellationToken.None);

        var firstToken = _scope.Db.EmailVerificationTokens.Single();
        _emailQueue.Clear();

        // Second resend
        await _sut.ResendAsync(user.Email, CancellationToken.None);

        _scope.Db.ChangeTracker.Clear();
        var all = _scope.Db.EmailVerificationTokens.Where(t => t.UserId == user.Id).ToList();
        all.Should().HaveCount(2);

        var revoked = all.Single(t => t.Id == firstToken.Id);
        revoked.RevokedAt.Should().NotBeNull();

        var fresh = all.Single(t => t.Id != firstToken.Id);
        fresh.RevokedAt.Should().BeNull();
        fresh.ConsumedAt.Should().BeNull();

        _emailQueue.Messages.Should().ContainSingle(m => m.TemplateSlug == "email_verification");
    }

    [Fact]
    public async Task ResendAsync_already_verified_user_still_issues_new_token_and_revokes_prior()
    {
        // Spec behavior: "Given a verified user, when POST /api/v1/auth/email-verification/resend
        // is called, then all of their non-consumed verification tokens are revoked and a new
        // evtk_ token is issued (24-hour lifetime)."
        var user = await SeedUserAsync(emailVerified: true);

        // Seed an existing (unconsumed, unrevoked) token for the already-verified user.
        var (_, existingHash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        _scope.Db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = existingHash,
            IssuedAt  = _clock.GetUtcNow().UtcDateTime.AddHours(-1),
            ExpiresAt = _clock.GetUtcNow().UtcDateTime.AddHours(23),
        });
        await _scope.Db.SaveChangesAsync();

        await _sut.ResendAsync(user.Email, CancellationToken.None);

        _scope.Db.ChangeTracker.Clear();
        var all = _scope.Db.EmailVerificationTokens.Where(t => t.UserId == user.Id).ToList();

        // Prior token is revoked; a new token exists.
        all.Should().HaveCount(2);
        all.Where(t => t.RevokedAt != null).Should().HaveCount(1, "prior unconsumed token must be revoked");
        var fresh = all.Single(t => t.RevokedAt == null);
        fresh.ConsumedAt.Should().BeNull();
        fresh.ExpiresAt.Should().Be(_clock.GetUtcNow().UtcDateTime.AddHours(24));

        _emailQueue.Messages.Should().ContainSingle(m =>
            m.To == user.Email && m.TemplateSlug == "email_verification");
    }

    [Fact]
    public async Task ResendAsync_sixth_resend_within_24h_is_silently_rate_limited()
    {
        var user = await SeedUserAsync();

        // 5 allowed resends
        for (var i = 0; i < 5; i++)
        {
            _emailQueue.Clear();
            await _sut.ResendAsync(user.Email, CancellationToken.None);
        }

        _emailQueue.Clear();
        await _sut.ResendAsync(user.Email, CancellationToken.None);

        _emailQueue.Messages.Should().BeEmpty();
    }

    // ── ConfirmAsync ──────────────────────────────────────────────────────────

    private async Task<string> SeedVerificationTokenAsync(User user)
    {
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        _scope.Db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
            IssuedAt  = _clock.GetUtcNow().UtcDateTime,
            ExpiresAt = _clock.GetUtcNow().UtcDateTime.AddHours(24),
        });
        await _scope.Db.SaveChangesAsync();
        return plaintext;
    }

    [Fact]
    public async Task ConfirmAsync_with_valid_token_sets_user_email_verified_true_and_consumes_row()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedVerificationTokenAsync(user);

        var err = await _sut.ConfirmAsync(plaintext, CancellationToken.None);

        err.Should().Be(EmailVerificationError.None);

        _scope.Db.ChangeTracker.Clear();
        var updated = await _scope.Db.Users.FindAsync(user.Id);
        updated!.EmailVerified.Should().BeTrue();

        var tokenRow = _scope.Db.EmailVerificationTokens.Single();
        tokenRow.ConsumedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task ConfirmAsync_with_expired_token_returns_TokenInvalid()
    {
        var user = await SeedUserAsync();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        _scope.Db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
            IssuedAt  = _clock.GetUtcNow().UtcDateTime.AddHours(-25),
            ExpiresAt = _clock.GetUtcNow().UtcDateTime.AddHours(-1), // expired
        });
        await _scope.Db.SaveChangesAsync();

        var err = await _sut.ConfirmAsync(plaintext, CancellationToken.None);

        err.Should().Be(EmailVerificationError.TokenInvalid);
    }

    [Fact]
    public async Task ConfirmAsync_with_already_consumed_token_returns_TokenInvalid()
    {
        var user = await SeedUserAsync();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        _scope.Db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
            IssuedAt   = _clock.GetUtcNow().UtcDateTime,
            ExpiresAt  = _clock.GetUtcNow().UtcDateTime.AddHours(24),
            ConsumedAt = _clock.GetUtcNow().UtcDateTime.AddMinutes(-1),
        });
        await _scope.Db.SaveChangesAsync();

        var err = await _sut.ConfirmAsync(plaintext, CancellationToken.None);

        err.Should().Be(EmailVerificationError.TokenInvalid);
    }

    [Fact]
    public async Task ConfirmAsync_with_revoked_token_returns_TokenInvalid()
    {
        var user = await SeedUserAsync();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        _scope.Db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
            IssuedAt  = _clock.GetUtcNow().UtcDateTime,
            ExpiresAt = _clock.GetUtcNow().UtcDateTime.AddHours(24),
            RevokedAt = _clock.GetUtcNow().UtcDateTime.AddMinutes(-1),
        });
        await _scope.Db.SaveChangesAsync();

        var err = await _sut.ConfirmAsync(plaintext, CancellationToken.None);

        err.Should().Be(EmailVerificationError.TokenInvalid);
    }

    [Fact]
    public async Task ConfirmAsync_token_without_evtk_prefix_returns_TokenInvalid()
    {
        await _scope.Db.Database.MigrateAsync();

        var err = await _sut.ConfirmAsync("prst_somethingsomething", CancellationToken.None);

        err.Should().Be(EmailVerificationError.TokenInvalid);
    }
}
