namespace ApiTool.Backend.Sso;

/// <summary>Discriminated error type for all SSO operations.</summary>
public enum SsoError
{
    /// <summary>No error — operation succeeded.</summary>
    None,

    /// <summary>The caller lacks the required role to perform this operation.</summary>
    PermissionDenied,

    /// <summary>The target organisation does not exist.</summary>
    OrgNotFound,

    /// <summary>The supplied SSO configuration is invalid or missing required fields.</summary>
    InvalidConfig,

    /// <summary>The SAMLResponse signature did not validate against the trusted IdP certificate.</summary>
    SignatureInvalid,

    /// <summary>The email from the SAML assertion is not a member of the target organisation.</summary>
    UserNotMember,

    /// <summary>SSO has not been configured or enabled for this organisation.</summary>
    SsoNotEnabled,

    /// <summary>The SAML assertion's <c>NotOnOrAfter</c> timestamp has passed.</summary>
    AssertionExpired,

    /// <summary>OIDC discovery document could not be fetched for the configured issuer URL.</summary>
    OidcDiscoveryFailed,

    /// <summary>The <c>state</c> parameter returned by the IdP does not match the stored cookie.</summary>
    OidcStateMismatch,

    /// <summary>The <c>nonce</c> claim in the id_token does not match the stored cookie.</summary>
    OidcNonceMismatch,

    /// <summary>The id_token failed JWT signature or claims validation.</summary>
    OidcInvalidIdToken,

    /// <summary>The authorization-code / token endpoint exchange failed.</summary>
    OidcTokenExchangeFailed,

    /// <summary>The organisation's subscription tier does not include SSO. Maps to
    /// HTTP 402 on authenticated config endpoints and HTTP 404 on public flow endpoints.</summary>
    TierIneligible,
}
