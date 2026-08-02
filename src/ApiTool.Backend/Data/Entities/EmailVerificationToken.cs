using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Hash-stored, time-bound, single-use email-verification token.
/// Refs docs/SPECIFICATION.md:8495-8509 (column shape) and :10987-11006 (DDL).
/// </summary>
/// <remarks>
/// 24-hour lifetime. Re-send rotates: existing un-consumed rows for the user
/// transition <see cref="RevokedAt"/> to NOW() before a new row is inserted,
/// ensuring at most one usable verification token per user at any time.
/// </remarks>
public sealed class EmailVerificationToken
{
    /// <summary>Primary key — random UUID v4.</summary>
    public Guid Id { get; set; }

    /// <summary>The user this token was issued for.</summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    [GdprUserAttribution]
    public Guid UserId { get; set; }

    /// <summary>SHA-256 hash of the raw <c>evtk_*</c> token.</summary>
    public byte[] TokenHash { get; set; } = [];

    /// <summary>UTC timestamp when the token was issued.</summary>
    public DateTime IssuedAt { get; set; }

    /// <summary>UTC timestamp after which the token must not be accepted.</summary>
    public DateTime ExpiresAt { get; set; }

    /// <summary>UTC timestamp when the token was consumed; null while live.</summary>
    public DateTime? ConsumedAt { get; set; }

    /// <summary>UTC timestamp when the token was revoked (rotation on re-send).</summary>
    public DateTime? RevokedAt { get; set; }
}
