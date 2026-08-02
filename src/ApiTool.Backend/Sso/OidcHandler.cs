using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Microsoft.AspNetCore.WebUtilities;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Sso;

/// <summary>
/// Production <see cref="IOidcHandler"/> that performs OIDC authorization-code + PKCE + id_token
/// validation using the transitive <c>Microsoft.IdentityModel.*</c> packages.
/// </summary>
public sealed class OidcHandler(
    IOidcDiscoveryClient discovery,
    IHttpClientFactory httpClientFactory) : IOidcHandler
{
    /// <inheritdoc/>
    public async Task<OidcAuthorizeRequest> BuildAuthorizeAsync(
        OidcConfig config,
        string state,
        string nonce,
        string pkceVerifier,
        CancellationToken ct)
    {
        var conf = await discovery.GetConfigurationAsync(config.IssuerUrl, ct);

        // PKCE code challenge: Base64url(SHA-256(ASCII(verifier)))
        var codeChallenge = Base64UrlEncoder.Encode(
            SHA256.HashData(Encoding.ASCII.GetBytes(pkceVerifier)));

        var qs = new Dictionary<string, string?>
        {
            ["response_type"] = "code",
            ["client_id"] = config.ClientId,
            ["redirect_uri"] = config.RedirectUri,
            ["scope"] = config.Scopes,
            ["state"] = state,
            ["nonce"] = nonce,
            ["code_challenge"] = codeChallenge,
            ["code_challenge_method"] = "S256",
        };

        var authorizeUrl = QueryHelpers.AddQueryString(conf.AuthorizationEndpoint, qs);
        return new OidcAuthorizeRequest(authorizeUrl, state);
    }

    /// <inheritdoc/>
    public async Task<OidcValidationResult> ExchangeAndValidateAsync(
        OidcConfig config,
        string clientSecret,
        string code,
        string pkceVerifier,
        string expectedNonce,
        DateTime nowUtc,
        CancellationToken ct)
    {
        // Fetch discovery — fail fast if unreachable.
        Microsoft.IdentityModel.Protocols.OpenIdConnect.OpenIdConnectConfiguration conf;
        try
        {
            conf = await discovery.GetConfigurationAsync(config.IssuerUrl, ct);
        }
        catch (OidcDiscoveryException)
        {
            return Fail(SsoErrorCodes.OidcDiscoveryFailed);
        }

        // Exchange authorization code for tokens.
        var http = httpClientFactory.CreateClient("oidc");
        var form = new FormUrlEncodedContent(new[]
        {
            new KeyValuePair<string, string>("grant_type", "authorization_code"),
            new KeyValuePair<string, string>("code", code),
            new KeyValuePair<string, string>("redirect_uri", config.RedirectUri),
            new KeyValuePair<string, string>("client_id", config.ClientId),
            new KeyValuePair<string, string>("client_secret", clientSecret),
            new KeyValuePair<string, string>("code_verifier", pkceVerifier),
        });

        HttpResponseMessage resp;
        try
        {
            resp = await http.PostAsync(conf.TokenEndpoint, form, ct);
        }
        catch (HttpRequestException)
        {
            return Fail(SsoErrorCodes.OidcTokenExchangeFailed);
        }

        if (!resp.IsSuccessStatusCode)
            return Fail(SsoErrorCodes.OidcTokenExchangeFailed);

        string responseBody;
        try
        {
            responseBody = await resp.Content.ReadAsStringAsync(ct);
        }
        catch (HttpRequestException)
        {
            return Fail(SsoErrorCodes.OidcTokenExchangeFailed);
        }

        using var doc = JsonDocument.Parse(responseBody);
        if (!doc.RootElement.TryGetProperty("id_token", out var idTokenEl))
            return Fail(SsoErrorCodes.OidcTokenExchangeFailed);

        var idToken = idTokenEl.GetString();
        if (string.IsNullOrWhiteSpace(idToken))
            return Fail(SsoErrorCodes.OidcTokenExchangeFailed);

        // Validate id_token signature and standard claims.
        var validationParams = new TokenValidationParameters
        {
            ValidIssuer = conf.Issuer,
            ValidAudience = config.ClientId,
            IssuerSigningKeys = conf.SigningKeys,
            ValidateLifetime = true,
            ClockSkew = TimeSpan.FromMinutes(2),
        };

        ClaimsPrincipal principal;
        JwtSecurityToken jwt;
        try
        {
            principal = new JwtSecurityTokenHandler().ValidateToken(
                idToken, validationParams, out var validated);
            jwt = (JwtSecurityToken)validated;
        }
        catch (SecurityTokenException)
        {
            return Fail(SsoErrorCodes.OidcInvalidIdToken);
        }

        // Validate nonce to prevent replay attacks.
        var nonceClaim = jwt.Claims.FirstOrDefault(c => c.Type == "nonce")?.Value;
        if (nonceClaim != expectedNonce)
            return Fail(SsoErrorCodes.OidcNonceMismatch);

        // Require the email claim. JwtSecurityTokenHandler maps the short "email" claim type
        // to the long-form ClaimTypes.Email URI, so we check both forms.
        var email = principal.FindFirstValue(ClaimTypes.Email)
                    ?? principal.FindFirstValue(JwtRegisteredClaimNames.Email)
                    ?? principal.FindFirstValue("email");
        if (string.IsNullOrWhiteSpace(email))
            return Fail(SsoErrorCodes.OidcInvalidIdToken);

        var emailVerifiedClaim = principal.FindFirstValue("email_verified");
        if (emailVerifiedClaim is not null &&
            bool.TryParse(emailVerifiedClaim, out var verified) && !verified)
        {
            return Fail(SsoErrorCodes.OidcInvalidIdToken);
        }

        var sub = principal.FindFirstValue(JwtRegisteredClaimNames.Sub);
        return new OidcValidationResult(true, null, email, sub);
    }

    private static OidcValidationResult Fail(string code) =>
        new(false, code, null, null);
}
