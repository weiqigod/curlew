// Refs docs/SPECIFICATION.md:8425 (daily reconciliation — GET /installation/repositories).
namespace ApiTool.Backend.GitHub.Installations;

/// <summary>
/// Abstraction over the GitHub App API for listing installation repositories.
/// The real HTTP implementation using installation-token exchange is provided by M14-018.
/// This slice ships a stub that is registered only in non-Testing production environments.
/// </summary>
public interface IGitHubInstallationsApi
{
    /// <summary>
    /// Lists the repositories accessible to the given installation using a fresh installation token.
    /// Refs docs/SPECIFICATION.md:8425 (GET /installation/repositories).
    /// </summary>
    Task<IReadOnlyList<RepoRef>> ListInstallationRepositoriesAsync(
        long installationId, CancellationToken ct);
}
