namespace ApiTool.Backend.Sso;

/// <summary>Machine-readable error code strings for SSO API responses.</summary>
public static class SsoErrorCodes
{
    /// <summary>A required SSO configuration field is missing or invalid.</summary>
    public const string InvalidSsoConfig = "invalid_sso_config";

    /// <summary>The SAMLResponse signature did not validate.</summary>
    public const string SamlSignatureInvalid = "saml_signature_invalid";

    /// <summary>The email from the SAML assertion is not a member of the organisation.</summary>
    public const string SsoUserNotMember = "sso_user_not_member";

    /// <summary>The caller does not have the required role.</summary>
    public const string PermissionDenied = "permission_denied";

    /// <summary>SSO has not been configured or enabled for this organisation.</summary>
    public const string SsoNotEnabled = "sso_not_enabled";

    /// <summary>The SAML assertion's <c>NotOnOrAfter</c> timestamp has passed.</summary>
    public const string AssertionExpired = "assertion_expired";

    /// <summary>OIDC discovery document could not be fetched for the configured issuer URL.</summary>
    public const string OidcDiscoveryFailed = "oidc_discovery_failed";

    /// <summary>The state parameter returned by the IdP does not match the stored cookie.</summary>
    public const string OidcStateMismatch = "oidc_state_mismatch";

    /// <summary>The nonce claim in the id_token does not match the stored cookie.</summary>
    public const string OidcNonceMismatch = "oidc_nonce_mismatch";

    /// <summary>The id_token failed JWT signature or claims validation.</summary>
    public const string OidcInvalidIdToken = "oidc_invalid_id_token";

    /// <summary>The authorization-code / token endpoint exchange failed.</summary>
    public const string OidcTokenExchangeFailed = "oidc_token_exchange_failed";

    /// <summary>The organisation's subscription tier does not permit SSO.
    /// Returned on authenticated config endpoints with HTTP 402.</summary>
    public const string SsoTierIneligible = "sso_tier_ineligible";
}
