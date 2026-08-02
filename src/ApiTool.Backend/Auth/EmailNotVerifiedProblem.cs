// RFC 7807 problem-detail builder for the email-not-verified 403 response.
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Factory for the RFC 7807 problem-detail response returned when
/// a gated endpoint is called by a user whose email is not yet verified.
/// Refs docs/SPECIFICATION.md:8576-8580 (verification gating policy).
/// </summary>
public static class EmailNotVerifiedProblem
{
    private const string BaseType = "https://apitool.dev/errors";

    /// <summary>
    /// Returns a 403 problem detail for a verified-email gate failure.
    /// </summary>
    public static IResult Forbidden(HttpContext http) =>
        HttpResults.Problem(new ProblemDetails
        {
            Type     = $"{BaseType}/email-not-verified",
            Title    = "Email address not verified.",
            Detail   = "This action requires a verified email address. Please verify your email and try again.",
            Status   = StatusCodes.Status403Forbidden,
            Instance = http.Request.Path,
        });

    /// <summary>
    /// Returns a 401 for anonymous callers reaching a verified-email-gated endpoint.
    /// </summary>
    public static IResult Unauthenticated(HttpContext http) =>
        HttpResults.Problem(new ProblemDetails
        {
            Type     = $"{BaseType}/unauthenticated",
            Title    = "Authentication required.",
            Detail   = "You must be authenticated to access this resource.",
            Status   = StatusCodes.Status401Unauthorized,
            Instance = http.Request.Path,
        });
}
