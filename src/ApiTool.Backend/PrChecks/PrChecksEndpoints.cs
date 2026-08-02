using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Http;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.PrChecks;

/// <summary>Registers the pr-checks endpoints onto the route builder.</summary>
public static class PrChecksEndpoints
{
    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    /// <summary>Maps all pr-checks endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapPrChecksEndpoints(this IEndpointRouteBuilder app)
    {
        // ── v4.2.1 upload endpoint (new, bearer-token resolved org) ───────────
        // M16-014: added optional `provider` field (github|gitlab; absent = github).
        app.MapPost("/api/v1/pr-checks", PrChecksUploadEndpoint.HandleAsync)
            .RequireAuthorization()
            .WithName("UploadPrCheck")
            .WithTags("PrChecks")
            .WithDescription(
                "Posts a PR check status. The optional `provider` field selects the VCS integration: " +
                "'github' (default, backward-compatible) or 'gitlab'. " +
                "For GitLab, requires an active gitlab_installations row for the organisation.")
            .Produces<PrCheckUploadResponse>(StatusCodes.Status200OK)
            .Produces<PrCheckUploadResponse>(StatusCodes.Status202Accepted)
            .Produces(StatusCodes.Status400BadRequest)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status403Forbidden)
            .Produces(StatusCodes.Status404NotFound)
            .Produces(423) // 423 Locked — GitLab PAT revoked
            .Produces(StatusCodes.Status429TooManyRequests)
            .Produces(StatusCodes.Status502BadGateway);

        // ── v4.2 legacy endpoints (org-id in path) ────────────────────────────
        var orgGroup = app
            .MapGroup("/api/v1/organizations/{orgId}/pr-checks")
            .RequireAuthorization()
            .WithTags("PrChecks");

        orgGroup.MapPost("", PostPrCheck)
            .WithName("PostPrCheck")
            .Accepts<PrCheckRequest>("application/json")
            .Produces<PrCheckDto>(StatusCodes.Status200OK)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        orgGroup.MapGet("", ListPrChecks)
            .WithName("ListPrChecks")
            .Produces<ListPrChecksResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> PostPrCheck(
        string orgId,
        HttpRequest request,
        CurrentUserAccessor users,
        PrChecksService svc,
        AppDbContext db,
        CancellationToken ct)
    {
        PrCheckRequest? body;
        try
        {
            body = await request.ReadFromJsonAsync<PrCheckRequest>(SnakeCaseOptions, ct);
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

        var orgGuid = await OrgResolver.ResolveAsync(orgId, db, ct);
        if (orgGuid is null)
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var (dto, error, message) = await svc.PostAsync(userId.Value, orgGuid.Value, body, ct);

        return error switch
        {
            PrCheckError.None => HttpResults.Ok(dto),
            PrCheckError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", message ?? "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            PrCheckError.InvalidState => HttpResults.Json(
                new ErrorResponse("invalid_request", message ?? "Invalid payload."),
                statusCode: StatusCodes.Status400BadRequest),
            PrCheckError.ResultNotFound => HttpResults.Json(
                new ErrorResponse("result_not_found", message ?? "Result not found."),
                statusCode: StatusCodes.Status400BadRequest),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListPrChecks(
        string orgId,
        int? limit,
        CurrentUserAccessor users,
        PrChecksService svc,
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

        var (checks, error) = await svc.ListAsync(userId.Value, orgGuid.Value, limit ?? 10, ct);

        return error switch
        {
            PrCheckError.None => HttpResults.Ok(new ListPrChecksResponse(checks)),
            PrCheckError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    /// <summary>Response wrapper for the list pr-checks endpoint.</summary>
    public sealed record ListPrChecksResponse(IReadOnlyList<PrCheckDto> PrChecks);
}
