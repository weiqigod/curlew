using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Results.Dashboard;

/// <summary>RFC 7807 problem-detail helpers for dashboard endpoint errors.</summary>
public static class DashboardProblem
{
    private const string BaseType = "https://api.apitool.dev/errors";

    /// <summary>
    /// Emits a 400 <c>unsupported-window</c> problem detail per Open Decision 10.
    /// </summary>
    public static IResult UnsupportedWindow(HttpContext http, string? received)
    {
        var requestId = http.TraceIdentifier;
        http.Response.Headers["X-Request-Id"] = requestId;

        var problem = new ProblemDetails
        {
            Type   = $"{BaseType}/unsupported-window",
            Title  = "Unsupported window",
            Status = StatusCodes.Status400BadRequest,
            Detail = $"Allowed values: {string.Join(", ", DashboardWindowExtensions.AllowedValues)}.",
            Extensions =
            {
                ["code"]           = "unsupported_window",
                ["request_id"]     = requestId,
                ["allowed_values"] = DashboardWindowExtensions.AllowedValues,
                ["received"]       = received,
            },
        };

        return HttpResults.Json(problem, contentType: "application/problem+json", statusCode: 400);
    }
}
