// Refs M16-014 (per-installation GitLab rate-limit tracker, Guid-keyed).
namespace ApiTool.Backend.GitLab;

/// <summary>
/// Tracks GitLab API rate-limit state per installation. Observes <c>RateLimit-Remaining</c>
/// and <c>RateLimit-Reset</c> response headers per spec :9246.
/// Guid-keyed because <c>gitlab_installations.Id</c> is a surrogate UUID (unlike GitHub's
/// long installation ids).
/// </summary>
public interface IGitLabRateLimitTracker
{
    /// <summary>
    /// Returns <see langword="true"/> if the installation is currently rate-limited and
    /// new requests should be queued rather than sent.
    /// </summary>
    bool IsBlocked(Guid installationId);

    /// <summary>
    /// Updates rate-limit state from GitLab response headers.
    /// </summary>
    /// <param name="installationId">The gitlab_installations surrogate PK.</param>
    /// <param name="remaining">Value of <c>RateLimit-Remaining</c>, or null if absent.</param>
    /// <param name="retryAfter">Value of <c>Retry-After</c> in seconds, or null if absent.</param>
    /// <param name="resetAt">UTC timestamp when the rate limit resets (<c>RateLimit-Reset</c> epoch), or null.</param>
    void Update(Guid installationId, int? remaining, int? retryAfter, DateTimeOffset? resetAt);

    /// <summary>Removes rate-limit state for the given installation.</summary>
    void Clear(Guid installationId);
}
