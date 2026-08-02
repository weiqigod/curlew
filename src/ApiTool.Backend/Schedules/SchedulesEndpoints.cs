using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Http;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Schedules;

/// <summary>Registers the schedules endpoints onto the route builder.</summary>
public static class SchedulesEndpoints
{
    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    /// <summary>Maps all schedules endpoints onto the given route builder.</summary>
    /// <param name="app">The route builder to extend.</param>
    /// <returns>The same builder, for chaining.</returns>
    public static IEndpointRouteBuilder MapSchedulesEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}/schedules")
            .RequireAuthorization()
            .WithTags("Schedules");

        group.MapPost("", CreateSchedule)
            .WithName("CreateSchedule")
            .Accepts<CreateScheduleRequest>("application/json")
            .Produces<ScheduleDto>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict)
            .ProducesProblem(StatusCodes.Status402PaymentRequired)
            .ProducesProblem(StatusCodes.Status422UnprocessableEntity);

        group.MapGet("", ListSchedules)
            .WithName("ListSchedules")
            .Produces<ListSchedulesResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        group.MapGet("{name}", GetSchedule)
            .WithName("GetSchedule")
            .Produces<ScheduleDto>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        group.MapPost("{name}/run-now", RunNow)
            .WithName("RunNowSchedule")
            .Produces<RunNowResponse>(StatusCodes.Status202Accepted)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        group.MapGet("{name}/runs", ListRuns)
            .WithName("ListScheduleRuns")
            .Produces<ListRunsResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> CreateSchedule(
        string orgId,
        HttpContext http,
        HttpRequest request,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        SchedulesService svc,
        CancellationToken ct)
    {
        CreateScheduleRequest? body;
        try
        {
            body = await request.ReadFromJsonAsync<CreateScheduleRequest>(SnakeCaseOptions, ct);
        }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == ScheduleExecutorError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, ScheduleExecutorTierGate.RequiredMinimum, "schedule_executor", ct);
        if (gateErr == ScheduleExecutorError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        var (dto, error, message) = await svc.CreateAsync(userId.Value, orgGuid, body, ct);

        return error switch
        {
            ScheduleError.None => HttpResults.Json(dto, statusCode: StatusCodes.Status201Created),
            ScheduleError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", message ?? "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            ScheduleError.InvalidCron => HttpResults.Json(
                new ErrorResponse("invalid_cron", message ?? "Invalid cron expression."),
                statusCode: StatusCodes.Status400BadRequest),
            ScheduleError.ScheduleNameTaken => HttpResults.Json(
                new ErrorResponse("schedule_name_taken", message ?? "Schedule name is already taken."),
                statusCode: StatusCodes.Status409Conflict),
            ScheduleError.InvalidName => HttpResults.Json(
                new ErrorResponse("invalid_name", message ?? "Schedule name is required."),
                statusCode: StatusCodes.Status400BadRequest),
            ScheduleError.InvalidCollectionRef => HttpResults.Json(
                new ErrorResponse("invalid_collection_ref", message ?? "collection_ref is required."),
                statusCode: StatusCodes.Status400BadRequest),
            ScheduleError.InvalidTimezone => ScheduleProblem.InvalidTimezone(http, message),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListSchedules(
        string orgId,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        SchedulesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == ScheduleExecutorError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, ScheduleExecutorTierGate.RequiredMinimum, "schedule_executor", ct);
        if (gateErr == ScheduleExecutorError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        var (schedules, error) = await svc.ListAsync(userId.Value, orgGuid, ct);

        return error switch
        {
            ScheduleError.None => HttpResults.Ok(new ListSchedulesResponse(schedules)),
            ScheduleError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> GetSchedule(
        string orgId,
        string name,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        SchedulesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("schedule_not_found", "Schedule not found."),
                statusCode: StatusCodes.Status404NotFound);
        }

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == ScheduleExecutorError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, ScheduleExecutorTierGate.RequiredMinimum, "schedule_executor", ct);
        if (gateErr == ScheduleExecutorError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        var (dto, error) = await svc.GetByNameAsync(userId.Value, orgGuid, name, ct);

        return error switch
        {
            ScheduleError.None => HttpResults.Ok(dto),
            ScheduleError.NotFound => HttpResults.Json(
                new ErrorResponse("schedule_not_found", "Schedule not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> RunNow(
        string orgId,
        string name,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        SchedulesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == ScheduleExecutorError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, ScheduleExecutorTierGate.RequiredMinimum, "schedule_executor", ct);
        if (gateErr == ScheduleExecutorError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        var (dto, error) = await svc.RunNowAsync(userId.Value, orgGuid, name, ct);

        return error switch
        {
            ScheduleError.None => HttpResults.Json(
                new RunNowResponse(dto!.RunId, dto.Status),
                statusCode: StatusCodes.Status202Accepted),
            ScheduleError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            ScheduleError.NotFound => HttpResults.Json(
                new ErrorResponse("schedule_not_found", "Schedule not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListRuns(
        string orgId,
        string name,
        int? limit,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        SchedulesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var gateErr = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == ScheduleExecutorError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, ScheduleExecutorTierGate.RequiredMinimum, "schedule_executor", ct);
        if (gateErr == ScheduleExecutorError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        var (runs, error) = await svc.ListRunsAsync(userId.Value, orgGuid, name, limit ?? 20, ct);

        return error switch
        {
            ScheduleError.None => HttpResults.Ok(new ListRunsResponse(runs)),
            ScheduleError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            ScheduleError.NotFound => HttpResults.Json(
                new ErrorResponse("schedule_not_found", "Schedule not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    /// <summary>Response wrapper for list schedules.</summary>
    /// <param name="Schedules">List of schedules for the organization.</param>
    public sealed record ListSchedulesResponse(IReadOnlyList<ScheduleDto> Schedules);

    /// <summary>Response wrapper for list runs.</summary>
    /// <param name="Runs">Newest-first list of scheduled runs.</param>
    public sealed record ListRunsResponse(IReadOnlyList<ScheduledRunDto> Runs);

    /// <summary>Response body for a successful run-now call (202 Accepted).</summary>
    /// <param name="RunId">Wire-format run id (e.g. <c>run_abc123…</c>).</param>
    /// <param name="Status">Status string, always <c>"queued"</c>.</param>
    public sealed record RunNowResponse(string RunId, string Status);
}
