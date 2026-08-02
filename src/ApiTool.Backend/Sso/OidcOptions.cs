namespace ApiTool.Backend.Sso;

/// <summary>Configuration options for the OIDC SSO subsystem.</summary>
public sealed class OidcOptions
{
    /// <summary>The configuration section name in appsettings.json.</summary>
    public const string Section = "Oidc";

    /// <summary>
    /// Backend base URL used to synthesise the default <c>redirect_uri</c>
    /// when the caller omits it from the <see cref="OidcConfigRequest"/>.
    /// </summary>
    public string BackendBaseUrl { get; set; } = "http://localhost:5000";

    /// <summary>How long to cache the OIDC discovery document per issuer URL.</summary>
    public TimeSpan DiscoveryCacheTtl { get; set; } = TimeSpan.FromMinutes(15);

    /// <summary>Maximum age of the short-lived OIDC state/nonce/PKCE cookies.</summary>
    public TimeSpan StateCookieTtl { get; set; } = TimeSpan.FromMinutes(10);

    /// <summary>Name of the HTTP-only cookie that carries the OIDC state parameter.</summary>
    public string StateCookieName { get; set; } = "apitool_oidc_state";

    /// <summary>Name of the HTTP-only cookie that carries the OIDC nonce.</summary>
    public string NonceCookieName { get; set; } = "apitool_oidc_nonce";

    /// <summary>Name of the HTTP-only cookie that carries the PKCE code verifier.</summary>
    public string PkceCookieName { get; set; } = "apitool_oidc_pkce";
}
