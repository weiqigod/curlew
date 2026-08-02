using System.IdentityModel.Tokens.Jwt;
using System.Net;
using System.Security.Claims;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Sso;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Unit tests for <see cref="OidcHandler"/> using <see cref="FakeOidcDiscoveryClient"/>
/// and a stub <see cref="HttpMessageHandler"/> for the token endpoint.</summary>
public sealed class OidcHandlerTests
{
    // ── Shared RSA key used across tests ─────────────────────────────────────

    private static readonly RSA Rsa = RSA.Create(2048);
    private static readonly RsaSecurityKey RsaKey = new(Rsa) { KeyId = "test-kid" };
    private static readonly SigningCredentials Creds = new(RsaKey, SecurityAlgorithms.RsaSha256);

    private const string Issuer = "https://idp.example.com";
    private const string ClientId = "apitool-client";
    private const string RedirectUri = "http://localhost:5000/api/v1/sso/oidc/test/callback";

    // ── Stub infrastructure ───────────────────────────────────────────────────

    private sealed class StubHttpHandler(Func<HttpRequestMessage, HttpResponseMessage> responder)
        : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage req, CancellationToken ct)
            => Task.FromResult(responder(req));
    }

    private static OidcHandler BuildHandler(
        Func<HttpRequestMessage, HttpResponseMessage> tokenResponder,
        out FakeOidcDiscoveryClient fakeDiscovery)
    {
        var discovery = new FakeOidcDiscoveryClient();
        var oidcCfg = new OpenIdConnectConfiguration
        {
            Issuer = Issuer,
            AuthorizationEndpoint = $"{Issuer}/authorize",
            TokenEndpoint = $"{Issuer}/token",
        };
        oidcCfg.SigningKeys.Add(RsaKey);
        discovery.SetConfig(Issuer, oidcCfg);
        fakeDiscovery = discovery;

        var services = new ServiceCollection();
        services.AddHttpClient("oidc").ConfigurePrimaryHttpMessageHandler(() =>
            new StubHttpHandler(tokenResponder));

        var sp = services.BuildServiceProvider();
        var factory = sp.GetRequiredService<IHttpClientFactory>();
        return new OidcHandler(discovery, factory);
    }

    private static OidcConfig MakeConfig() =>
        new(Issuer, ClientId, RedirectUri);

    // ── Token helpers ─────────────────────────────────────────────────────────

    private static string MakeIdToken(string email, string nonce, bool expired = false, bool noEmail = false, bool emailVerifiedFalse = false)
    {
        var now = DateTime.UtcNow;
        var claims = new List<Claim>
        {
            new(JwtRegisteredClaimNames.Sub, email),
            new("nonce", nonce),
        };
        if (!noEmail)
            claims.Add(new(JwtRegisteredClaimNames.Email, email));
        if (emailVerifiedFalse)
            claims.Add(new("email_verified", "false"));

        var jwt = new JwtSecurityToken(
            issuer: Issuer,
            audience: ClientId,
            claims: claims,
            notBefore: expired ? now.AddHours(-2) : now.AddMinutes(-1),
            expires: expired ? now.AddHours(-1) : now.AddHours(1),
            signingCredentials: Creds);

        return new JwtSecurityTokenHandler().WriteToken(jwt);
    }

    private static HttpResponseMessage MakeTokenResponse(string? idToken) =>
        new(HttpStatusCode.OK)
        {
            Content = new StringContent(
                idToken is null
                    ? """{"access_token":"at"}"""
                    : JsonSerializer.Serialize(new { id_token = idToken, access_token = "at" }),
                Encoding.UTF8, "application/json"),
        };

    // ── BuildAuthorizeAsync ───────────────────────────────────────────────────

    [Fact]
    public async Task BuildAuthorize_returns_url_with_required_query_params()
    {
        var handler = BuildHandler(_ => new HttpResponseMessage(HttpStatusCode.OK), out _);
        var result = await handler.BuildAuthorizeAsync(
            MakeConfig(), "my-state", "my-nonce", "my-verifier", default);

        result.AuthorizeUrl.Should().Contain("response_type=code");
        result.AuthorizeUrl.Should().Contain($"client_id={ClientId}");
        result.AuthorizeUrl.Should().Contain("state=my-state");
        result.AuthorizeUrl.Should().Contain("nonce=my-nonce");
        result.AuthorizeUrl.Should().Contain("code_challenge=");
        result.AuthorizeUrl.Should().Contain("code_challenge_method=S256");
        result.State.Should().Be("my-state");
    }

    [Fact]
    public async Task BuildAuthorize_throws_oidc_discovery_failed_when_discovery_client_fails()
    {
        var handler = BuildHandler(_ => new HttpResponseMessage(HttpStatusCode.OK), out var disc);
        disc.FailMode = true;

        await FluentActions.Invoking(() =>
                handler.BuildAuthorizeAsync(MakeConfig(), "s", "n", "v", default))
            .Should().ThrowAsync<OidcDiscoveryException>();
    }

    // ── ExchangeAndValidateAsync ──────────────────────────────────────────────

    [Fact]
    public async Task ExchangeAndValidate_success_returns_email_from_id_token()
    {
        var nonce = "test-nonce";
        var idToken = MakeIdToken("user@example.com", nonce);
        var handler = BuildHandler(_ => MakeTokenResponse(idToken), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", nonce, DateTime.UtcNow, default);

        result.Success.Should().BeTrue();
        result.Email.Should().Be("user@example.com");
        result.ErrorCode.Should().BeNull();
    }

    [Fact]
    public async Task ExchangeAndValidate_rejects_nonce_mismatch()
    {
        var idToken = MakeIdToken("user@example.com", "correct-nonce");
        var handler = BuildHandler(_ => MakeTokenResponse(idToken), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", "wrong-nonce", DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcNonceMismatch);
    }

    [Fact]
    public async Task ExchangeAndValidate_rejects_expired_id_token()
    {
        var nonce = "n";
        var idToken = MakeIdToken("user@example.com", nonce, expired: true);
        var handler = BuildHandler(_ => MakeTokenResponse(idToken), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", nonce, DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcInvalidIdToken);
    }

    [Fact]
    public async Task ExchangeAndValidate_rejects_wrong_signing_key()
    {
        var nonce = "n";
        // Sign with a different RSA key not in the discovery config
        using var otherRsa = RSA.Create(2048);
        var otherKey = new RsaSecurityKey(otherRsa);
        var otherCreds = new SigningCredentials(otherKey, SecurityAlgorithms.RsaSha256);

        var now = DateTime.UtcNow;
        var jwt = new JwtSecurityToken(
            issuer: Issuer, audience: ClientId,
            claims: [new Claim(JwtRegisteredClaimNames.Email, "u@e.com"), new Claim("nonce", nonce)],
            notBefore: now.AddMinutes(-1), expires: now.AddHours(1),
            signingCredentials: otherCreds);
        var idToken = new JwtSecurityTokenHandler().WriteToken(jwt);
        var handler = BuildHandler(_ => MakeTokenResponse(idToken), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", nonce, DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcInvalidIdToken);
    }

    [Fact]
    public async Task ExchangeAndValidate_returns_token_exchange_failed_on_5xx()
    {
        var handler = BuildHandler(
            _ => new HttpResponseMessage(HttpStatusCode.InternalServerError), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", "n", DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcTokenExchangeFailed);
    }

    [Fact]
    public async Task ExchangeAndValidate_returns_invalid_id_token_when_email_claim_missing()
    {
        var nonce = "n";
        var idToken = MakeIdToken("user@example.com", nonce, noEmail: true);
        var handler = BuildHandler(_ => MakeTokenResponse(idToken), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", nonce, DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcInvalidIdToken);
    }

    [Fact]
    public async Task ExchangeAndValidate_rejects_when_id_token_missing_from_response()
    {
        var handler = BuildHandler(_ => MakeTokenResponse(null), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", "n", DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcTokenExchangeFailed);
    }

    [Fact]
    public async Task ExchangeAndValidate_returns_discovery_failed_when_discovery_client_fails()
    {
        var handler = BuildHandler(_ => new HttpResponseMessage(HttpStatusCode.OK), out var disc);
        disc.FailMode = true;

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", "n", DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcDiscoveryFailed);
    }

    [Fact]
    public async Task ExchangeAndValidate_rejects_audience_mismatch()
    {
        // Token signed with correct key but issued to a different audience (client_id)
        var nonce = "n";
        var now = DateTime.UtcNow;
        var jwt = new JwtSecurityToken(
            issuer: Issuer,
            audience: "other-client",  // wrong audience
            claims:
            [
                new Claim(JwtRegisteredClaimNames.Email, "user@example.com"),
                new Claim("nonce", nonce),
            ],
            notBefore: now.AddMinutes(-1),
            expires: now.AddHours(1),
            signingCredentials: Creds);
        var idToken = new JwtSecurityTokenHandler().WriteToken(jwt);
        var handler = BuildHandler(_ => MakeTokenResponse(idToken), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", nonce, DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcInvalidIdToken);
    }

    [Fact]
    public async Task ExchangeAndValidate_rejects_email_not_verified()
    {
        // Token where email_verified claim is explicitly "false" — must be rejected
        var nonce = "n";
        var idToken = MakeIdToken("user@example.com", nonce, emailVerifiedFalse: true);
        var handler = BuildHandler(_ => MakeTokenResponse(idToken), out _);

        var result = await handler.ExchangeAndValidateAsync(
            MakeConfig(), "secret", "code", "verifier", nonce, DateTime.UtcNow, default);

        result.Success.Should().BeFalse();
        result.ErrorCode.Should().Be(SsoErrorCodes.OidcInvalidIdToken);
    }
}
