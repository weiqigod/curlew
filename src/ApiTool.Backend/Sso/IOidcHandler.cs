namespace ApiTool.Backend.Sso;

/// <summary>Abstraction over OIDC authorize-URL building and authorization-code / id_token exchange.</summary>
public interface IOidcHandler
{
    /// <summary>
    /// Builds the full IdP authorize URL with state, nonce, PKCE challenge, and scope parameters.
    /// </summary>
    /// <param name="config">The organisation's OIDC configuration.</param>
    /// <param name="state">Random state parameter for CSRF protection.</param>
    /// <param name="nonce">Random nonce that will be embedded in the id_token.</param>
    /// <param name="pkceVerifier">PKCE code verifier (plain random bytes, base64url-encoded).</param>
    /// <param name="ct">Cancellation token.</param>
    /// <exception cref="OidcDiscoveryException">When discovery fails.</exception>
    Task<OidcAuthorizeRequest> BuildAuthorizeAsync(
        OidcConfig config,
        string state,
        string nonce,
        string pkceVerifier,
        CancellationToken ct);

    /// <summary>
    /// Exchanges an authorization code for an id_token and validates its signature and claims.
    /// Returns the email claim on success.
    /// </summary>
    /// <param name="config">The organisation's OIDC configuration.</param>
    /// <param name="clientSecret">The OAuth 2.0 client secret for the token exchange.</param>
    /// <param name="code">The authorization code from the IdP callback.</param>
    /// <param name="pkceVerifier">The PKCE code verifier that was used to build the authorize URL.</param>
    /// <param name="expectedNonce">The nonce value that must match the id_token's nonce claim.</param>
    /// <param name="nowUtc">Current UTC time used for id_token expiry checks.</param>
    /// <param name="ct">Cancellation token.</param>
    Task<OidcValidationResult> ExchangeAndValidateAsync(
        OidcConfig config,
        string clientSecret,
        string code,
        string pkceVerifier,
        string expectedNonce,
        DateTime nowUtc,
        CancellationToken ct);
}

/// <summary>Result of building an OIDC authorization URL.</summary>
/// <param name="AuthorizeUrl">The full IdP authorization URL including all required query parameters.</param>
/// <param name="State">The state value embedded in the URL (for correlation).</param>
public sealed record OidcAuthorizeRequest(string AuthorizeUrl, string State);

/// <summary>Result of validating an OIDC id_token.</summary>
/// <param name="Success">Whether the exchange and validation succeeded.</param>
/// <param name="ErrorCode">Machine-readable error code on failure; <see langword="null"/> on success.</param>
/// <param name="Email">The email claim from the id_token; <see langword="null"/> on failure.</param>
/// <param name="Subject">The sub (subject) claim from the id_token; <see langword="null"/> on failure.</param>
public sealed record OidcValidationResult(
    bool Success,
    string? ErrorCode,
    string? Email,
    string? Subject);
