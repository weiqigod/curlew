// Refs docs/SPECIFICATION.md:8416-8417 (webhook-first install path).
using ApiTool.Backend.GitHub.Installations;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Handles the <c>installation.created</c> GitHub webhook event by upserting a
/// <c>github_installations</c> row with <c>org_id = NULL</c> (webhook-first path).
/// Inbound HTTP routing and signature verification are added in M14-019.
/// </summary>
public class GithubInstallationCreatedHandler(GithubInstallationsService svc)
{
    /// <summary>Processes an installation.created payload.</summary>
    public virtual Task HandleAsync(GithubInstallationCreatedPayload payload, CancellationToken ct) =>
        svc.UpsertFromWebhookAsync(
            payload.InstallationId,
            payload.AppId,
            payload.AccountLogin,
            payload.AccountType,
            payload.RepoSelection,
            payload.Repositories,
            ct);
}
