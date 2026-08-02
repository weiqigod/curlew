# Implementation Plan: M5-001

## Overview

Deliver SAML 2.0 SSO for the Curlew backend: owner-only endpoint to store an
org's IdP configuration, an SP-initiated login redirect that builds a signed
AuthnRequest, and an Assertion Consumer Service (ACS) endpoint that verifies a
signed SAMLResponse and issues an Curlew session cookie.

## Task Details

- **ID:** M5-001
- **Title:** Backend: SAML 2.0 SSO auth flow
- **Phase:** M5: Enterprise Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task    | Title                 | Status |
|---------|-----------------------|--------|
| M4-003  | Backend org RBAC      | done   |

## Key Architectural Decisions

1. **No heavyweight SAML library.** The task scope says
   "Sustainsys.Saml2 (or equivalent)". Sustainsys hooks ASP.NET's auth pipeline
   as middleware and is awkward for minimal APIs; it also imposes configuration
   coupling we don't want. Project philosophy favours the standard library.
   We implement SAML XML construction/signing/verification directly using
   `System.Security.Cryptography.Xml.SignedXml` and `System.Xml` (both BCL).
   Everything needed — `<samlp:AuthnRequest>` serialisation, `SAMLRequest` query
   parameter encoding (DEFLATE + base64 + URL encode per HTTP-Redirect binding),
   base64-decoding of a POSTed `SAMLResponse`, validating the embedded
   `ds:Signature`, and extracting the assertion subject — is attainable with
   BCL alone.

2. **Pluggable `ISamlHandler` abstraction.** A real `SamlHandler` runs the BCL
   crypto; a `FakeSamlHandler` registered by `BackendFactory` short-circuits
   crypto for deterministic tests. The fake still enforces the same
   error-path behaviours (signature-invalid, email-not-member) via a
   controllable `Mode` flag on the request payload — so tests exercise
   the real endpoint and service logic without needing real XML signing in
   every test. A separate small suite exercises the real handler against a
   self-signed cert generated with `CertificateRequest` (also BCL).

3. **Storage:** SSO toggles & config live in the existing `organizations.settings`
   JSONB column. A new `sso_credentials` table stores IdP public certificates
   so they don't bloat the settings JSON and can be rotated via kid.

4. **Session cookie.** After a successful ACS we mint a short-lived JWT with
   the existing `JwtOptions` signing key and set it as an HTTP-only cookie
   `curlew_session`, then 302 to `SamlOptions.WebPortalUrl` (default
   `http://localhost:3000/sso/callback`).

5. **Public ACS/login endpoints.** `/api/v1/sso/saml/{orgId}/login` and
   `/api/v1/sso/saml/{orgId}/acs` are registered **outside** the
   authenticated route group — they must work without a bearer token. Only
   `PUT /api/v1/organizations/{id}/sso/saml` requires auth (owner role).

6. **SsoConfig record shape:**
   ```csharp
   public sealed record SsoConfig(
       string IdpMetadataUrl,
       string AcsUrl,
       string EntityId,
       string? IdpSsoUrl = null,
       string? IdpCertPem = null);
   ```
   Stored in `organizations.settings` under `sso_config`.

7. **Migration filename:** `20260418100000_AddSsoCredentials.cs` — adds only
   the new `sso_credentials` table (no alter on `organizations`).

## Implementation Steps

Steps are ordered smallest blast radius first — value types → storage schema
→ service → endpoints → wiring → fixtures/observability.

### Step 1: Domain primitives and request/DTO types

**Rationale:** These are leaf types with no dependencies. Writing them first
lets the service and handler compile incrementally against a stable contract.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/SsoConfig.cs` | create | `record SsoConfig(IdpMetadataUrl, AcsUrl, EntityId, IdpSsoUrl?, IdpCertPem?)` |
| `src/ApiTool.Backend/Sso/OrganizationSettings.cs` | create | Typed view over JSON blob: `SsoEnabled`, `SsoProvider`, `SsoConfig` with (de)serialize helpers |
| `src/ApiTool.Backend/Sso/SamlConfigRequest.cs` | create | PUT body: `record SamlConfigRequest(string? IdpMetadataUrl, string? AcsUrl, string? EntityId, string? IdpSsoUrl, string? IdpCertPem)` |
| `src/ApiTool.Backend/Sso/SsoError.cs` | create | Enum: `None, PermissionDenied, OrgNotFound, InvalidConfig, SignatureInvalid, UserNotMember, SsoNotEnabled, AssertionExpired` |
| `src/ApiTool.Backend/Sso/SsoErrorCodes.cs` | create | `const string` codes: `invalid_sso_config`, `saml_signature_invalid`, `sso_user_not_member`, `permission_denied`, `sso_not_enabled` |

#### New Code

```csharp
// src/ApiTool.Backend/Sso/SsoConfig.cs
namespace ApiTool.Backend.Sso;

