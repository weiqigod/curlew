using System.Text.Json;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Rbac.CustomRoles;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Results;

/// <summary>Registers the results endpoints onto the route builder.</summary>
public static class ResultsEndpoints
{
    /// <summary>Maximum body size (5 MB) for POST /results.</summary>
    public const long MaxRequestBytes = 5 * 1024 * 1024;

    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    /// <summary>Maps all results endpoints onto the given route builder.</summary>
    /// <param name="app">The route builder to extend.</param>
    /// <returns>The same builder, for chaining.</returns>
    public static IEndpointRouteBuilder MapResultsEndpoints(this IEndpointRouteBuilder app)
    {
        var orgGroup = app
            .MapGroup("/api/v1/organizations/{orgId}/results")
            .RequireAuthorization()
            .WithTags("Results");

        orgGroup.MapPost("", IngestResult)
            .WithName("IngestResult")
            .WithMetadata(new RequestSizeLimitAttribute(MaxRequestBytes))
            .RequireRateLimiting("results-ingest")
            .Accepts<UploadResultRequest>("application/json")
            .Produces<IngestResultResponse>(StatusCodes.Status202Accepted)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status413PayloadTooLarge)
            .Produces<ErrorResponse>(StatusCodes.Status429TooManyRequests);

        orgGroup.MapGet("", ListResults)
            .WithName("ListResults")
            .Produces<ListResultsResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        app.MapGet("/api/v1/results/{resultId}", GetResult)
            .RequireAuthorization()
            .WithTags("Results")
            .WithName("GetResult")
            .Produces<ResultDetailDto>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> IngestResult(
        string orgId,
        HttpRequest request,
        CurrentUserAccessor users,
        ResultsService svc,
        RoleResolver roleResolver,
        AppDbContext db,
        IAuditWriter audit,
        CancellationToken ct)
    {
        // Check Content-Length before deserialization so tests can verify the size limit.
        // On Kestrel, RequestSizeLimitAttribute also enforces this at the transport layer.
        if (request.ContentLength.HasValue && request.ContentLength.Value > MaxRequestBytes)
        {
            return HttpResults.Json(
                new ErrorResponse("payload_too_large", "Request body exceeds the maximum allowed size of 5 MB."),
                statusCode: StatusCodes.Status413RequestEntityTooLarge);
        }

        UploadResultRequest? body;
        try
        {
            body = await request.ReadFromJsonAsync<UploadResultRequest>(SnakeCaseOptions, ct);
        }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_result_schema", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var orgGuid = await OrgResolver.ResolveAsync(orgId, db, ct);
        if (orgGuid is null)
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        // Permission check — ensures members holding a custom role that excludes
        // results.upload are denied. Built-in owners/admins/members all have this
        // permission, so existing tests are unaffected.
        if (!await roleResolver.HasPermissionAsync(userId.Value, orgGuid.Value, Permissions.ResultsUpload, ct))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "results.upload permission is required."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var (dto, error, message, fieldPointer) = await svc.IngestAsync(userId.Value, orgGuid.Value, body, ct);

        if (error == ResultError.None)
        {
            // Parse the raw GUID from the "res_<hex>" wire format so it can be stored as TargetId.
            var resultGuid = ResultId.TryParse(dto!.Id, out var rg) ? rg : Guid.Empty;
            // Resolve the actor email so the audit-log UI can display a human-readable
            // name instead of the raw user id. CurrentUserAccessor already upserted the
            // row via ResolveAsync above, so this lookup is a hot-path read.
            var actorEmail = await db.Users
                .Where(u => u.Id == userId.Value)
                .Select(u => u.Email)
                .FirstOrDefaultAsync(ct);
            audit.Append(new AuditEvent(
                OrgId: orgGuid.Value,
                ActorId: userId.Value,
                EventType: "results.upload",
                TargetType: "result",
                TargetId: resultGuid,
                Payload: new
                {
                    collection = body?.CollectionName,
                    pass = body?.PassCount ?? 0,
                    fail = body?.FailCount ?? 0,
                },
                ActorEmail: string.IsNullOrEmpty(actorEmail) ? null : actorEmail));
            await db.SaveChangesAsync(ct);
        }

        return error switch
        {
            ResultError.None => HttpResults.Json(
                new IngestResultResponse(dto!.Id, "accepted"),
                statusCode: StatusCodes.Status202Accepted),
            ResultError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", message ?? "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            ResultError.InvalidSchema => HttpResults.Json(
                new ErrorResponse("invalid_result_schema", message ?? "Invalid payload.", fieldPointer),
                statusCode: StatusCodes.Status400BadRequest),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListResults(
        string orgId,
        int? limit,
        CurrentUserAccessor users,
        ResultsService svc,
        AppDbContext db,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var orgGuid = await OrgResolver.ResolveAsync(orgId, db, ct);
        if (orgGuid is null)
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var (results, error) = await svc.ListAsync(userId.Value, orgGuid.Value, limit ?? 10, ct);

        return error switch
        {
            ResultError.None => HttpResults.Ok(new ListResultsResponse(results)),
            ResultError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> GetResult(
        string resultId,
        CurrentUserAccessor users,
        ResultsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!ResultId.TryParse(resultId, out var resultGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("result_not_found", "Result not found."),
                statusCode: StatusCodes.Status404NotFound);
        }

        var (dto, error) = await svc.GetDetailAsync(userId.Value, resultGuid, ct);

        return error switch
        {
            ResultError.None => HttpResults.Ok(dto),
            ResultError.NotFound => HttpResults.Json(
                new ErrorResponse("result_not_found", "Result not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    /// <summary>Response body for a successful result ingestion (202 Accepted).</summary>
    /// <param name="ResultId">The wire-format result id (e.g. <c>res_abc123…</c>).</param>
    /// <param name="Status">Acceptance status string, always <c>"accepted"</c>.</param>
    public sealed record IngestResultResponse(string ResultId, string Status);

    /// <summary>Response wrapper for the list results endpoint.</summary>
    /// <param name="Results">Newest-first list of result headers.</param>
    public sealed record ListResultsResponse(IReadOnlyList<ResultDto> Results);
}
