namespace ApiTool.Backend.Audit;

/// <summary>
/// A single audit event payload. IP address and User-Agent are enriched automatically
/// by <see cref="AuditWriter"/> from the scoped <see cref="AuditContext"/>.
/// </summary>
/// <param name="OrgId">Organization the event belongs to.</param>
/// <param name="ActorId">
/// Identifier of the user who triggered the event, or <see cref="Guid.Empty"/> when
/// no user was identified (e.g. anonymous login-failure paths).
/// </param>
/// <param name="EventType">Dot-namespaced event type, e.g. <c>sso.login</c>.</param>
/// <param name="TargetType">Optional target resource type, e.g. <c>invitation</c>.</param>
/// <param name="TargetId">Optional primary key of the target resource.</param>
/// <param name="Payload">Optional JSON-serialisable payload with event-specific details.</param>
/// <param name="PreviousState">Optional JSON-serialisable resource state before the event.</param>
/// <param name="NewState">Optional JSON-serialisable resource state after the event.</param>
/// <param name="Success">Whether the operation succeeded (default true).</param>
/// <param name="FailureReason">Short machine-readable reason when <paramref name="Success"/> is false.</param>
/// <param name="ActorEmail">
/// Optional email address of the actor, when available at the call site (SSO login,
/// results ingest, invitation-accept, etc). Rendered by the audit-log UI in preference
/// to the raw user id so admins see a meaningful name, and written even when
/// <paramref name="Success"/> is false so failed-login rows identify the attempted user.
/// </param>
public sealed record AuditEvent(
    Guid OrgId,
    Guid ActorId,
    string EventType,
    string? TargetType = null,
    Guid? TargetId = null,
    object? Payload = null,
    object? PreviousState = null,
    object? NewState = null,
    bool Success = true,
    string? FailureReason = null,
    string? ActorEmail = null);
