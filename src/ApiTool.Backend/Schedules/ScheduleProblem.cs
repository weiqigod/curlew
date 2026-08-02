using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Schedules;

/// <summary>
/// RFC 7807 problem-detail factory for schedule-specific error responses.
/// </summary>
public static class ScheduleProblem
{
    private const string BaseType = "https://api.apitool.dev/errors";

    private static readonly string[] ValidExamples =
        ["UTC", "Europe/Stockholm", "America/New_York", "Asia/Tokyo", "Australia/Sydney"];

    /// <summary>
    /// Builds a 422 Unprocessable Entity problem-detail for an unrecognized IANA timezone.
    /// Includes a <c>valid_examples</c> extension array with common IANA identifiers.
    /// </summary>
    /// <param name="http">The current HTTP context (for the request path).</param>
    /// <param name="detail">Human-readable detail from the validation failure.</param>
    public static IResult InvalidTimezone(HttpContext http, string? detail)
    {
        var problem = new ProblemDetails
        {
            Type = $"{BaseType}/invalid-timezone",
            Title = "Invalid IANA timezone",
            Detail = detail ?? "The provided timezone is not a recognized IANA identifier.",
            Status = StatusCodes.Status422UnprocessableEntity,
            Instance = http.Request.Path,
        };
        problem.Extensions["valid_examples"] = ValidExamples;
        return HttpResults.Json(
            problem,
            contentType: "application/problem+json",
            statusCode: StatusCodes.Status422UnprocessableEntity);
    }
}
