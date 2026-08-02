using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Factory methods for problem-detail responses specific to the deletion state machine.
/// Each method returns a <see cref="ProblemDetails"/> wired with a machine-readable
/// <c>code</c> extension field for client-side error handling.
/// Refs docs/SPECIFICATION.md v4-5.
/// </summary>
internal static class DeletionProblems
{
    /// <summary>The X-Reauth-Token header is absent.</summary>
    public static IResult ReauthRequired() =>
        TypedResults.Problem(
            statusCode: StatusCodes.Status401Unauthorized,
            title: "Re-authentication required",
            detail: "Supply a fresh drto_ token via the X-Reauth-Token header (POST /api/v1/auth/reauth).",
            extensions: new Dictionary<string, object?> { ["code"] = "reauth_required" });

    /// <summary>The token's 5-minute TTL has elapsed.</summary>
    public static IResult ReauthExpired() =>
        TypedResults.Problem(
            statusCode: StatusCodes.Status401Unauthorized,
            title: "Re-authentication token expired",
            detail: "The X-Reauth-Token has expired. Request a new one at POST /api/v1/auth/reauth.",
            extensions: new Dictionary<string, object?> { ["code"] = "reauth_expired" });

    /// <summary>The token has already been used.</summary>
    public static IResult ReauthConsumed() =>
        TypedResults.Problem(
            statusCode: StatusCodes.Status401Unauthorized,
            title: "Re-authentication token already used",
            detail: "This X-Reauth-Token was already consumed. Request a new one at POST /api/v1/auth/reauth.",
            extensions: new Dictionary<string, object?> { ["code"] = "reauth_consumed" });

    /// <summary>The token could not be found or belongs to a different user.</summary>
    public static IResult ReauthInvalid() =>
        TypedResults.Problem(
            statusCode: StatusCodes.Status401Unauthorized,
            title: "Invalid re-authentication token",
            detail: "The X-Reauth-Token is invalid.",
            extensions: new Dictionary<string, object?> { ["code"] = "reauth_invalid" });

    /// <summary>A deletion request is already pending for this user.</summary>
    public static IResult AlreadyPending() =>
        TypedResults.Problem(
            statusCode: StatusCodes.Status409Conflict,
            title: "Deletion already pending",
            detail: "An account deletion request is already pending. Use POST .../cancel to abort it first.",
            extensions: new Dictionary<string, object?> { ["code"] = "already_pending" });

    /// <summary>No pending deletion request exists when the caller tries to cancel.</summary>
    public static IResult NoPendingRequest() =>
        TypedResults.Problem(
            statusCode: StatusCodes.Status404NotFound,
            title: "No pending deletion request",
            detail: "There is no pending account deletion request to cancel.",
            extensions: new Dictionary<string, object?> { ["code"] = "no_pending_request" });

    /// <summary>
    /// The user is the sole owner of one or more organizations that still have other members.
    /// The re-auth token is NOT consumed — the user may retry after transferring ownership.
    /// </summary>
    public static IResult OwnerCannotLeave(IReadOnlyList<BlockingOrg> blockingOrgs) =>
        TypedResults.Problem(
            statusCode: StatusCodes.Status409Conflict,
            title: "Owner cannot leave",
            detail: "You are the sole owner of one or more organizations with other members. Transfer ownership or remove all members before requesting deletion.",
            extensions: new Dictionary<string, object?>
            {
                ["code"]         = "owner_cannot_leave",
                ["error_code"]   = "OwnerCannotLeave",
                ["blocking_orgs"] = blockingOrgs,
            });
}
