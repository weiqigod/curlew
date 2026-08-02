using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Hash-stored, time-bound, single-use password-reset token.
/// Refs docs/SPECIFICATION.md:8479-8493 (column shape) and :10964-10985 (DDL).
/// </summary>
/// <remarks>
/// 30-minute lifetime (set by <see cref="ExpiresAt"/> at insert time).
/// <see cref="ConsumedAt"/> is set on successful confirm; <see cref="RevokedAt"/>
/// on explicit user revoke (Devices &amp; Sessions). Either makes the row dead.
/// The raw <c>prst_*</c> token is never persisted — only its SHA-256 hash.
/// </remarks>
public sealed class PasswordResetToken
{
    /// <summary>Primary key — random UUID v4.</summary>
    public Guid Id { get; set; }

    /// <summary>The user this token was issued for.</summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    [GdprUserAttribution]
    public Guid UserId { get; set; }

    /// <summary>SHA-256 hash of the raw <c>prst_*</c> token.</summary>
    public byte[] TokenHash { get; set; } = [];

    /// <summary>UTC timestamp when the token was issued.</summary>
    public DateTime IssuedAt { get; set; }

    /// <summary>UTC timestamp after which the token must not be accepted.</summary>
    public DateTime ExpiresAt { get; set; }

    /// <summary>UTC timestamp when the token was consumed; null while live.</summary>
    public DateTime? ConsumedAt { get; set; }

    /// <summary>UTC timestamp when the token was revoked; null while live.</summary>
    public DateTime? RevokedAt { get; set; }

    /// <summary>IP address of the requester at issue time, for forensic audit.</summary>
    public string? RequesterIp { get; set; }

    /// <summary>User-Agent of the requester at issue time, for forensic audit.</summary>
    public string? RequesterUa { get; set; }
}
