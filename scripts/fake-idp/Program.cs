// Fake SAML 2.0 IdP sidecar for M5-020 E2E tests.
//
// Endpoints:
//   GET  /healthz           → 200 "ok"
//   GET  /metadata          → 200 SAML EntityDescriptor XML
//   POST /saml/sso          → 200 HTML page with auto-submit form → backend ACS
//
// The RSA keypair is mounted at /certs/key.pem and /certs/cert.pem by docker-compose.test.yml.
// The NameID is always "qa@acme.example" — suitable for test purposes only.
//
// SAML signing: the Assertion element is signed (not the whole Response) to match
// the SignedXml validation in ApiTool.Backend/Sso/SamlHandler.cs which looks for
// a Signature whose Reference URI points to the Assertion's ID attribute.

using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using System.Security.Cryptography.Xml;
using System.Text;
using System.Xml;

var builder = WebApplication.CreateBuilder(args);
builder.WebHost.UseUrls("http://0.0.0.0:8088");
var app = builder.Build();

const string CertPath = "/certs/cert.pem";
const string KeyPath  = "/certs/key.pem";
const string NameId = "qa@acme.example";

// Configurable so the CI host browser (outside the compose network) can reach
// the backend at localhost:5000 while in-compose callers still hit http://backend:5000.
// An empty/whitespace env value is treated as unset (guards against a misconfigured
// compose override producing assertions that start with "/<orgGuid>/acs").
// A trailing slash is trimmed so a future override like ".../saml/" does not mint
// double-slash URLs.
var rawAcsBase = Environment.GetEnvironmentVariable("FAKE_IDP_ACS_BASE");
var backendAcsBase = string.IsNullOrWhiteSpace(rawAcsBase)
    ? "http://backend:5000/api/v1/sso/saml"
    : rawAcsBase.TrimEnd('/');

// ── GET /healthz ─────────────────────────────────────────────────────────────
app.MapGet("/healthz", () => Results.Ok("ok"));

// ── GET /metadata ─────────────────────────────────────────────────────────────
app.MapGet("/metadata", () =>
{
    if (!File.Exists(CertPath))
        return Results.Problem("cert not mounted at " + CertPath, statusCode: 500);

    var pem = File.ReadAllText(CertPath);
    var certBody = ExtractBase64FromPem(pem);

    var template = File.ReadAllText("/app/metadata-template.xml");
    var xml = template.Replace("{{CERT_BODY}}", certBody);

    return Results.Content(xml, "text/xml", Encoding.UTF8);
});

// ── /saml/sso ─────────────────────────────────────────────────────────────────
// The SP redirects the user here. Both HTTP bindings are accepted because the
// production SamlHandler uses HTTP-Redirect (GET with SAMLRequest in the query)
// for the AuthnRequest leg, while some IdPs and integration tests POST the
// SAMLRequest as a form field. Either way we ignore the SAMLRequest and issue
// the same signed assertion back to the ACS URL.
//
// Query params forwarded by the SP:
//   SAMLRequest=<deflated-base64-AuthnRequest> (ignored)
//   RelayState=<orgGuid>
app.MapMethods("/saml/sso", new[] { "GET", "POST" }, async (HttpContext ctx) =>
{
    var orgGuid = ctx.Request.Query["RelayState"].FirstOrDefault() ?? string.Empty;

    // Validate that orgGuid is a well-formed hex GUID (e.g. "3fa85f64-5717-4562-b3fc-2c963f66afa6")
    // to prevent malformed RelayState from corrupting the HTML form.
    if (!Guid.TryParse(orgGuid, out _))
    {
        ctx.Response.StatusCode = 400;
        await ctx.Response.WriteAsync("RelayState must be a valid GUID");
        return;
    }

    if (!File.Exists(KeyPath) || !File.Exists(CertPath))
    {
        ctx.Response.StatusCode = 500;
        await ctx.Response.WriteAsync("key or cert not mounted");
        return;
    }

    var keyPem  = await File.ReadAllTextAsync(KeyPath);
    var certPem = await File.ReadAllTextAsync(CertPath);

    var acsUrl = $"{backendAcsBase}/{orgGuid}/acs";
    var samlResponse = BuildSignedSamlResponse(keyPem, certPem, acsUrl, orgGuid, NameId);
    var responseBase64 = Convert.ToBase64String(Encoding.UTF8.GetBytes(samlResponse));

    // HTML-encode values before interpolating into HTML attributes to prevent injection.
    var acsUrlEncoded = System.Net.WebUtility.HtmlEncode(acsUrl);
    var responseBase64Encoded = System.Net.WebUtility.HtmlEncode(responseBase64);
    var orgGuidEncoded = System.Net.WebUtility.HtmlEncode(orgGuid);

    // Return an HTML auto-submit form so the browser POSTs the SAMLResponse
    // to the backend ACS endpoint without any user interaction.
    var html = $$"""
        <!DOCTYPE html>
        <html>
        <body onload="document.forms[0].submit()">
          <form method="POST" action="{{acsUrlEncoded}}">
            <input type="hidden" name="SAMLResponse" value="{{responseBase64Encoded}}" />
            <input type="hidden" name="RelayState" value="{{orgGuidEncoded}}" />
          </form>
        </body>
        </html>
        """;

    ctx.Response.ContentType = "text/html; charset=utf-8";
    await ctx.Response.WriteAsync(html);
});

