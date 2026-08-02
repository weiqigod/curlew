namespace ApiTool.Backend.Sso;

/// <summary>Stored IdP configuration for a SAML 2.0 SSO connection.</summary>
/// <param name="IdpMetadataUrl">URL to the IdP's SAML metadata XML document.</param>
/// <param name="AcsUrl">Assertion Consumer Service URL — where the IdP posts the SAMLResponse.</param>
/// <param name="EntityId">The SP entity ID (URI, typically the app's base URL).</param>
/// <param name="IdpSsoUrl">
/// Optional: the IdP's SSO redirect URL. Used when <paramref name="IdpMetadataUrl"/> is not
/// resolvable at runtime; if <see langword="null"/> the handler fetches it from metadata.
/// </param>
/// <param name="IdpCertPem">
/// Optional: PEM-encoded X.509 certificate for the IdP's signing key.
/// If provided it is stored in <c>sso_credentials</c>; otherwise the cert is fetched from metadata.
/// </param>
public sealed record SsoConfig(
    string IdpMetadataUrl,
    string AcsUrl,
    string EntityId,
    string? IdpSsoUrl = null,
    string? IdpCertPem = null);
