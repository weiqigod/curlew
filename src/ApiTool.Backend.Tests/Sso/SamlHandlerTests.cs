using System.IO.Compression;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using System.Security.Cryptography.Xml;
using System.Text;
using System.Xml;
using ApiTool.Backend.Sso;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Tests for <see cref="SamlHandler"/> using a self-signed RSA key pair.</summary>
public sealed class SamlHandlerTests
{
    // ── Test fixtures ─────────────────────────────────────────────────────────

    private static (X509Certificate2 cert, string certPem) GenerateSelfSignedCert()
    {
        // On .NET 6+ CreateSelfSigned already includes the private key.
        using var rsa = RSA.Create(2048);
        var req = new CertificateRequest(
            "CN=test-idp",
            rsa,
            HashAlgorithmName.SHA256,
            RSASignaturePadding.Pkcs1);
        var cert = req.CreateSelfSigned(DateTimeOffset.UtcNow.AddDays(-1), DateTimeOffset.UtcNow.AddYears(1));
        var pem = cert.ExportCertificatePem();
        return (cert, pem);
    }

    private static SsoConfig MakeConfig(string idpSsoUrl = "https://idp.example.com/sso") =>
        new SsoConfig(
            IdpMetadataUrl: "https://idp.example.com/metadata",
            AcsUrl: "https://sp.example.com/acs",
            EntityId: "https://sp.example.com",
            IdpSsoUrl: idpSsoUrl);

    private static string BuildSignedSamlResponse(X509Certificate2 cert, string email, DateTime notOnOrAfter)
    {
        var doc = new XmlDocument { PreserveWhitespace = true };
        var ns = "urn:oasis:names:tc:SAML:2.0:protocol";
        var assertionNs = "urn:oasis:names:tc:SAML:2.0:assertion";

        var responseId = "_resp_" + Guid.NewGuid().ToString("N");
        var assertionId = "_assert_" + Guid.NewGuid().ToString("N");
        var now = DateTime.UtcNow;

        doc.LoadXml($"""
            <samlp:Response xmlns:samlp="{ns}"
                            xmlns:saml="{assertionNs}"
                            ID="{responseId}"
                            Version="2.0"
                            IssueInstant="{now:O}">
              <saml:Assertion xmlns:saml="{assertionNs}"
                              ID="{assertionId}"
                              Version="2.0"
                              IssueInstant="{now:O}">
                <saml:Issuer>https://idp.example.com</saml:Issuer>
                <saml:Subject>
                  <saml:NameID Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress">{email}</saml:NameID>
                  <saml:SubjectConfirmation Method="urn:oasis:names:tc:SAML:2.0:cm:bearer">
                    <saml:SubjectConfirmationData NotOnOrAfter="{notOnOrAfter:O}" Recipient="https://sp.example.com/acs"/>
                  </saml:SubjectConfirmation>
                </saml:Subject>
                <saml:Conditions NotBefore="{now.AddMinutes(-5):O}" NotOnOrAfter="{notOnOrAfter:O}">
                  <saml:AudienceRestriction>
                    <saml:Audience>https://sp.example.com</saml:Audience>
                  </saml:AudienceRestriction>
                </saml:Conditions>
                <saml:AuthnStatement AuthnInstant="{now:O}">
                  <saml:AuthnContext>
                    <saml:AuthnContextClassRef>urn:oasis:names:tc:SAML:2.0:ac:classes:Password</saml:AuthnContextClassRef>
                  </saml:AuthnContext>
                </saml:AuthnStatement>
              </saml:Assertion>
            </samlp:Response>
            """);

        // Sign the Assertion element
        var rsaKey = cert.GetRSAPrivateKey() ?? throw new InvalidOperationException("Test cert has no private key.");
        var signedXml = new SignedXml(doc) { SigningKey = rsaKey };
        signedXml.SignedInfo!.CanonicalizationMethod = SignedXml.XmlDsigExcC14NTransformUrl;

        var reference = new Reference { Uri = "#" + assertionId };
        reference.AddTransform(new XmlDsigEnvelopedSignatureTransform());
        reference.AddTransform(new XmlDsigExcC14NTransform());
        reference.DigestMethod = SignedXml.XmlDsigSHA256Url;
        signedXml.AddReference(reference);

        var keyInfo = new KeyInfo();
        keyInfo.AddClause(new KeyInfoX509Data(cert));
        signedXml.KeyInfo = keyInfo;

        signedXml.ComputeSignature();

        var assertionEl = doc.GetElementsByTagName("Assertion", assertionNs)[0] as XmlElement
            ?? throw new InvalidOperationException("Assertion element not found in test document.");
        assertionEl.AppendChild(doc.ImportNode(signedXml.GetXml(), true));

        return Convert.ToBase64String(Encoding.UTF8.GetBytes(doc.OuterXml));
    }