public sealed record SsoConfig(
    string IdpMetadataUrl,
    string AcsUrl,
    string EntityId,
    string? IdpSsoUrl = null,
    string? IdpCertPem = null);
```

```csharp
// src/ApiTool.Backend/Sso/OrganizationSettings.cs
using System.Text.Json;

namespace ApiTool.Backend.Sso;

public sealed class OrganizationSettings
{
    public bool SsoEnabled { get; set; }
    public string? SsoProvider { get; set; }   // "saml" | "oidc" | null
    public SsoConfig? SsoConfig { get; set; }
    // other keys preserved opaquely
    public Dictionary<string, JsonElement> Extras { get; set; } = new();

    public static OrganizationSettings FromJson(string json) { /* …*/ }
    public string ToJson() { /* …*/ }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class OrganizationSettingsTests
{
    [Theory]
    [InlineData("{}", false, null)]
    [InlineData("{\"sso_enabled\":true,\"sso_provider\":\"saml\"}", true, "saml")]
    public void FromJson_reads_sso_flags(string json, bool enabled, string? provider) { … }

    [Fact]
    public void ToJson_round_trips_sso_config_and_preserves_unknown_keys() { … }

    [Fact]
    public void ToJson_produces_snake_case_keys() { … }
}
```

#### Impact on Existing Tests

None — all new files.

---

### Step 2: `sso_credentials` entity and EF migration

**Rationale:** Schema change comes next so the service can load certs without
plumbing. No migration impacts existing data.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Data/Entities/SsoCredential.cs` | create | `{ Id, OrgId, Kid, PublicCertPem, CreatedAt }` |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modify | `DbSet<SsoCredential> SsoCredentials`, `OnModelCreating` mapping (`sso_credentials` table, `(OrgId, Kid)` unique index, FK cascade) |
| `src/ApiTool.Backend/Migrations/20260418100000_AddSsoCredentials.cs` | create (scaffold via `dotnet ef migrations add AddSsoCredentials`) | Up/Down for the new table + designer + snapshot update |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modify (generated) | Includes new entity |
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | modify | Add test asserting `sso_credentials` table exists with unique `(org_id, kid)` index |

#### New Code — entity

```csharp
// src/ApiTool.Backend/Data/Entities/SsoCredential.cs
namespace ApiTool.Backend.Data.Entities;

public sealed class SsoCredential
{
    public Guid Id { get; set; }
    public Guid OrgId { get; set; }
    public string Kid { get; set; } = string.Empty;
    public string PublicCertPem { get; set; } = string.Empty;
    public DateTime CreatedAt { get; set; }
}
```

#### New Code — model builder fragment (inside `OnModelCreating`)

```csharp
b.Entity<SsoCredential>(e =>
{
    e.ToTable("sso_credentials");
    e.HasKey(x => x.Id);
    e.Property(x => x.Kid).HasMaxLength(100).IsRequired();
    e.Property(x => x.PublicCertPem).IsRequired();
    e.HasIndex(x => new { x.OrgId, x.Kid }).IsUnique();
    e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
});
```

#### Tests to Write FIRST (RED phase)

```csharp
// AppDbContextSchemaTests.cs — add method
[Fact]
public async Task SsoCredentials_has_table_and_unique_org_kid_index()
{
    await using var scope = TestDb.CreateOpen();
    await scope.Db.Database.EnsureCreatedAsync();
    // Insert one credential, insert duplicate (same OrgId, Kid) — should throw.
}
```

#### Impact on Existing Tests

- `AppDbContextSchemaTests` adds a new test (no existing failure).
- The model snapshot regeneration affects a single auto-generated file — no
  hand-written tests break.

---

### Step 3: `ISamlHandler` abstraction and test fake

**Rationale:** The service layer depends on the handler; registering a
deterministic fake early lets us drive the whole endpoint+service TDD loop
without real XML crypto in every test.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/ISamlHandler.cs` | create | Abstraction |
| `src/ApiTool.Backend/Sso/SamlAuthnRequest.cs` | create | Result record for build step |
| `src/ApiTool.Backend/Sso/SamlAssertion.cs` | create | Result record for validate step |
| `src/ApiTool.Backend/Sso/SamlHandler.cs` | create | Production BCL implementation |
| `src/ApiTool.Backend/Sso/SamlOptions.cs` | create | Config options (`WebPortalUrl`, `SessionCookieName`, `SessionTtl`) |
| `src/ApiTool.Backend.Tests/Sso/FakeSamlHandler.cs` | create | In-memory deterministic fake |

#### New Code — interface

```csharp
// src/ApiTool.Backend/Sso/ISamlHandler.cs
namespace ApiTool.Backend.Sso;

public interface ISamlHandler
{
    /// <summary>Builds an HTTP-Redirect-binding AuthnRequest.</summary>
    /// <returns>
    /// A tuple with (redirectUrl, relayState). redirectUrl includes
    /// SAMLRequest query parameter (DEFLATE+base64+URL-encoded).
    /// </returns>
    SamlAuthnRequest BuildAuthnRequest(SsoConfig config, Guid orgId, string relayState);

