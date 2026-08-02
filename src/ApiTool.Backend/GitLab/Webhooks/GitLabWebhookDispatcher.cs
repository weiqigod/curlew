// Refs M16-015 plan Decision E (Push/MR stored-only), Decision J (dispatcher contract).
namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Routes verified GitLab webhook events to type-specific handlers.
/// Unknown event types are logged and dropped — GitLab still receives 200 to stop retrying.
/// Per Decision E (plan): Push Hook and Merge Request Hook are stored but receive no business logic in M16.
/// </summary>
public sealed class GitLabWebhookDispatcher(
    GitLabPipelineHookHandler pipelineHandler,
    ILogger<GitLabWebhookDispatcher> log) : IGitLabWebhookDispatcher
{
    /// <inheritdoc/>
    public async Task DispatchAsync(GitLabWebhookEnvelope envelope, CancellationToken ct)
    {
        switch (envelope.EventType)
        {
            case "Pipeline Hook":
                await pipelineHandler.HandleAsync(
                    GitLabWebhookPayloadParser.ParsePipelineHook(envelope.Payload),
                    envelope.InstallationId, ct);
                break;

            case "Push Hook":
            case "Merge Request Hook":
                // Stored for debugging only — no business logic in M16.
                // Refs docs/SPECIFICATION.md:9263.
                log.LogInformation(
                    "gitlab_webhook_stored_only event_type={EventType} event_uuid={EventUuid} installation_id={InstallationId}",
                    envelope.EventType, envelope.EventUuid, envelope.InstallationId);
                break;

            default:
                log.LogInformation(
                    "gitlab_webhook_unhandled event_type={EventType} event_uuid={EventUuid}",
                    envelope.EventType, envelope.EventUuid);
                break;
        }
    }
}
