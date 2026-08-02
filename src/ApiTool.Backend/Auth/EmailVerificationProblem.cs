// RFC 7807 problem-detail builders for email-verification error responses.
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Factory for RFC 7807 problem-detail responses used by the email-verification endpoints.
/// </summary>
public static class EmailVerificationProblem
{
    private const string BaseType = "https://apitool.dev/errors";

    /// <summary>
    /// Returns a 400 problem detail for an invalid, expired, consumed, or revoked token.
    /// Refs docs/SPECIFICATION.md:8507 (email-verification-token-invalid error).
    /// </summary>
    public static IResult TokenInvalid(HttpContext http) =>
        HttpResults.Problem(new ProblemDetails
        {
            Type     = $"{BaseType}/email-verification-token-invalid",
            Title    = "Email verification token is invalid.",
            Detail   = "The token is unknown, expired, revoked, or has already been used.",
            Status   = StatusCodes.Status400BadRequest,
            Instance = http.Request.Path,
        });
}
