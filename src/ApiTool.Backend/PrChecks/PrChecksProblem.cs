// RFC 7807 problem-details responses for PR-check posting failures.
// Refs docs/SPECIFICATION.md:8537-8549.
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Builds RFC 7807 problem-details <see cref="IResult"/> values for
/// <c>POST /api/v1/pr-checks</c> error paths.
/// Mirrors the pattern of <c>RefreshProblem</c> in the Auth namespace.
/// </summary>
internal static class PrChecksProblem
{
    private const string BaseType = "https://api.apitool.dev/errors";

    /// <summary>No GitHub App installation found for this organisation.</summary>
    public static IResult NoInstallation(HttpContext http) => Build(http, 404,
        $"{BaseType}/prcheck-no-installation",
        "GitHub App not installed",
        "Connect GitHub from your dashboard before posting PR checks.",
        "PRCHECK_NO_INSTALLATION");

    /// <summary>The repository is not covered by the installation's repo selection.</summary>
    public static IResult RepoNotCovered(HttpContext http, string repo) => Build(http, 403,
        $"{BaseType}/prcheck-repo-not-covered",
        "Repository not covered",
        $"Repository '{repo}' is not covered by the GitHub App installation. Grant access in GitHub App settings.",
        "PRCHECK_REPO_NOT_COVERED");

    /// <summary>The GitHub App installation is suspended.</summary>
    public static IResult InstallationSuspended(HttpContext http) => Build(http, 423,
        $"{BaseType}/prcheck-installation-suspended",
        "GitHub App installation suspended",
        "The GitHub App installation is suspended. Resume it in GitHub settings.",
        "PRCHECK_INSTALLATION_SUSPENDED");

    /// <summary>The GitHub App installation was deleted.</summary>
    public static IResult InstallationDeleted(HttpContext http) => Build(http, 410,
        $"{BaseType}/prcheck-installation-deleted",
        "GitHub App installation deleted",
        "The GitHub App installation was deleted. Reinstall from your dashboard.",
        "PRCHECK_INSTALLATION_DELETED");

    /// <summary>GitHub rate-limited our POST; request queued for retry.</summary>
    public static IResult RateLimited(HttpContext http) => Build(http, 429,
        $"{BaseType}/prcheck-github-rate-limited",
        "GitHub rate-limited",
        "GitHub is rate-limiting this installation. The check has been queued for retry.",
        "PRCHECK_GITHUB_RATE_LIMITED");

    /// <summary>GitHub returned 5xx; request queued for retry.</summary>
    public static IResult Unavailable(HttpContext http) => Build(http, 502,
        $"{BaseType}/prcheck-github-unavailable",
        "GitHub unavailable",
        "GitHub returned a server error. The check has been queued for retry.",
        "PRCHECK_GITHUB_UNAVAILABLE");

    /// <summary>Permanent failure after exhausting retries.</summary>
    public static IResult PermanentFailure(HttpContext http) => Build(http, 500,
        $"{BaseType}/prcheck-permanent-failure",
        "Check run posting failed",
        "Posting the check run failed permanently. Contact support if the issue persists.",
        "PRCHECK_PERMANENT_FAILURE");

    /// <summary>Raw request body contained a GitHub installation token pattern.</summary>
    public static IResult TokenLeakDetected(HttpContext http) => Build(http, 400,
        $"{BaseType}/prcheck-token-leak-detected",
        "GitHub token detected in request body",
        "The request body appears to contain a GitHub installation token (ghs_…). Remove the token before retrying.",
        "PRCHECK_TOKEN_LEAK_DETECTED");

    /// <summary>The <c>state</c> field is not a valid conclusion value.</summary>
    public static IResult InvalidState(HttpContext http, string detail) => Build(http, 400,
        $"{BaseType}/prcheck-invalid-state",
        "Invalid state",
        detail,
        "PRCHECK_INVALID_STATE");

    // ── GitLab failure modes (M16-014) ─────────────────────────────────────────

    /// <summary>No active GitLab installation found for this organisation.</summary>
    public static IResult GitLabNoInstallation(HttpContext http) => Build(http, 404,
        $"{BaseType}/prcheck-gitlab-no-installation",
        "GitLab integration not configured",
        "Connect a GitLab project (PAT) from your dashboard before posting GitLab statuses.",
        "PRCHECK_GITLAB_NO_INSTALLATION");

    /// <summary>The GitLab Project Access Token has been revoked.</summary>
    public static IResult GitLabTokenRevoked(HttpContext http) => Build(http, 423,
        $"{BaseType}/prcheck-gitlab-token-revoked",
        "GitLab PAT revoked",
        "The GitLab Project Access Token has been revoked. Rotate it from your dashboard.",
        "PRCHECK_GITLAB_TOKEN_REVOKED");

    /// <summary>GitLab returned a network or 5xx error; request queued for retry.</summary>
    public static IResult GitLabUnreachable(HttpContext http) => Build(http, 502,
        $"{BaseType}/prcheck-gitlab-unreachable",
        "GitLab unreachable",
        "GitLab returned a network or 5xx error. The check has been queued for retry.",
        "PRCHECK_GITLAB_UNREACHABLE");

    /// <summary>GitLab base URL is HTTP without GITLAB__ALLOW_HTTP=true override.</summary>
    public static IResult GitLabHttpInsecure(HttpContext http, string baseUrl) => Build(http, 400,
        $"{BaseType}/prcheck-gitlab-http-insecure",
        "Insecure GitLab base URL",
        $"GitLab base URL '{baseUrl}' is HTTP. Set GITLAB__ALLOW_HTTP=true to override (dev/test only).",
        "PRCHECK_GITLAB_HTTP_INSECURE");

    /// <summary>GitLab is rate-limiting this installation; request queued for retry.</summary>
    public static IResult GitLabRateLimited(HttpContext http) => Build(http, 429,
        $"{BaseType}/prcheck-gitlab-rate-limited",
        "GitLab rate-limited",
        "GitLab is rate-limiting this installation. The check has been queued for retry.",
        "PRCHECK_GITLAB_RATE_LIMITED");

    // ── Private helper ─────────────────────────────────────────────────────────

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
            Type = type,
            Title = title,
            Status = status,
            Detail = detail,
            Extensions =
            {
                ["code"] = code,
                ["request_id"] = requestId,
            },
        };

        return HttpResults.Json(problem, contentType: "application/problem+json", statusCode: status);
    }
}
