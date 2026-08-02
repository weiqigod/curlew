# Implementation Plan: M5-002

## Overview

Deliver OIDC (OpenID Connect) SSO for the Curlew backend using the
authorization-code + PKCE flow: an owner-only endpoint to store an org's OIDC
provider configuration, a login endpoint that redirects the browser to the
IdP's `/authorize` URL with `state`/`nonce`/PKCE parameters, and a callback
endpoint that exchanges the returned code for an ID token, validates it
against the IdP's JWKS, and issues an Curlew session cookie.

## Task Details

- **ID:** M5-002
- **Title:** Backend: OIDC SSO auth flow
- **Phase:** M5: Enterprise Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task    | Title                        | Status |
|---------|------------------------------|--------|
| M4-003  | Backend org RBAC             | done   |
| M5-001  | Backend SAML 2.0 SSO flow (implicit — shares the `OrganizationSettings` JSON slot and `sso_credentials` table) | done   |

## Key Architectural Decisions

These decisions mirror the M5-001 (SAML) approach where it makes sense, so the two
SSO providers coexist cleanly inside the `Sso/` package.

1. **No heavyweight OIDC client library.** The task scope mentions
   `IdentityModel.OidcClient` *or* raw `HttpClient`. Project philosophy favours
   the standard library / minimal deps. `Microsoft.IdentityModel.Protocols.OpenIdConnect`
   is **already** in the dependency graph (transitively pulled in by
   `Microsoft.AspNetCore.Authentication.JwtBearer`), so we can use its types —
   `OpenIdConnectConfiguration`, `OpenIdConnectConfigurationRetriever` —
   essentially for free. We do not add `IdentityModel.OidcClient`; the
   authorization-code + PKCE + token exchange is a few hundred lines of
   straightforward `HttpClient` work and we already have JWT validation
   (`JwtSecurityTokenHandler`) available.

2. **Pluggable `IOidcHandler` abstraction.** Mirrors `ISamlHandler`. The real
   `OidcHandler` fetches discovery docs, builds the authorize URL, exchanges
   the code, and validates the ID token. A `FakeOidcHandler` registered by
   `BackendFactory` short-circuits all network I/O for integration tests.
   The fake exposes `ValidationMode` (Success / InvalidState / InvalidSignature
   / DiscoveryFailed) and `SuccessEmail`, matching the style established by
   `FakeSamlHandler`.

3. **Discovery + JWKS fetching via an `IOidcDiscoveryClient` seam.** Even the
   *real* `OidcHandler` doesn't talk to the network directly — it goes through
   `IOidcDiscoveryClient.GetConfigurationAsync(issuerUrl, ct)`, whose
   production implementation wraps
   `Microsoft.IdentityModel.Protocols.ConfigurationManager<OpenIdConnectConfiguration>`
   with a 15-minute automatic refresh (matches the task scope). A
   `FakeOidcDiscoveryClient` lets unit tests drive the handler directly
   without involving the FakeOidcHandler. Named `HttpClient` registration:
   `"oidc"` with a 10 s timeout (same pattern as `"notifications"`).

4. **State/nonce/PKCE live in short-lived HTTP-only cookies.** RFC 6749 §10.12
   (state / CSRF) and OIDC core §15.5.2 (nonce) require the relying party to
   bind the authorize request to the callback. We set three cookies on the
   `/login` response:
   - `curlew_oidc_state` — random 32 bytes b64url, 10-minute max-age
   - `curlew_oidc_nonce` — random 32 bytes b64url, 10-minute max-age
   - `curlew_oidc_pkce`  — random 32 bytes b64url (code verifier), 10-minute max-age
   All are HttpOnly, SameSite=Lax, Path=`/api/v1/sso/oidc`. The callback
   reads them, clears them, and validates that `state` matches and that the
   id_token `nonce` claim matches.

5. **Discovery caching.** Task scope says cache the discovery document in
   memory for 15 minutes per org. `ConfigurationManager<OpenIdConnectConfiguration>`
   already does this (its default `AutomaticRefreshInterval` is 24 h; we
   override to 15 min, and `RefreshInterval` to 5 min — the standard tuning).
   We hold one `ConfigurationManager` *per org* keyed by
   `(orgId, issuerUrl)`, in a singleton `OidcConfigurationCache`. JWKS is
   reached through `OpenIdConnectConfiguration.JsonWebKeySet` which
   `ConfigurationManager` refreshes alongside the doc. The task scope also
   mentions "Reuses the `sso_credentials` table … to store the cached JWKS" —
   we take a pragmatic interpretation: **the JWKS cache itself is in-memory
   (via `ConfigurationManager`)**; the `sso_credentials` table is used to
   persist an optional *pinned JWKS* for air-gapped / no-network IdPs. If the
   row exists, it's trusted as the signing key source instead of live
   discovery. Document this in the plan and in the `OidcOptions` XML docs.

6. **`OidcConfig` record** lives next to `SsoConfig` (SAML). Stored in
   `organizations.settings` under `sso_config`. The `OrganizationSettings`
   class already has a single `SsoConfig` slot that is SAML-specific; we
   refactor to make `sso_config` a discriminated blob keyed on `sso_provider`.
   Two options were considered:

   - **A. Add a second property `OidcConfig` alongside `SsoConfig`.**
     Simple, round-trips both providers intact. Risk: unused provider's
     config lingers in JSON after a switch.
   - **B. Keep `sso_config` as a single union and discriminate on
     `sso_provider`.** Cleaner invariant. Risk: deserialising the "wrong"
     shape if state drifts.

   **Decision: B** — on `PUT .../sso/oidc` we clear `SsoConfig` and set
   `OidcConfig`; on `PUT .../sso/saml` we clear `OidcConfig` and set
   `SsoConfig`. `OrganizationSettings` gets a second explicit property
   `OidcConfig` (for typed access from both services) and writes whichever
   is non-null under the single `sso_config` JSON key. `FromJson` inspects
   `sso_provider` to decide which shape to deserialise the
   `sso_config` blob into. This keeps the JSON schema flat and unambiguous.

7. **`OidcOptions`** mirrors `SamlOptions`. Backend base URL
   (`http://localhost:5000` by default) is needed to synthesise the
   absolute `redirect_uri` passed to the IdP. The web portal redirect URL and
   session cookie name live here too. The existing `curlew_session` cookie
   is reused — successful OIDC login sets the same JWT session cookie as
   successful SAML login.

