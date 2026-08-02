namespace ApiTool.Backend.Sso;

/// <summary>Configuration options for the SAML 2.0 SSO subsystem.</summary>
public sealed class SamlOptions
{
    /// <summary>The configuration section name in appsettings.json.</summary>
    public const string Section = "Saml";

    /// <summary>
    /// The URL of the web portal to redirect to after a successful ACS handshake.
    /// The session cookie is set and the browser is 302'd here.
    /// </summary>
    public string WebPortalUrl { get; set; } = "http://localhost:3000/sso/callback";

    /// <summary>The name of the HTTP-only session cookie set after successful SSO login.</summary>
    public string SessionCookieName { get; set; } = "apitool_session";

    /// <summary>Lifetime of the minted session JWT.</summary>
    public TimeSpan SessionTtl { get; set; } = TimeSpan.FromHours(8);
}
