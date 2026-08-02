using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>A minimal user record upserted from JWT claims on first authenticated request.</summary>
public sealed class User
{
    /// <summary>Primary key — sourced from the JWT <c>sub</c> claim.</summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    [GdprUserAttribution]
    public Guid Id { get; set; }

    /// <summary>Email address sourced from the JWT <c>email</c> claim.</summary>
    public string Email { get; set; } = string.Empty;

    /// <summary>UTC timestamp when the user row was first created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>Argon2id PHC-encoded password hash; null for SSO-only users.</summary>
    public string? PasswordHash { get; set; }

    /// <summary>True for users created via the first-boot admin bootstrap.</summary>
    public bool IsAdmin { get; set; }

    /// <summary>
    /// True once the user has clicked a valid email-verification link.
    /// Gates Stripe checkout (per spec :8576) and org-invite acceptance (per spec :8577).
    /// </summary>
    public bool EmailVerified { get; set; }

    /// <summary>
    /// UTC timestamp when the user initiated GDPR account deletion via
    /// <c>POST /api/v1/users/me/deletion-requests</c>; null while no request is pending.
    /// Refs docs/SPECIFICATION.md v4-5.
    /// </summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    public DateTime? PendingDeletionAt { get; set; }

    /// <summary>
    /// UTC timestamp when <see cref="IUserAnonymiser"/> finalised the deletion (M18-006).
    /// Mutually exclusive with <see cref="PendingDeletionAt"/> in steady state — once
    /// anonymised, the user row's PII columns are cleared.
    /// </summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    public DateTime? AnonymisedAt { get; set; }
}
