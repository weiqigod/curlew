using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Schedules;

/// <summary>
/// Registers the schedule-executor worker-pull endpoints:
/// <c>GET  /api/v1/schedules/next-run</c>,
/// <c>POST /api/v1/schedules/runs/{runId}/heartbeat</c>,
/// <c>POST /api/v1/schedules/runs/{runId}/result</c>.
/// All three are gated by <see cref="ScheduleExecutorTierGate"/> (Team tier or above).
/// </summary>
public static class ScheduleExecutorEndpoints
{
    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    /// <summary>Maps schedule-executor endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapScheduleExecutorEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/v1/schedules")
            .RequireAuthorization()
            .WithTags("ScheduleExecutor");

        group.MapGet("next-run", NextRun)
            .WithName("ScheduleExecutorNextRun")
            .Produces<NextRunResponse>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status204NoContent)
            .ProducesProblem(StatusCodes.Status402PaymentRequired)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status403Forbidden);

        group.MapPost("runs/{runId}/heartbeat", Heartbeat)
            .WithName("ScheduleExecutorHeartbeat")
            .Accepts<ScheduleHeartbeatRequest>("application/json")
            .Produces(StatusCodes.Status200OK)
            .ProducesProblem(StatusCodes.Status402PaymentRequired)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status404NotFound)
            .Produces(StatusCodes.Status409Conflict);

        group.MapPost("runs/{runId}/result", SubmitResult)
            .WithName("ScheduleExecutorResult")
            .Accepts<ScheduleResultRequest>("application/json")
            .Produces(StatusCodes.Status200OK)
            .ProducesProblem(StatusCodes.Status402PaymentRequired)
            .Produces(StatusCodes.Status400BadRequest)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status404NotFound)
            .Produces(StatusCodes.Status409Conflict);

        return app;
    }

    // ── GET /api/v1/schedules/next-run ────────────────────────────────────────

    private static async Task<IResult> NextRun(
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        ScheduleExecutorService svc,
        AppDbContext db,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        var orgResult = await ResolveOrgAsync(db, userId.Value, ct);
        if (orgResult.error is not null) return orgResult.error;
        var orgId = orgResult.orgId!.Value;

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgId, ct);
        if (gateErr != ScheduleExecutorError.None)
            return await MapGateErrorAsync(http, gateErr, db, orgId, ct);

        var workerId = http.User.FindFirst("worker_id")?.Value
                    ?? http.Request.Headers["X-Worker-Id"].FirstOrDefault();

        var (dto, err) = await svc.ClaimNextAsync(orgId, workerId, ct);
        return err switch
        {
            ScheduleClaimError.None             => HttpResults.Ok(dto),
            ScheduleClaimError.NoRunsAvailable  => HttpResults.NoContent(),
            ScheduleClaimError.DecryptionFailed => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    // ── POST /api/v1/schedules/runs/{runId}/heartbeat ─────────────────────────

    private static async Task<IResult> Heartbeat(
        string runId,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        ScheduleExecutorService svc,
        AppDbContext db,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        var orgResult = await ResolveOrgAsync(db, userId.Value, ct);
        if (orgResult.error is not null) return orgResult.error;
        var orgId = orgResult.orgId!.Value;

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgId, ct);
        if (gateErr != ScheduleExecutorError.None)
            return await MapGateErrorAsync(http, gateErr, db, orgId, ct);

        if (!RunId.TryParse(runId, out var runGuid))
            return NotFound404();

        ScheduleHeartbeatRequest? body;
        try
        {
            body = await http.Request.ReadFromJsonAsync<ScheduleHeartbeatRequest>(ct);
        }
        catch (JsonException)
        {
            return HttpResults.BadRequest();
        }

        if (string.IsNullOrWhiteSpace(body?.ClaimToken) || !Guid.TryParse(body.ClaimToken, out var token))
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "claim_token must be a valid UUID."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var err = await svc.HeartbeatAsync(orgId, runGuid, token, ct);
        return err switch
        {
            ScheduleClaimError.None        => HttpResults.Ok(),
            ScheduleClaimError.NotFound    => NotFound404(),
            ScheduleClaimError.ClaimReaped => ClaimReaped409(http),
            ScheduleClaimError.StaleClaim  => StaleClaimConflict409(http),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    // ── POST /api/v1/schedules/runs/{runId}/result ────────────────────────────

    private static async Task<IResult> SubmitResult(
        string runId,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        ScheduleExecutorService svc,
        AppDbContext db,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        var orgResult = await ResolveOrgAsync(db, userId.Value, ct);
        if (orgResult.error is not null) return orgResult.error;
        var orgId = orgResult.orgId!.Value;

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgId, ct);
        if (gateErr != ScheduleExecutorError.None)
            return await MapGateErrorAsync(http, gateErr, db, orgId, ct);

        if (!RunId.TryParse(runId, out var runGuid))
            return NotFound404();

        ScheduleResultRequest? body;
        try
        {
            body = await http.Request.ReadFromJsonAsync<ScheduleResultRequest>(ct);
        }
        catch (JsonException)
        {
            return HttpResults.BadRequest();
        }

        if (body is null)
            return HttpResults.BadRequest();

        var err = await svc.SubmitResultAsync(userId.Value, orgId, runGuid, body, ct);
        return err switch
        {
            ScheduleClaimError.None             => HttpResults.Ok(),
            ScheduleClaimError.InvalidRequest   => HttpResults.BadRequest(),
            ScheduleClaimError.NotFound         => NotFound404(),
            ScheduleClaimError.AlreadyCompleted => AlreadyCompletedConflict409(http),
            ScheduleClaimError.ClaimReaped      => ClaimReaped409(http),
            ScheduleClaimError.StaleClaim       => StaleClaimConflict409(http),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    // ── private helpers ───────────────────────────────────────────────────────

    /// <summary>
    /// Resolves the caller's org by finding their most-recent org membership.
    /// Returns 403 if the user belongs to no org (unbound worker token).
    /// </summary>
    private static async Task<(Guid? orgId, IResult? error)> ResolveOrgAsync(
        AppDbContext db, Guid userId, CancellationToken ct)
    {
        var membership = await db.OrganizationMembers
            .Where(m => m.UserId == userId)
            .OrderByDescending(m => m.JoinedAt)
            .FirstOrDefaultAsync(ct);

        if (membership is null)
        {
            return (null, HttpResults.Json(
                new ErrorResponse("no_org_membership",
                    "The authenticated user is not a member of any organization."),
                statusCode: StatusCodes.Status403Forbidden));
        }

        return (membership.OrgId, null);
    }

    /// <summary>Maps a tier-gate denial to the appropriate RFC 7807 HTTP response.</summary>
    private static async Task<IResult> MapGateErrorAsync(
        HttpContext http,
        ScheduleExecutorError err,
        AppDbContext db,
        Guid orgId,
        CancellationToken ct)
    {
        if (err == ScheduleExecutorError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        // TierIneligible: delegate to factory which performs the single allowed Subscriptions query.
        return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
            http, db, orgId, ScheduleExecutorTierGate.RequiredMinimum, "schedule_executor", ct);
    }

    private static IResult NotFound404() =>
        HttpResults.Json(
            new ErrorResponse("not_found", "Run not found or you do not have access."),
            statusCode: StatusCodes.Status404NotFound);

    private static IResult ClaimReaped409(HttpContext http) =>
        HttpResults.Json(new ProblemDetails
        {
            Type   = "https://api.apitool.dev/errors/claim-reaped",
            Title  = "Claim reaped",
            Status = StatusCodes.Status409Conflict,
            Detail = "The worker's claim has been reaped due to missing heartbeats.",
            Extensions = { ["request_id"] = http.TraceIdentifier },
        }, contentType: "application/problem+json", statusCode: StatusCodes.Status409Conflict);

    private static IResult StaleClaimConflict409(HttpContext http) =>
        HttpResults.Json(new ProblemDetails
        {
            Type   = "https://api.apitool.dev/errors/stale-claim",
            Title  = "Stale claim",
            Status = StatusCodes.Status409Conflict,
            Detail = "The presented claim token does not match the current claim.",
            Extensions = { ["request_id"] = http.TraceIdentifier },
        }, contentType: "application/problem+json", statusCode: StatusCodes.Status409Conflict);

    private static IResult AlreadyCompletedConflict409(HttpContext http) =>
        HttpResults.Json(new ProblemDetails
        {
            Type   = "https://api.apitool.dev/errors/already-completed",
            Title  = "Already completed",
            Status = StatusCodes.Status409Conflict,
            Detail = "The run is already in a terminal state.",
            Extensions = { ["request_id"] = http.TraceIdentifier },
        }, contentType: "application/problem+json", statusCode: StatusCodes.Status409Conflict);
}
