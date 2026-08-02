// RFC 7807 problem-details responses for trials endpoint failures.
// Refs docs/SPECIFICATION.md:5800-5860.
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Builds RFC 7807 problem-details <see cref="IResult"/> values for trial activation error paths.
/// </summary>
internal static class TrialsProblem
{
    private const string BaseType = "https://api.apitool.dev/errors";

    /// <summary>The requested feature slug is not registered.</summary>
    public static IResult UnknownFeature(HttpContext http, string feature) => Build(http, 404,
        type:   $"{BaseType}/feature-unknown",
        title:  "Feature not found",
        detail: $"Feature '{feature}' is not a trialable feature slug.",
        code:   TrialsSentinelErrors.FeatureUnknown);

    /// <summary>A trial for the requested feature has already been consumed.</summary>
    public static IResult AlreadyConsumed(HttpContext http, TrialActivationResult.AlreadyConsumed ac) =>
        Build(http, 409,
            type:   $"{BaseType}/trial-already-consumed",
            title:  "Trial already consumed",
            detail: $"A trial for '{ac.Feature}' was previously granted on {ac.GrantedAt:O}.",
            code:   TrialsSentinelErrors.AlreadyConsumed,
            extensions: new Dictionary<string, object?>
            {
                ["previous_grant"] = new
                {
                    feature    = ac.Feature,
                    kind       = ac.Kind.ToString().ToLowerInvariant(),
                    granted_at = ac.GrantedAt,
                    expires_at = ac.ExpiresAt,
                },
            });

    /// <summary>The request body is missing required fields.</summary>
    public static IResult InvalidRequest(HttpContext http, string detail) => Build(http, 400,
        type:   $"{BaseType}/trial-invalid-request",
        title:  "Invalid request",
        detail: detail,
        code:   "TRIAL_INVALID_REQUEST");

    // ── Private helper ────────────────────────────────────────────────────────

    private static IResult Build(
        HttpContext http,
        int status,
        string type,
        string title,
        string detail,
        string code,
        Dictionary<string, object?>? extensions = null)
    {
        var requestId = http.TraceIdentifier;
        http.Response.Headers["X-Request-Id"] = requestId;

        var problem = new ProblemDetails
        {
            Type   = type,
            Title  = title,
            Status = status,
            Detail = detail,
        };
        problem.Extensions["code"]       = code;
        problem.Extensions["request_id"] = requestId;
        if (extensions is not null)
        {
            foreach (var (k, v) in extensions)
                problem.Extensions[k] = v;
        }

        return HttpResults.Json(problem, contentType: "application/problem+json", statusCode: status);
    }
}
