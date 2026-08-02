namespace ApiTool.Backend.Audit;

/// <summary>Query filters for <see cref="AuditLogQueryService"/>.</summary>
public sealed record AuditLogFilter(
    string? EventType = null,
    Guid? UserId = null,
    DateTime? From = null,
    DateTime? To = null,
    int? Limit = null);
