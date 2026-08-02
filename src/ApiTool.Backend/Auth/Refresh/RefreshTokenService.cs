// Refs docs/SPECIFICATION.md:7901-7944 (refresh-token rotation semantics).
// Refs RFC 9700 §4.14 (rotation-on-use with family revocation).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Transactional core for refresh-token rotation and reuse detection.
/// The handler (<see cref="AuthRefreshEndpoints"/>) does HTTP-shape work only;
/// this service owns all business logic.
/// <para>
/// Decision #12 (plan): the rotate happy path wraps in <c>BeginTransactionAsync</c> only
/// when the provider supports transactions (Postgres/SQLite). EF Core InMemory
/// (used in BackendFactory integration tests) does not support transactions;
/// when <c>IsInMemory()</c> is true the service skips the explicit transaction wrapper
/// and relies on <c>SaveChangesAsync</c> atomicity within the single EF change-tracker.
/// Unit tests use SQLite-in-memory (via <see cref="TestInfrastructure.TestDb"/>) which
/// DOES support transactions, providing real semantics coverage.
/// </para>
/// </summary>
public sealed class RefreshTokenService(
    AppDbContext db,
    IEmailQueue emailQueue,
    TimeProvider clock,
    ILogger<RefreshTokenService> logger)
{
    private const string ReuseReason = "reuse_detected";

    /// <summary>
    /// Validates and rotates the presented refresh token.
    /// Returns a discriminated <see cref="RotateResult"/> describing the outcome.
    /// </summary>
    public async Task<RotateResult> RotateAsync(
        string presentedPlaintext,
        Guid presentedDeviceId,
        string? clientIp,
        string? userAgent,
        CancellationToken ct)
    {
        var tokenHash = RefreshTokenIssuer.Hash(presentedPlaintext);

        var row = await db.RefreshTokens
            .FirstOrDefaultAsync(r => r.TokenHash == tokenHash, ct);

        if (row is null)
            return new RotateResult(RotateOutcome.NotFound, null, null, null, null);

        if (row.RevokedAt is not null)
            return new RotateResult(RotateOutcome.Revoked, null, null, row.UserId, row.FamilyId);

        var now = clock.GetUtcNow().UtcDateTime;

        if (row.ExpiresAt < now)
            return new RotateResult(RotateOutcome.Expired, null, null, row.UserId, row.FamilyId);

        if (row.DeviceId != presentedDeviceId)
            return new RotateResult(RotateOutcome.DeviceMismatch, null, null, row.UserId, row.FamilyId);

        if (row.RotatedAt is not null)
        {
            // Reuse detected — revoke entire family and alert the user
            await RevokeEntireFamilyAsync(row.FamilyId, row.UserId, clientIp, ct);
            return new RotateResult(RotateOutcome.Reused, null, null, row.UserId, row.FamilyId);
        }

        // Happy path: rotate
        var (newPlaintext, newRow) = await RotateHappyPathAsync(row, presentedDeviceId, now, clientIp, userAgent, ct);
        return new RotateResult(RotateOutcome.Success, newRow, newPlaintext, row.UserId, row.FamilyId);
    }

    // ── Private helpers ──────────────────────────────────────────────────────

    private async Task<(string Plaintext, RefreshToken NewRow)> RotateHappyPathAsync(
        RefreshToken row,
        Guid deviceId,
        DateTime now,
        string? clientIp,
        string? userAgent,
        CancellationToken ct)
    {
        // Determine family root's issued_at for lifetime clamping.
        DateTime familyRootIssuedAt;
        if (row.ParentId is null)
        {
            // row IS the family root
            familyRootIssuedAt = row.IssuedAt;
        }
        else
        {
            // Load the family root: the row whose Id equals FamilyId
            var root = await db.RefreshTokens
                .FirstOrDefaultAsync(r => r.Id == row.FamilyId, ct);
            if (root is null)
            {
                // Data-integrity anomaly: the family root row is missing.
                // Fall back to the current row's IssuedAt so rotation can still proceed,
                // but log a warning so the anomaly is observable in production.
                logger.LogWarning(
                    "refresh-token: family root missing for FamilyId={FamilyId} RowId={RowId}; " +
                    "falling back to row.IssuedAt for lifetime clamping — this may grant a longer window than intended",
                    row.FamilyId, row.Id);
            }

            familyRootIssuedAt = root?.IssuedAt ?? row.IssuedAt;
        }

        var issuer          = new RefreshTokenIssuer(clock);
        var (plaintext, newRow) = issuer.Mint(
            row.UserId, deviceId, row.FamilyId, parentId: row.Id,
            familyRootIssuedAt: familyRootIssuedAt, clientIp, userAgent);

        if (db.Database.ProviderName == "Microsoft.EntityFrameworkCore.InMemory")
        {
            // EF Core InMemory does not support transactions — rely on SaveChangesAsync atomicity.
            row.RotatedAt = now;
            db.RefreshTokens.Update(row);
            db.RefreshTokens.Add(newRow);
            await db.SaveChangesAsync(ct);
        }
        else
        {
            await using var tx = await db.Database.BeginTransactionAsync(ct);
            row.RotatedAt = now;
            db.RefreshTokens.Update(row);
            db.RefreshTokens.Add(newRow);
            await db.SaveChangesAsync(ct);
            await tx.CommitAsync(ct);
        }

        logger.LogInformation(
            "refresh-token: rotated family={FamilyId} old={OldId} new={NewId}",
            row.FamilyId, row.Id, newRow.Id);

        return (plaintext, newRow);
    }

    /// <summary>
    /// Revokes every non-revoked refresh-token row for the user, marking each with
    /// the supplied <paramref name="reason"/>. Used after password reset (RFC 9700 §4.14)
    /// and on explicit user-initiated session revoke. Returns the number of rows revoked.
    /// </summary>
    public async Task<int> RevokeAllFamiliesForUserAsync(
        Guid userId,
        string reason,
        CancellationToken ct)
    {
        ArgumentException.ThrowIfNullOrEmpty(reason);
        var now = clock.GetUtcNow().UtcDateTime;

        var rows = await db.RefreshTokens
            .Where(r => r.UserId == userId && r.RevokedAt == null)
            .ToListAsync(ct);

        foreach (var r in rows)
        {
            r.RevokedAt    = now;
            r.RevokeReason = reason;
        }

        await db.SaveChangesAsync(ct);

        if (rows.Count > 0)
            logger.LogInformation(
                "refresh-token: revoked {Count} families for userId={UserId} reason={Reason}",
                rows.Count, userId, reason);

        return rows.Count;
    }

    private async Task RevokeEntireFamilyAsync(
        Guid familyId,
        Guid userId,
        string? clientIp,
        CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;

        var familyRows = await db.RefreshTokens
            .Where(r => r.FamilyId == familyId && r.RevokedAt == null)
            .ToListAsync(ct);

        foreach (var r in familyRows)
        {
            r.RevokedAt    = now;
            r.RevokeReason = ReuseReason;
        }

        await db.SaveChangesAsync(ct);

        logger.LogWarning(
            "refresh-token: reuse detected, revoked {Count} tokens in family={FamilyId} userId={UserId}",
            familyRows.Count, familyId, userId);

        // Look up user email for the security alert template
        var user = await db.Users.FindAsync([userId], ct);
        var email = user?.Email ?? string.Empty;
        var firstName = email.Contains('@') ? email[..email.IndexOf('@', StringComparison.Ordinal)] : email;

        await emailQueue.EnqueueAsync(new EmailMessage(
            To: email,
            TemplateSlug: "account_security_alert",
            Variables: new Dictionary<string, string>
            {
                ["first_name"]  = firstName,
                ["event_time"]  = now.ToString("o"),
                ["event_ip"]    = clientIp ?? "unknown",
                ["relogin_url"] = "https://app.apitool.dev/login",
            },
            EnqueuedAt: clock.GetUtcNow()), ct);
    }
}
