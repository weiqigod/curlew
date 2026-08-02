namespace ApiTool.Backend.Sso;

/// <summary>Stored IdP configuration for an OpenID Connect SSO connection.</summary>
/// <param name="IssuerUrl">The OIDC issuer URL (e.g., <c>https://login.example.com</c>).
/// Discovery document is fetched from <c>{IssuerUrl}/.well-known/openid-configuration</c>.</param>
/// <param name="ClientId">OAuth 2.0 client identifier registered with the IdP.</param>
/// <param name="RedirectUri">Absolute URL of this app's callback endpoint, registered with the IdP.</param>
/// <param name="Scopes">Requested scopes. Defaults to <c>openid email profile</c>.</param>
/// <remarks>
/// <c>ClientSecret</c> is NOT stored here — it is persisted in
/// <c>sso_credentials.PublicCertPem</c> with KID = <c>"oidc_client_secret"</c>
/// to avoid storing secrets in the settings JSON column.
/// </remarks>
public sealed record OidcConfig(
    string IssuerUrl,
    string ClientId,
    string RedirectUri,
    string Scopes = "openid email profile");