    // ── Test helpers ─────────────────────────────────────────────────────────

    /// <summary>Extracts a named query parameter from a URL.</summary>
    private static string GetQueryParam(string url, string name)
    {
        var query = new Uri(url).Query.TrimStart('?');
        foreach (var part in query.Split('&'))
        {
            var eq = part.IndexOf('=');
            if (eq < 0) continue;
            var key = Uri.UnescapeDataString(part[..eq]);
            if (key == name)
                return Uri.UnescapeDataString(part[(eq + 1)..]);
        }
        throw new InvalidOperationException($"Query param '{name}' not found in URL: {url}");
    }

    private static string DeflateDecompress(byte[] deflated)
    {
        using var ms = new MemoryStream(deflated);
        using var deflate = new DeflateStream(ms, CompressionMode.Decompress);
        using var sr = new StreamReader(deflate, Encoding.UTF8);
        return sr.ReadToEnd();
    }

    // ── BuildAuthnRequest ─────────────────────────────────────────────────────

    [Fact]
    public void BuildAuthnRequest_returns_url_with_SAMLRequest_query_param()
    {
        var handler = new SamlHandler();
        var config = MakeConfig();
        var orgId = Guid.NewGuid();

        var result = handler.BuildAuthnRequest(config, orgId, "relay-state-1");

        result.RedirectUrl.Should().Contain("SAMLRequest=");
        result.RedirectUrl.Should().StartWith("https://idp.example.com/sso");
    }

    [Fact]
    public void BuildAuthnRequest_deflates_base64_encodes_per_http_redirect_binding()
    {
        var handler = new SamlHandler();
        var config = MakeConfig();
        var orgId = Guid.NewGuid();

        var result = handler.BuildAuthnRequest(config, orgId, "relay");

        // Extract SAMLRequest value, base64-decode, DEFLATE-decompress, parse as XML
        var base64 = GetQueryParam(result.RedirectUrl, "SAMLRequest");
        var bytes = Convert.FromBase64String(base64);
        var xml = DeflateDecompress(bytes);

        xml.Should().Contain("AuthnRequest");
        xml.Should().Contain("https://sp.example.com");  // entity id
    }

    [Fact]
    public void BuildAuthnRequest_request_id_is_included_in_redirect_url_xml()
    {
        var handler = new SamlHandler();
        var config = MakeConfig();
        var orgId = Guid.NewGuid();

        var result = handler.BuildAuthnRequest(config, orgId, "relay");

        result.RequestId.Should().NotBeNullOrWhiteSpace();

        // Verify the request ID appears in the decoded XML
        var base64 = GetQueryParam(result.RedirectUrl, "SAMLRequest");
        var bytes = Convert.FromBase64String(base64);
        var xml = DeflateDecompress(bytes);

        xml.Should().Contain(result.RequestId);
    }

    // ── ValidateResponse ──────────────────────────────────────────────────────

