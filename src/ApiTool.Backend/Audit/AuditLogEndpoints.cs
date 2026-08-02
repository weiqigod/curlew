using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Rbac;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Audit;

/// <summary>Registers the audit log endpoints.</summary>
public static class AuditLogEndpoints
{
    /// <summary>Maps <c>GET /api/v1/organizations/{orgId}/audit-log</c>.</summary>
    public static IEndpointRouteBuilder MapAuditLogEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}/audit-log")
            .RequireAuthorization()
            .RequireRateLimiting("audit-log-read")
            .WithTags("AuditLog");

        group.MapGet("", GetAuditLog)
            .WithName("GetAuditLog")
            .Produces<ListAuditLogResponse>(StatusCodes.Status200OK, "application/json", "application/x-ndjson", "text/csv")
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status402PaymentRequired)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required."),
        statusCode: StatusCodes.Status401Unauthorized);

    /// <summary>
    /// Returns audit log entries for the given organization.
    /// <para>
    /// When <paramref name="format"/> is <c>csv</c> or <c>jsonl</c>, streams all matching rows
    /// via <c>Transfer-Encoding: chunked</c> without the <c>MaxLimit=200</c> cap — requires
    /// Enterprise tier (<see cref="AuditLogExportTierGate"/>).
    /// </para>
    /// <para>
    /// When <paramref name="format"/> is absent, <c>json</c>, or any other value, returns a
    /// paginated <see cref="ListAuditLogResponse"/> with <c>MaxLimit=200</c> applied. No tier
    /// gate is consulted on this path.
    /// </para>
    /// </summary>
    /// <param name="format">
    /// Output format. Accepted values: <c>jsonl</c> (newline-delimited JSON), <c>csv</c>
    /// (RFC-4180 CSV with header row), <c>json</c> (paginated JSON, default).
    /// </param>
    private static async Task<IResult> GetAuditLog(
        string orgId,
        string? event_type,
        string? user_id,
        DateTime? from,
        DateTime? to,
        int? limit,
        string? format,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        AuditLogQueryService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Organization not found."),
                statusCode: StatusCodes.Status403Forbidden);

        Guid? userIdFilter = null;
        if (user_id is not null)
        {
            if (!Guid.TryParseExact(user_id, "N", out var uid))
                return HttpResults.Json(
                    new ErrorResponse("invalid_filter", "user_id must be a valid GUID in hex format (N)."),
                    statusCode: StatusCodes.Status400BadRequest);
            userIdFilter = uid;
        }

        if (from.HasValue && to.HasValue && from > to)
            return HttpResults.Json(
                new ErrorResponse("invalid_filter", "from must be before to."),
                statusCode: StatusCodes.Status400BadRequest);

        var filter = new AuditLogFilter(event_type, userIdFilter, from, to, limit);

        if (IsExportFormat(format, out var exportFormat))
        {
            // RBAC: require audit_log.export permission (M18-002).
            var rbac = await svc.EnsureExporterAuthorisedAsync(userId.Value, orgGuid, ct);
            if (rbac == AuditLogError.PermissionDenied)
                return HttpResults.Json(
                    new ErrorResponse("permission_denied", $"Missing permission: {Permissions.AuditLogExport}."),
                    statusCode: StatusCodes.Status403Forbidden);

            var gateErr = await AuditLogExportTierGate.EnsureEnterpriseAsync(tierGate, orgGuid, ct);
            if (gateErr == AuditLogExportError.OrgNotFound)
                return TierGateProblemFactory.AuthenticatedOrgNotFound(http);
            if (gateErr == AuditLogExportError.TierIneligible)
                return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                    http, db, orgGuid, AuditLogExportTierGate.RequiredMinimum, "audit_log_export", ct);

            var rows = svc.StreamForExportAsync(orgGuid, filter, ct);
            var now = DateTime.UtcNow;
            if (exportFormat == ExportFormat.Jsonl)
                await AuditLogExportStreamer.WriteJsonlAsync(http.Response, orgId, rows, now, ct);
            else
                await AuditLogExportStreamer.WriteCsvAsync(http.Response, orgId, rows, now, ct);

            return HttpResults.Empty;
        }

        // Paginated JSON path — MaxLimit=200 still applies (regression-guarded).
        var (items, err, msg) = await svc.QueryAsync(userId.Value, orgGuid, filter, ct);

        if (err == AuditLogError.InvalidFilter)
            return HttpResults.Json(
                new ErrorResponse("invalid_filter", msg!),
                statusCode: StatusCodes.Status400BadRequest);

        if (err == AuditLogError.PermissionDenied)
            return HttpResults.Json(
                new ErrorResponse("permission_denied", msg ?? $"Missing permission: {Permissions.AuditLogView}."),
                statusCode: StatusCodes.Status403Forbidden);

        return HttpResults.Ok(new ListAuditLogResponse(items));
    }

    private enum ExportFormat { Jsonl, Csv }

    private static bool IsExportFormat(string? format, out ExportFormat parsed)
    {
        if (string.Equals(format, "jsonl", StringComparison.OrdinalIgnoreCase))
        {
            parsed = ExportFormat.Jsonl;
            return true;
        }

        if (string.Equals(format, "csv", StringComparison.OrdinalIgnoreCase))
        {
            parsed = ExportFormat.Csv;
            return true;
        }

        parsed = default;
        return false;
    }
}

/// <summary>Response body for the audit log list endpoint.</summary>
public sealed record ListAuditLogResponse(IReadOnlyList<AuditLogEntryDto> Items);