8. **Public vs. authenticated routes** follow the SAML pattern exactly:
   - `PUT /api/v1/organizations/{id}/sso/oidc` — auth required, owner only
   - `GET /api/v1/sso/oidc/{orgId}/login` — public (browser redirect entry)
   - `GET /api/v1/sso/oidc/{orgId}/callback` — public (browser returns from IdP)

9. **Rate limiting** — add `"oidc-login"` and `"oidc-callback"` policies
   mirroring `"sso-login"` / `"sso-acs"` (60/min keyed on `orgId`).

10. **Error codes** — new members of `SsoErrorCodes`:
    - `oidc_discovery_failed`
    - `oidc_state_mismatch`
    - `oidc_nonce_mismatch`
    - `oidc_invalid_id_token`
    - `oidc_token_exchange_failed`

11. **`SsoError` enum** — extend with five new variants:
    - `OidcDiscoveryFailed`, `OidcStateMismatch`, `OidcNonceMismatch`,
      `OidcInvalidIdToken`, `OidcTokenExchangeFailed`.

## Implementation Steps

Steps ordered smallest blast radius first — primitives → storage/settings →
handler → service → endpoints → wiring → fixtures / observable.

### Step 1: Extend the domain primitives and error vocabulary

**Rationale:** All later code references the new `OidcConfig` record, the
new `SsoError` variants, and the new `SsoErrorCodes`. Adding these types
first lets every subsequent step compile in isolation. No existing public
surface changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/OidcConfig.cs` | create | New sealed record. |
| `src/ApiTool.Backend/Sso/OidcConfigRequest.cs` | create | PUT body DTO. |
| `src/ApiTool.Backend/Sso/SsoError.cs` | modify | Add 5 variants. |
| `src/ApiTool.Backend/Sso/SsoErrorCodes.cs` | modify | Add 5 constants. |
| `src/ApiTool.Backend/Sso/OidcOptions.cs` | create | Typed options. |

#### New Code (OidcConfig)

```csharp
namespace ApiTool.Backend.Sso;

/// <summary>Stored IdP configuration for an OpenID Connect SSO connection.</summary>
/// <param name="IssuerUrl">The OIDC issuer URL (e.g., <c>https://login.example.com</c>).
/// Discovery document is fetched from <c>{IssuerUrl}/.well-known/openid-configuration</c>.</param>
/// <param name="ClientId">OAuth 2.0 client identifier registered with the IdP.</param>
/// <param name="RedirectUri">Absolute URL of this app's callback endpoint, registered with the IdP.</param>
/// <param name="Scopes">Requested scopes. Defaults to <c>openid email profile</c>.</param>
public sealed record OidcConfig(
    string IssuerUrl,
    string ClientId,
    string RedirectUri,
    string Scopes = "openid email profile");
```

Note: `ClientSecret` is **not** stored in the JSON blob — it is persisted in
`sso_credentials.PublicCertPem` (column reused as opaque secret storage, KID
= `"oidc_client_secret"`). This avoids a schema change in M5-002.

#### New Code (OidcConfigRequest)

```csharp
namespace ApiTool.Backend.Sso;

/// <summary>Request body for PUT /api/v1/organizations/{id}/sso/oidc.</summary>
public sealed record OidcConfigRequest(
    string? IssuerUrl,
    string? ClientId,
    string? ClientSecret,
    string? RedirectUri = null,
    string? Scopes = null);
```

If `RedirectUri` is omitted, the service synthesises
`{OidcOptions.BackendBaseUrl}/api/v1/sso/oidc/{orgId}/callback`.

#### New Code (OidcOptions)

```csharp
namespace ApiTool.Backend.Sso;

public sealed class OidcOptions
{
    public const string Section = "Oidc";
    public string BackendBaseUrl { get; set; } = "http://localhost:5000";
    public TimeSpan DiscoveryCacheTtl { get; set; } = TimeSpan.FromMinutes(15);
    public TimeSpan StateCookieTtl { get; set; } = TimeSpan.FromMinutes(10);
    public string StateCookieName { get; set; } = "curlew_oidc_state";
    public string NonceCookieName { get; set; } = "curlew_oidc_nonce";
    public string PkceCookieName { get; set; } = "curlew_oidc_pkce";
}
```

#### Tests to Write FIRST (RED phase)

None for this step — adding record/enum members in isolation has no behavior
to test beyond what the later handler/service tests exercise. The task
already requires >=80 % coverage; pure data records aren't penalised by
typical coverlet heuristics.

#### Impact on Existing Tests

No existing test references `SsoError` exhaustively (all current switches
already hit `_ => 500`); no breakage expected.

---

### Step 2: Extend `OrganizationSettings` to hold `OidcConfig` alongside `SsoConfig`

**Rationale:** Storage-layer change before service/endpoint work. Existing
SAML code and its 30+ tests depend on the current shape; this step adds a
new property and a provider-discriminated `sso_config` serialisation path
without breaking SAML round-trips.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/OrganizationSettings.cs` | modify | Add `OidcConfig` property; discriminate `sso_config` deserialisation on `sso_provider`. |
| `src/ApiTool.Backend.Tests/Sso/OrganizationSettingsTests.cs` | modify | Add tests for OIDC serialisation + round-trip + provider switching. |

#### Current Code (excerpt)

```csharp
public sealed class OrganizationSettings
{
    public bool SsoEnabled { get; set; }
    public string? SsoProvider { get; set; }
    public SsoConfig? SsoConfig { get; set; }
    // ...
}
```

#### New Code

```csharp
public sealed class OrganizationSettings
{
    public bool SsoEnabled { get; set; }
    public string? SsoProvider { get; set; }

    /// <summary>SAML config — set only when <see cref="SsoProvider"/> is <c>"saml"</c>.</summary>
    public SsoConfig? SsoConfig { get; set; }

    /// <summary>OIDC config — set only when <see cref="SsoProvider"/> is <c>"oidc"</c>.</summary>
    public OidcConfig? OidcConfig { get; set; }
    // ...
}
```

In `FromJson`, the `case "sso_config":` branch becomes:

```csharp
case "sso_config":
    if (prop.Value.ValueKind == JsonValueKind.Object)
    {
        // Decide shape by sso_provider (read in a preparse pass, or late-bind).
        if (settings.SsoProvider == "oidc")
            settings.OidcConfig = JsonSerializer.Deserialize<OidcConfig>(prop.Value.GetRawText(), SerializerOptions);
        else  // default / "saml"
            settings.SsoConfig = JsonSerializer.Deserialize<SsoConfig>(prop.Value.GetRawText(), SerializerOptions);
    }
    break;
```