    /// <summary>Validates a base64-encoded SAMLResponse POSTed to the ACS.</summary>
    /// <returns>
    /// assertion with Email, NameId on success; or error code otherwise.
    /// Signature validation uses the IdP cert stored against the org.
    /// </returns>
    SamlValidationResult ValidateResponse(SsoConfig config, string samlResponseBase64, string idpCertPem, DateTime nowUtc);
}

public sealed record SamlAuthnRequest(string RedirectUrl, string RequestId);

public sealed record SamlValidationResult(
    bool Success,
    string? ErrorCode,
    string? Email,
    string? NameId);
```

#### Tests to Write FIRST (RED phase)

```csharp
// SamlHandlerTests.cs — exercises the real handler against a self-signed cert
public sealed class SamlHandlerTests
{
    [Fact]
    public void BuildAuthnRequest_returns_url_with_SAMLRequest_query_param() { … }

    [Fact]
    public void BuildAuthnRequest_deflates_base64_encodes_per_http_redirect_binding() { … }

    [Fact]
    public void ValidateResponse_accepts_signed_assertion_from_trusted_cert()
    {
        // Generate self-signed cert via CertificateRequest, build a signed
        // SAMLResponse XML, base64-encode, then validate.
    }

    [Fact]
    public void ValidateResponse_rejects_tampered_signature() { … }

    [Fact]
    public void ValidateResponse_rejects_expired_assertion() { … }

