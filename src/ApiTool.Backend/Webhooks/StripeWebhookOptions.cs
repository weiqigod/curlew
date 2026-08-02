namespace ApiTool.Backend.Webhooks;

/// <summary>
/// Configuration for the Stripe webhook ingest pipeline. Bound from
/// <c>ApiTool:Stripe:Webhook:*</c> with environment-variable equivalents
/// <c>APITOOL__STRIPE__WEBHOOK__SECRETS</c> (canonical) and
/// <c>APITOOL__STRIPE__WEBHOOK_SECRETS</c> (single-underscore alias per spec :6850).
/// </summary>
public sealed class StripeWebhookOptions
{
    /// <summary>Configuration section name.</summary>
    public const string Section = "ApiTool:Stripe:Webhook";

    /// <summary>
    /// Comma-separated list of webhook signing secrets. Multiple values support
    /// the 24-hour rotation grace window — verification iterates the list and
    /// succeeds on any match. Empty entries are skipped.
    /// </summary>
    public string Secrets { get; set; } = string.Empty;

    /// <summary>
    /// Timestamp tolerance for HMAC verification, in seconds. Default 300 (5 minutes).
    /// Setting to 0 is rejected at boot — Stripe.Net treats 0 as "skip the
    /// timestamp check entirely", which is a footgun. Spec ref :6854.
    /// </summary>
    public int ToleranceSeconds { get; set; } = 300;

    /// <summary>Splits <see cref="Secrets"/> on comma, trims whitespace, drops empties.</summary>
    public IReadOnlyList<string> SecretList()
    {
        if (string.IsNullOrWhiteSpace(Secrets)) return Array.Empty<string>();
        return Secrets.Split(',', StringSplitOptions.TrimEntries | StringSplitOptions.RemoveEmptyEntries);
    }
}
