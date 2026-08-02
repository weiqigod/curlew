namespace ApiTool.Backend.Webhooks;

/// <summary>Configuration for <see cref="StripeWebhookCleanupHost"/>.</summary>
public sealed class StripeWebhookCleanupOptions
{
    /// <summary>Configuration section name.</summary>
    public const string Section = "ApiTool:Stripe:Webhook:Cleanup";

    /// <summary>Days to retain processed rows. Default 90 per spec :6852.</summary>
    public int RetentionDays { get; set; } = 90;
}
