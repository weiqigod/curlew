namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>Configuration for <see cref="GithubWebhookCleanupHost"/>.</summary>
public sealed class GithubWebhookCleanupOptions
{
    /// <summary>Configuration section name.</summary>
    public const string Section = "ApiTool:GitHub:Webhook:Cleanup";

    /// <summary>Days to retain processed rows. Default 90 per spec :8595.</summary>
    public int RetentionDays { get; set; } = 90;
}
