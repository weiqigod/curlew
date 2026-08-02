namespace ApiTool.Backend.Sso;

/// <summary>Request body for PUT /api/v1/organizations/{id}/sso/saml.</summary>
/// <param name="IdpMetadataUrl">URL to the IdP's SAML metadata XML document.</param>
/// <param name="AcsUrl">Assertion Consumer Service URL.</param>
/// <param name="EntityId">Service Provider entity ID URI.</param>
/// <param name="IdpSsoUrl">Optional: IdP SSO redirect URL override.</param>
/// <param name="IdpCertPem">Optional: PEM-encoded IdP signing certificate.</param>
public sealed record SamlConfigRequest(
    string? IdpMetadataUrl,
    string? AcsUrl,
    string? EntityId,
    string? IdpSsoUrl = null,
    string? IdpCertPem = null);
