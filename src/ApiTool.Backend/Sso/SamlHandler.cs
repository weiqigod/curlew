using System.IO.Compression;
using System.Security.Cryptography.X509Certificates;
using System.Security.Cryptography.Xml;
using System.Text;
using System.Xml;

namespace ApiTool.Backend.Sso;

/// <summary>
/// Production SAML 2.0 handler using BCL cryptography.
/// <para>
/// Login uses the HTTP-Redirect binding: the <c>AuthnRequest</c> is DEFLATE-compressed,
/// base64-encoded, and URL-encoded into a <c>SAMLRequest</c> query parameter.
/// </para>
/// <para>
/// ACS uses the HTTP-POST binding: the <c>SAMLResponse</c> is a base64-encoded XML document
/// whose embedded <c>Signature</c> element is validated with <see cref="SignedXml"/>.
/// </para>
/// </summary>
public sealed class SamlHandler : ISamlHandler
{
    private const string SamlpNs = "urn:oasis:names:tc:SAML:2.0:protocol";
    private const string SamlNs = "urn:oasis:names:tc:SAML:2.0:assertion";

    /// <inheritdoc/>
    public SamlAuthnRequest BuildAuthnRequest(SsoConfig config, Guid orgId, string relayState)
    {
        var requestId = "_" + Guid.NewGuid().ToString("N");
        var now = DateTime.UtcNow;

        var xml = $"""
            <samlp:AuthnRequest xmlns:samlp="{SamlpNs}"
                                xmlns:saml="{SamlNs}"
                                ID="{requestId}"
                                Version="2.0"
                                IssueInstant="{now:O}"
                                Destination="{config.IdpSsoUrl ?? string.Empty}"
                                AssertionConsumerServiceURL="{config.AcsUrl}"
                                ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST">
              <saml:Issuer>{config.EntityId}</saml:Issuer>
            </samlp:AuthnRequest>
            """;

        // DEFLATE compress (raw, no zlib header), base64-encode per HTTP-Redirect binding spec
        var xmlBytes = Encoding.UTF8.GetBytes(xml);
        byte[] deflated;
        using (var ms = new MemoryStream())
        {
            using (var deflate = new DeflateStream(ms, CompressionLevel.Optimal, leaveOpen: true))
            {
                deflate.Write(xmlBytes, 0, xmlBytes.Length);
            }
            deflated = ms.ToArray();
        }

        var base64 = Convert.ToBase64String(deflated);
        var encoded = Uri.EscapeDataString(base64);

        var idpSsoUrl = config.IdpSsoUrl ?? string.Empty;
        var separator = idpSsoUrl.Contains('?') ? "&" : "?";
        var redirectUrl = $"{idpSsoUrl}{separator}SAMLRequest={encoded}&RelayState={Uri.EscapeDataString(relayState)}";

        return new SamlAuthnRequest(redirectUrl, requestId);
    }

