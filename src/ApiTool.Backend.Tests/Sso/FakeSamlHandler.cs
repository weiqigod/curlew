using ApiTool.Backend.Sso;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>
/// Deterministic in-process <see cref="ISamlHandler"/> for integration tests.
/// Controls the outcome of <see cref="ValidateResponse"/> via <see cref="ValidationMode"/>.
/// </summary>
public sealed class FakeSamlHandler : ISamlHandler
{
    /// <summary>Controls the <see cref="ValidateResponse"/> outcome for all subsequent calls.</summary>
    public FakeValidationMode ValidationMode { get; set; } = FakeValidationMode.Success;

    /// <summary>The email that will be returned on a successful validation.</summary>
    public string SuccessEmail { get; set; } = "sso-user@example.com";

    /// <inheritdoc/>
    public SamlAuthnRequest BuildAuthnRequest(SsoConfig config, Guid orgId, string relayState)
    {
        var requestId = "_fake_" + orgId.ToString("N");
        var samlRequest = Convert.ToBase64String(System.Text.Encoding.UTF8.GetBytes($"<FakeAuthnRequest ID=\"{requestId}\"/>"));
        var redirectUrl = $"{config.IdpSsoUrl ?? "https://fake-idp.test/sso"}?SAMLRequest={samlRequest}&RelayState={Uri.EscapeDataString(relayState)}";
        return new SamlAuthnRequest(redirectUrl, requestId);
    }

    /// <inheritdoc/>
    public SamlValidationResult ValidateResponse(SsoConfig config, string samlResponseBase64, string idpCertPem, DateTime nowUtc) =>
        ValidationMode switch
        {
            FakeValidationMode.Success =>
                new SamlValidationResult(true, null, SuccessEmail, SuccessEmail),
            FakeValidationMode.SignatureInvalid =>
                new SamlValidationResult(false, SsoErrorCodes.SamlSignatureInvalid, null, null),
            FakeValidationMode.Expired =>
                new SamlValidationResult(false, SsoErrorCodes.AssertionExpired, null, null),
            _ =>
                new SamlValidationResult(false, SsoErrorCodes.SamlSignatureInvalid, null, null),
        };
}

/// <summary>Controls the behaviour of <see cref="FakeSamlHandler.ValidateResponse"/>.</summary>
public enum FakeValidationMode
{
    /// <summary>Returns a successful result with a preset email.</summary>
    Success,

    /// <summary>Simulates a tampered or invalid signature.</summary>
    SignatureInvalid,

    /// <summary>Simulates an expired assertion.</summary>
    Expired,
}
