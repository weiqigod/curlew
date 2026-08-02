// Refs docs/SPECIFICATION.md:8461-8493 (password reset flow).
using ApiTool.Backend.Auth.Refresh;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Concrete implementation of <see cref="IPasswordResetService"/>.
/// Uses per-email DB count for rate limiting (mirrors InvitationsService resend pattern)
/// and defers per-IP limiting to the ASP.NET RateLimiter middleware.
/// </summary>
public sealed class PasswordResetService(
    AppDbContext db,
    PasswordHasher hasher,
    IPasswordStrengthChecker strengthChecker,
    RefreshTokenService refreshSvc,
    IEmailQueue emailQueue,
    IOptions<AppOptions> appOptions,
    TimeProvider clock,
    ILogger<PasswordResetService> logger) : IPasswordResetService
{
    private const int MaxRequestsPerEmailPerDay = 3;
    private static readonly TimeSpan RequestWindow = TimeSpan.FromHours(24);
    private static readonly TimeSpan TokenLifetime = TimeSpan.FromMinutes(30);
    private const int MinimumStrengthScore = 3;

    /// <inheritdoc/>
    public async Task RequestAsync(
        string email, string? requesterIp, string? requesterUa, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(email)) return;
        var normalised = email.Trim().ToLowerInvariant();

        var user = await db.Users.FirstOrDefaultAsync(u => u.Email == normalised, ct);
        if (user is null) return; // silent — enumeration defense

        // Per-email rate limit
        var since  = clock.GetUtcNow().UtcDateTime - RequestWindow;
        var recent = await db.PasswordResetTokens
            .CountAsync(t => t.UserId == user.Id && t.IssuedAt >= since, ct);
        if (recent >= MaxRequestsPerEmailPerDay)
        {
            logger.LogInformation(
                "password_reset_request_rate_limited userId={UserId} count={Count}",
                user.Id, recent);
            return; // silent — same response as success path
        }

        // Mint token + persist row
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix);
        var now     = clock.GetUtcNow().UtcDateTime;
        var expires = now + TokenLifetime;
        db.PasswordResetTokens.Add(new PasswordResetToken
        {
            Id          = Guid.NewGuid(),
            UserId      = user.Id,
            TokenHash   = hash,
            IssuedAt    = now,
            ExpiresAt   = expires,
            RequesterIp = requesterIp,
            RequesterUa = requesterUa,
        });
        await db.SaveChangesAsync(ct);

        // Enqueue email
        var resetUrl = $"{appOptions.Value.WebAppUrl.TrimEnd('/')}/auth/password-reset/confirm?token={plaintext}";
        await emailQueue.EnqueueAsync(new EmailMessage(
            To: user.Email,
            TemplateSlug: "password_reset",
            Variables: new Dictionary<string, string>
            {
                ["user_email"]       = user.Email,
                ["reset_url"]        = resetUrl,
                ["expires_at_local"] = expires.ToString("yyyy-MM-dd HH:mm 'UTC'"),
                ["requester_ip"]     = requesterIp ?? "unknown",
                ["requester_ua"]     = requesterUa ?? "unknown",
            },
            EnqueuedAt: clock.GetUtcNow()), ct);
    }

    /// <inheritdoc/>
    public async Task<(PasswordResetError Error, int? Score)> ConfirmAsync(
        string token, string newPassword, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(token) ||
            !token.StartsWith(AuthTokenIssuer.PasswordResetPrefix, StringComparison.Ordinal))
            return (PasswordResetError.TokenInvalid, null);

        if (string.IsNullOrEmpty(newPassword))
            return (PasswordResetError.PasswordTooWeak, 0);

        var hash = AuthTokenIssuer.Hash(token);
        var now  = clock.GetUtcNow().UtcDateTime;

        var row = await db.PasswordResetTokens
            .FirstOrDefaultAsync(r => r.TokenHash == hash, ct);
        if (row is null
            || row.ConsumedAt is not null
            || row.RevokedAt  is not null
            || row.ExpiresAt  <= now)
        {
            return (PasswordResetError.TokenInvalid, null);
        }

        // Need user.Email for zxcvbn user-input penalty
        var user = await db.Users.FindAsync([row.UserId], ct);
        if (user is null) return (PasswordResetError.TokenInvalid, null);

        var score = strengthChecker.Score(newPassword, [user.Email]);
        if (score < MinimumStrengthScore)
            return (PasswordResetError.PasswordTooWeak, score);

        // Transactional update — skip transaction wrapper for InMemory provider.
        // RevokeAllFamiliesForUserAsync enlists on the same AppDbContext, so it must
        // run inside the same transaction block to guarantee atomicity (spec behavior:
        // password_hash update + consumed_at set + ALL refresh families revoked are one unit).
        var useTransaction = db.Database.ProviderName != "Microsoft.EntityFrameworkCore.InMemory";
        if (useTransaction)
        {
            await using var tx = await db.Database.BeginTransactionAsync(ct);
            await ApplyConfirmAsync(row, user, newPassword, now, ct);
            await refreshSvc.RevokeAllFamiliesForUserAsync(user.Id, "password_reset", ct);
            await tx.CommitAsync(ct);
        }
        else
        {
            await ApplyConfirmAsync(row, user, newPassword, now, ct);
            await refreshSvc.RevokeAllFamiliesForUserAsync(user.Id, "password_reset", ct);
        }

        logger.LogInformation("password_reset_confirmed userId={UserId}", user.Id);
        return (PasswordResetError.None, null);
    }

    private async Task ApplyConfirmAsync(
        PasswordResetToken row, User user, string newPassword, DateTime now, CancellationToken ct)
    {
        user.PasswordHash  = hasher.Hash(newPassword);
        row.ConsumedAt = now;
        await db.SaveChangesAsync(ct);
    }
}