    /// <inheritdoc/>
    public SamlValidationResult ValidateResponse(
        SsoConfig config,
        string samlResponseBase64,
        string idpCertPem,
        DateTime nowUtc)
    {
        try
        {
            var xmlBytes = Convert.FromBase64String(samlResponseBase64);
            var xmlStr = Encoding.UTF8.GetString(xmlBytes);

            var doc = new XmlDocument { PreserveWhitespace = true };
            doc.XmlResolver = null;  // XXE mitigation
            doc.LoadXml(xmlStr);

            // Load trusted IdP cert (from our database, not from the document)
            var cert = LoadCertFromPem(idpCertPem);

            // Locate the Signature element and verify it
            var signedXml = new SignedXml(doc);
            var sigEl = doc.GetElementsByTagName("Signature", "http://www.w3.org/2000/09/xmldsig#")
                            .Cast<XmlElement>()
                            .FirstOrDefault();
            if (sigEl is null)
                return Fail(SsoErrorCodes.SamlSignatureInvalid);

            signedXml.LoadXml(sigEl);

            // Verify against our trusted cert only — never trust key material from the document
            var rsaKey = cert.GetRSAPublicKey()
                ?? throw new InvalidOperationException("IdP certificate does not contain an RSA public key.");
            if (!signedXml.CheckSignature(rsaKey))
                return Fail(SsoErrorCodes.SamlSignatureInvalid);

            // Extract assertion data — must be identified BEFORE the wrapping check
            var assertionEl = doc.GetElementsByTagName("Assertion", SamlNs)
                                  .Cast<XmlElement>()
                                  .FirstOrDefault();
            if (assertionEl is null)
                return Fail(SsoErrorCodes.SamlSignatureInvalid);

            // Signature-wrapping mitigation: confirm the signature's Reference URI matches
            // the ID of the assertion element we are about to parse.  An attacker who
            // injects a second Assertion element before the legitimately-signed one would
            // cause FirstOrDefault() to return the wrong element; this check catches that.
            var assertionId = assertionEl.GetAttribute("ID");
            var refUri = signedXml.SignedInfo?.References.Cast<Reference>().FirstOrDefault()?.Uri;
            if (string.IsNullOrEmpty(assertionId) || refUri != "#" + assertionId)
                return Fail(SsoErrorCodes.SamlSignatureInvalid);

            // Validate NotOnOrAfter
            var notOnOrAfter = GetNotOnOrAfter(assertionEl);
            if (notOnOrAfter.HasValue && nowUtc >= notOnOrAfter.Value)
                return Fail(SsoErrorCodes.AssertionExpired);

            // Extract email from NameID
            var nameId = GetNameId(assertionEl);
            if (string.IsNullOrWhiteSpace(nameId))
                return Fail(SsoErrorCodes.SamlSignatureInvalid);

            return new SamlValidationResult(true, null, nameId, nameId);
        }
        catch (FormatException)
        {
            return Fail(SsoErrorCodes.SamlSignatureInvalid);
        }
        catch (XmlException)
        {
            return Fail(SsoErrorCodes.SamlSignatureInvalid);
        }
        catch (System.Security.Cryptography.CryptographicException)
        {
            return Fail(SsoErrorCodes.SamlSignatureInvalid);
        }
        catch (InvalidOperationException)
        {
            return Fail(SsoErrorCodes.SamlSignatureInvalid);
        }
    }

    private static SamlValidationResult Fail(string errorCode) =>
        new(false, errorCode, null, null);

    private static X509Certificate2 LoadCertFromPem(string pem) =>
        X509CertificateLoader.LoadCertificate(
            Convert.FromBase64String(
                string.Concat(pem
                    .Split('\n', StringSplitOptions.RemoveEmptyEntries)
                    .Where(l => !l.StartsWith("-----", StringComparison.Ordinal)))));

    private static DateTime? GetNotOnOrAfter(XmlElement assertionEl)
    {
        // Check SubjectConfirmationData NotOnOrAfter first, then Conditions
        var subjectConfirmationData = assertionEl
            .GetElementsByTagName("SubjectConfirmationData", SamlNs)
            .Cast<XmlElement>()
            .FirstOrDefault();

        var attr = subjectConfirmationData?.GetAttribute("NotOnOrAfter")
                ?? assertionEl.GetElementsByTagName("Conditions", SamlNs)
                               .Cast<XmlElement>()
                               .FirstOrDefault()
                               ?.GetAttribute("NotOnOrAfter");

        if (attr is not null && DateTime.TryParse(attr, null, System.Globalization.DateTimeStyles.RoundtripKind, out var dt))
            return dt.ToUniversalTime();

        return null;
    }

    private static string? GetNameId(XmlElement assertionEl)
    {
        var nameIdEl = assertionEl
            .GetElementsByTagName("NameID", SamlNs)
            .Cast<XmlElement>()
            .FirstOrDefault();
        return nameIdEl?.InnerText?.Trim();
    }
}