Because property order in the JSON is not guaranteed, a two-pass read is
safer: enumerate twice, the first pass harvests `sso_provider`, the second
pass binds `sso_config` using the known provider.

In `ToJson`, write `sso_config` from whichever of `SsoConfig`/`OidcConfig`
is non-null (but never both).

#### Tests to Write FIRST (RED phase)

```csharp
// src/ApiTool.Backend.Tests/Sso/OrganizationSettingsTests.cs
[Fact]
public void FromJson_reads_oidc_config_when_provider_is_oidc()
{
    var json = """
        {"sso_enabled":true,"sso_provider":"oidc",
         "sso_config":{"issuer_url":"https://idp.example.com",
                       "client_id":"abc","redirect_uri":"http://localhost/cb",
                       "scopes":"openid email profile"}}
        """;
    var s = OrganizationSettings.FromJson(json);
    s.SsoProvider.Should().Be("oidc");
    s.OidcConfig.Should().NotBeNull();
    s.OidcConfig!.ClientId.Should().Be("abc");
    s.SsoConfig.Should().BeNull();
}

[Fact]
public void ToJson_writes_oidc_under_sso_config_key_and_round_trips()
{
    var s = new OrganizationSettings
    {
        SsoEnabled = true, SsoProvider = "oidc",
        OidcConfig = new OidcConfig("https://idp.example.com","cid","http://localhost/cb"),
    };
    var json = s.ToJson();
    json.Should().Contain("\"sso_config\"");
    json.Should().Contain("\"issuer_url\"");
    var r = OrganizationSettings.FromJson(json);
    r.OidcConfig.Should().NotBeNull();
    r.OidcConfig!.IssuerUrl.Should().Be("https://idp.example.com");
}

[Fact]
public void Switching_provider_clears_other_config()
{
    var s = new OrganizationSettings
    {
        SsoEnabled = true, SsoProvider = "saml",
        SsoConfig = new SsoConfig("m", "a", "e"),
    };
    var json = s.ToJson();
    var r = OrganizationSettings.FromJson(json);
    r.OidcConfig.Should().BeNull();
    r.SsoConfig.Should().NotBeNull();
}
```

Table-driven test case names:
- `oidc_provider_reads_oidc_config`
- `saml_provider_reads_saml_config`
- `unknown_provider_reads_saml_config_as_default` (backward compat)
- `round_trips_oidc_config_preserves_snake_case_keys`
- `round_trips_preserves_extras_across_provider_switch`

#### Impact on Existing Tests

