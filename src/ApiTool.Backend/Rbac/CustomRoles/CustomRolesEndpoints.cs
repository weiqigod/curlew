using ApiTool.Backend.Auth;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Registers the custom roles endpoints onto the route builder.</summary>
public static class CustomRolesEndpoints
{
    /// <summary>Maps all custom roles endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapCustomRolesEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{id}/roles")
            .RequireAuthorization()
            .WithTags("CustomRoles");

        group.MapPost("", CreateRole)
            .WithName("CreateCustomRole")
            .Produces<CustomRoleDto>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict);

        group.MapGet("", ListRoles)
            .WithName("ListCustomRoles")
            .Produces<ListRolesResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapPatch("{roleId}", UpdateRole)
            .WithName("UpdateCustomRole")
            .Produces<CustomRoleDto>()
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict);

        group.MapDelete("{roleId}", DeleteRole)
            .WithName("DeleteCustomRole")
            .Produces(StatusCodes.Status204NoContent)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> CreateRole(
        string id,
        [FromBody] CreateRoleRequest body,
        CurrentUserAccessor users,
        CustomRolesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (dto, err, message) = await svc.CreateAsync(userId.Value, orgGuid, body, ct);

        return err switch
        {
            CustomRoleError.None => HttpResults.Json(dto, statusCode: StatusCodes.Status201Created),
            CustomRoleError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only org owners can create custom roles."),
                statusCode: StatusCodes.Status403Forbidden),
            CustomRoleError.InvalidName => HttpResults.Json(
                new ErrorResponse("invalid_name", message ?? "Invalid role name."),
                statusCode: StatusCodes.Status400BadRequest),
            CustomRoleError.InvalidPermission => HttpResults.Json(
                new ErrorResponse("invalid_permission", $"Unknown permission key.", message),
                statusCode: StatusCodes.Status400BadRequest),
            CustomRoleError.RoleNameTaken => HttpResults.Json(
                new ErrorResponse("role_name_taken", "A role with this name already exists in the organization."),
                statusCode: StatusCodes.Status409Conflict),
            CustomRoleError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListRoles(
        string id,
        CurrentUserAccessor users,
        CustomRolesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (roles, err) = await svc.ListAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            CustomRoleError.None => HttpResults.Ok(new ListRolesResponse(roles)),
            CustomRoleError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> DeleteRole(
        string id,
        string roleId,
        CurrentUserAccessor users,
        CustomRolesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!RoleId.TryParse(roleId, out var roleGuid))
            return HttpResults.Json(
                new ErrorResponse("role_not_found", "Role not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (err, memberCount) = await svc.DeleteAsync(userId.Value, orgGuid, roleGuid, ct);

        return err switch
        {
            CustomRoleError.None => HttpResults.NoContent(),
            CustomRoleError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only org owners can delete custom roles."),
                statusCode: StatusCodes.Status403Forbidden),
            CustomRoleError.RoleNotFound => HttpResults.Json(
                new ErrorResponse("role_not_found", "Role not found."),
                statusCode: StatusCodes.Status404NotFound),
            CustomRoleError.RoleInUse => HttpResults.Json(
                new { code = "role_in_use", message = "Role is still assigned to members.", member_count = memberCount },
                statusCode: StatusCodes.Status409Conflict),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> UpdateRole(
        string id,
        string roleId,
        [FromBody] UpdateRoleRequest body,
        CurrentUserAccessor users,
        CustomRolesService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!RoleId.TryParse(roleId, out var roleGuid))
            return HttpResults.Json(
                new ErrorResponse("role_not_found", "Role not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (dto, err, message) = await svc.UpdateAsync(userId.Value, orgGuid, roleGuid, body, ct);
        return err switch
        {
            CustomRoleError.None => HttpResults.Ok(dto),
            CustomRoleError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only org owners can edit custom roles."),
                statusCode: StatusCodes.Status403Forbidden),
            CustomRoleError.InvalidName => HttpResults.Json(
                new ErrorResponse("invalid_name", message ?? "Invalid role name."),
                statusCode: StatusCodes.Status400BadRequest),
            CustomRoleError.InvalidPermission => HttpResults.Json(
                new ErrorResponse("invalid_permission", "Unknown permission key.", message),
                statusCode: StatusCodes.Status400BadRequest),
            CustomRoleError.RoleNameTaken => HttpResults.Json(
                new ErrorResponse("role_name_taken", "A role with this name already exists in the organization."),
                statusCode: StatusCodes.Status409Conflict),
            CustomRoleError.RoleNotFound => HttpResults.Json(
                new ErrorResponse("role_not_found", "Role not found."),
                statusCode: StatusCodes.Status404NotFound),
            CustomRoleError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    /// <summary>Response wrapper for the list roles endpoint.</summary>
    private sealed record ListRolesResponse(IReadOnlyList<CustomRoleDto> Roles);
}
