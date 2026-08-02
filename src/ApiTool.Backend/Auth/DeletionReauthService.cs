using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Concrete <see cref="IDeletionReauthService"/>. Issues and consumes single-use
/// 5-minute <c>drto_</c> tokens that gate <c>POST /api/v1/users/me/deletion-requests</c>.
/// Refs docs/SPECIFICATION.md v4-5.
/// </summary>
public sealed class DeletionReauthService(
    AppDbContext db,
    PasswordHasher hasher,
    TimeProvider clock) : IDeletionReauthService
{
    private static readonly TimeSpan TokenLifetime = TimeSpan.FromMinutes(5);

    /// <inheritdoc/>
    public async Task<(ReauthError Error, string? Token, DateTime? ExpiresAt)> IssueAsync(
        Guid userId, string password, CancellationToken ct)
    {
        var user = await db.Users.FindAsync([userId], ct);
        if (user is null || user.PasswordHash is null)
            return (ReauthError.WrongPassword, null, null);

        if (!hasher.Verify(password, user.PasswordHash))
            return (ReauthError.WrongPassword, null, null);

        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.DeletionReauthPrefix);
        var now = clock.GetUtcNow().UtcDateTime;
        var expiresAt = now + TokenLifetime;

        db.DeletionReauthTokens.Add(new DeletionReauthToken
        {
            Id = Guid.NewGuid(),
            UserId = userId,
            TokenHash = hash,
            IssuedAt = now,
            ExpiresAt = expiresAt,
        });
        await db.SaveChangesAsync(ct);

        return (ReauthError.None, plaintext, expiresAt);
    }

    /// <inheritdoc/>
    public async Task<ReauthError> ConsumeAsync(Guid userId, string rawToken, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(rawToken) ||
            !rawToken.StartsWith(AuthTokenIssuer.DeletionReauthPrefix, StringComparison.Ordinal))
            return ReauthError.TokenInvalid;

        var hash = AuthTokenIssuer.Hash(rawToken);
        var now = clock.GetUtcNow().UtcDateTime;

        var row = await db.DeletionReauthTokens
            .FirstOrDefaultAsync(r => r.TokenHash == hash, ct);

        if (row is null || row.UserId != userId)
            return ReauthError.TokenInvalid;

        if (row.ConsumedAt is not null)
            return ReauthError.TokenConsumed;

        if (row.ExpiresAt <= now)
            return ReauthError.TokenExpired;

        row.ConsumedAt = now;
        await db.SaveChangesAsync(ct);

        return ReauthError.None;
    }
}
