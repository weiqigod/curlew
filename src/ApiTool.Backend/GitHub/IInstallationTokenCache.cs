namespace ApiTool.Backend.GitHub;

/// <summary>
/// In-process cache for GitHub App installation tokens.
/// Refs docs/SPECIFICATION.md:8456-8470.
/// </summary>
public interface IInstallationTokenCache
{
    /// <summary>
    /// Returns a valid installation access token for <paramref name="installationId"/>,
    /// fetching a new one from GitHub if the cached entry is absent or within 5 minutes of expiry.
    /// </summary>
    Task<string> GetOrRefreshAsync(long installationId, CancellationToken ct);

    /// <summary>Removes the cached token for <paramref name="installationId"/>, forcing a refresh on next call.</summary>
    void Evict(long installationId);
}
