using ApiTool.Backend.Auth;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Organizations;

/// <summary>Registers the <c>/api/v1/organizations</c> endpoint group.</summary>
public static class OrganizationsEndpoints
{
    /// <summary>Maps all organization endpoints onto the given route builder.</summary>
    /// <param name="app">The route builder to extend.</param>
    /// <returns>The same builder, for chaining.</returns>
    public static IEndpointRouteBuilder MapOrganizationsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations")
            .RequireAuthorization()
            .WithTags("Organizations");

        group.MapGet("", ListOrganizations)
            .WithName("ListOrganizations")
            .Produces<ListOrganizationsResponse>()
            .Produces(StatusCodes.Status401Unauthorized);

        group.MapPost("", CreateOrganization)
            .WithName("CreateOrganization")
            .Produces<OrganizationDto>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict);

        group.MapGet("{id}", GetOrganization)
            .WithName("GetOrganization")
            .Produces<OrganizationDto>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> ListOrganizations(
        CurrentUserAccessor users,
        OrganizationService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var orgs = await svc.ListForUserAsync(userId.Value, ct);
        return HttpResults.Ok(new ListOrganizationsResponse(orgs));
    }

    private static async Task<IResult> CreateOrganization(
        [FromBody] CreateOrganizationRequest body,
        CurrentUserAccessor users,
        OrganizationService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var (dto, error, message) = await svc.CreateAsync(userId.Value, body.Name, body.Slug, ct);

        return error switch
        {
            OrgError.None => HttpResults.Created($"/api/v1/organizations/{dto!.Id}", dto),
            OrgError.InvalidName => HttpResults.Json(
                new ErrorResponse("invalid_name", message!),
                statusCode: StatusCodes.Status400BadRequest),
            OrgError.InvalidSlug => HttpResults.Json(
                new ErrorResponse("invalid_slug", message!),
                statusCode: StatusCodes.Status400BadRequest),
            OrgError.SlugTaken => HttpResults.Json(
                new ErrorResponse("organization_slug_taken", message!),
                statusCode: StatusCodes.Status409Conflict),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> GetOrganization(
        string id,
        CurrentUserAccessor users,
        OrganizationService svc,
        CancellationToken ct)
    {
        if (!OrgId.TryParse(id, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var (dto, error) = await svc.GetByIdForUserAsync(userId.Value, orgGuid, ct);

        return error switch
        {
            OrgError.None => HttpResults.Ok(dto),
            OrgError.NotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    /// <summary>Response wrapper for the list organizations endpoint.</summary>
    /// <param name="Organizations">The organizations the requesting user is a member of.</param>
    private sealed record ListOrganizationsResponse(IReadOnlyList<OrganizationDto> Organizations);
}
