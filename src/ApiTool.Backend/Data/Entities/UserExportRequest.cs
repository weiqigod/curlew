using System.ComponentModel.DataAnnotations;
using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// A per-user GDPR data-export request row.
/// Lifecycle: <c>Queued → Building → Ready</c> (or <c>Failed</c>).
/// A signed download URL is regenerated on each GET using <c>expires_at</c> as the TTL anchor.
/// At most one request per 24h per user (enforced at the endpoint, not via a unique index).
/// </summary>
/// <remarks>
/// Marked <see cref="GdprTableKind.NotUserAttributable"/> so it is exempt from the GDPR
/// manifest and coverage tests.  Although the row carries a <c>UserId</c> FK, it is
/// pure export-infrastructure metadata (not personal data content) and is hard-deleted
/// by FK cascade when the owning user is removed — no extra GDPR handling needed.
/// </remarks>
[GdprTable(GdprTableKind.NotUserAttributable)]
public sealed class UserExportRequest
{
    /// <summary>Primary key — random UUID v4.</summary>
    public Guid Id { get; set; }

    /// <summary>
    /// The user who initiated the request.  Row is hard-deleted on user deletion via FK cascade.
    /// </summary>
    public Guid UserId { get; set; }

    /// <summary>Current lifecycle state of the request.</summary>
    public UserExportStatus Status { get; set; }

    /// <summary>
    /// Optimistic concurrency token.  EF Core increments this on every update so that a
    /// concurrent <see cref="UserExportBuilderHost"/> tick that races to claim the same row
    /// will receive a <see cref="Microsoft.EntityFrameworkCore.DbUpdateConcurrencyException"/>
    /// and skip the row rather than double-processing it.
    /// </summary>
    [ConcurrencyCheck]
    public Guid Version { get; set; } = Guid.NewGuid();

    /// <summary>The object-store key where the bundle was written. Null until <c>Ready</c>.</summary>
    public string? ObjectKey { get; set; }

    /// <summary>UTC timestamp when the request was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp when the bundle was written to the object store. Null until <c>Ready</c>.</summary>
    public DateTime? ReadyAt { get; set; }

    /// <summary>UTC timestamp when the signed URL expires (24h after <see cref="ReadyAt"/>). Null until <c>Ready</c>.</summary>
    public DateTime? ExpiresAt { get; set; }

    /// <summary>
    /// Error classification when <see cref="Status"/> is <c>Failed</c>.
    /// Carries the exception type name, NOT the message (PII risk — messages can leak bucket names or credentials).
    /// </summary>
    public string? FailureReason { get; set; }
}
