namespace ApiTool.Backend.Notifications;

/// <summary>
/// Production implementation of <see cref="ISlackWebhookPoster"/> that uses
/// <see cref="IHttpClientFactory"/> with the named client <c>"notifications"</c>.
/// </summary>
public sealed class SlackWebhookPoster(IHttpClientFactory httpClientFactory) : ISlackWebhookPoster
{
    /// <inheritdoc/>
    public async Task<int> PostAsync(string webhookUrl, string payload, CancellationToken ct)
    {
        var client = httpClientFactory.CreateClient("notifications");
        using var content = new StringContent(payload, System.Text.Encoding.UTF8, "application/json");
        var response = await client.PostAsync(new Uri(webhookUrl), content, ct);
        return (int)response.StatusCode;
    }
}
