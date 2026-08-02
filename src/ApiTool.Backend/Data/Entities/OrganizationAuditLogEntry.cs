using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>An immutable audit log entry recording a significant organization event.</summary>
public sealed class OrganizationAuditLogEntry
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>
    /// The organization this event belongs to. Null for account-level events that
    /// must not be exposed in any organization's audit stream.
    /// </summary>
    public Guid? OrgId { get; set; }

    /// <summary>
    /// The user who triggered the event. NULL when the user has been anonymised (M18-006)
    /// or the event has no attributed user. Distinct from <see cref="Guid.Empty"/> which
    /// the codebase uses for "system/anonymous" events. Mapped to column <c>actor_id</c>.
    /// </summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionAnonymise)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    [GdprUserAttribution]
    public Guid? ActorId { get; set; }

    /// <summary>
    /// Email of the actor at the time of the event, when known. Rendered by the
    /// audit-log UI in preference to the raw user id. Nullable because bulk/system
    /// events and anonymous failure rows may not carry an email.
    /// </summary>
    [GdprAnonymise(AnonymiseAs.DeletedUserToken)]
    public string? ActorEmail { get; set; }

    /// <summary>Dot-namespaced event type, e.g. <c>org.created</c>.</summary>
    public string EventType { get; set; } = string.Empty;

    /// <summary>Serialized JSON payload with event-specific details.</summary>
    public string PayloadJson { get; set; } = "{}";

    /// <summary>The type of the target resource, e.g. <c>invitation</c>, <c>subscription</c>.</summary>
    public string? TargetType { get; set; }

    /// <summary>The primary key of the target resource when applicable.</summary>
    public Guid? TargetId { get; set; }

    /// <summary>Serialized JSON representing the resource state before the event.</summary>
    public string? PreviousStateJson { get; set; }

    /// <summary>Serialized JSON representing the resource state after the event.</summary>
    public string? NewStateJson { get; set; }

    /// <summary>IP address of the requester (IPv4 or IPv6, up to 45 chars).</summary>
    public string? IpAddress { get; set; }

    /// <summary>User-Agent header from the HTTP request, truncated to 500 chars.</summary>
    public string? UserAgent { get; set; }

    /// <summary>Whether the event represented a successful operation (default: true).</summary>
    public bool Success { get; set; } = true;

    /// <summary>Human-readable reason for failure when <see cref="Success"/> is false.</summary>
    public string? FailureReason { get; set; }

    /// <summary>UTC timestamp when the event occurred.</summary>
    public DateTime CreatedAt { get; set; }
}
