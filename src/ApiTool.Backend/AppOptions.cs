namespace ApiTool.Backend;

/// <summary>Configuration options for the public web app URL (used by Stripe redirects, etc.).</summary>
public sealed class AppOptions
{
    /// <summary>The section name in appsettings.json. Binds from APITOOL__APP__* env vars.</summary>
    public const string Section = "ApiTool:App";

    /// <summary>
    /// Absolute base URL of the public web app (e.g. <c>https://app.apitool.dev</c>).
    /// Required: used to construct Stripe billing portal return URLs.
    /// Bound from <c>APITOOL__APP__WEBAPPURL</c>.
    /// </summary>
    public string WebAppUrl { get; set; } = string.Empty;
}
