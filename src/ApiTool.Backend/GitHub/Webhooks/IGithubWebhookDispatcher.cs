namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Routes a verified GitHub webhook event to its type/action-specific handler.
/// </summary>
public interface IGithubWebhookDispatcher
{
    /// <summary>Process a single verified webhook delivery.</summary>
    Task DispatchAsync(GithubWebhookEnvelope envelope, CancellationToken ct);
}
