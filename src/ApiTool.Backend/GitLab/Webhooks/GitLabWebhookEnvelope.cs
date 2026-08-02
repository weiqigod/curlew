// Refs M16-015 plan Decision J.
using System.Text.Json;

namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Carries the verified, idempotency-checked GitLab webhook payload through the dispatch pipeline.
/// Mirrors <c>GithubWebhookEnvelope</c> but uses GitLab-specific header shapes.
/// </summary>
/// <param name="EventUuid">Value of <c>X-Gitlab-Event-UUID</c> (the idempotency key).</param>
/// <param name="EventType">Value of <c>X-Gitlab-Event</c> (e.g. "Pipeline Hook").</param>
/// <param name="InstallationId">The matched <c>gitlab_installations.id</c>.</param>
/// <param name="Payload">Parsed JSON document (caller disposes after dispatch).</param>
public sealed record GitLabWebhookEnvelope(
    string EventUuid,
    string EventType,
    Guid InstallationId,
    JsonDocument Payload);
