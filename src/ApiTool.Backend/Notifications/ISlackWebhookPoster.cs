namespace ApiTool.Backend.Notifications;

/// <summary>Seam for posting payloads to a Slack incoming webhook.</summary>
public interface ISlackWebhookPoster
{
    /// <summary>
    /// Posts a JSON payload to the given webhook URL.
    /// </summary>
    /// <param name="webhookUrl">The target webhook URL.</param>
    /// <param name="payload">The JSON string to send.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>HTTP status code returned by the endpoint.</returns>
    Task<int> PostAsync(string webhookUrl, string payload, CancellationToken ct);
}
