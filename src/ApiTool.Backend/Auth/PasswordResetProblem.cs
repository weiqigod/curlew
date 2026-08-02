// RFC 7807 problem-detail builders for password-reset error responses.
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Factory for RFC 7807 problem-detail responses used by the password-reset endpoints.
/// </summary>
public static class PasswordResetProblem
{
    private const string BaseType = "https://apitool.dev/errors";

    /// <summary>
    /// Returns a 400 problem detail for an invalid, expired, or consumed token.
    /// Refs docs/SPECIFICATION.md:8488 (token-invalid error).
    /// </summary>
    public static IResult TokenInvalid(HttpContext http) =>
        HttpResults.Problem(new ProblemDetails
        {
            Type     = $"{BaseType}/password-reset-token-invalid",
            Title    = "Password reset token is invalid.",
            Detail   = "The token is unknown, expired, or has already been used.",
            Status   = StatusCodes.Status400BadRequest,
            Instance = http.Request.Path,
        });

    /// <summary>
    /// Returns a 422 problem detail for a password that does not meet strength requirements.
    /// Includes the zxcvbn <paramref name="score"/> in the extensions.
    /// Refs docs/SPECIFICATION.md:8531 (password-too-weak error).
    /// </summary>
    public static IResult PasswordTooWeak(HttpContext http, int score)
    {
        var problem = new ProblemDetails
        {
            Type     = $"{BaseType}/password-too-weak",
            Title    = "Password does not meet strength requirements.",
            Detail   = "The provided password scored below the minimum zxcvbn score of 3.",
            Status   = StatusCodes.Status422UnprocessableEntity,
            Instance = http.Request.Path,
        };
        problem.Extensions["score"] = score;
        return HttpResults.Problem(problem);
    }
}