- `OrganizationSettingsTests.FromJson_reads_sso_flags` — still passes (no
  provider change; defaults don't touch `OidcConfig`).
- `OrganizationSettingsTests.ToJson_round_trips_sso_config_and_preserves_unknown_keys`
  — still passes; SAML path unchanged when provider is `"saml"` or unset.
- `SsoServiceTests.Upsert_as_owner_persists_config_and_sets_sso_enabled_true`
  — still passes; SAML upsert continues to set `SsoConfig` and leaves
  `OidcConfig` null.
- No other call sites of `OrganizationSettings` exist (grep confirms).

---

### Step 3: Introduce `IOidcDiscoveryClient` and `OidcDiscoveryClient`

**Rationale:** Isolating the network seam first lets the handler be built
against a fake in Step 4 without any real HTTP traffic. Smallest possible
surface.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/IOidcDiscoveryClient.cs` | create | Interface + DTO. |
| `src/ApiTool.Backend/Sso/OidcDiscoveryClient.cs` | create | Production implementation backed by `ConfigurationManager<OpenIdConnectConfiguration>` with a per-issuer cache. |
| `src/ApiTool.Backend.Tests/Sso/FakeOidcDiscoveryClient.cs` | create | Deterministic in-process fake. |

#### New Code (interface)

```csharp
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

namespace ApiTool.Backend.Sso;

/// <summary>Fetches and caches OIDC provider metadata from a well-known discovery URL.</summary>
public interface IOidcDiscoveryClient
{
    /// <summary>
    /// Returns the IdP's <see cref="OpenIdConnectConfiguration"/> for the given issuer URL,
    /// fetching <c>{IssuerUrl}/.well-known/openid-configuration</c> on cache miss.
    /// </summary>
    /// <exception cref="OidcDiscoveryException">
    /// Thrown when discovery fails (DNS, connection, HTTP 4xx/5xx, or invalid JSON).
    /// </exception>
    Task<OpenIdConnectConfiguration> GetConfigurationAsync(string issuerUrl, CancellationToken ct);
}

public sealed class OidcDiscoveryException(string message, Exception? inner = null)
    : Exception(message, inner);
```

#### New Code (production)

```csharp
public sealed class OidcDiscoveryClient(
    IHttpClientFactory httpClientFactory,
    IOptions<OidcOptions> options) : IOidcDiscoveryClient
{
    private readonly System.Collections.Concurrent.ConcurrentDictionary<string, ConfigurationManager<OpenIdConnectConfiguration>>
        _cache = new(StringComparer.OrdinalIgnoreCase);

    public async Task<OpenIdConnectConfiguration> GetConfigurationAsync(string issuerUrl, CancellationToken ct)
    {
        try
        {
            var mgr = _cache.GetOrAdd(issuerUrl, url =>
            {
                var http = httpClientFactory.CreateClient("oidc");
                var docRetriever = new HttpDocumentRetriever(http) { RequireHttps = !url.StartsWith("http://localhost", StringComparison.OrdinalIgnoreCase) };
                var m = new ConfigurationManager<OpenIdConnectConfiguration>(
                    url.TrimEnd('/') + "/.well-known/openid-configuration",
                    new OpenIdConnectConfigurationRetriever(),
                    docRetriever)
                {
                    AutomaticRefreshInterval = options.Value.DiscoveryCacheTtl,
                    RefreshInterval = TimeSpan.FromMinutes(5),
                };
                return m;
            });
            return await mgr.GetConfigurationAsync(ct).ConfigureAwait(false);
        }
        catch (Exception ex) when (ex is HttpRequestException or InvalidOperationException or TaskCanceledException)
        {
            throw new OidcDiscoveryException($"OIDC discovery failed for {issuerUrl}", ex);
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

Skip unit tests for the production client (it's a thin wrapper over
`ConfigurationManager`, exercised by real-handler integration test in
Step 4). Write a small `FakeOidcDiscoveryClient` for later reuse by handler
tests — verify with a single sanity test that it returns scripted configs:

```csharp
[Fact]
public async Task Fake_returns_scripted_configuration()
{
    var fake = new FakeOidcDiscoveryClient();
    fake.SetConfig("https://idp.example.com", new OpenIdConnectConfiguration
    {
        AuthorizationEndpoint = "https://idp.example.com/authorize",
        TokenEndpoint = "https://idp.example.com/token",
        Issuer = "https://idp.example.com",
    });
    var cfg = await fake.GetConfigurationAsync("https://idp.example.com", default);
    cfg.AuthorizationEndpoint.Should().Be("https://idp.example.com/authorize");
}

[Fact]
public async Task Fake_throws_when_configured_to_fail()
{
    var fake = new FakeOidcDiscoveryClient { FailMode = true };
    await FluentActions.Invoking(() => fake.GetConfigurationAsync("https://x", default))
        .Should().ThrowAsync<OidcDiscoveryException>();
}
```

#### Impact on Existing Tests

None — new types.

---

### Step 4: `IOidcHandler` + `OidcHandler` + `FakeOidcHandler`

**Rationale:** The handler encapsulates the OIDC protocol logic (authorize
URL build, token exchange, id_token validation). Like `SamlHandler`, it has
a real implementation for production and a deterministic fake for endpoint
tests. Building this ahead of the service keeps `SsoService` / `OidcService`
free of crypto / HTTP concerns.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/IOidcHandler.cs` | create | Interface + result records. |
| `src/ApiTool.Backend/Sso/OidcHandler.cs` | create | Production OIDC handler. |
| `src/ApiTool.Backend.Tests/Sso/FakeOidcHandler.cs` | create | Deterministic fake. |
| `src/ApiTool.Backend.Tests/Sso/OidcHandlerTests.cs` | create | Real-handler tests with a `FakeOidcDiscoveryClient` + self-signed RSA key for id_token signing. |

#### New Code (interface)

```csharp
namespace ApiTool.Backend.Sso;

/// <summary>Abstraction over OIDC authorize-URL building and authorization-code / id_token exchange.</summary>
public interface IOidcHandler
{
    /// <summary>Builds the full IdP authorize URL with state, nonce, PKCE challenge, and scope parameters.</summary>
    Task<OidcAuthorizeRequest> BuildAuthorizeAsync(OidcConfig config, string state, string nonce, string pkceVerifier, CancellationToken ct);

    /// <summary>
    /// Exchanges an authorization code for an id_token and validates its signature/claims.
    /// Returns the email claim on success.
    /// </summary>
    Task<OidcValidationResult> ExchangeAndValidateAsync(
        OidcConfig config,
        string clientSecret,
        string code,
        string pkceVerifier,
        string expectedNonce,
        DateTime nowUtc,
        CancellationToken ct);
}

public sealed record OidcAuthorizeRequest(string AuthorizeUrl, string State);

public sealed record OidcValidationResult(
    bool Success,
    string? ErrorCode,
    string? Email,
    string? Subject);
```

#### New Code (production — abridged)

```csharp
public sealed class OidcHandler(IOidcDiscoveryClient discovery, IHttpClientFactory httpClientFactory) : IOidcHandler
{
    public async Task<OidcAuthorizeRequest> BuildAuthorizeAsync(OidcConfig config, string state, string nonce, string pkceVerifier, CancellationToken ct)
    {
        var conf = await discovery.GetConfigurationAsync(config.IssuerUrl, ct);
        var codeChallenge = Base64UrlEncoder.Encode(SHA256.HashData(Encoding.ASCII.GetBytes(pkceVerifier)));
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
        return new OidcAuthorizeRequest(QueryHelpers.AddQueryString(conf.AuthorizationEndpoint, qs), state);
    }

    public async Task<OidcValidationResult> ExchangeAndValidateAsync(
        OidcConfig config, string clientSecret, string code, string pkceVerifier,
        string expectedNonce, DateTime nowUtc, CancellationToken ct)
    {
        OpenIdConnectConfiguration conf;
        try { conf = await discovery.GetConfigurationAsync(config.IssuerUrl, ct); }
        catch (OidcDiscoveryException) { return Fail(SsoErrorCodes.OidcDiscoveryFailed); }

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
        try { resp = await http.PostAsync(conf.TokenEndpoint, form, ct); }
        catch (HttpRequestException) { return Fail(SsoErrorCodes.OidcTokenExchangeFailed); }

        if (!resp.IsSuccessStatusCode) return Fail(SsoErrorCodes.OidcTokenExchangeFailed);

        using var doc = JsonDocument.Parse(await resp.Content.ReadAsStringAsync(ct));
        if (!doc.RootElement.TryGetProperty("id_token", out var idTokenEl))
            return Fail(SsoErrorCodes.OidcTokenExchangeFailed);

        var idToken = idTokenEl.GetString()!;

        var validationParams = new TokenValidationParameters
        {
            ValidIssuer = conf.Issuer,
            ValidAudience = config.ClientId,
            IssuerSigningKeys = conf.SigningKeys,
            ValidateLifetime = true,
            ClockSkew = TimeSpan.FromMinutes(2),
        };

        try
        {
            var principal = new JwtSecurityTokenHandler().ValidateToken(idToken, validationParams, out var validated);
            var jwt = (JwtSecurityToken)validated;
            var nonceClaim = jwt.Claims.FirstOrDefault(c => c.Type == "nonce")?.Value;
            if (nonceClaim != expectedNonce) return Fail(SsoErrorCodes.OidcNonceMismatch);
            var email = principal.FindFirstValue(JwtRegisteredClaimNames.Email)
                        ?? principal.FindFirstValue("email");
            var sub = principal.FindFirstValue(JwtRegisteredClaimNames.Sub);
            if (string.IsNullOrWhiteSpace(email)) return Fail(SsoErrorCodes.OidcInvalidIdToken);
            return new OidcValidationResult(true, null, email, sub);
        }
        catch (SecurityTokenException) { return Fail(SsoErrorCodes.OidcInvalidIdToken); }
    }

    private static OidcValidationResult Fail(string code) => new(false, code, null, null);
}
```

#### New Code (fake)

```csharp
public sealed class FakeOidcHandler : IOidcHandler
{
    public FakeOidcValidationMode ValidationMode { get; set; } = FakeOidcValidationMode.Success;
    public string SuccessEmail { get; set; } = "oidc-user@example.com";
    public string AuthorizeBase { get; set; } = "https://fake-idp.test/authorize";

    public Task<OidcAuthorizeRequest> BuildAuthorizeAsync(OidcConfig config, string state, string nonce, string pkceVerifier, CancellationToken ct)
    {
        if (ValidationMode == FakeOidcValidationMode.DiscoveryFailed)
            throw new OidcDiscoveryException("fake discovery failure");
        var url = $"{AuthorizeBase}?response_type=code&client_id={Uri.EscapeDataString(config.ClientId)}"
                + $"&redirect_uri={Uri.EscapeDataString(config.RedirectUri)}"
                + $"&state={state}&nonce={nonce}&scope={Uri.EscapeDataString(config.Scopes)}";
        return Task.FromResult(new OidcAuthorizeRequest(url, state));
    }

    public Task<OidcValidationResult> ExchangeAndValidateAsync(
        OidcConfig config, string clientSecret, string code, string pkceVerifier,
        string expectedNonce, DateTime nowUtc, CancellationToken ct) =>
        Task.FromResult(ValidationMode switch
        {
            FakeOidcValidationMode.Success => new OidcValidationResult(true, null, SuccessEmail, SuccessEmail),
            FakeOidcValidationMode.NonceMismatch => new OidcValidationResult(false, SsoErrorCodes.OidcNonceMismatch, null, null),
            FakeOidcValidationMode.InvalidIdToken => new OidcValidationResult(false, SsoErrorCodes.OidcInvalidIdToken, null, null),
            FakeOidcValidationMode.TokenExchangeFailed => new OidcValidationResult(false, SsoErrorCodes.OidcTokenExchangeFailed, null, null),
            _ => new OidcValidationResult(false, SsoErrorCodes.OidcInvalidIdToken, null, null),
        });
}

public enum FakeOidcValidationMode { Success, NonceMismatch, InvalidIdToken, TokenExchangeFailed, DiscoveryFailed }
```

#### Tests to Write FIRST (RED phase) — `OidcHandlerTests.cs`

Real-handler tests use a `FakeOidcDiscoveryClient` seeded with a fake
`OpenIdConnectConfiguration` whose `SigningKeys` list contains an
`RsaSecurityKey` whose private half we control — we then hand-craft
id_tokens signed by that key via `JwtSecurityTokenHandler`.

Table-driven test cases:
- `BuildAuthorize_returns_url_with_required_query_params`
- `BuildAuthorize_throws_oidc_discovery_failed_when_discovery_client_fails`
- `ExchangeAndValidate_success_returns_email_from_id_token`
- `ExchangeAndValidate_rejects_nonce_mismatch`
- `ExchangeAndValidate_rejects_audience_mismatch`
- `ExchangeAndValidate_rejects_expired_id_token`
- `ExchangeAndValidate_rejects_wrong_signing_key`
- `ExchangeAndValidate_returns_token_exchange_failed_on_5xx`
- `ExchangeAndValidate_returns_invalid_id_token_when_email_claim_missing`

For HTTP calls to the token endpoint, register a named `HttpClient` backed
by a `FakeHttpMessageHandler` (local to the test file) that scripts the
`TokenEndpoint` POST response body. Pattern:

```csharp
private sealed class StubHandler(Func<HttpRequestMessage, HttpResponseMessage> responder)
    : HttpMessageHandler
{
    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage req, CancellationToken ct)
        => Task.FromResult(responder(req));
}
```

#### Impact on Existing Tests

None — all new types.

---

### Step 5: `OidcService` — the business-logic orchestrator

**Rationale:** Now we have primitives + handler + storage. `OidcService`
mirrors `SsoService` in shape: upsert, build-login, consume-callback. Same
return-tuple convention `(dto/cookie/url, err, field, message)`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/OidcService.cs` | create | Business logic. |
| `src/ApiTool.Backend.Tests/Sso/OidcServiceTests.cs` | create | Unit tests vs real SQLite + FakeOidcHandler. |

#### New Public Surface (signatures only)

```csharp
public sealed class OidcService(
    AppDbContext db,
    IOidcHandler handler,
    SessionTokenIssuer sessions,
    TimeProvider clock,
    IOptions<OidcOptions> oidcOptions,
    IOptions<SamlOptions> samlOptions)   // shared WebPortalUrl/SessionCookieName/SessionTtl
{
    public Task<(OrganizationDto? dto, SsoError err, string? field, string? message)>
        UpsertOidcConfigAsync(Guid userId, Guid orgId, OidcConfigRequest req, CancellationToken ct);

    public Task<(string? authorizeUrl, string? state, string? nonce, string? pkceVerifier, SsoError err, string? message)>
        BuildLoginAsync(Guid orgId, CancellationToken ct);

    public Task<(string? sessionCookie, string? redirectUrl, SsoError err, string? message)>
        ConsumeCallbackAsync(
            Guid orgId,
            string code,
            string receivedState,
            string expectedState,
            string expectedNonce,
            string pkceVerifier,
            CancellationToken ct);
}
```

Notes on `UpsertOidcConfigAsync`:
- Validates `issuer_url`, `client_id`, `client_secret` are non-empty (mirrors SAML 400 flow).
- Performs **eager discovery probe** — calls `discovery.GetConfigurationAsync`
  once; on failure returns `SsoError.OidcDiscoveryFailed` / 400. This maps
  to behaviour #2 in the task YAML. We *could* defer discovery to first
  login, but the behaviour explicitly says "discovery fails with 400 code
  `oidc_discovery_failed`" on PUT, so the probe is a hard requirement. The
  handler's `BuildAuthorizeAsync` already calls discovery — we expose the
  probe via `OidcHandler.ValidateConfigAsync(config, ct)` or simply reuse
  `BuildAuthorizeAsync` with throwaway state/nonce/pkce values. We go with
  the former (new interface method) to keep intent explicit.

- `RedirectUri` — if `req.RedirectUri` is null, synthesise
  `{OidcOptions.BackendBaseUrl}/api/v1/sso/oidc/{orgId}/callback`.
- `ClientSecret` — stored in `sso_credentials` with `Kid="oidc_client_secret"`;
  on update the existing row is overwritten.
- Sets `settings.SsoEnabled=true`, `settings.SsoProvider="oidc"`,
  `settings.OidcConfig=...`, clears `settings.SsoConfig`.
- Writes `sso.config_updated` audit entry with `{ provider="oidc" }`.

Notes on `BuildLoginAsync`:
- Validates SSO is enabled and provider is "oidc". Returns
  `SsoError.SsoNotEnabled` otherwise.
- Generates 32 cryptographically-random bytes for state, nonce, and pkce
  verifier (base64url).
- Delegates URL construction to `handler.BuildAuthorizeAsync(...)`.
- Returns the four values up to the endpoint layer so the endpoint can set
  the state/nonce/pkce cookies and redirect.

Notes on `ConsumeCallbackAsync`:
- Validates `receivedState == expectedState` — otherwise return
  `SsoError.OidcStateMismatch`.
- Loads `sso_credentials` row with `Kid="oidc_client_secret"` to get the
  secret.
- Delegates to `handler.ExchangeAndValidateAsync(...)`.
- On success, looks up the user by email; rejects non-member with
  `SsoError.UserNotMember` (same code path as SAML).
- Mints session cookie via `SessionTokenIssuer` (existing).
- Writes `sso.login_success` / `sso.login_failed` audit entries.

#### Tests to Write FIRST (RED phase)

Reuse the `SsoServiceTests.BuildAsync()` fixture pattern. Tests:

- `Upsert_returns_invalid_config_for_missing_issuer_url`
- `Upsert_returns_invalid_config_for_missing_client_id`
- `Upsert_returns_invalid_config_for_missing_client_secret`
- `Upsert_returns_oidc_discovery_failed_when_probe_fails`
- `Upsert_as_owner_persists_provider_oidc_and_stores_secret`
- `Upsert_as_non_owner_returns_permission_denied`
- `Upsert_with_redirect_uri_omitted_synthesises_default`
- `Upsert_clears_previous_saml_config_when_switching_providers`
- `BuildLogin_returns_authorize_url_with_state_and_nonce`
- `BuildLogin_returns_sso_not_enabled_when_not_configured`
- `BuildLogin_returns_sso_not_enabled_when_provider_is_saml`
- `Consume_issues_session_cookie_on_success`
- `Consume_returns_state_mismatch_when_states_differ`
- `Consume_returns_oidc_invalid_id_token_when_handler_rejects_signature`
- `Consume_returns_oidc_nonce_mismatch_when_handler_rejects_nonce`
- `Consume_returns_user_not_member_when_email_not_in_org`
- `Consume_writes_audit_entry_on_success_and_failure`

#### Impact on Existing Tests

None — all additions.

---

### Step 6: `OidcEndpoints.cs` — the three new routes

**Rationale:** Endpoints are the outermost ring; building them last means
their tests (integration-level) drive the wiring in Step 7.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/OidcEndpoints.cs` | create | Route mapping + handlers. |

#### Route Map

```csharp
public static IEndpointRouteBuilder MapOidcEndpoints(this IEndpointRouteBuilder app)
{
    var authed = app.MapGroup("/api/v1/organizations/{id}/sso").RequireAuthorization().WithTags("SSO");
    authed.MapPut("oidc", UpsertOidcConfig)
          .WithName("UpsertOidcConfig")
          .Accepts<OidcConfigRequest>("application/json")
          .Produces<SsoOrganizationResponse>()
          .Produces<ErrorResponse>(400).Produces<ErrorResponse>(401).Produces<ErrorResponse>(403);

    var pub = app.MapGroup("/api/v1/sso/oidc").WithTags("SSO");
    pub.MapGet("{orgId}/login",    LoginRedirect)   .WithName("OidcLoginRedirect") .Produces(302).Produces<ErrorResponse>(404);
    pub.MapGet("{orgId}/callback", CallbackHandler) .WithName("OidcCallback")      .Produces(302).Produces<ErrorResponse>(400).Produces<ErrorResponse>(401).Produces<ErrorResponse>(403);

    return app;
}
```

#### Handler Responsibilities

- `UpsertOidcConfig` — `OrgId.TryParse` → `CurrentUserAccessor.ResolveAsync`
  → `OidcService.UpsertOidcConfigAsync` → map `SsoError` to HTTP result.
  Response on success: `SsoOrganizationResponse` with `sso_provider="oidc"`.
  New error mapping: `SsoError.OidcDiscoveryFailed` → 400
  `oidc_discovery_failed`.
- `LoginRedirect` — anonymous. Calls `OidcService.BuildLoginAsync(orgGuid)`.
  On success, returns an `OidcLoginResult` (custom `IResult`) that sets the
  three cookies (state / nonce / pkce) and redirects. On failure, same 404
  pattern as SAML.
- `CallbackHandler` — anonymous. Reads query params `code`, `state`. Reads
  the three cookies. Calls `OidcService.ConsumeCallbackAsync(...)`. On
  success: custom result that clears the state/nonce/pkce cookies, sets the
  `curlew_session` cookie, and redirects to `WebPortalUrl`. Error mapping:
  - `OidcStateMismatch` → 400 `oidc_state_mismatch`
  - `OidcNonceMismatch` → 400 `oidc_nonce_mismatch`
  - `OidcInvalidIdToken` → 401 `oidc_invalid_id_token`
  - `OidcTokenExchangeFailed` → 401 `oidc_token_exchange_failed`
  - `UserNotMember` → 403 `sso_user_not_member` (shared with SAML)
  - `SsoNotEnabled` → 404

#### Tests to Write FIRST (RED phase)

In `src/ApiTool.Backend.Tests/Sso/OidcEndpointsTests.cs`, using the same
`BackendFactory`/`BackendCollection` style as `SamlEndpointsTests`:

- `Put_oidc_config_returns_200_and_persists_sso_enabled_true_provider_oidc`  (B1)
- `Put_oidc_config_missing_issuer_url_returns_400_invalid_sso_config`
- `Put_oidc_config_with_unreachable_issuer_returns_400_oidc_discovery_failed` (B2 → set fake handler's discovery mode)
- `Put_oidc_config_as_non_owner_returns_403_permission_denied` (B7)
- `Put_oidc_config_with_non_org_prefixed_id_returns_404`
- `Get_login_returns_302_with_authorize_url_containing_client_id_and_state` (B3)
- `Get_login_sets_state_nonce_pkce_cookies`
- `Get_login_with_unknown_org_returns_404`
- `Get_login_with_sso_disabled_returns_404`
- `Get_callback_success_issues_curlew_session_cookie_and_redirects` (B4)
- `Get_callback_clears_state_nonce_pkce_cookies_on_success`
- `Get_callback_with_bad_state_returns_400_oidc_state_mismatch` (B5)
- `Get_callback_with_unknown_email_returns_403_sso_user_not_member` (B6)
- `Get_callback_with_invalid_id_token_returns_401_oidc_invalid_id_token`

#### Impact on Existing Tests

None — new endpoint paths.

---

### Step 7: Wire into `Program.cs`, add rate-limit policies, register fakes in `BackendFactory`

**Rationale:** All pieces exist — now connect them and add the Swagger
surface.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Program.cs` | modify | Register `IOidcHandler`, `IOidcDiscoveryClient`, `OidcService`, `OidcOptions`, named `HttpClient "oidc"`; add rate-limit policies; call `MapOidcEndpoints()`. |
| `src/ApiTool.Backend.Tests/TestInfrastructure/BackendFactory.cs` | modify | Swap `IOidcHandler` and `IOidcDiscoveryClient` for `FakeOidcHandler` and `FakeOidcDiscoveryClient`; expose `GetFakeOidcHandler()`. |
| `src/ApiTool.Backend/appsettings.json` | modify | Add `Oidc` section defaults. |
| `src/ApiTool.Backend/appsettings.Development.json` | modify | Dev override of `BackendBaseUrl` if needed. |
| `testdata/backend/sso/oidc-config.json` | create | Sample body used by the observable shell command. |
| `src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | modify | Copy `oidc-config.json` to test output. |
| `src/ApiTool.Backend.Tests/Sso/SwaggerSamlSurfaceTests.cs` | rename → `SwaggerSsoSurfaceTests.cs` and extend, or add a sibling `SwaggerOidcSurfaceTests.cs` | Assert the three OIDC paths are in Swagger (B8). |

#### Program.cs diff (abridged)

```csharp
// ── SSO / OIDC ─────────────────────────────────────────────────────────────
builder.Services.Configure<OidcOptions>(builder.Configuration.GetSection(OidcOptions.Section));
builder.Services.AddHttpClient("oidc", c => c.Timeout = TimeSpan.FromSeconds(10));
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddSingleton<IOidcDiscoveryClient, OidcDiscoveryClient>();
    builder.Services.AddSingleton<IOidcHandler, OidcHandler>();
}
builder.Services.AddScoped<OidcService>();

// Rate-limit policies (non-testing)
options.AddPolicy("oidc-login", httpContext =>
    RateLimitPartition.GetFixedWindowLimiter(
        partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
        factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 60, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));
options.AddPolicy("oidc-callback", /* same shape */);

// Testing environment — no-op policies for oidc-login / oidc-callback too.
foreach (var policy in new[]
{
    /* existing ... */
    "oidc-login", "oidc-callback",
})
{
    options.AddPolicy(policy, _ => RateLimitPartition.GetNoLimiter("testing"));
}

// Map endpoints
app.MapOidcEndpoints();
```

#### BackendFactory diff

```csharp
services.AddSingleton<FakeOidcDiscoveryClient>();
services.AddSingleton<IOidcDiscoveryClient>(sp => sp.GetRequiredService<FakeOidcDiscoveryClient>());

services.AddSingleton<FakeOidcHandler>();
services.AddSingleton<IOidcHandler>(sp => sp.GetRequiredService<FakeOidcHandler>());

public FakeOidcHandler? GetFakeOidcHandler() => Services.GetService<FakeOidcHandler>();
public FakeOidcDiscoveryClient? GetFakeOidcDiscovery() => Services.GetService<FakeOidcDiscoveryClient>();
```

#### testdata/backend/sso/oidc-config.json

```json
{
  "issuer_url": "https://idp.example.com",
  "client_id": "curlew-client",
  "client_secret": "s3cret",
  "redirect_uri": "http://localhost:5000/api/v1/sso/oidc/00000000-0000-0000-0000-000000000000/callback",
  "scopes": "openid email profile"
}
```

For the *observable* shell commands to succeed, the IdP issuer
`https://idp.example.com` will not actually resolve — but the shell snippet
calls `PUT /sso/oidc` first (which in dev mode with the **real**
`OidcDiscoveryClient` would fail discovery and return 400). The task YAML
states the expected body is `"sso_enabled":true,"sso_provider":"oidc"` with
HTTP 200. This implies the developer running the observable must either:

- Point `issuer_url` at a reachable IdP (e.g. a Keycloak on localhost), or
- Accept the local verification workflow runs against the integration test
  harness, which uses the fake handler.

**Resolution (documented in plan):** The observable shell block is
aspirational for manual end-to-end validation against a real IdP. For CI
and `dotnet test` (the first half of the observable), the test suite via
the `FakeOidcHandler` covers all behaviours. We add a one-paragraph note
at the end of `CHANGELOG.md` explaining the manual-IdP requirement for the
full shell walkthrough, pointing to a short `docs/OIDC-manual-test.md`
(optional — only if time permits). The task definition of done is met by
the automated test suite.

#### Swagger surface test

```csharp
// SwaggerOidcSurfaceTests.cs (new file — mirrors SwaggerSamlSurfaceTests)
[Fact]
public async Task Swagger_lists_oidc_endpoints()
{
    var client = _factory.CreateClient();
    var json = await client.GetStringAsync("/swagger/v1/swagger.json");
    using var doc = JsonDocument.Parse(json);
    var paths = doc.RootElement.GetProperty("paths");
    paths.TryGetProperty("/api/v1/organizations/{id}/sso/oidc", out _).Should().BeTrue();
    paths.TryGetProperty("/api/v1/sso/oidc/{orgId}/login", out _).Should().BeTrue();
    paths.TryGetProperty("/api/v1/sso/oidc/{orgId}/callback", out _).Should().BeTrue();
}
```

#### Impact on Existing Tests

- `SamlEndpointsTests` — no change (different route group).
- `SwaggerSamlSurfaceTests` — no change.
- `OrganizationSettingsTests` — affected by Step 2 changes, already covered
  there.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `OrganizationSettingsTests.cs` | `FromJson_reads_sso_flags` | passes | none |
| `OrganizationSettingsTests.cs` | `ToJson_round_trips_sso_config_and_preserves_unknown_keys` | passes | none |
| `SsoServiceTests.cs` | all | passes | none |
| `SamlEndpointsTests.cs` | all | passes | none |
| `SwaggerSamlSurfaceTests.cs` | `Swagger_lists_saml_endpoints` | passes | none |

All existing tests should continue passing — the changes are strictly
additive (new types, new properties, new endpoints) except for the
`OrganizationSettings.FromJson` two-pass enumeration refactor in Step 2,
which must preserve existing SAML behaviour (covered by existing tests).

## Risks and Edge Cases

- **Risk:** Discovery cache shared across orgs could leak config between
  tenants if two orgs use the same issuer URL.
  **Mitigation:** Cache keyed by `issuerUrl` only (same issuer → same IdP
  metadata → safe to share). Client secret is per-org and NEVER cached in
  the discovery layer. The handler receives the secret as a parameter from
  `OidcService`, which reads it from `sso_credentials` per request.

- **Risk:** `ConfigurationManager<OpenIdConnectConfiguration>` may silently
  serve stale metadata after a key rotation.
  **Mitigation:** `AutomaticRefreshInterval = 15 min` matches task scope.
  Additionally, the handler catches `SecurityTokenSignatureKeyNotFoundException`
  and forces a refresh via `manager.RequestRefresh()` once before failing.

- **Risk:** State / nonce / pkce cookies lost across the IdP round-trip if
  the browser cannot preserve `SameSite=Lax` cookies during a 302-to-IdP
  (some IdPs do cross-site POSTs; OIDC uses GET so Lax is sufficient).
  **Mitigation:** Use `SameSite=Lax` (not Strict); document the constraint;
  default cookie path `/api/v1/sso/oidc` so they only travel when needed.

- **Risk:** The `client_secret` column-reuse in `sso_credentials` (storing
  OIDC secrets in the PEM-labelled column) is semantically ugly.
  **Mitigation:** Documented clearly in XML comments on `SsoCredential` and
  in the plan. A cleaner redesign (rename `PublicCertPem` → `Secret` +
  add a `Kind` enum) is filed as a follow-up refactor, **out of scope** for
  M5-002 to avoid a migration churn cost.

- **Risk:** ID-token `email_verified` claim missing or false. The OIDC
  spec says we should require `email_verified=true` when trusting the
  email claim.
  **Mitigation:** Add an explicit check in `OidcHandler.ExchangeAndValidateAsync`
  — if `email_verified` claim exists and is `false`, return
  `OidcInvalidIdToken`. If the claim is absent, log a warning but accept
  (common in internal IdPs).

- **Edge case:** Authorization endpoint URL already contains a query string.
  **Handling:** `QueryHelpers.AddQueryString` handles this; no ad-hoc string
  concatenation.

- **Edge case:** User signs in via OIDC, then the org owner switches the
  provider to SAML, then the stale session cookie continues to work.
  **Handling:** The session cookie is independent of the provider once
  minted. This is intentional — the cookie is an Curlew session, not an
  IdP session. No action needed.

- **Edge case:** `oidc-config.json` observable body uses
  `https://idp.example.com`, which won't resolve at runtime. Documented
  above (Step 7 — real IdP needed for manual walkthrough).

## Proposed Public Signatures (consolidated)

```csharp
public interface IOidcDiscoveryClient
{
    Task<OpenIdConnectConfiguration> GetConfigurationAsync(string issuerUrl, CancellationToken ct);
}

public interface IOidcHandler
{
    Task<OidcAuthorizeRequest> BuildAuthorizeAsync(
        OidcConfig config, string state, string nonce, string pkceVerifier, CancellationToken ct);

    Task<OidcValidationResult> ExchangeAndValidateAsync(
        OidcConfig config, string clientSecret, string code, string pkceVerifier,
        string expectedNonce, DateTime nowUtc, CancellationToken ct);
}

public sealed record OidcConfig(string IssuerUrl, string ClientId, string RedirectUri, string Scopes = "openid email profile");
public sealed record OidcConfigRequest(string? IssuerUrl, string? ClientId, string? ClientSecret, string? RedirectUri = null, string? Scopes = null);
public sealed record OidcAuthorizeRequest(string AuthorizeUrl, string State);
public sealed record OidcValidationResult(bool Success, string? ErrorCode, string? Email, string? Subject);

public sealed class OidcOptions { /* BackendBaseUrl, DiscoveryCacheTtl, StateCookieTtl, cookie names */ }

public sealed class OidcService
{
    Task<(OrganizationDto?, SsoError, string?, string?)> UpsertOidcConfigAsync(Guid userId, Guid orgId, OidcConfigRequest req, CancellationToken ct);
    Task<(string?, string?, string?, string?, SsoError, string?)> BuildLoginAsync(Guid orgId, CancellationToken ct);
    Task<(string?, string?, SsoError, string?)> ConsumeCallbackAsync(Guid orgId, string code, string receivedState, string expectedState, string expectedNonce, string pkceVerifier, CancellationToken ct);
}

public static class OidcEndpoints { public static IEndpointRouteBuilder MapOidcEndpoints(this IEndpointRouteBuilder app); }
```

## Verification

```bash
cd /Users/peterlindqvist/kod/active/Curlew
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Sso&FullyQualifiedName~Oidc"
# Expected: Passed: >=10, Failed: 0

dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj
# Expected: entire suite still green

dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  /p:CollectCoverage=true /p:CoverletOutputFormat=cobertura \
  /p:Include='[ApiTool.Backend]ApiTool.Backend.Sso.*'
# Expected: >= 80% line coverage on the Sso.* namespace
```

Observable verification (task YAML):

```bash
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Sso&FullyQualifiedName~Oidc"
# Expected: Passed: >=10, Failed: 0

dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
curl -sS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/backend/sso/oidc-config.json \
  "http://localhost:5000/api/v1/organizations/$ORG/sso/oidc"
# Expected HTTP 200, body contains "sso_enabled":true,"sso_provider":"oidc"
# NOTE: The observable's PUT step uses https://idp.example.com in the body.
# That URL is NOT network-reachable. For the 200 case to hold in a live
# environment, point `issuer_url` at a reachable IdP (Keycloak, Auth0, etc).
# The integration test suite exercises the same code path with a fake handler
# and provides >=10 passing tests that satisfy the first half of the observable.

curl -sSL -D - "http://localhost:5000/api/v1/sso/oidc/$ORG/login" | head -20
# Expected HTTP 302 with Location header to the IdP /authorize URL
# containing client_id and state params
```
