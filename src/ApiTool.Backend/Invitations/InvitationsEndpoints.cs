using ApiTool.Backend.Auth;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Invitations;

/// <summary>Registers the invitation endpoints.</summary>
public static class InvitationsEndpoints
{
    /// <summary>Maps all invitation endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapInvitationsEndpoints(this IEndpointRouteBuilder app)
    {
        // Organization-scoped invitation endpoints.
        var orgGroup = app
            .MapGroup("/api/v1/organizations/{id}/invitations")
            .RequireAuthorization()
            .WithTags("Invitations");

        orgGroup.MapPost("", CreateInvitation)
            .WithName("CreateInvitation")
            .Produces<CreateInvitationResponse>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict);

        orgGroup.MapGet("", ListInvitations)
            .WithName("ListInvitations")
            .Produces<ListInvitationsResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        orgGroup.MapPost("{inviteId}/resend", ResendInvitation)
            .WithName("ResendInvitation")
            .Produces<ResendInvitationResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .Produces<ErrorResponse>(StatusCodes.Status429TooManyRequests);

        orgGroup.MapDelete("{inviteId}", RevokeInvitation)
            .WithName("RevokeInvitation")
            .Produces<RevokeInvitationResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        // Global accept endpoint.
        var acceptGroup = app
            .MapGroup("/api/v1/invitations")
            .RequireAuthorization()
            .WithTags("Invitations");

        acceptGroup.MapPost("accept", AcceptInvitation)
            .WithName("AcceptInvitation")
            .RequireVerifiedEmail()   // M16-003: gates on email_verified = true
            .Produces<AcceptInvitationResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .Produces<ErrorResponse>(StatusCodes.Status410Gone)
            .ProducesProblem(StatusCodes.Status403Forbidden);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> CreateInvitation(
        string id,
        [FromBody] CreateInvitationRequest body,
        CurrentUserAccessor users,
        InvitationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (dto, rawToken, err, msg) = await svc.CreateAsync(userId.Value, orgGuid, body.Email, body.Role, ct);

        return err switch
        {
            InvitationError.None => HttpResults.Created(
                $"/api/v1/organizations/{id}/invitations/{dto!.Id}",
                new CreateInvitationResponse(dto, rawToken!)),
            InvitationError.InvalidEmail => HttpResults.Json(
                new ErrorResponse("invalid_email", msg!),
                statusCode: StatusCodes.Status400BadRequest),
            InvitationError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", msg!),
                statusCode: StatusCodes.Status403Forbidden),
            InvitationError.InvitationPending => HttpResults.Json(
                new ErrorResponse("invitation_pending", msg!),
                statusCode: StatusCodes.Status409Conflict),
            InvitationError.AlreadyMember => HttpResults.Json(
                new ErrorResponse("already_member", msg!),
                statusCode: StatusCodes.Status409Conflict),
            InvitationError.SeatLimitReached => HttpResults.Json(
                new ErrorResponse("seat_limit_reached", msg!),
                statusCode: StatusCodes.Status409Conflict),
            InvitationError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", msg!),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListInvitations(
        string id,
        CurrentUserAccessor users,
        InvitationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (invitations, err) = await svc.ListPendingAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            InvitationError.None => HttpResults.Ok(new ListInvitationsResponse(invitations)),
            InvitationError.OrganizationNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ResendInvitation(
        string id,
        string inviteId,
        CurrentUserAccessor users,
        InvitationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!InvitationId.TryParse(inviteId, out var inviteGuid))
            return HttpResults.Json(
                new ErrorResponse("invitation_not_found", "Invitation not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (dto, err, msg) = await svc.ResendAsync(userId.Value, orgGuid, inviteGuid, ct);

        return err switch
        {
            InvitationError.None => HttpResults.Ok(new ResendInvitationResponse(dto!)),
            InvitationError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", msg!),
                statusCode: StatusCodes.Status403Forbidden),
            InvitationError.InvitationNotFound => HttpResults.Json(
                new ErrorResponse("invitation_not_found", msg!),
                statusCode: StatusCodes.Status404NotFound),
            InvitationError.ResendLimitReached => HttpResults.Json(
                new ErrorResponse("resend_limit_reached", msg!),
                statusCode: StatusCodes.Status429TooManyRequests),
            InvitationError.ResendCooldown => HttpResults.Json(
                new ErrorResponse("resend_cooldown", msg!),
                statusCode: StatusCodes.Status429TooManyRequests),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> RevokeInvitation(
        string id,
        string inviteId,
        CurrentUserAccessor users,
        InvitationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!InvitationId.TryParse(inviteId, out var inviteGuid))
            return HttpResults.Json(
                new ErrorResponse("invitation_not_found", "Invitation not found."),
                statusCode: StatusCodes.Status404NotFound);

        var err = await svc.RevokeAsync(userId.Value, orgGuid, inviteGuid, ct);

        return err switch
        {
            InvitationError.None => HttpResults.Ok(new RevokeInvitationResponse("Invitation revoked.")),
            InvitationError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only admins and owners can revoke invitations."),
                statusCode: StatusCodes.Status403Forbidden),
            InvitationError.InvitationNotFound => HttpResults.Json(
                new ErrorResponse("invitation_not_found", "Invitation not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> AcceptInvitation(
        [FromBody] AcceptInvitationRequest body,
        CurrentUserAccessor users,
        InvitationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var (dto, role, err, msg) = await svc.AcceptAsync(userId.Value, body.Token, ct);

        return err switch
        {
            InvitationError.None => HttpResults.Ok(new AcceptInvitationResponse(dto!)),
            InvitationError.InvitationNotFound => HttpResults.Json(
                new ErrorResponse("invitation_not_found", msg!),
                statusCode: StatusCodes.Status404NotFound),
            InvitationError.InvitationExpired => HttpResults.Json(
                new ErrorResponse("invitation_expired", msg!),
                statusCode: StatusCodes.Status410Gone),
            InvitationError.AlreadyMember => HttpResults.Json(
                new ErrorResponse("already_member", msg!),
                statusCode: StatusCodes.Status409Conflict),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    // Response types.
    private sealed record CreateInvitationResponse(InvitationDto Invitation, string Token);
    private sealed record ListInvitationsResponse(IReadOnlyList<InvitationDto> Invitations);
    private sealed record ResendInvitationResponse(InvitationDto Invitation);
    private sealed record RevokeInvitationResponse(string Message);
    private sealed record AcceptInvitationResponse(InvitationDto Invitation);
}
