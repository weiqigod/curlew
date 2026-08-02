// RFC 7807 problem-details responses for refresh-token failures.
// Refs docs/SPECIFICATION.md:8243-8267.
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Builds RFC 7807 problem-details <see cref="IResult"/> values for refresh-token error paths.
/// All responses use Content-Type: <c>application/problem+json</c> and carry a stable
/// <c>code</c> extension for machine-readable error handling per spec :8243-8267.
/// X-Request-Id is set on <see cref="HttpResponse"/> for client-side correlation.
/// </summary>
internal static class RefreshProblem
{
    private const string BaseType = "https://api.apitool.dev/errors";

    /// <summary>Refresh token has already been rotated; entire family revoked.</summary>
    public static IResult Reused(HttpContext http) => Build(http, 401,
        type:   $"{BaseType}/refresh-token-reused",
        title:  "Refresh token reuse detected",
        detail: "Token has been rotated; entire token family revoked. Re-authenticate via 'apitest login'.",
        code:   RefreshSentinelErrors.Reused);

    /// <summary>Refresh token is past its absolute 365-day lifetime.</summary>
    public static IResult Expired(HttpContext http) => Build(http, 401,
        type:   $"{BaseType}/refresh-token-expired",
        title:  "Refresh token expired",
        detail: "Token has reached its maximum age. Re-authenticate via 'apitest login'.",
        code:   RefreshSentinelErrors.Expired);

    /// <summary>Presented device_id does not match the token's bound device_id.</summary>
    public static IResult DeviceMismatch(HttpContext http) => Build(http, 401,
        type:   $"{BaseType}/auth-device-mismatch",
        title:  "Device mismatch",
        detail: "device_id does not match the refresh token's bound device.",
        code:   RefreshSentinelErrors.DeviceMismatch);

    /// <summary>Token is invalid, not recognised, or has been revoked.</summary>
    public static IResult Invalid(HttpContext http, string detail) => Build(http, 401,
        type:   $"{BaseType}/auth-invalid-refresh",
        title:  "Invalid refresh token",
        detail: detail,
        code:   RefreshSentinelErrors.Invalid);

    // ── Private helper ────────────────────────────────────────────────────────

    private static IResult Build(
        HttpContext http,
        int status,
        string type,
        string title,
        string detail,
        string code)
    {
        var requestId = http.TraceIdentifier;
        http.Response.Headers["X-Request-Id"] = requestId;

        var problem = new ProblemDetails
        {
            Type   = type,
            Title  = title,
            Status = status,
            Detail = detail,
            Extensions =
            {
                ["code"]       = code,
                ["request_id"] = requestId,
            },
        };

        return HttpResults.Json(problem, contentType: "application/problem+json", statusCode: status);
    }
}