    [Fact]
    public void ValidateResponse_returns_email_from_NameID_or_attribute() { … }
}
```

#### Impact on Existing Tests

None — all new files.

---

### Step 4: `SsoService`

**Rationale:** Service sits above the handler and below the endpoints — TDD
it with unit tests driving the fake handler, using direct DB contexts (same
pattern as `OrganizationService`).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/SsoService.cs` | create | Business logic (RBAC, persist, build-login, validate-acs, mint-session) |
| `src/ApiTool.Backend/Auth/SessionTokenIssuer.cs` | create | Mints a short-lived JWT session cookie value |
| `src/ApiTool.Backend.Tests/Sso/SsoServiceTests.cs` | create | Unit tests using in-memory SQLite + FakeSamlHandler |

#### Proposed Signatures

```csharp
public sealed class SsoService(AppDbContext db, ISamlHandler saml, SessionTokenIssuer sessions, TimeProvider clock, IOptions<SamlOptions> options)
{
    public Task<(OrganizationDto? dto, SsoError err, string? field, string? message)>
        UpsertSamlConfigAsync(Guid userId, Guid orgId, SamlConfigRequest req, CancellationToken ct);

    public Task<(string? redirectUrl, SsoError err)>
        BuildLoginAsync(Guid orgId, CancellationToken ct);

    public Task<(string? sessionCookie, string? redirectUrl, SsoError err, string? message)>
        ConsumeAssertionAsync(Guid orgId, string samlResponseBase64, CancellationToken ct);
}
```

#### New Code — `SessionTokenIssuer`

```csharp
public sealed class SessionTokenIssuer(IOptions<JwtOptions> jwtOptions, TimeProvider clock)
{
    public string IssueForUser(Guid userId, string email, TimeSpan ttl) { /* HS256 JWT */ }
}
```

#### Tests to Write FIRST (RED phase)

```csharp
public sealed class SsoServiceTests
{
    // UpsertSamlConfigAsync
    [Theory]
    [InlineData(null, "acs", "entity", "idp_metadata_url")]       // missing metadata_url
    [InlineData("meta", null, "entity", "acs_url")]               // missing acs_url
    [InlineData("meta", "acs", null, "entity_id")]                // missing entity_id
    public async Task Upsert_returns_invalid_config_for_missing_field(string? meta, string? acs, string? entity, string missingField) { … }

    [Fact]
    public async Task Upsert_as_owner_persists_config_and_sets_sso_enabled_true() { … }

    [Fact]
    public async Task Upsert_as_non_owner_returns_permission_denied() { … }

    [Fact]
    public async Task Upsert_stores_idp_cert_in_sso_credentials_when_pem_given() { … }

    // BuildLoginAsync
    [Fact]
    public async Task Build_login_returns_redirect_url_with_saml_request_query() { … }

    [Fact]
    public async Task Build_login_returns_sso_not_enabled_when_not_configured() { … }

    // ConsumeAssertionAsync
    [Fact]
    public async Task Consume_issues_session_cookie_when_email_matches_member() { … }

    [Fact]
    public async Task Consume_returns_signature_invalid_from_handler_failure() { … }

    [Fact]
    public async Task Consume_returns_user_not_member_when_email_not_in_org() { … }

    [Fact]
    public async Task Consume_writes_audit_log_entry_on_success() { … }

    [Fact]
    public async Task Consume_writes_audit_log_entry_on_failure() { … }
}
```

#### Impact on Existing Tests

None — new files; no existing code depends on these types.

---

### Step 5: SAML endpoints

**Rationale:** HTTP layer last — mechanical translation of service results to
status codes/responses.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Sso/SamlEndpoints.cs` | create | `MapSamlEndpoints` with 3 routes |
| `src/ApiTool.Backend/Program.cs` | modify | Register `ISamlHandler`, `SamlHandler`, `SessionTokenIssuer`, `SsoService`; configure `SamlOptions`; call `app.MapSamlEndpoints()`; no auth on login/acs subroutes |
| `src/ApiTool.Backend.Tests/Sso/SamlEndpointsTests.cs` | create | HTTP integration tests matching the 8 behaviors |
| `src/ApiTool.Backend.Tests/Sso/SwaggerSamlSurfaceTests.cs` | create | Swagger asserts the 3 endpoints are listed |
| `src/ApiTool.Backend.Tests/TestInfrastructure/BackendFactory.cs` | modify | Register `FakeSamlHandler` (replace `SamlHandler`) and `SamlOptions` test values |

#### Endpoint Layout

```csharp
// src/ApiTool.Backend/Sso/SamlEndpoints.cs
public static IEndpointRouteBuilder MapSamlEndpoints(this IEndpointRouteBuilder app)
{
    // 1) PUT config — auth required, owner only
    var authed = app.MapGroup("/api/v1/organizations/{id}/sso").RequireAuthorization().WithTags("SSO");
    authed.MapPut("saml", UpsertSamlConfig)
          .Accepts<SamlConfigRequest>("application/json")
          .Produces<OrganizationDto>()
          .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
          .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
          .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

    // 2) GET login — public
    var pub = app.MapGroup("/api/v1/sso/saml").WithTags("SSO");
    pub.MapGet("{orgId}/login", LoginRedirect)
       .Produces(StatusCodes.Status302Found)
       .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

    // 3) POST acs — public, form-urlencoded SAMLResponse
    pub.MapPost("{orgId}/acs", AcsCallback)
       .Produces(StatusCodes.Status302Found)
       .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
       .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

    return app;
}
```

#### Tests to Write FIRST (RED phase) — behaviours 1–8

```csharp
public sealed class SamlEndpointsTests
{
    // B1
    [Fact] public async Task Put_saml_config_returns_200_and_persists_sso_enabled_true() { … }
    // B2
    [Fact] public async Task Put_saml_config_missing_idp_metadata_url_returns_400_invalid_sso_config_with_field_pointer() { … }
    // B3
    [Fact] public async Task Get_login_returns_302_with_signed_SAMLRequest_query_param() { … }
    // B4
    [Fact] public async Task Post_acs_with_valid_response_returns_302_with_curlew_session_cookie() { … }
    // B5
    [Fact] public async Task Post_acs_with_invalid_signature_returns_401_saml_signature_invalid_and_no_cookie() { … }
    // B6
    [Fact] public async Task Post_acs_with_unknown_email_returns_403_sso_user_not_member() { … }
    // B7
    [Fact] public async Task Put_saml_config_as_non_owner_returns_403_permission_denied() { … }
}

public sealed class SwaggerSamlSurfaceTests
{
    // B8
    [Fact] public async Task Swagger_lists_saml_endpoints()
    {
        paths.TryGetProperty("/api/v1/organizations/{id}/sso/saml", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/sso/saml/{orgId}/login", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/sso/saml/{orgId}/acs", out _).Should().BeTrue();
    }
}
```

#### Impact on Existing Tests

- `BackendFactory` swaps in `FakeSamlHandler`; no existing test uses
  `ISamlHandler`, so no breakage.
- `Program.cs` adds registrations; no existing DI graph affected.

---

### Step 6: Fixtures and observable

**Rationale:** Last mile so the task's observable block runs.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/backend/sso/saml-config.json` | create | Sample config matching the observable curl |
| `src/ApiTool.Backend.Tests/Sso/TestData/test-idp-cert.pem` | create | Self-signed cert (deterministic or regenerated at test-fixture time) |
| `src/ApiTool.Backend.Tests/Sso/TestData/test-idp-key.pem` | create | Matching RSA key (marked with `<Content CopyToOutputDirectory="PreserveNewest">` or generated in code) |
| `src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | modify | Add `<Content>` include for `testdata/backend/sso/saml-config.json` |
| `management/backlog.yaml` | modify (Step 7) | Mark task planned |

#### `testdata/backend/sso/saml-config.json` (draft content)

```json
{
  "idp_metadata_url": "https://idp.example.com/metadata",
  "acs_url": "http://localhost:5000/api/v1/sso/saml/{{ORG_ID}}/acs",
  "entity_id": "http://localhost:5000",
  "idp_sso_url": "https://idp.example.com/sso",
  "idp_cert_pem": "-----BEGIN CERTIFICATE-----\nMII...\n-----END CERTIFICATE-----"
}
```

Note: the observable uses `--data-binary @…saml-config.json` — the ACS URL
does **not** need to include the real org id literal; the endpoint only
validates that the three required fields are present. The fixture can use a
placeholder ACS URL because the PUT path supplies the org id.

#### Decision log (open questions resolved)

- **Q:** Should the fixture embed a placeholder `{{ORG_ID}}` or a dummy UUID?
  **A:** Dummy fixed UUID (`00000000-0000-0000-0000-000000000000`) — the
  endpoint doesn't need the ACS URL to match the caller; the spec only
  validates presence.
- **Q:** Should test-idp-cert.pem be checked in or generated?
  **A:** Generated at test-fixture time via `CertificateRequest` — keeps the
  repo free of binary/PEM blobs and avoids cert expiry rot.
- **Q:** Should `sso-login`/`sso-acs` have rate limits?
  **A:** Yes — add `sso-login` (60/min per org) and `sso-acs` (60/min per org)
  to the non-testing rate-limiter config in Program.cs. Testing path adds
  no-op entries (same pattern as the existing six policies).

## Test Impact Summary

| Test File                                             | Test Function | Impact | Action Required |
|-------------------------------------------------------|---------------|--------|-----------------|
| `AppDbContextSchemaTests.cs`                          | existing      | none   | add new test for `sso_credentials` (additive) |
| `JwtAuthenticationTests.cs`                           | all           | none   | — |
| All `NotificationsEndpointsTests`, `SubscriptionsEndpointsTests`, etc. | all | none | — |
| _new_ `OrganizationSettingsTests`                     | new           | —      | write in Step 1 |
| _new_ `SamlHandlerTests`                              | new           | —      | write in Step 3 |
| _new_ `SsoServiceTests`                               | new           | —      | write in Step 4 |
| _new_ `SamlEndpointsTests`                            | new           | —      | write in Step 5 |
| _new_ `SwaggerSamlSurfaceTests`                       | new           | —      | write in Step 5 |

## Risks and Edge Cases

- **Risk:** Implementing SAML XML signing from BCL is subtle (canonicalisation,
  exclusive C14N, reference digests). → **Mitigation:** scope to HTTP-Redirect
  binding for login (sign query string) and HTTP-POST binding for assertion
  (`SignedXml.CheckSignature` with `KeyInfo` resolved to IdP cert). Start
  with assertion validation tests green against a locally-signed
  `SAMLResponse` (via a small test helper), then implement.
- **Risk:** `SignedXml` vulnerabilities (XXE, signature-wrapping). →
  **Mitigation:** `XmlDocument.XmlResolver = null`, `PreserveWhitespace = true`,
  require the `Signature` element to reference the root `Response`/`Assertion`
  element's `Id`, and enforce the cert from our trusted `SsoCredential` row
  (never from the document itself).
- **Risk:** Public ACS endpoint is attacker-reachable. → **Mitigation:**
  RateLimit partition by orgId (60/min per policy), InResponseTo validation
  against pending AuthnRequests, NotOnOrAfter check, audit-log every request.
- **Risk:** `BackendFactory` test host doesn't set cookies properly with
  in-memory TestServer → **Mitigation:** Use `Set-Cookie` header assertion in
  the `HttpResponseMessage.Headers`, not the cookie container.
- **Risk:** Minimal-API endpoint groups order matters for auth — if `sso/saml`
  is accidentally nested under the authed organizations group, public routes
  will 401. → **Mitigation:** use two top-level `MapGroup`s with explicit
  `RequireAuthorization()` only on the PUT group.
- **Edge case:** User exists in DB but is not a member of the target org. →
  **Handling:** `sso_user_not_member` (403). Do not auto-create membership.
- **Edge case:** Multiple `SsoCredential` rows per org (cert rotation). →
  **Handling:** pick newest by `CreatedAt`; schema already supports multiple.
- **Edge case:** Upsert called twice. → **Handling:** Idempotent —
  deserialize, overwrite keys, serialize back; replace cert row by `(OrgId, Kid)`
  upsert where Kid defaults to SHA256 of cert bytes.
- **Edge case:** `sso_enabled=false` attempt to hit `/login`. → **Handling:**
  return 404 (do not leak org existence).

## Verification

```bash
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj -warnaserror
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Sso"
```

Observable verification (task YAML):

```bash
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Sso&FullyQualifiedName~Saml"
# Expected: Passed: >=10, Failed: 0

dotnet run --project src/ApiTool.Backend &
sleep 2
TOKEN=$(./scripts/test-token.sh owner@example.com)
ORG=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  http://localhost:5000/api/v1/organizations | jq -r '.organizations[0].id')
curl -sS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @testdata/backend/sso/saml-config.json \
  "http://localhost:5000/api/v1/organizations/$ORG/sso/saml"
# Expected HTTP 200, body contains "sso_enabled":true,"sso_provider":"saml"
curl -sSL -D - "http://localhost:5000/api/v1/sso/saml/$ORG/login" | head -20
# Expected HTTP 302 with Location header to IdP SSO URL and SAMLRequest query param
```

## Quality Checklist

- [x] Every file to be modified has been fully read (not skimmed)
- [x] Every affected `_test.go`/`*Tests.cs` file has been read
- [x] All call sites of changed interfaces identified
- [x] Before/after code snippets for non-trivial changes
- [x] Impact on existing tests explicitly listed
- [x] Edge cases and risks identified with mitigations
- [x] Tests specified BEFORE implementation (TDD)
- [x] Signatures include `CancellationToken` where appropriate
- [x] Error wrapping uses structured errors (`SsoError` enum + `ErrorResponse` body)
- [x] No stuttering in names (`Sso.SsoService` is conventional for C# since namespace + type distinguishes; kept to match project pattern of `Organizations.OrganizationService`, `Notifications.NotificationsService`, etc.)
- [x] Table-driven / parametrised test cases named upfront
- [x] Steps ordered by blast radius (leaf types → DB → handler → service → endpoints)
- [x] Observable verification command is concrete and runnable
- [x] Plan file matches the template structure
- [ ] Plan committed to the feature branch (Step 5)
- [ ] Backlog status updated to `planned` (Step 6)
