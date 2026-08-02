namespace ApiTool.Backend.Audit;

/// <summary>JSON DTO returned by <c>GET /api/v1/organizations/{id}/audit-log</c>.</summary>
/// <param name="EventType">Dot-namespaced event type.</param>
/// <param name="UserId">Raw hex user id of the actor, or <see langword="null"/> for anonymous failure paths.</param>
/// <param name="UserEmail">
/// Email of the actor captured at the time of the event, when available. The web UI
/// prefers this over <paramref name="UserId"/> so rows display a human-readable name.
/// </param>
/// <param name="TargetType">Optional target resource type.</param>
/// <param name="TargetId">Optional target resource id.</param>
/// <param name="CreatedAt">UTC timestamp when the event occurred.</param>
/// <param name="IpAddress">Requester's IP address, when captured.</param>
/// <param name="Success">Whether the operation succeeded.</param>
/// <param name="FailureReason">Short machine-readable failure reason when <paramref name="Success"/> is false.</param>
public sealed record AuditLogEntryDto(
    string EventType,
    string? UserId,
    string? UserEmail,
    string? TargetType,
    string? TargetId,
    DateTime CreatedAt,
    string? IpAddress,
    bool Success,
    string? FailureReason);
