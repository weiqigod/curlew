using ApiTool.Backend.Sso;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>
/// Deterministic in-process <see cref="IOidcHandler"/> for integration tests.
/// Controls the outcome of all handler methods via <see cref="ValidationMode"/>.
/// </summary>
public sealed class FakeOidcHandler : IOidcHandler
{
    /// <summary>Controls the outcome of <see cref="ExchangeAndValidateAsync"/> (and build when <see cref="FakeOidcValidationMode.DiscoveryFailed"/>).</summary>
    public FakeOidcValidationMode ValidationMode { get; set; } = FakeOidcValidationMode.Success;

    /// <summary>The email returned on a successful exchange.</summary>
    public string SuccessEmail { get; set; } = "oidc-user@example.com";

    /// <summary>Base URL for the fake IdP authorize endpoint.</summary>
    public string AuthorizeBase { get; set; } = "https://fake-idp.test/authorize";

    /// <inheritdoc/>
    public Task<OidcAuthorizeRequest> BuildAuthorizeAsync(
        OidcConfig config,
        string state,
        string nonce,
        string pkceVerifier,
        CancellationToken ct)
    {
        if (ValidationMode == FakeOidcValidationMode.DiscoveryFailed)
            throw new OidcDiscoveryException("Fake discovery failure");

        var url = $"{AuthorizeBase}"
                + $"?response_type=code"
                + $"&client_id={Uri.EscapeDataString(config.ClientId)}"
                + $"&redirect_uri={Uri.EscapeDataString(config.RedirectUri)}"
                + $"&state={Uri.EscapeDataString(state)}"
                + $"&nonce={Uri.EscapeDataString(nonce)}"
                + $"&scope={Uri.EscapeDataString(config.Scopes)}"
                + $"&code_challenge_method=S256";

        return Task.FromResult(new OidcAuthorizeRequest(url, state));
    }

    /// <inheritdoc/>
    public Task<OidcValidationResult> ExchangeAndValidateAsync(
        OidcConfig config,
        string clientSecret,
        string code,
        string pkceVerifier,
        string expectedNonce,
        DateTime nowUtc,
        CancellationToken ct) =>
        Task.FromResult(ValidationMode switch
        {
            FakeOidcValidationMode.Success =>
                new OidcValidationResult(true, null, SuccessEmail, SuccessEmail),
            FakeOidcValidationMode.NonceMismatch =>
                new OidcValidationResult(false, SsoErrorCodes.OidcNonceMismatch, null, null),
            FakeOidcValidationMode.InvalidIdToken =>
                new OidcValidationResult(false, SsoErrorCodes.OidcInvalidIdToken, null, null),
            FakeOidcValidationMode.TokenExchangeFailed =>
                new OidcValidationResult(false, SsoErrorCodes.OidcTokenExchangeFailed, null, null),
            FakeOidcValidationMode.DiscoveryFailed =>
                new OidcValidationResult(false, SsoErrorCodes.OidcDiscoveryFailed, null, null),
            _ =>
                new OidcValidationResult(false, SsoErrorCodes.OidcInvalidIdToken, null, null),
        });
}

/// <summary>Controls the behaviour of <see cref="FakeOidcHandler"/>.</summary>
public enum FakeOidcValidationMode
{
    /// <summary>Returns a successful result with a preset email.</summary>
    Success,

    /// <summary>Simulates a nonce mismatch in the id_token.</summary>
    NonceMismatch,

    /// <summary>Simulates an invalid id_token signature or claims.</summary>
    InvalidIdToken,

    /// <summary>Simulates a token endpoint exchange failure.</summary>
    TokenExchangeFailed,

    /// <summary>Simulates a discovery failure — BuildAuthorize throws <see cref="OidcDiscoveryException"/>.</summary>
    DiscoveryFailed,
}
