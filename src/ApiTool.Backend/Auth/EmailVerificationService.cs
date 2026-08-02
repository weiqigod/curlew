// Refs docs/SPECIFICATION.md:8495-8509 (email verification flow).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Concrete implementation of <see cref="IEmailVerificationService"/>.
/// Rotate-on-resend semantics: each ResendAsync revokes prior active tokens
/// before inserting a fresh one, keeping at most one usable token per user.
/// </summary>
public sealed class EmailVerificationService(
    AppDbContext db,
    IEmailQueue emailQueue,
    IOptions<AppOptions> appOptions,
    TimeProvider clock,
    ILogger<EmailVerificationService> logger) : IEmailVerificationService
{
    private const int MaxResendsPerEmailPerDay = 5;
    private static readonly TimeSpan ResendWindow  = TimeSpan.FromHours(24);
    private static readonly TimeSpan TokenLifetime = TimeSpan.FromHours(24);

    /// <inheritdoc/>
    public async Task ResendAsync(string email, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(email)) return;
        var normalised = email.Trim().ToLowerInvariant();

        var user = await db.Users.FirstOrDefaultAsync(u => u.Email == normalised, ct);
        if (user is null) return; // silent — enumeration defense

        // Per-email rate limit
        var since  = clock.GetUtcNow().UtcDateTime - ResendWindow;
        var recent = await db.EmailVerificationTokens
            .CountAsync(t => t.UserId == user.Id && t.IssuedAt >= since, ct);
        if (recent >= MaxResendsPerEmailPerDay)
        {
            logger.LogInformation(
                "email_verification_resend_rate_limited userId={UserId} count={Count}",
                user.Id, recent);
            return;
        }

        var now = clock.GetUtcNow().UtcDateTime;

        // Rotate: revoke all active (non-consumed) tokens for this user
        var activeTokens = await db.EmailVerificationTokens
            .Where(t => t.UserId == user.Id && t.ConsumedAt == null && t.RevokedAt == null)
            .ToListAsync(ct);
        foreach (var t in activeTokens)
            t.RevokedAt = now;

        // Mint and insert new token
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        var expires = now + TokenLifetime;
        db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id        = Guid.NewGuid(),
            UserId    = user.Id,
            TokenHash = hash,
            IssuedAt  = now,
            ExpiresAt = expires,
        });
        await db.SaveChangesAsync(ct);

        // Enqueue email
        var verificationUrl = $"{appOptions.Value.WebAppUrl.TrimEnd('/')}/auth/email-verification/confirm?token={plaintext}";
        await emailQueue.EnqueueAsync(new EmailMessage(
            To: user.Email,
            TemplateSlug: "email_verification",
            Variables: new Dictionary<string, string>
            {
                ["user_email"]        = user.Email,
                ["verification_url"]  = verificationUrl,
                ["expires_at_local"]  = expires.ToString("yyyy-MM-dd HH:mm 'UTC'"),
            },
            EnqueuedAt: clock.GetUtcNow()), ct);
    }

    /// <inheritdoc/>
    public async Task<EmailVerificationError> ConfirmAsync(string token, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(token) ||
            !token.StartsWith(AuthTokenIssuer.EmailVerificationPrefix, StringComparison.Ordinal))
            return EmailVerificationError.TokenInvalid;

        var hash = AuthTokenIssuer.Hash(token);
        var now  = clock.GetUtcNow().UtcDateTime;

        var row = await db.EmailVerificationTokens
            .FirstOrDefaultAsync(r => r.TokenHash == hash, ct);
        if (row is null
            || row.ConsumedAt is not null
            || row.RevokedAt  is not null
            || row.ExpiresAt  <= now)
        {
            return EmailVerificationError.TokenInvalid;
        }

        var user = await db.Users.FindAsync([row.UserId], ct);
        if (user is null) return EmailVerificationError.TokenInvalid;

        user.EmailVerified = true;
        row.ConsumedAt     = now;
        await db.SaveChangesAsync(ct);

        logger.LogInformation("email_verification_confirmed userId={UserId}", user.Id);
        return EmailVerificationError.None;
    }
}