app.Run();

// ── Helpers ───────────────────────────────────────────────────────────────────

/// <summary>
/// Builds a SAML 2.0 Response XML with the Assertion element signed using SignedXml.
/// The Signature covers only the Assertion (matching SamlHandler.cs validation logic).
/// </summary>
static string BuildSignedSamlResponse(
    string keyPem, string certPem, string acsUrl, string relayState, string nameId)
{
    var now           = DateTime.UtcNow;
    var notOnOrAfter  = now.AddMinutes(10);
    var responseId    = "_" + Guid.NewGuid().ToString("N");
    var assertionId   = "_" + Guid.NewGuid().ToString("N");
    const string IdpEntityId = "http://fake-idp:8088/metadata";

    // Build XML without signature first — we'll inject it after signing
    var xmlStr = $"""
        <samlp:Response
          xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"
          xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"
          ID="{responseId}"
          Version="2.0"
          IssueInstant="{now:O}"
          Destination="{acsUrl}"
          InResponseTo="">
          <saml:Issuer>{IdpEntityId}</saml:Issuer>
          <samlp:Status>
            <samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Success" />
          </samlp:Status>
          <saml:Assertion
            ID="{assertionId}"
            Version="2.0"
            IssueInstant="{now:O}">
            <saml:Issuer>{IdpEntityId}</saml:Issuer>
            <saml:Subject>
              <saml:NameID Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress">{nameId}</saml:NameID>
              <saml:SubjectConfirmation Method="urn:oasis:names:tc:SAML:2.0:cm:bearer">
                <saml:SubjectConfirmationData
                  NotOnOrAfter="{notOnOrAfter:O}"
                  Recipient="{acsUrl}"
                  InResponseTo="" />
              </saml:SubjectConfirmation>
            </saml:Subject>
            <saml:Conditions
              NotBefore="{now:O}"
              NotOnOrAfter="{notOnOrAfter:O}">
              <saml:AudienceRestriction>
                <saml:Audience>{acsUrl}</saml:Audience>
              </saml:AudienceRestriction>
            </saml:Conditions>
            <saml:AuthnStatement AuthnInstant="{now:O}">
              <saml:AuthnContext>
                <saml:AuthnContextClassRef>urn:oasis:names:tc:SAML:2.0:ac:classes:Password</saml:AuthnContextClassRef>
              </saml:AuthnContext>
            </saml:AuthnStatement>
            <saml:AttributeStatement>
              <saml:Attribute Name="email" NameFormat="urn:oasis:names:tc:SAML:2.0:attrname-format:basic">
                <saml:AttributeValue>{nameId}</saml:AttributeValue>
              </saml:Attribute>
            </saml:AttributeStatement>
          </saml:Assertion>
        </samlp:Response>
        """;

    var doc = new XmlDocument { PreserveWhitespace = true };
    doc.XmlResolver = null;
    doc.LoadXml(xmlStr);

    // Load private key
    using var rsa = RSA.Create();
    rsa.ImportFromPem(keyPem);

    // Load cert for the KeyInfo element
    var certBody = ExtractBase64FromPem(certPem);
    var certBytes = Convert.FromBase64String(certBody);

    // Sign the Assertion element (SamlHandler validates a signature whose Reference
    // URI is "#<assertionId>", so we sign the assertion — not the whole document).
    var signedXml = new SignedXml(doc);
    signedXml.SigningKey = rsa;
    signedXml.SignedInfo!.CanonicalizationMethod = SignedXml.XmlDsigExcC14NTransformUrl;

    // Reference to the Assertion element by its ID
    var reference = new Reference("#" + assertionId);
    reference.AddTransform(new XmlDsigEnvelopedSignatureTransform());
    reference.AddTransform(new XmlDsigExcC14NTransform());
    signedXml.AddReference(reference);

    // Add KeyInfo containing the cert so validators can find the key
    var keyInfo = new KeyInfo();
    var keyInfoData = new KeyInfoX509Data();
    keyInfoData.AddCertificate(X509CertificateLoader.LoadCertificate(certBytes));
    keyInfo.AddClause(keyInfoData);
    signedXml.KeyInfo = keyInfo;

    signedXml.ComputeSignature();
    var sigElement = signedXml.GetXml();

    // Inject signature as the first child of the Assertion element
    const string SamlNs = "urn:oasis:names:tc:SAML:2.0:assertion";
    var assertionEl = doc.GetElementsByTagName("Assertion", SamlNs)
                          .Cast<XmlElement>()
                          .First();

    var importedSig = doc.ImportNode(sigElement, true);
    assertionEl.InsertBefore(importedSig, assertionEl.FirstChild);

    var sb = new StringBuilder();
    using var writer = XmlWriter.Create(sb, new XmlWriterSettings { Encoding = Encoding.UTF8, Indent = false });
    doc.WriteTo(writer);
    writer.Flush();
    return sb.ToString();
}

/// <summary>Extracts the base64 body from a PEM-encoded certificate or key.</summary>
static string ExtractBase64FromPem(string pem)
{
    var lines = pem.Split('\n', StringSplitOptions.RemoveEmptyEntries);
    return string.Concat(lines.Where(l => !l.StartsWith("-----", StringComparison.Ordinal)));
}
