using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Audit;

/// <summary>
/// Concrete <see cref="IAuditWriter"/> that resolves ip/user_agent from the scoped
/// <see cref="AuditContext"/> and writes <see cref="OrganizationAuditLogEntry"/> rows
/// via the injected <see cref="AppDbContext"/>.
/// </summary>
public sealed class AuditWriter(AppDbContext db, AuditContext audit, TimeProvider clock) : IAuditWriter
{
    private static readonly JsonSerializerOptions Json = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
    };

    /// <inheritdoc/>
    public void Append(AuditEvent evt)
    {
        db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            // Guid.Empty is the public AuditEvent sentinel for an account-level
            // event. Persist it as NULL so the organization FK remains valid and
            // the event cannot leak into an unrelated organization's audit log.
            OrgId = evt.OrgId == Guid.Empty ? null : evt.OrgId,
            ActorId = evt.ActorId,
            EventType = evt.EventType,
            TargetType = evt.TargetType,
            TargetId = evt.TargetId,
            PayloadJson = evt.Payload is null
                ? "{}"
                : JsonSerializer.Serialize(evt.Payload, Json),
            PreviousStateJson = evt.PreviousState is null
                ? null
                : JsonSerializer.Serialize(evt.PreviousState, Json),
            NewStateJson = evt.NewState is null
                ? null
                : JsonSerializer.Serialize(evt.NewState, Json),
            IpAddress = audit.IpAddress,
            UserAgent = audit.UserAgent,
            Success = evt.Success,
            FailureReason = evt.FailureReason,
            ActorEmail = evt.ActorEmail,
            CreatedAt = clock.GetUtcNow().UtcDateTime,
        });
    }
}
