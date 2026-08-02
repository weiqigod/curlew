using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Rbac.CustomRoles;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Audit;

/// <summary>Queries the organization audit log with RBAC enforcement and filtering.</summary>
public sealed class AuditLogQueryService(AppDbContext db, RoleResolver roles)
{
    private const int DefaultLimit = 50;
    private const int MaxLimit = 200;

    /// <summary>Returns audit log entries for the given organization.</summary>
    public async Task<(IReadOnlyList<AuditLogEntryDto> items, AuditLogError err, string? msg)>
        QueryAsync(Guid userId, Guid orgId, AuditLogFilter filter, CancellationToken ct)
    {
        // Validate date range
        if (filter.From.HasValue && filter.To.HasValue && filter.From > filter.To)
            return ([], AuditLogError.InvalidFilter, "from must be before to.");

        // RBAC: require audit_log.view permission (M18-002)
        if (!await roles.HasPermissionAsync(userId, orgId, Permissions.AuditLogView, ct))
            return ([], AuditLogError.PermissionDenied, $"Missing permission: {Permissions.AuditLogView}.");

        var limit = Math.Clamp(filter.Limit ?? DefaultLimit, 1, MaxLimit);
        var rows = await BuildFilteredQuery(orgId, filter)
            .OrderByDescending(e => e.CreatedAt)
            .Take(limit)
            .ToListAsync(ct);

        return (rows.Select(ToDto).ToList(), AuditLogError.None, null);
    }

    /// <summary>
    /// Verifies the caller has the <c>audit_log.export</c> permission on the org.
    /// Returns <see cref="AuditLogError.PermissionDenied"/> if not. Used by the
    /// bulk-export endpoint to fail fast before opening the stream.
    /// </summary>
    public async Task<AuditLogError> EnsureExporterAuthorisedAsync(
        Guid userId, Guid orgId, CancellationToken ct)
    {
        if (!await roles.HasPermissionAsync(userId, orgId, Permissions.AuditLogExport, ct))
            return AuditLogError.PermissionDenied;
        return AuditLogError.None;
    }

    /// <summary>
    /// Streams all rows matching <paramref name="filter"/> for an org, newest-first,
    /// without applying the <c>MaxLimit=200</c> cap that <see cref="QueryAsync"/>
    /// enforces. Caller is responsible for tier-gating and RBAC; this method
    /// performs no auth checks (RBAC is deliberately decoupled so the endpoint
    /// can fail fast before opening the response stream).
    /// </summary>
    public async IAsyncEnumerable<AuditLogEntryDto> StreamForExportAsync(
        Guid orgId, AuditLogFilter filter,
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken ct)
    {
        var q = BuildFilteredQuery(orgId, filter)
            .OrderByDescending(e => e.CreatedAt)
            .Select(e => ToDto(e));

        await foreach (var dto in q.AsAsyncEnumerable().WithCancellation(ct))
            yield return dto;
    }

    private IQueryable<OrganizationAuditLogEntry> BuildFilteredQuery(Guid orgId, AuditLogFilter filter)
    {
        var q = db.OrganizationAuditLog.Where(e => e.OrgId == orgId);

        if (filter.EventType is not null)
            q = q.Where(e => e.EventType == filter.EventType);
        if (filter.UserId.HasValue)
            q = q.Where(e => e.ActorId == (Guid?)filter.UserId.Value);
        if (filter.From.HasValue)
            q = q.Where(e => e.CreatedAt >= filter.From.Value);
        if (filter.To.HasValue)
            q = q.Where(e => e.CreatedAt <= filter.To.Value);

        return q;
    }

    private static AuditLogEntryDto ToDto(OrganizationAuditLogEntry e) => new(
        EventType: e.EventType,
        UserId: e.ActorId is null || e.ActorId == Guid.Empty ? null : e.ActorId.Value.ToString("N"),
        UserEmail: e.ActorEmail,
        TargetType: e.TargetType,
        TargetId: e.TargetId?.ToString("N"),
        CreatedAt: e.CreatedAt,
        IpAddress: e.IpAddress,
        Success: e.Success,
        FailureReason: e.FailureReason);
}
