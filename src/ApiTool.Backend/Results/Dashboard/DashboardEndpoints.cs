using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Rbac;
using Microsoft.AspNetCore.Http;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Results.Dashboard;

/// <summary>
/// Maps <c>GET /api/v1/organizations/{orgId}/results/stats</c> and
/// <c>GET /api/v1/organizations/{orgId}/results/failures</c>.
/// Both endpoints are gated by <see cref="DashboardTierGate.EnsureTeamOrAboveAsync"/> (Team tier required)
/// and the <c>dashboard.view</c> RBAC permission.
/// </summary>
public static class DashboardEndpoints
{
    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    /// <summary>Maps dashboard result-aggregation endpoints onto the route builder.</summary>
    public static IEndpointRouteBuilder MapDashboardEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}/results")
            .RequireAuthorization()
            .WithTags("Dashboard");

        group.MapGet("/stats", GetStats)
            .WithName("GetResultsStats")
            .Produces<StatsResponse>(StatusCodes.Status200OK)
            .ProducesProblem(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        group.MapGet("/failures", GetFailures)
            .WithName("GetResultsFailures")
            .Produces<FailuresResponse>(StatusCodes.Status200OK)
            .ProducesProblem(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        return app;
    }

    // ── handlers ─────────────────────────────────────────────────────────────

    private static async Task<IResult> GetStats(
        string orgId,
        string? window,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        RoleResolver roles,
        DashboardResultsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var orgGuid = await OrgResolver.ResolveAsync(orgId, db, ct);
        if (orgGuid is null)
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);

        // 1. Tier gate (402 on Free)
        var gateErr = await DashboardTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid.Value, ct);
        if (gateErr == DashboardError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid.Value, DashboardTierGate.RequiredMinimum, "dashboard", ct);
        if (gateErr == DashboardError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        // 2. RBAC (403 if missing dashboard.view)
        if (!await roles.HasPermissionAsync(userId.Value, orgGuid.Value, Permissions.DashboardView, ct))
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);

        // 3. Parse window
        if (!DashboardWindowExtensions.TryParse(window, out var parsedWindow))
            return DashboardProblem.UnsupportedWindow(http, window);

        // 4. Execute
        var resp = await svc.GetStatsAsync(orgGuid.Value, parsedWindow, ct);
        return HttpResults.Ok(resp);
    }

    private static async Task<IResult> GetFailures(
        string orgId,
        string? window,
        int? limit,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        RoleResolver roles,
        DashboardResultsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var orgGuid = await OrgResolver.ResolveAsync(orgId, db, ct);
        if (orgGuid is null)
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);

        // 1. Tier gate (402 on Free)
        var gateErr = await DashboardTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid.Value, ct);
        if (gateErr == DashboardError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid.Value, DashboardTierGate.RequiredMinimum, "dashboard", ct);
        if (gateErr == DashboardError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        // 2. RBAC (403 if missing dashboard.view)
        if (!await roles.HasPermissionAsync(userId.Value, orgGuid.Value, Permissions.DashboardView, ct))
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);

        // 3. Parse window
        if (!DashboardWindowExtensions.TryParse(window, out var parsedWindow))
            return DashboardProblem.UnsupportedWindow(http, window);

        // 4. Execute
        var requestedLimit = limit ?? DashboardResultsService.DefaultFailuresLimit;
        var resp = await svc.GetFailuresAsync(orgGuid.Value, parsedWindow, requestedLimit, ct);

        // 5. Emit Warning header if limit was clamped (RFC 7234 §5.5 + body field)
        if (resp.LimitClamped)
        {
            http.Response.Headers["Warning"] =
                $"299 - \"limit_clamped: requested={requestedLimit}, applied={resp.Limit}\"";
        }

        return HttpResults.Ok(resp);
    }
}