    [Fact]
    public void ValidateResponse_accepts_signed_assertion_from_trusted_cert()
    {
        var (cert, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();
        var notOnOrAfter = DateTime.UtcNow.AddMinutes(10);

        var responseBase64 = BuildSignedSamlResponse(cert, "user@example.com", notOnOrAfter);
        var result = handler.ValidateResponse(config, responseBase64, pem, DateTime.UtcNow);

        result.Success.Should().BeTrue();
        result.Email.Should().Be("user@example.com");
        result.ErrorCode.Should().BeNull();
    }

    [Fact]
    public void ValidateResponse_rejects_tampered_signature()
    {
        var (cert, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();
        var notOnOrAfter = DateTime.UtcNow.AddMinutes(10);

        var responseBase64 = BuildSignedSamlResponse(cert, "user@example.com", notOnOrAfter);

        // Tamper: change the base64 payload slightly
        var xml = Encoding.UTF8.GetString(Convert.FromBase64String(responseBase64));
        var tamperedXml = xml.Replace("user@example.com", "hacker@evil.com");
        var tamperedBase64 = Convert.ToBase64String(Encoding.UTF8.GetBytes(tamperedXml));

        var result = handler.ValidateResponse(config, tamperedBase64, pem, DateTime.UtcNow);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.SamlSignatureInvalid);
    }

    [Fact]
    public void ValidateResponse_rejects_expired_assertion()
    {
        var (cert, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();
        var notOnOrAfter = DateTime.UtcNow.AddMinutes(-5);  // already expired

        var responseBase64 = BuildSignedSamlResponse(cert, "user@example.com", notOnOrAfter);
        var result = handler.ValidateResponse(config, responseBase64, pem, DateTime.UtcNow);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.AssertionExpired);
    }

    [Fact]
    public void ValidateResponse_returns_email_from_NameID()
    {
        var (cert, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();
        var notOnOrAfter = DateTime.UtcNow.AddMinutes(10);

        var responseBase64 = BuildSignedSamlResponse(cert, "alice@example.com", notOnOrAfter);
        var result = handler.ValidateResponse(config, responseBase64, pem, DateTime.UtcNow);

        result.Success.Should().BeTrue();
        result.Email.Should().Be("alice@example.com");
        result.NameId.Should().Be("alice@example.com");
    }

    // ── SamlHandler failure-path tests ────────────────────────────────────────

    [Fact]
    public void ValidateResponse_returns_signature_invalid_when_no_signature_element()
    {
        var (_, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();
        var assertionNs = "urn:oasis:names:tc:SAML:2.0:assertion";
        var samlpNs = "urn:oasis:names:tc:SAML:2.0:protocol";

        // Build a response with NO Signature element
        var xmlNoSig = $"""
            <samlp:Response xmlns:samlp="{samlpNs}" xmlns:saml="{assertionNs}"
                            ID="_resp1" Version="2.0" IssueInstant="{DateTime.UtcNow:O}">
              <saml:Assertion xmlns:saml="{assertionNs}" ID="_assert1" Version="2.0" IssueInstant="{DateTime.UtcNow:O}">
                <saml:Subject>
                  <saml:NameID>user@example.com</saml:NameID>
                </saml:Subject>
              </saml:Assertion>
            </samlp:Response>
            """;
        var base64 = Convert.ToBase64String(Encoding.UTF8.GetBytes(xmlNoSig));

        var result = handler.ValidateResponse(config, base64, pem, DateTime.UtcNow);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.SamlSignatureInvalid);
    }

    [Fact]
    public void ValidateResponse_returns_signature_invalid_when_no_assertion_element()
    {
        var (cert, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();
        var samlpNs = "urn:oasis:names:tc:SAML:2.0:protocol";
        var dSigNs = "http://www.w3.org/2000/09/xmldsig#";

        // Build a response with a Signature but no Assertion element
        var xmlNoAssertion = $"""
            <samlp:Response xmlns:samlp="{samlpNs}"
                            ID="_resp2" Version="2.0" IssueInstant="{DateTime.UtcNow:O}">
              <ds:Signature xmlns:ds="{dSigNs}"/>
            </samlp:Response>
            """;
        var base64 = Convert.ToBase64String(Encoding.UTF8.GetBytes(xmlNoAssertion));

        var result = handler.ValidateResponse(config, base64, pem, DateTime.UtcNow);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.SamlSignatureInvalid);
    }

    [Fact]
    public void ValidateResponse_returns_signature_invalid_when_nameID_is_empty()
    {
        var (cert, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();
        var notOnOrAfter = DateTime.UtcNow.AddMinutes(10);

        // Build a response where NameID is empty
        var responseBase64 = BuildSignedSamlResponse(cert, "", notOnOrAfter);

        var result = handler.ValidateResponse(config, responseBase64, pem, DateTime.UtcNow);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.SamlSignatureInvalid);
    }

    [Fact]
    public void ValidateResponse_returns_signature_invalid_for_malformed_base64_input()
    {
        var (_, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();

        // Not valid base64
        var result = handler.ValidateResponse(config, "!!!not-valid-base64!!!", pem, DateTime.UtcNow);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.SamlSignatureInvalid);
    }

    [Fact]
    public void ValidateResponse_returns_signature_invalid_for_non_xml_payload()
    {
        var (_, pem) = GenerateSelfSignedCert();
        var handler = new SamlHandler();
        var config = MakeConfig();

        // Valid base64 but not XML
        var notXml = Convert.ToBase64String(Encoding.UTF8.GetBytes("this is not xml at all"));
        var result = handler.ValidateResponse(config, notXml, pem, DateTime.UtcNow);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.SamlSignatureInvalid);
    }
}
