// Refs M16-015 plan Decision J.
namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Routes a verified, idempotency-checked GitLab webhook envelope to its type-specific handler.
/// Mirrors <c>IGithubWebhookDispatcher</c>.
/// </summary>
public interface IGitLabWebhookDispatcher
{
    /// <summary>Dispatches <paramref name="envelope"/> to the appropriate handler.</summary>
    Task DispatchAsync(GitLabWebhookEnvelope envelope, CancellationToken ct);
}
