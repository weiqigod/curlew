namespace ApiTool.Backend.Subscriptions;

/// <summary>Configuration options for the Stripe gateway selection.</summary>
public sealed class StripeOptions
{
    /// <summary>Configuration section name. Binds from APITOOL__STRIPE__* env vars.</summary>
    public const string Section = "ApiTool:Stripe";

    /// <summary>
    /// Gateway mode: <c>fake</c> (default) uses <see cref="FakeStripeGateway"/>;
    /// <c>live</c> uses <see cref="StripeGateway"/> which requires real Stripe SDK wiring.
    /// </summary>
    public string Mode { get; set; } = "fake";

    /// <summary>Stripe secret API key. Required when Mode = "live". Bound from APITOOL__STRIPE__APIKEY.</summary>
    public string ApiKey { get; set; } = string.Empty;

    /// <summary>
    /// Override base URL for the Stripe API (used for stripe-mock in CI).
    /// Bound from APITOOL__STRIPE__APIBASE. Null = default https://api.stripe.com.
    /// </summary>
    public string? ApiBase { get; set; }
}
