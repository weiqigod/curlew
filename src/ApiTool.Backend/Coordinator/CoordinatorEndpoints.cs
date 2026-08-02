using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Http;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Coordinator;

/// <summary>Registers the coordinator endpoints onto the route builder.</summary>
public static class CoordinatorEndpoints
{
    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required."),
        statusCode: StatusCodes.Status401Unauthorized);

    /// <summary>Maps all coordinator endpoints onto the given route builder.</summary>
    /// <param name="app">The route builder to extend.</param>
    /// <returns>The same builder, for chaining.</returns>
    public static IEndpointRouteBuilder MapCoordinatorEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}/coordinator")
            .RequireAuthorization()
            .WithTags("Coordinator");

        group.MapPost("jobs", CreateJob)
            .WithName("CreateCoordinatorJob")
            .Accepts<CreateJobRequest>("application/json")
            .Produces<CoordinatorJobDto>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        group.MapGet("jobs/{jobId}", GetJob)
            .WithName("GetCoordinatorJob")
            .Produces<CoordinatorJobDto>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapPost("jobs/{jobId}/claim", ClaimShard)
            .WithName("ClaimCoordinatorShard")
            .Accepts<ClaimRequest>("application/json")
            .Produces<CoordinatorShardDto>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status204NoContent)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapPost("jobs/{jobId}/shards/{shardId}/result", SubmitResult)
            .WithName("SubmitCoordinatorShardResult")
            .Accepts<SubmitResultRequest>("application/json")
            .Produces(StatusCodes.Status202Accepted)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapPost("jobs/{jobId}/shards/{shardId}/heartbeat", Heartbeat)
            .WithName("CoordinatorShardHeartbeat")
            .Accepts<HeartbeatRequest>("application/json")
            .Produces(StatusCodes.Status204NoContent)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        return app;
    }

    // ── Handlers ──────────────────────────────────────────────────────────────

    private static async Task<IResult> CreateJob(
        string orgId,
        HttpRequest request,
        CurrentUserAccessor users,
        CoordinatorService svc,
        CancellationToken ct)
    {
        CreateJobRequest? body;
        try { body = await request.ReadFromJsonAsync<CreateJobRequest>(SnakeCaseOptions, ct); }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return Forbidden403("Permission denied.");

        var (dto, error, message) = await svc.CreateJobAsync(userId.Value, orgGuid, body, ct);

        return error switch
        {
            CoordinatorError.None => HttpResults.Json(dto, statusCode: StatusCodes.Status201Created),
            CoordinatorError.PermissionDenied => Forbidden403(message ?? "Permission denied."),
            CoordinatorError.InvalidShardCount => BadRequest400("invalid_shard_count", message ?? "Invalid shard_count."),
            CoordinatorError.InvalidRequest => BadRequest400("invalid_request", message ?? "Invalid request."),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> GetJob(
        string orgId,
        string jobId,
        CurrentUserAccessor users,
        CoordinatorService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return Forbidden403("Permission denied.");

        if (!CoordinatorJobId.TryParse(jobId, out var jobGuid))
            return NotFound404("Job not found.");

        var (dto, error) = await svc.GetJobAsync(userId.Value, orgGuid, jobGuid, ct);

        return error switch
        {
            CoordinatorError.None => HttpResults.Ok(dto),
            CoordinatorError.PermissionDenied => Forbidden403("Permission denied."),
            CoordinatorError.NotFound => NotFound404("Job not found."),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ClaimShard(
        string orgId,
        string jobId,
        HttpRequest request,
        CurrentUserAccessor users,
        CoordinatorService svc,
        CancellationToken ct)
    {
        ClaimRequest? body;
        try { body = await request.ReadFromJsonAsync<ClaimRequest>(SnakeCaseOptions, ct); }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return Forbidden403("Permission denied.");

        if (!CoordinatorJobId.TryParse(jobId, out var jobGuid))
            return NotFound404("Job not found.");

        var (dto, error) = await svc.ClaimAsync(userId.Value, orgGuid, jobGuid, body, ct);

        return error switch
        {
            CoordinatorError.None => HttpResults.Ok(dto),
            CoordinatorError.NoShardsAvailable => HttpResults.NoContent(),
            CoordinatorError.PermissionDenied => Forbidden403("Permission denied."),
            CoordinatorError.NotFound => NotFound404("Job not found."),
            CoordinatorError.InvalidRequest => BadRequest400("invalid_request", "worker_id is required."),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> SubmitResult(
        string orgId,
        string jobId,
        string shardId,
        HttpRequest request,
        CurrentUserAccessor users,
        CoordinatorService svc,
        CancellationToken ct)
    {
        SubmitResultRequest? body;
        try { body = await request.ReadFromJsonAsync<SubmitResultRequest>(SnakeCaseOptions, ct); }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return Forbidden403("Permission denied.");

        if (!CoordinatorJobId.TryParse(jobId, out var jobGuid))
            return NotFound404("Job not found.");

        if (!ShardId.TryParse(shardId, out var shardGuid))
            return NotFound404("Shard not found.");

        var (error, message) = await svc.SubmitResultAsync(userId.Value, orgGuid, jobGuid, shardGuid, body, ct);

        return error switch
        {
            CoordinatorError.None => HttpResults.Accepted(),
            CoordinatorError.PermissionDenied => Forbidden403(message ?? "Permission denied."),
            CoordinatorError.ShardNotClaimed => Forbidden403(message ?? "Shard is not owned by the calling worker."),
            CoordinatorError.NotFound => NotFound404(message ?? "Not found."),
            CoordinatorError.InvalidState => BadRequest400("invalid_state", message ?? "Invalid state."),
            CoordinatorError.InvalidRequest => BadRequest400("invalid_request", message ?? "Invalid request."),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> Heartbeat(
        string orgId,
        string jobId,
        string shardId,
        HttpRequest request,
        CurrentUserAccessor users,
        CoordinatorService svc,
        CancellationToken ct)
    {
        HeartbeatRequest? body;
        try { body = await request.ReadFromJsonAsync<HeartbeatRequest>(SnakeCaseOptions, ct); }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return Forbidden403("Permission denied.");

        if (!CoordinatorJobId.TryParse(jobId, out var jobGuid))
            return NotFound404("Job not found.");

        if (!ShardId.TryParse(shardId, out var shardGuid))
            return NotFound404("Shard not found.");

        var (error, message) = await svc.HeartbeatAsync(userId.Value, orgGuid, jobGuid, shardGuid, body, ct);

        return error switch
        {
            CoordinatorError.None => HttpResults.NoContent(),
            CoordinatorError.PermissionDenied => Forbidden403(message ?? "Permission denied."),
            CoordinatorError.ShardNotClaimed => Forbidden403(message ?? "Shard is not owned by the calling worker."),
            CoordinatorError.NotFound => NotFound404(message ?? "Not found."),
            CoordinatorError.InvalidState => BadRequest400("invalid_state", message ?? "Invalid state."),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    // ── Shared result helpers ─────────────────────────────────────────────────

    private static IResult Forbidden403(string message) => HttpResults.Json(
        new ErrorResponse("permission_denied", message),
        statusCode: StatusCodes.Status403Forbidden);

    private static IResult NotFound404(string message) => HttpResults.Json(
        new ErrorResponse("not_found", message),
        statusCode: StatusCodes.Status404NotFound);

    private static IResult BadRequest400(string code, string message) => HttpResults.Json(
        new ErrorResponse(code, message),
        statusCode: StatusCodes.Status400BadRequest);
}
