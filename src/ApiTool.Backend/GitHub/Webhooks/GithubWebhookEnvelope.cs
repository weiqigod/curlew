using System.Text.Json;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Parsed metadata + raw JSON payload for a verified GitHub webhook delivery.
/// The dispatcher inspects (EventType, Action) to route to the correct handler.
/// </summary>
public sealed record GithubWebhookEnvelope(
    Guid DeliveryId, string EventType, string? Action, JsonDocument Payload);
