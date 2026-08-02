using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>HTTP integration tests for the three SAML SSO endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class SamlEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private readonly string _ownerToken;

    public SamlEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var ownerEmail = $"sso-owner-{_ownerId:N}@example.com";
        _ownerToken = TestTokens.Create(_ownerId, ownerEmail);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", _ownerToken);
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    // ── Helpers ───────────────────────────────────────────────────────────────

    private async Task<string> CreateOrgAndReturnGuidAsync()
    {
        var slug = $"sso-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "SsoOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        // id is in "org_<32hex>" format — extract the 32-hex suffix (no dashes)
        var wireId = doc.RootElement.GetProperty("id").GetString()!;
        // wireId looks like org_<32hexchars>; strip prefix, return raw 32-hex UUID
        var hex = wireId["org_".Length..];  // 32-char N-format GUID hex
        // M15-002: SSO requires Enterprise subscription. Seed it so existing
        // tests continue to exercise the happy path.
        await _factory.SeedSubscriptionAsync(Guid.Parse(hex), SubscriptionTier.Enterprise);
        return hex;
    }

    private async Task<string> CreateOrgWithTierAsync(SubscriptionTier? tier)
    {
        var slug = $"sso-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "SsoOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var hex = doc.RootElement.GetProperty("id").GetString()!["org_".Length..];
        if (tier is not null)
            await _factory.SeedSubscriptionAsync(Guid.Parse(hex), tier.Value);
        return hex;
    }

    private static object ValidSamlConfig() => new
    {
        idp_metadata_url = "https://idp.example.com/metadata",
        acs_url = "https://sp.example.com/acs",
        entity_id = "https://sp.example.com",
        idp_sso_url = "https://idp.example.com/sso",
    };

    // ── B1: PUT /organizations/{id}/sso/saml — success ────────────────────────

    [Fact]
    public async Task Put_saml_config_returns_200_and_persists_sso_enabled_true()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();

        var response = await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid:N}/sso/saml", ValidSamlConfig());

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("sso_enabled").GetBoolean().Should().BeTrue();
        doc.RootElement.GetProperty("sso_provider").GetString().Should().Be("saml");
    }

    // ── B2: PUT — missing field ───────────────────────────────────────────────

    [Fact]
    public async Task Put_saml_config_missing_idp_metadata_url_returns_400_invalid_sso_config_with_field_pointer()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();

        var body = new { acs_url = "https://sp.example.com/acs", entity_id = "https://sp.example.com" };
        var response = await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid:N}/sso/saml", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_sso_config");
        doc.RootElement.GetProperty("field").GetString().Should().Be("idp_metadata_url");
    }

    // ── B3: GET /sso/saml/{orgId}/login — public, returns 302 ────────────────

    [Fact]
    public async Task Get_login_returns_302_with_SAMLRequest_query_param()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();

        // Configure SSO first (requires auth)
        await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid:N}/sso/saml", ValidSamlConfig());

        // Login is public — use anon client
        var anonClient = _factory.CreateClient(new Microsoft.AspNetCore.Mvc.Testing.WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        var response = await anonClient.GetAsync($"/api/v1/sso/saml/{orgGuid}/login");

        response.StatusCode.Should().Be(HttpStatusCode.Redirect);
        response.Headers.Location?.ToString().Should().Contain("SAMLRequest=");
    }

    // ── B4: POST /sso/saml/{orgId}/acs — valid response, cookie issued ────────

    [Fact]
    public async Task Post_acs_with_valid_response_returns_302_with_apitool_session_cookie()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();
        await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid:N}/sso/saml", ValidSamlConfig());

        var anonClient = _factory.CreateClient(new Microsoft.AspNetCore.Mvc.Testing.WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        // FakeSamlHandler returns success by default with "sso-user@example.com"
        // but that user must exist and be a member — seed them via the BackendFactory's EF context
        // The easiest approach: ensure the user is the owner (email set in ctor)
        // However the fake handler returns "sso-user@example.com" not the owner's email.
        // We need to seed a member with the fake handler's email.
        // The factory-level FakeSamlHandler SuccessEmail is "sso-user@example.com".
        // We need to add that user via the same db used by the test host.
        // Use a fresh scope from the factory.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var ssoUserId = Guid.NewGuid();
        var ssoUserEmail = "sso-user@example.com";  // matches FakeSamlHandler default
        db.Users.Add(new ApiTool.Backend.Data.Entities.User
        {
            Id = ssoUserId,
            Email = ssoUserEmail,
            CreatedAt = DateTime.UtcNow,
        });
        // Make them a member of the org
        if (!await db.OrganizationMembers.AnyAsync(m =>
            m.OrgId == Guid.Parse(orgGuid) && m.UserId == ssoUserId))
        {
            db.OrganizationMembers.Add(new ApiTool.Backend.Data.Entities.OrganizationMember
            {
                OrgId = Guid.Parse(orgGuid),
                UserId = ssoUserId,
                Role = ApiTool.Backend.Data.Entities.OrgRole.Member,
                JoinedAt = DateTime.UtcNow,
            });
        }
        await db.SaveChangesAsync();

        var formContent = new FormUrlEncodedContent(new[]
        {
            new KeyValuePair<string, string>("SAMLResponse", Convert.ToBase64String("fake-saml-response"u8.ToArray())),
        });
        var response = await anonClient.PostAsync($"/api/v1/sso/saml/{orgGuid}/acs", formContent);

        response.StatusCode.Should().Be(HttpStatusCode.Redirect);
        var setCookie = response.Headers.TryGetValues("Set-Cookie", out var cookies)
            ? cookies.FirstOrDefault()
            : null;
        setCookie.Should().NotBeNull(because: "a session cookie should be set on success");
        setCookie.Should().Contain("apitool_session");
    }

    // ── B5: POST /acs — invalid signature ────────────────────────────────────

    [Fact]
    public async Task Post_acs_with_invalid_signature_returns_401_saml_signature_invalid_and_no_cookie()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();
        await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid:N}/sso/saml", ValidSamlConfig());

        // Set fake handler to return signature-invalid
        var fakeSamlHandler = _factory.GetFakeSamlHandler();
        fakeSamlHandler!.ValidationMode = FakeValidationMode.SignatureInvalid;

        try
        {
            var anonClient = _factory.CreateClient();
            var formContent = new FormUrlEncodedContent(new[]
            {
                new KeyValuePair<string, string>("SAMLResponse", Convert.ToBase64String("bad"u8.ToArray())),
            });
            var response = await anonClient.PostAsync($"/api/v1/sso/saml/{orgGuid}/acs", formContent);

            response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
            var json = await response.Content.ReadAsStringAsync();
            using var doc = JsonDocument.Parse(json);
            doc.RootElement.GetProperty("code").GetString().Should().Be("saml_signature_invalid");

            response.Headers.Contains("Set-Cookie").Should().BeFalse(
                because: "no session cookie should be set on auth failure");
        }
        finally
        {
            fakeSamlHandler.ValidationMode = FakeValidationMode.Success;
        }
    }

    // ── B6: POST /acs — email not member ─────────────────────────────────────

    [Fact]
    public async Task Post_acs_with_unknown_email_returns_403_sso_user_not_member()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();
        await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid:N}/sso/saml", ValidSamlConfig());

        var fakeSamlHandler = _factory.GetFakeSamlHandler();
        fakeSamlHandler!.ValidationMode = FakeValidationMode.Success;
        var original = fakeSamlHandler.SuccessEmail;
        fakeSamlHandler.SuccessEmail = "nobody@external.com";  // not a member

        try
        {
            var anonClient = _factory.CreateClient();
            var formContent = new FormUrlEncodedContent(new[]
            {
                new KeyValuePair<string, string>("SAMLResponse", Convert.ToBase64String("saml"u8.ToArray())),
            });
            var response = await anonClient.PostAsync($"/api/v1/sso/saml/{orgGuid}/acs", formContent);

            response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
            var json = await response.Content.ReadAsStringAsync();
            using var doc = JsonDocument.Parse(json);
            doc.RootElement.GetProperty("code").GetString().Should().Be("sso_user_not_member");
        }
        finally
        {
            fakeSamlHandler.SuccessEmail = original;
        }
    }

    // ── B7: PUT — non-owner returns 403 ──────────────────────────────────────

    [Fact]
    public async Task Put_saml_config_as_non_owner_returns_403_permission_denied()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();

        // Create a member user and get their token
        var (memberToken, _) = TestTokens.CreateNew($"sso-member-{Guid.NewGuid():N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var response = await memberClient.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid:N}/sso/saml", ValidSamlConfig());

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("permission_denied");
    }

    // ── Finding #3: missing error-branch endpoint tests ───────────────────────

    [Fact]
    public async Task Put_saml_config_with_non_org_prefixed_id_returns_404()
    {
        // UpsertSamlConfig: invalid org ID format (no "org_" prefix) should return 404
        var response = await _client.PutAsJsonAsync(
            "/api/v1/organizations/not-an-org-id/sso/saml", ValidSamlConfig());

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("organization_not_found");
    }

    [Fact]
    public async Task Get_login_with_unknown_org_guid_returns_404()
    {
        // LoginRedirect: org that doesn't exist returns 404
        var unknownOrgId = Guid.NewGuid();
        var anonClient = _factory.CreateClient(new Microsoft.AspNetCore.Mvc.Testing.WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        var response = await anonClient.GetAsync($"/api/v1/sso/saml/{unknownOrgId}/login");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("organization_not_found");
    }

    [Fact]
    public async Task Post_acs_with_sso_not_enabled_returns_404()
    {
        // AcsCallback: org exists but SSO is not configured — returns 404
        var orgGuid = await CreateOrgAndReturnGuidAsync();
        // Note: SSO is NOT configured for this org

        var anonClient = _factory.CreateClient();
        var formContent = new FormUrlEncodedContent(new[]
        {
            new KeyValuePair<string, string>("SAMLResponse", Convert.ToBase64String("fake"u8.ToArray())),
        });
        var response = await anonClient.PostAsync($"/api/v1/sso/saml/{orgGuid}/acs", formContent);

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("sso_not_enabled");
    }

    [Fact]
    public async Task Post_acs_with_unknown_org_guid_returns_404()
    {
        // AcsCallback: org doesn't exist at all — returns 404
        var unknownOrgId = Guid.NewGuid();
        var anonClient = _factory.CreateClient();
        var formContent = new FormUrlEncodedContent(new[]
        {
            new KeyValuePair<string, string>("SAMLResponse", Convert.ToBase64String("fake"u8.ToArray())),
        });
        var response = await anonClient.PostAsync($"/api/v1/sso/saml/{unknownOrgId}/acs", formContent);

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("organization_not_found");
    }

    // ── GET /organizations/{id}/sso — read SSO config ─────────────────────────

    [Fact]
    public async Task Get_sso_config_as_owner_returns_200_with_sso_enabled_false_when_not_configured()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();

        var response = await _client.GetAsync($"/api/v1/organizations/org_{orgGuid}/sso");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("sso_enabled").GetBoolean().Should().BeFalse();
        // sso_provider is null/absent when SSO is not configured
        if (doc.RootElement.TryGetProperty("sso_provider", out var providerProp))
            providerProp.ValueKind.Should().Be(JsonValueKind.Null);
    }

    [Fact]
    public async Task Get_sso_config_after_saml_upsert_returns_saml_config_without_cert()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();
        await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid}/sso/saml", ValidSamlConfig());

        var response = await _client.GetAsync($"/api/v1/organizations/org_{orgGuid}/sso");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("sso_enabled").GetBoolean().Should().BeTrue();
        doc.RootElement.GetProperty("sso_provider").GetString().Should().Be("saml");
        var samlConfig = doc.RootElement.GetProperty("saml_config");
        samlConfig.GetProperty("idp_metadata_url").GetString()
            .Should().Be("https://idp.example.com/metadata");
        // No cert field exposed
        samlConfig.TryGetProperty("idp_cert_pem", out _).Should().BeFalse();
    }

    [Fact]
    public async Task Get_sso_config_as_non_owner_returns_403()
    {
        var orgGuid = await CreateOrgAndReturnGuidAsync();

        var (memberToken, _) = TestTokens.CreateNew($"sso-member-get-{Guid.NewGuid():N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var response = await memberClient.GetAsync($"/api/v1/organizations/org_{orgGuid}/sso");

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    [Fact]
    public async Task Get_sso_config_without_auth_returns_401()
    {
        var anonClient = _factory.CreateClient();
        var response = await anonClient.GetAsync("/api/v1/organizations/org_00000000000000000000000000000001/sso");

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Get_sso_config_with_invalid_org_id_format_returns_404()
    {
        var response = await _client.GetAsync("/api/v1/organizations/not-an-org-id/sso");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task Get_sso_config_with_unknown_org_guid_returns_404_organization_not_found()
    {
        // M15-002 edge case: a valid-format but non-existent org must return 404 with
        // code=organization_not_found, not 402. Validates that SsoTierGate.EnsureEnterpriseAsync
        // checks org existence before the subscription row, preventing information leakage.
        var unknownOrgGuid = Guid.NewGuid().ToString("N");

        var response = await _client.GetAsync($"/api/v1/organizations/org_{unknownOrgGuid}/sso");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("organization_not_found");
    }

    // ── M15-002 tier-matrix endpoint tests ────────────────────────────────────

    public static IEnumerable<object?[]> NonEnterpriseTiers => new List<object?[]>
    {
        new object?[] { null },
        new object?[] { SubscriptionTier.Free },
        new object?[] { SubscriptionTier.Professional },
        new object?[] { SubscriptionTier.Team },
    };

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Get_sso_config_non_enterprise_returns_402(SubscriptionTier? tier)
    {
        var orgGuid = await CreateOrgWithTierAsync(tier);

        var response = await _client.GetAsync($"/api/v1/organizations/org_{orgGuid}/sso");

        ((int)response.StatusCode).Should().Be(402);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("sso_tier_ineligible");
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Put_saml_config_non_enterprise_returns_402(SubscriptionTier? tier)
    {
        var orgGuid = await CreateOrgWithTierAsync(tier);

        var response = await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid}/sso/saml", ValidSamlConfig());

        ((int)response.StatusCode).Should().Be(402);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("sso_tier_ineligible");
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Saml_login_non_enterprise_returns_404_no_store_empty_body(SubscriptionTier? tier)
    {
        var orgGuid = await CreateOrgWithTierAsync(tier);
        var anon = _factory.CreateClient(new WebApplicationFactoryClientOptions { AllowAutoRedirect = false });

        var response = await anon.GetAsync($"/api/v1/sso/saml/{orgGuid}/login");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        response.Headers.CacheControl?.NoStore.Should().BeTrue();
        var body = await response.Content.ReadAsStringAsync();
        body.Should().BeEmpty();
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Saml_acs_non_enterprise_returns_404_no_store_empty_body(SubscriptionTier? tier)
    {
        var orgGuid = await CreateOrgWithTierAsync(tier);
        var anon = _factory.CreateClient();
        var formContent = new FormUrlEncodedContent(new[]
        {
            new KeyValuePair<string, string>("SAMLResponse", Convert.ToBase64String("x"u8.ToArray())),
        });

        var response = await anon.PostAsync($"/api/v1/sso/saml/{orgGuid}/acs", formContent);

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        response.Headers.CacheControl?.NoStore.Should().BeTrue();
        var body = await response.Content.ReadAsStringAsync();
        body.Should().BeEmpty();
    }
}
