// Refs docs/SPECIFICATION.md:8557 (handler wiring per event_type/action).
namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Routes verified GitHub events to type/action-specific handlers per spec :8557.
/// Unknown (event_type, action) pairs are logged and dropped — GitHub still receives
/// 200 so it stops retrying.
/// </summary>
public sealed class GithubWebhookDispatcher(
    GithubInstallationCreatedHandler created,
    GithubInstallationRepositoriesHandler repos,
    GithubInstallationLifecycleHandler lifecycle,
    GithubCheckRunHandler checkRun,
    ILogger<GithubWebhookDispatcher> log) : IGithubWebhookDispatcher
{
    /// <inheritdoc/>
    public async Task DispatchAsync(GithubWebhookEnvelope envelope, CancellationToken ct)
    {
        switch (envelope.EventType, envelope.Action)
        {
            case ("installation", "created"):
                await created.HandleAsync(
                    GithubWebhookPayloadParser.ParseInstallationCreated(envelope.Payload), ct);
                break;
            case ("installation", "deleted"):
                await lifecycle.HandleDeletedAsync(
                    GithubWebhookPayloadParser.ParseLifecycle(envelope.Payload), ct);
                break;
            case ("installation", "suspend"):
                await lifecycle.HandleSuspendedAsync(
                    GithubWebhookPayloadParser.ParseLifecycle(envelope.Payload), ct);
                break;
            case ("installation", "unsuspend"):
                await lifecycle.HandleUnsuspendedAsync(
                    GithubWebhookPayloadParser.ParseLifecycle(envelope.Payload), ct);
                break;
            case ("installation_repositories", "added"):
                await repos.HandleAddedAsync(
                    GithubWebhookPayloadParser.ParseInstallationRepositories(envelope.Payload, "added"), ct);
                break;
            case ("installation_repositories", "removed"):
                await repos.HandleRemovedAsync(
                    GithubWebhookPayloadParser.ParseInstallationRepositories(envelope.Payload, "removed"), ct);
                break;
            case ("check_run", "rerequested"):
                await checkRun.HandleRerequestedAsync(
                    GithubWebhookPayloadParser.ParseCheckRunRerequested(envelope.Payload), ct);
                break;
            default:
                log.LogInformation(
                    "github_webhook_unhandled event_type={EventType} action={Action} delivery_id={DeliveryId}",
                    envelope.EventType, envelope.Action, envelope.DeliveryId);
                break;
        }
    }
}
