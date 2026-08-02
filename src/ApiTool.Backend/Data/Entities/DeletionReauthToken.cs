using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Hash-stored, single-use, 5-minute re-authentication token gating
/// <c>POST /api/v1/users/me/deletion-requests</c>. The raw <c>drto_*</c> token
/// is never persisted — only its SHA-256 hash. Refs docs/SPECIFICATION.md v4-5.
/// </summary>
public sealed class DeletionReauthToken
{
    /// <summary>Primary key — random UUID v4.</summary>
    public Guid Id { get; set; }

    /// <summary>The user this token was issued for.</summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    [GdprUserAttribution]
    public Guid UserId { get; set; }

    /// <summary>SHA-256 hash of the raw <c>drto_*</c> token.</summary>
    public byte[] TokenHash { get; set; } = [];

    /// <summary>UTC timestamp when the token was issued.</summary>
    public DateTime IssuedAt { get; set; }

    /// <summary>UTC timestamp after which the token must not be accepted (5-minute TTL).</summary>
    public DateTime ExpiresAt { get; set; }

    /// <summary>UTC timestamp when the token was consumed; null while live (single-use semantics).</summary>
    public DateTime? ConsumedAt { get; set; }
}
