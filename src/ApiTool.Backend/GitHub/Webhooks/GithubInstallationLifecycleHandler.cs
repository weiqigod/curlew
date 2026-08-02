// Refs docs/SPECIFICATION.md:8560-8562 (installation lifecycle: suspend, unsuspend, delete).
using ApiTool.Backend.GitHub.Installations;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Handles <c>installation.{deleted,suspend,unsuspend}</c> GitHub webhook events.
/// Spec :8560-8562.
/// </summary>
public class GithubInstallationLifecycleHandler(GithubInstallationsService svc)
{
    /// <summary>Handles <c>installation.suspend</c>.</summary>
    public virtual Task HandleSuspendedAsync(GithubInstallationLifecyclePayload p, CancellationToken ct) =>
        svc.MarkSuspendedAsync(p.InstallationId, ct);

    /// <summary>Handles <c>installation.unsuspend</c>.</summary>
    public virtual Task HandleUnsuspendedAsync(GithubInstallationLifecyclePayload p, CancellationToken ct) =>
        svc.MarkUnsuspendedAsync(p.InstallationId, ct);

    /// <summary>Handles <c>installation.deleted</c>.</summary>
    public virtual Task HandleDeletedAsync(GithubInstallationLifecyclePayload p, CancellationToken ct) =>
        svc.MarkDeletedAsync(p.InstallationId, ct);
}
