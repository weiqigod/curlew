using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Opaque refresh token, hashed at rest. Family-revocation rotation tracking
/// per RFC 9700 §4.14. Refs docs/SPECIFICATION.md:9737-9760.
/// </summary>
/// <remarks>
/// <para>
/// <c>device_id</c> is a plain UUID column in M14-001.
/// The FK to <c>devices(id)</c> is added in M14-002 when the devices table lands.
/// See plan §"Refresh-tokens table" for rationale.
/// </para>
/// </remarks>
public sealed class RefreshToken
{
    /// <summary>Primary key — random UUID v4.</summary>
    public Guid Id { get; set; }

    /// <summary>SHA-256 hash of the opaque token value, stored at rest.</summary>
    public byte[] TokenHash { get; set; } = [];

    /// <summary>The user this token belongs to.</summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    [GdprUserAttribution]
    public Guid UserId { get; set; }

    /// <summary>The device this token was issued to. FK to devices(id) added in M14-002.</summary>
    public Guid DeviceId { get; set; }

    /// <summary>Family identifier used for chain-revocation on reuse detection.</summary>
    public Guid FamilyId { get; set; }

    /// <summary>The token this one was rotated from; null for the first token in the family.</summary>
    public Guid? ParentId { get; set; }

    /// <summary>UTC timestamp when the token was issued.</summary>
    public DateTime IssuedAt { get; set; }

    /// <summary>UTC timestamp after which the token must not be accepted.</summary>
    public DateTime ExpiresAt { get; set; }

    /// <summary>UTC timestamp when the token was rotated to produce a successor.</summary>
    public DateTime? RotatedAt { get; set; }

    /// <summary>UTC timestamp when the token was revoked.</summary>
    public DateTime? RevokedAt { get; set; }

    /// <summary>Human-readable reason for revocation.</summary>
    public string? RevokeReason { get; set; }

    /// <summary>IP address from which the token was last used, for audit purposes.</summary>
    public string? LastUsedIp { get; set; }

    /// <summary>User-agent header from the last use of this token, for audit purposes.</summary>
    public string? UserAgent { get; set; }
}
