namespace ApiTool.Backend.GitHub;

/// <summary>
/// Tracks GitHub API rate-limit state per installation.
/// Observes <c>X-RateLimit-Remaining</c> and <c>Retry-After</c> response headers.
/// </summary>
public interface IRateLimitTracker
{
    /// <summary>
    /// Returns <see langword="true"/> if the installation is currently rate-limited and
    /// new requests should be queued rather than sent.
    /// </summary>
    bool IsBlocked(long installationId);

    /// <summary>
    /// Updates rate-limit state from GitHub response headers.
    /// </summary>
    /// <param name="installationId">The GitHub installation id.</param>
    /// <param name="remaining">Value of <c>X-RateLimit-Remaining</c>, or null if header absent.</param>
    /// <param name="retryAfter">Value of <c>Retry-After</c> in seconds, or null if header absent.</param>
    /// <param name="resetAt">UTC timestamp when the rate limit resets (<c>X-RateLimit-Reset</c> epoch), or null.</param>
    void Update(long installationId, int? remaining, int? retryAfter, DateTimeOffset? resetAt);

    /// <summary>Removes rate-limit state for the given installation.</summary>
    void Clear(long installationId);
}
