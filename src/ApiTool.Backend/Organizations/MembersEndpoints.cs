using ApiTool.Backend.Auth;
using ApiTool.Backend.Rbac.CustomRoles;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Organizations;

/// <summary>Registers the member management endpoints.</summary>
public static class MembersEndpoints
{
    /// <summary>Maps all member endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapMembersEndpoints(this IEndpointRouteBuilder app)
    {
        var orgGroup = app
            .MapGroup("/api/v1/organizations/{id}")
            .RequireAuthorization()
            .WithTags("Members");

        orgGroup.MapGet("members", ListMembers)
            .WithName("ListMembers")
            .Produces<ListMembersResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        orgGroup.MapPatch("members/{memberId}", UpdateMember)
            .WithName("UpdateMember")
            .Produces<UpdateMemberResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        orgGroup.MapDelete("members/{memberId}", RemoveMember)
            .WithName("RemoveMember")
            .Produces<RemoveMemberResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        orgGroup.MapPost("transfer", TransferOwnership)
            .WithName("TransferOwnership")
            .Produces<TransferResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status422UnprocessableEntity);

        orgGroup.MapPost("leave", Leave)
            .WithName("LeaveOrganization")
            .Produces<LeaveResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        orgGroup.MapPatch("", UpdateOrganization)
            .WithName("UpdateOrganization")
            .Produces<OrganizationDto>()
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        orgGroup.MapDelete("", DeleteOrganization)
            .WithName("DeleteOrganization")
            .Produces<OrganizationDto>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        orgGroup.MapPost("cancel-deletion", CancelDeletion)
            .WithName("CancelDeletion")
            .Produces<OrganizationDto>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> ListMembers(
        string id,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (members, err) = await svc.ListMembersAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            MemberError.None => HttpResults.Ok(new ListMembersResponse(members)),
            MemberError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> UpdateMember(
        string id,
        string memberId,
        [FromBody] UpdateMemberRequest body,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!Guid.TryParseExact(memberId, "N", out var memberGuid))
            return HttpResults.Json(
                new ErrorResponse("member_not_found", "Member not found."),
                statusCode: StatusCodes.Status404NotFound);

        // Parse optional role_id from the request body.
        Guid? roleGuid = null;
        if (body.RoleId is not null)
        {
            if (!RoleId.TryParse(body.RoleId, out var parsedRoleId))
                return HttpResults.Json(
                    new ErrorResponse("invalid_role_id", "Invalid role_id format."),
                    statusCode: StatusCodes.Status400BadRequest);
            roleGuid = parsedRoleId;
        }

        // If neither role nor role_id is provided, return 400.
        if (body.Role is not null || roleGuid is not null)
        {
            var (member2, err2) = await svc.UpdateMemberAsync(userId.Value, orgGuid, memberGuid, body.Role, roleGuid, ct);

            return err2 switch
            {
                MemberError.None => HttpResults.Ok(new UpdateMemberResponse(member2!)),
                MemberError.PermissionDenied => HttpResults.Json(
                    new ErrorResponse("permission_denied", "Only org owners can change member roles."),
                    statusCode: StatusCodes.Status403Forbidden),
                MemberError.MemberNotFound => HttpResults.Json(
                    new ErrorResponse("member_not_found", "Member not found."),
                    statusCode: StatusCodes.Status404NotFound),
                MemberError.CannotChangeOwnerRole => HttpResults.Json(
                    new ErrorResponse("cannot_change_owner_role", "The owner's role cannot be changed directly. Use transfer ownership."),
                    statusCode: StatusCodes.Status403Forbidden),
                _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            };
        }

        return HttpResults.Json(
            new ErrorResponse("bad_request", "At least one of 'role' or 'role_id' must be provided."),
            statusCode: StatusCodes.Status400BadRequest);
    }

    private static async Task<IResult> RemoveMember(
        string id,
        string memberId,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!Guid.TryParseExact(memberId, "N", out var memberGuid))
            return HttpResults.Json(
                new ErrorResponse("member_not_found", "Member not found."),
                statusCode: StatusCodes.Status404NotFound);

