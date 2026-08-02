namespace ApiTool.Backend.Sso;

/// <summary>Abstraction over SAML 2.0 AuthnRequest building and SAMLResponse validation.</summary>
public interface ISamlHandler
{
    /// <summary>
    /// Builds an HTTP-Redirect-binding AuthnRequest for the given organisation.
    /// </summary>
    /// <param name="config">The organisation's IdP configuration.</param>
    /// <param name="orgId">The organisation identifier, embedded in the request as the SP entity.</param>
    /// <param name="relayState">Opaque relay state string to round-trip through the IdP.</param>
    /// <returns>
    /// A <see cref="SamlAuthnRequest"/> containing the full redirect URL (with a
    /// <c>SAMLRequest</c> query parameter that is DEFLATE-compressed, base64-encoded, and
    /// URL-encoded per the HTTP-Redirect binding) and the generated request ID.
    /// </returns>
    SamlAuthnRequest BuildAuthnRequest(SsoConfig config, Guid orgId, string relayState);

    /// <summary>
    /// Validates a base64-encoded SAMLResponse POSTed to the Assertion Consumer Service.
    /// </summary>
    /// <param name="config">The organisation's IdP configuration.</param>
    /// <param name="samlResponseBase64">The raw value of the <c>SAMLResponse</c> form field.</param>
    /// <param name="idpCertPem">PEM-encoded X.509 certificate of the trusted IdP signing key.</param>
    /// <param name="nowUtc">The current UTC time, used for <c>NotOnOrAfter</c> checks.</param>
    /// <returns>
    /// A <see cref="SamlValidationResult"/> with <see cref="SamlValidationResult.Success"/> set to
    /// <see langword="true"/> and the assertion's email/name-id populated on success, or
    /// <see cref="SamlValidationResult.Success"/> set to <see langword="false"/> with an error code
    /// on failure.
    /// </returns>
    SamlValidationResult ValidateResponse(SsoConfig config, string samlResponseBase64, string idpCertPem, DateTime nowUtc);
}

/// <summary>Result of building an HTTP-Redirect AuthnRequest.</summary>
/// <param name="RedirectUrl">
/// The full IdP redirect URL including the signed <c>SAMLRequest</c> query parameter.
/// </param>
/// <param name="RequestId">The unique ID embedded in the <c>&lt;samlp:AuthnRequest&gt;</c> element.</param>
public sealed record SamlAuthnRequest(string RedirectUrl, string RequestId);

/// <summary>Result of validating a SAMLResponse POSTed to the ACS endpoint.</summary>
/// <param name="Success">Whether validation succeeded.</param>
/// <param name="ErrorCode">Machine-readable error code on failure; <see langword="null"/> on success.</param>
/// <param name="Email">The subject email from the assertion; <see langword="null"/> on failure.</param>
/// <param name="NameId">The raw NameID from the assertion; <see langword="null"/> on failure.</param>
public sealed record SamlValidationResult(
    bool Success,
    string? ErrorCode,
    string? Email,
    string? NameId);
