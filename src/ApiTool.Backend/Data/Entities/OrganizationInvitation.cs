using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>A pending invitation for a user to join an organization.</summary>
[GdprTable(GdprTableKind.NotUserAttributable)]
public sealed class OrganizationInvitation
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>The organization the invitation is for.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Email address that was invited.</summary>
    public string Email { get; set; } = string.Empty;

    /// <summary>Lowercase-normalised email for deduplication lookups.</summary>
    public string EmailNormalized { get; set; } = string.Empty;

    /// <summary>Role the invitee will receive on acceptance.</summary>
    public OrgRole Role { get; set; }

    /// <summary>User who sent the invitation.</summary>
    public Guid InvitedBy { get; set; }

    /// <summary>SHA-256 hex hash of the raw token sent to the invitee; raw token is never stored.</summary>
    public string TokenHash { get; set; } = string.Empty;

    /// <summary>UTC expiry time for the invitation token.</summary>
    public DateTime ExpiresAt { get; set; }

    /// <summary>UTC timestamp when the invitation was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp when the invitation was accepted; null while pending.</summary>
    public DateTime? AcceptedAt { get; set; }

    /// <summary>UTC timestamp when the invitation was revoked; null if not revoked.</summary>
    public DateTime? RevokedAt { get; set; }

    /// <summary>User who revoked the invitation; null if not revoked.</summary>
    public Guid? RevokedBy { get; set; }

    /// <summary>UTC timestamp of the last send (creation or resend).</summary>
    public DateTime LastSentAt { get; set; }

    /// <summary>Total number of times this invitation has been sent (includes initial send).</summary>
    public int SendCount { get; set; }
}
