// Refs docs/SPECIFICATION.md:8419-8421 (installation_repositories.added/removed).
using ApiTool.Backend.GitHub.Installations;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Handles <c>installation_repositories.added</c> and <c>installation_repositories.removed</c>
/// GitHub webhook events by applying set-union / set-difference to the installation's repo_set.
/// Inbound HTTP routing and signature verification are added in M14-019.
/// </summary>
public class GithubInstallationRepositoriesHandler(GithubInstallationsService svc)
{
    /// <summary>Applies repo additions from an installation_repositories.added payload.</summary>
    public virtual Task HandleAddedAsync(GithubInstallationRepositoriesPayload payload, CancellationToken ct) =>
        svc.ApplyRepoAddedAsync(payload.InstallationId, payload.Repositories, ct);

    /// <summary>
    /// Applies repo removals from an installation_repositories.removed payload and marks
    /// in-flight pr_checks for removed repos as PRCHECK_REPO_NOT_COVERED.
    /// </summary>
    public virtual Task HandleRemovedAsync(GithubInstallationRepositoriesPayload payload, CancellationToken ct) =>
        svc.ApplyRepoRemovedAsync(payload.InstallationId, payload.Repositories, ct);
}
