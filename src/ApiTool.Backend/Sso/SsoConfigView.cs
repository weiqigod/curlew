namespace ApiTool.Backend.Sso;

/// <summary>Safe, secrets-stripped view of an organisation's SSO configuration.</summary>
public sealed record SsoConfigView(
    bool SsoEnabled,
    string? SsoProvider,           // "saml" | "oidc" | null
    SamlConfigView? SamlConfig,    // populated only when SsoProvider == "saml"
    OidcConfigView? OidcConfig);   // populated only when SsoProvider == "oidc"

/// <summary>Public-only view of a SAML config — no IdP cert.</summary>
public sealed record SamlConfigView(
    string IdpMetadataUrl,
    string AcsUrl,
    string EntityId,
    string? IdpSsoUrl);

/// <summary>Public-only view of an OIDC config — no client_secret.</summary>
public sealed record OidcConfigView(
    string IssuerUrl,
    string ClientId,
    string RedirectUri,
    string Scopes);
