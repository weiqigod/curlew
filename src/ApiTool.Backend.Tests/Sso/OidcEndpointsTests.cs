using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>HTTP integration tests for the three OIDC SSO endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class OidcEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;

    public OidcEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var ownerEmail = $"oidc-owner-{_ownerId:N}@example.com";
        var ownerToken = TestTokens.Create(_ownerId, ownerEmail);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", ownerToken);
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    // ── Helpers ───────────────────────────────────────────────────────────────

    private async Task<string> CreateOrgAsync()
    {
        var slug = $"oidc-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "OidcOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var wireId = doc.RootElement.GetProperty("id").GetString()!;
        var hex = wireId["org_".Length..];  // 32-char hex Guid
        // M15-002: SSO requires Enterprise subscription. Seed it so existing
        // tests continue to exercise the happy path.
        await _factory.SeedSubscriptionAsync(Guid.Parse(hex), SubscriptionTier.Enterprise);
        return hex;
    }

    private async Task<string> CreateOrgWithTierAsync(SubscriptionTier? tier)
    {
        var slug = $"oidc-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "OidcOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var hex = doc.RootElement.GetProperty("id").GetString()!["org_".Length..];
        if (tier is not null)
            await _factory.SeedSubscriptionAsync(Guid.Parse(hex), tier.Value);
        return hex;
    }

    private static object ValidOidcConfig() => new
    {
        issuer_url = "https://idp.example.com",
        client_id = "apitool-client",
        client_secret = "s3cret",
        redirect_uri = "http://localhost:5000/api/v1/sso/oidc/00000000/callback",
    };

    // Seed an org member matching FakeOidcHandler.SuccessEmail so the callback test can log in.
    private async Task SeedOidcUserAsync(Guid orgGuid)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var oidcEmail = _factory.GetFakeOidcHandler()?.SuccessEmail ?? "oidc-user@example.com";

        // Ensure user exists
        var existingUser = await db.Users.FirstOrDefaultAsync(u => u.Email == oidcEmail);
        if (existingUser is null)
        {
            existingUser = new ApiTool.Backend.Data.Entities.User
            {
                Id = Guid.NewGuid(),
                Email = oidcEmail,
                CreatedAt = DateTime.UtcNow,
            };
            db.Users.Add(existingUser);
            await db.SaveChangesAsync();
        }

        // Ensure member
        if (!await db.OrganizationMembers.AnyAsync(m => m.OrgId == orgGuid && m.UserId == existingUser.Id))
        {
            db.OrganizationMembers.Add(new ApiTool.Backend.Data.Entities.OrganizationMember
            {
                OrgId = orgGuid,
                UserId = existingUser.Id,
                Role = ApiTool.Backend.Data.Entities.OrgRole.Member,
                JoinedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }
    }

    // ── B1: PUT /organizations/{id}/sso/oidc — success ───────────────────────

    [Fact]
    public async Task Put_oidc_config_returns_200_and_persists_sso_enabled_true_provider_oidc()
    {
        var orgGuid = await CreateOrgAsync();

        var response = await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("sso_enabled").GetBoolean().Should().BeTrue();
        doc.RootElement.GetProperty("sso_provider").GetString().Should().Be("oidc");
    }

    // ── PUT — missing field ───────────────────────────────────────────────────

    [Fact]
    public async Task Put_oidc_config_missing_issuer_url_returns_400_invalid_sso_config()
    {
        var orgGuid = await CreateOrgAsync();
        var body = new { client_id = "cid", client_secret = "s3cret" };

        var response = await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid}/sso/oidc", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_sso_config");
        doc.RootElement.GetProperty("field").GetString().Should().Be("issuer_url");
    }

    // ── B2: PUT — discovery fails ─────────────────────────────────────────────

    [Fact]
    public async Task Put_oidc_config_with_unreachable_issuer_returns_400_oidc_discovery_failed()
    {
        var orgGuid = await CreateOrgAsync();
        var fakeHandler = _factory.GetFakeOidcHandler();
        fakeHandler!.ValidationMode = FakeOidcValidationMode.DiscoveryFailed;

        try
        {
            var response = await _client.PutAsJsonAsync(
                $"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());

            response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
            var json = await response.Content.ReadAsStringAsync();
            using var doc = JsonDocument.Parse(json);
            doc.RootElement.GetProperty("code").GetString().Should().Be("oidc_discovery_failed");
        }
        finally
        {
            fakeHandler.ValidationMode = FakeOidcValidationMode.Success;
        }
    }

    // ── B7: PUT — non-owner returns 403 ──────────────────────────────────────

    [Fact]
    public async Task Put_oidc_config_as_non_owner_returns_403_permission_denied()
    {
        var orgGuid = await CreateOrgAsync();
        var (memberToken, _) = TestTokens.CreateNew($"oidc-member-{Guid.NewGuid():N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var response = await memberClient.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("permission_denied");
    }

    [Fact]
    public async Task Put_oidc_config_with_non_org_prefixed_id_returns_404()
    {
        var response = await _client.PutAsJsonAsync(
            "/api/v1/organizations/not-an-org-id/sso/oidc", ValidOidcConfig());

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── B3: GET /sso/oidc/{orgId}/login — redirects to IdP ───────────────────

    [Fact]
    public async Task Get_login_returns_302_with_authorize_url_containing_client_id_and_state()
    {
        var orgGuid = await CreateOrgAsync();
        await _client.PutAsJsonAsync($"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());

        var anonClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        var response = await anonClient.GetAsync($"/api/v1/sso/oidc/{orgGuid}/login");

        response.StatusCode.Should().Be(HttpStatusCode.Redirect);
        var location = response.Headers.Location?.ToString();
        location.Should().NotBeNullOrWhiteSpace();
        location!.Should().Contain("client_id=");
        location.Should().Contain("state=");
    }

    [Fact]
    public async Task Get_login_sets_state_nonce_pkce_cookies()
    {
        var orgGuid = await CreateOrgAsync();
        await _client.PutAsJsonAsync($"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());

        var anonClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        var response = await anonClient.GetAsync($"/api/v1/sso/oidc/{orgGuid}/login");

        response.StatusCode.Should().Be(HttpStatusCode.Redirect);
        var cookies = response.Headers.TryGetValues("Set-Cookie", out var c) ? string.Join("; ", c) : "";
        cookies.Should().Contain("apitool_oidc_state");
        cookies.Should().Contain("apitool_oidc_nonce");
        cookies.Should().Contain("apitool_oidc_pkce");
    }

    [Fact]
    public async Task Get_login_with_unknown_org_returns_404()
    {
        var anonClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        var response = await anonClient.GetAsync($"/api/v1/sso/oidc/{Guid.NewGuid()}/login");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task Get_login_with_sso_disabled_returns_404()
    {
        var orgGuid = await CreateOrgAsync();
        // SSO not configured — should return 404

        var anonClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        var response = await anonClient.GetAsync($"/api/v1/sso/oidc/{orgGuid}/login");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── B4: GET /sso/oidc/{orgId}/callback — success, cookie issued ──────────

    [Fact]
    public async Task Get_callback_success_issues_apitool_session_cookie_and_redirects()
    {
        var orgGuid = await CreateOrgAsync();
        await _client.PutAsJsonAsync($"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());
        await SeedOidcUserAsync(Guid.Parse(orgGuid));

        // Simulate the callback: state must match
        var state = "test-state-value";
        var anonClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        // Set cookies that the callback handler reads
        var cookieHeader = $"apitool_oidc_state={state}; apitool_oidc_nonce=test-nonce; apitool_oidc_pkce=test-pkce";
        anonClient.DefaultRequestHeaders.Add("Cookie", cookieHeader);

        var response = await anonClient.GetAsync(
            $"/api/v1/sso/oidc/{orgGuid}/callback?code=abc&state={state}");

        response.StatusCode.Should().Be(HttpStatusCode.Redirect);
        var setCookie = response.Headers.TryGetValues("Set-Cookie", out var cookies)
            ? string.Join("; ", cookies)
            : "";
        setCookie.Should().Contain("apitool_session");
    }

    [Fact]
    public async Task Get_callback_clears_state_nonce_pkce_cookies_on_success()
    {
        var orgGuid = await CreateOrgAsync();
        await _client.PutAsJsonAsync($"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());
        await SeedOidcUserAsync(Guid.Parse(orgGuid));

        var state = "clear-state";
        var anonClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });
        anonClient.DefaultRequestHeaders.Add("Cookie",
            $"apitool_oidc_state={state}; apitool_oidc_nonce=n; apitool_oidc_pkce=p");

        var response = await anonClient.GetAsync(
            $"/api/v1/sso/oidc/{orgGuid}/callback?code=abc&state={state}");

        response.StatusCode.Should().Be(HttpStatusCode.Redirect);
        var setCookies = response.Headers.TryGetValues("Set-Cookie", out var c)
            ? string.Join(" ", c)
            : "";
        // Cookies cleared by setting MaxAge=0
        setCookies.Should().Contain("apitool_oidc_state");  // cleared cookie should be present in Set-Cookie
    }

    // ── B5: GET /callback — state mismatch ────────────────────────────────────

    [Fact]
    public async Task Get_callback_with_bad_state_returns_400_oidc_state_mismatch()
    {
        var orgGuid = await CreateOrgAsync();
        await _client.PutAsJsonAsync($"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());

        var anonClient = _factory.CreateClient();
        anonClient.DefaultRequestHeaders.Add("Cookie",
            "apitool_oidc_state=stored-state; apitool_oidc_nonce=n; apitool_oidc_pkce=p");

        var response = await anonClient.GetAsync(
            $"/api/v1/sso/oidc/{orgGuid}/callback?code=abc&state=different-state");

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("oidc_state_mismatch");
    }

    // ── B6: GET /callback — user not member ───────────────────────────────────

    [Fact]
    public async Task Get_callback_with_unknown_email_returns_403_sso_user_not_member()
    {
        var orgGuid = await CreateOrgAsync();
        await _client.PutAsJsonAsync($"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());
        var fakeHandler = _factory.GetFakeOidcHandler();
        var original = fakeHandler!.SuccessEmail;
        fakeHandler.SuccessEmail = "nobody@external.com";

        try
        {
            var state = "s1";
            var anonClient = _factory.CreateClient();
            anonClient.DefaultRequestHeaders.Add("Cookie",
                $"apitool_oidc_state={state}; apitool_oidc_nonce=n; apitool_oidc_pkce=p");

            var response = await anonClient.GetAsync(
                $"/api/v1/sso/oidc/{orgGuid}/callback?code=abc&state={state}");

            response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
            var json = await response.Content.ReadAsStringAsync();
            using var doc = JsonDocument.Parse(json);
            doc.RootElement.GetProperty("code").GetString().Should().Be("sso_user_not_member");
        }
        finally
        {
            fakeHandler.SuccessEmail = original;
        }
    }

    [Fact]
    public async Task Get_callback_with_invalid_id_token_returns_401_oidc_invalid_id_token()
    {
        var orgGuid = await CreateOrgAsync();
        await _client.PutAsJsonAsync($"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());
        var fakeHandler = _factory.GetFakeOidcHandler();
        fakeHandler!.ValidationMode = FakeOidcValidationMode.InvalidIdToken;

        try
        {
            var state = "s2";
            var anonClient = _factory.CreateClient();
            anonClient.DefaultRequestHeaders.Add("Cookie",
                $"apitool_oidc_state={state}; apitool_oidc_nonce=n; apitool_oidc_pkce=p");

            var response = await anonClient.GetAsync(
                $"/api/v1/sso/oidc/{orgGuid}/callback?code=abc&state={state}");

            response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
            var json = await response.Content.ReadAsStringAsync();
            using var doc = JsonDocument.Parse(json);
            doc.RootElement.GetProperty("code").GetString().Should().Be("oidc_invalid_id_token");
        }
        finally
        {
            fakeHandler.ValidationMode = FakeOidcValidationMode.Success;
        }
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
    public async Task Put_oidc_config_non_enterprise_returns_402(SubscriptionTier? tier)
    {
        var orgGuid = await CreateOrgWithTierAsync(tier);

        var response = await _client.PutAsJsonAsync(
            $"/api/v1/organizations/org_{orgGuid}/sso/oidc", ValidOidcConfig());

        ((int)response.StatusCode).Should().Be(402);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("sso_tier_ineligible");
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Oidc_login_non_enterprise_returns_404_no_store_empty_body(SubscriptionTier? tier)
    {
        var orgGuid = await CreateOrgWithTierAsync(tier);
        var anon = _factory.CreateClient(new WebApplicationFactoryClientOptions { AllowAutoRedirect = false });

        var response = await anon.GetAsync($"/api/v1/sso/oidc/{orgGuid}/login");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        response.Headers.CacheControl?.NoStore.Should().BeTrue();
        var body = await response.Content.ReadAsStringAsync();
        body.Should().BeEmpty();
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Oidc_callback_non_enterprise_returns_404_no_store_empty_body(SubscriptionTier? tier)
    {
        var orgGuid = await CreateOrgWithTierAsync(tier);
        var anon = _factory.CreateClient();

        var response = await anon.GetAsync(
            $"/api/v1/sso/oidc/{orgGuid}/callback?code=x&state=y");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        response.Headers.CacheControl?.NoStore.Should().BeTrue();
        var body = await response.Content.ReadAsStringAsync();
        body.Should().BeEmpty();
    }
}
