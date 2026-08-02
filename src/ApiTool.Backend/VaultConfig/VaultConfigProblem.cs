using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// RFC 7807 problem-detail factory for vault-config-specific error responses.
/// </summary>
public static class VaultConfigProblem
{
    private const string BaseType = "https://api.apitool.dev/errors";

    /// <summary>
    /// Builds a 422 Unprocessable Entity problem-detail for a template that contains
    /// values matching the literal-secret heuristic. Includes the offending key paths
    /// as the <c>offending_paths</c> extension array.
    /// </summary>
    /// <param name="http">The current HTTP context (for the request instance URI).</param>
    /// <param name="offendingPaths">Dot-separated JSON paths of the suspicious fields.</param>
    public static IResult SuspiciousValue(HttpContext http, IReadOnlyList<string> offendingPaths)
    {
        var problem = new ProblemDetails
        {
            Type = $"{BaseType}/vault-template-suspicious-value",
            Title = "Template contains likely-secret values",
            Detail =
                "One or more template fields match the literal-secret heuristic " +
                "(≥16 alphanumeric chars under a sensitive key name that is not a " +
                "recognized provider coordinate). Remove the literal values and replace " +
                "them with provider coordinate references.",
            Status = StatusCodes.Status422UnprocessableEntity,
            Instance = http.Request.Path,
        };
        problem.Extensions["offending_paths"] = offendingPaths;
        return HttpResults.Json(
            problem,
            contentType: "application/problem+json",
            statusCode: StatusCodes.Status422UnprocessableEntity);
    }
}