        var err = await svc.RemoveMemberAsync(userId.Value, orgGuid, memberGuid, ct);

        return err switch
        {
            MemberError.None => HttpResults.Ok(new RemoveMemberResponse("Member removed.")),
            MemberError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only admins and owners can remove members."),
                statusCode: StatusCodes.Status403Forbidden),
            MemberError.MemberNotFound => HttpResults.Json(
                new ErrorResponse("member_not_found", "Member not found."),
                statusCode: StatusCodes.Status404NotFound),
            MemberError.CannotRemoveOwner => HttpResults.Json(
                new ErrorResponse("cannot_remove_owner", "The owner cannot be removed. Transfer ownership first."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> TransferOwnership(
        string id,
        [FromBody] TransferOwnershipRequest body,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!Guid.TryParseExact(body.NewOwnerId, "N", out var newOwnerGuid))
            return HttpResults.Json(
                new ErrorResponse("invalid_target", "Invalid new owner id."),
                statusCode: StatusCodes.Status422UnprocessableEntity);

        var err = await svc.TransferOwnershipAsync(userId.Value, orgGuid, newOwnerGuid, ct);

        return err switch
        {
            MemberError.None => HttpResults.Ok(new TransferResponse("Ownership transferred.")),
            MemberError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only the owner can transfer ownership."),
                statusCode: StatusCodes.Status403Forbidden),
            MemberError.InvalidTransferTarget => HttpResults.Json(
                new ErrorResponse("invalid_transfer_target", "Transfer target must be an admin member."),
                statusCode: StatusCodes.Status422UnprocessableEntity),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> Leave(
        string id,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var err = await svc.LeaveAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            MemberError.None => HttpResults.Ok(new LeaveResponse("Left organization.")),
            MemberError.OwnerCannotLeave => HttpResults.Json(
                new ErrorResponse("owner_cannot_leave", "The owner cannot leave. Transfer ownership first."),
                statusCode: StatusCodes.Status403Forbidden),
            MemberError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> UpdateOrganization(
        string id,
        [FromBody] UpdateOrganizationRequest body,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (dto, err) = await svc.UpdateOrgAsync(userId.Value, orgGuid, body.Name, body.AuditLogRetentionDays, ct);

        return err switch
        {
            MemberError.None => HttpResults.Ok(dto),
            MemberError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only admins and owners can update the organization."),
                statusCode: StatusCodes.Status403Forbidden),
            MemberError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            MemberError.RetentionDaysExceedsCap => HttpResults.Json(
                new ErrorResponse("retention_days_exceeds_cap", "Retention days above 365 requires an Enterprise subscription."),
                statusCode: StatusCodes.Status400BadRequest),
            MemberError.RetentionDaysInvalid => HttpResults.Json(
                new ErrorResponse("retention_days_invalid", "Retention days must be a positive integer."),
                statusCode: StatusCodes.Status400BadRequest),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> DeleteOrganization(
        string id,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (dto, err) = await svc.DeleteOrgAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            MemberError.None => HttpResults.Ok(dto),
            MemberError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only the owner can delete the organization."),
                statusCode: StatusCodes.Status403Forbidden),
            MemberError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> CancelDeletion(
        string id,
        CurrentUserAccessor users,
        MembersService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (dto, err) = await svc.CancelDeletionAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            MemberError.None => HttpResults.Ok(dto),
            MemberError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only the owner can cancel deletion."),
                statusCode: StatusCodes.Status403Forbidden),
            MemberError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            MemberError.InvalidState => HttpResults.Json(
                new ErrorResponse("invalid_state", "Organization is not pending deletion."),
                statusCode: StatusCodes.Status409Conflict),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    // Response types.
    private sealed record ListMembersResponse(IReadOnlyList<MemberDto> Members);
    private sealed record UpdateMemberResponse(MemberDto Member);
    private sealed record RemoveMemberResponse(string Message);
    private sealed record TransferResponse(string Message);
    private sealed record LeaveResponse(string Message);
}
