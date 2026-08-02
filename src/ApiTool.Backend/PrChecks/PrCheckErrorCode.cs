namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Error codes for <c>PRCHECK_*</c> failures when posting to the GitHub Checks API.
/// Refs docs/SPECIFICATION.md:8537-8549.
/// </summary>
public enum PrCheckErrorCode
{
    /// <summary>No error.</summary>
    None,

    /// <summary>No GitHub App installation found for this organisation.</summary>
    NoInstallation,

    /// <summary>The repository is not covered by the installation's repo selection.</summary>
    RepoNotCovered,

    /// <summary>The GitHub App installation is suspended.</summary>
    InstallationSuspended,

    /// <summary>The GitHub App installation was deleted.</summary>
    InstallationDeleted,

    /// <summary>GitHub rate-limited our POST; request queued for retry.</summary>
    GithubRateLimited,

    /// <summary>GitHub returned 5xx; request queued for retry.</summary>
    GithubUnavailable,

    /// <summary>Permanent failure after exhausting retries.</summary>
    PermanentFailure,

    /// <summary>Raw request body contained a GitHub installation token pattern.</summary>
    TokenLeakDetected,

    // M16-014 — GitLab provider error codes

    /// <summary>No active GitLab installation found for this organisation.</summary>
    GitLabNoInstallation,

    /// <summary>The GitLab Project Access Token has been revoked (401 from GitLab).</summary>
    GitLabTokenRevoked,

    /// <summary>GitLab returned a network or 5xx error; request queued for retry.</summary>
    GitLabUnreachable,

    /// <summary>GitLab base URL is HTTP without GITLAB__ALLOW_HTTP=true override.</summary>
    GitLabHttpInsecure,

    /// <summary>GitLab is rate-limiting this installation; request queued for retry.</summary>
    GitLabRateLimited,

    /// <summary>Permanent failure posting to GitLab after exhausting retries.</summary>
    GitLabPermanentFailure,
}
