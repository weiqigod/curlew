using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Sso;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Tests for <see cref="OidcService"/> against in-memory SQLite with a <see cref="FakeOidcHandler"/>.</summary>
public sealed class OidcServiceTests
{
    // ── Test fixture ──────────────────────────────────────────────────────────

    private sealed class Fixture : IAsyncDisposable
    {
        public TestDbScope Scope { get; init; } = null!;
        public AppDbContext Db => Scope.Db;
        public OidcService Svc { get; init; } = null!;
        public FakeOidcHandler FakeHandler { get; init; } = null!;
        public Guid OwnerId { get; init; }
        public Guid MemberId { get; init; }
        public string MemberEmail { get; init; } = null!;
        public Guid OrgId { get; init; }

        public ValueTask DisposeAsync() => Scope.DisposeAsync();
    }

    private static async Task<Fixture> BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var ownerId = Guid.NewGuid();
        var memberId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        var memberEmail = $"oidc-member-{memberId:N}@example.com";

        db.Users.Add(new User { Id = ownerId, Email = $"oidc-owner-{ownerId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Users.Add(new User { Id = memberId, Email = memberEmail, CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "OidcTestOrg",
            Slug = $"oidc-{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = ownerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = memberId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow,
        });
        // M15-002: SSO requires Enterprise subscription. Seed one so the existing
        // suite (which exercises the happy path of every OidcService method) still
        // exercises those paths.
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = SubscriptionTier.Enterprise,
            Status = SubscriptionStatus.Active,
            SeatCount = 5,
            SeatLimit = 25,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddDays(30),
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var jwtOptions = Options.Create(new ApiTool.Backend.Auth.JwtOptions
        {
            SigningKey = "test-session-signing-key-32bytes!!",
            Issuer = "test",
            Audience = "test",
        });
        var samlOptions = Options.Create(new SamlOptions
        {
            WebPortalUrl = "http://localhost:3000/sso/callback",
            SessionCookieName = "apitool_session",
            SessionTtl = TimeSpan.FromHours(8),
        });
        var oidcOptions = Options.Create(new OidcOptions
        {
            BackendBaseUrl = "http://localhost:5000",
        });

        var fakeHandler = new FakeOidcHandler { SuccessEmail = memberEmail };
        var sessions = new SessionTokenIssuer(jwtOptions, TimeProvider.System);
        var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);

        var svc = new OidcService(db, fakeHandler, sessions, TimeProvider.System, oidcOptions, samlOptions, auditWriter);

        return new Fixture
        {
            Scope = scope,
            Svc = svc,
            FakeHandler = fakeHandler,
            OwnerId = ownerId,
            MemberId = memberId,
            MemberEmail = memberEmail,
            OrgId = orgId,
        };
    }

    private static OidcConfigRequest ValidRequest() => new OidcConfigRequest(
        IssuerUrl: "https://idp.example.com",
        ClientId: "apitool-client",
        ClientSecret: "s3cret",
        RedirectUri: "http://localhost:5000/api/v1/sso/oidc/test/callback",
        Scopes: "openid email profile");

    // ── UpsertOidcConfigAsync ─────────────────────────────────────────────────

    [Fact]
    public async Task Upsert_returns_invalid_config_for_missing_issuer_url()
    {
        await using var f = await BuildAsync();
        var req = ValidRequest() with { IssuerUrl = null };

        var (dto, err, field, _) = await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        err.Should().Be(SsoError.InvalidConfig);
        field.Should().Be("issuer_url");
        dto.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_returns_invalid_config_for_missing_client_id()
    {
        await using var f = await BuildAsync();
        var req = ValidRequest() with { ClientId = null };

        var (dto, err, field, _) = await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        err.Should().Be(SsoError.InvalidConfig);
        field.Should().Be("client_id");
        dto.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_returns_invalid_config_for_missing_client_secret()
    {
        await using var f = await BuildAsync();
        var req = ValidRequest() with { ClientSecret = null };

        var (dto, err, field, _) = await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        err.Should().Be(SsoError.InvalidConfig);
        field.Should().Be("client_secret");
        dto.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_returns_oidc_discovery_failed_when_probe_fails()
    {
        await using var f = await BuildAsync();
        f.FakeHandler.ValidationMode = FakeOidcValidationMode.DiscoveryFailed;

        var (dto, err, _, _) = await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.OidcDiscoveryFailed);
        dto.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_as_owner_persists_provider_oidc_and_stores_secret()
    {
        await using var f = await BuildAsync();

        var (dto, err, _, _) = await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.None);
        dto.Should().NotBeNull();

        var org = await f.Db.Organizations.FindAsync(f.OrgId);
        var settings = OrganizationSettings.FromJson(org!.SettingsJson);
        settings.SsoEnabled.Should().BeTrue();
        settings.SsoProvider.Should().Be("oidc");
        settings.OidcConfig.Should().NotBeNull();

        // Secret stored in sso_credentials
        var cred = await f.Db.SsoCredentials
            .FirstOrDefaultAsync(c => c.OrgId == f.OrgId && c.Kid == "oidc_client_secret");
        cred.Should().NotBeNull();
        cred!.PublicCertPem.Should().Be("s3cret");
    }

    [Fact]
    public async Task Upsert_as_non_owner_returns_permission_denied()
    {
        await using var f = await BuildAsync();

        var (dto, err, _, _) = await f.Svc.UpsertOidcConfigAsync(f.MemberId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.PermissionDenied);
        dto.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_with_redirect_uri_omitted_synthesises_default()
    {
        await using var f = await BuildAsync();
        var req = ValidRequest() with { RedirectUri = null };

        var (dto, err, _, _) = await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        err.Should().Be(SsoError.None);
        var org = await f.Db.Organizations.FindAsync(f.OrgId);
        var settings = OrganizationSettings.FromJson(org!.SettingsJson);
        settings.OidcConfig!.RedirectUri.Should().Contain("/api/v1/sso/oidc/");
        settings.OidcConfig.RedirectUri.Should().Contain("/callback");
    }

    [Fact]
    public async Task Upsert_clears_previous_saml_config_when_switching_providers()
    {
        await using var f = await BuildAsync();

        // Set SAML config first by manipulating settings directly
        var org = await f.Db.Organizations.FindAsync(f.OrgId);
        var settings = OrganizationSettings.FromJson(org!.SettingsJson);
        settings.SsoEnabled = true;
        settings.SsoProvider = "saml";
        settings.SsoConfig = new SsoConfig("https://saml.idp/meta", "https://sp/acs", "https://sp");
        org.SettingsJson = settings.ToJson();
        await f.Db.SaveChangesAsync();

        // Now upsert OIDC
        var (dto, err, _, _) = await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.None);
        var updatedOrg = await f.Db.Organizations.FindAsync(f.OrgId);
        var updatedSettings = OrganizationSettings.FromJson(updatedOrg!.SettingsJson);
        updatedSettings.SsoProvider.Should().Be("oidc");
        updatedSettings.OidcConfig.Should().NotBeNull();
        updatedSettings.SsoConfig.Should().BeNull();
    }

    // ── BuildLoginAsync ───────────────────────────────────────────────────────

    [Fact]
    public async Task BuildLogin_returns_authorize_url_with_state_and_nonce()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        var (authorizeUrl, state, nonce, pkce, err, _) =
            await f.Svc.BuildLoginAsync(f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.None);
        authorizeUrl.Should().NotBeNullOrWhiteSpace();
        authorizeUrl!.Should().Contain("state=");
        state.Should().NotBeNullOrWhiteSpace();
        nonce.Should().NotBeNullOrWhiteSpace();
        pkce.Should().NotBeNullOrWhiteSpace();
    }

    [Fact]
    public async Task BuildLogin_returns_sso_not_enabled_when_not_configured()
    {
        await using var f = await BuildAsync();

        var (url, state, nonce, pkce, err, _) =
            await f.Svc.BuildLoginAsync(f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.SsoNotEnabled);
        url.Should().BeNull();
    }

    [Fact]
    public async Task BuildLogin_returns_sso_not_enabled_when_provider_is_saml()
    {
        await using var f = await BuildAsync();

        // Set provider to SAML directly
        var org = await f.Db.Organizations.FindAsync(f.OrgId);
        var settings = OrganizationSettings.FromJson(org!.SettingsJson);
        settings.SsoEnabled = true;
        settings.SsoProvider = "saml";
        settings.SsoConfig = new SsoConfig("https://saml.idp/meta", "https://sp/acs", "https://sp");
        org.SettingsJson = settings.ToJson();
        await f.Db.SaveChangesAsync();

        var (url, state, nonce, pkce, err, _) =
            await f.Svc.BuildLoginAsync(f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.SsoNotEnabled);
        url.Should().BeNull();
    }

    // ── ConsumeCallbackAsync ──────────────────────────────────────────────────

    [Fact]
    public async Task Consume_issues_session_cookie_on_success()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        var (cookie, redirectUrl, err, _) = await f.Svc.ConsumeCallbackAsync(
            f.OrgId, "code", "state1", "state1", "nonce1", "verifier1", CancellationToken.None);

        err.Should().Be(SsoError.None);
        cookie.Should().NotBeNullOrWhiteSpace();
        redirectUrl.Should().Be("http://localhost:3000/sso/callback");
    }

    [Fact]
    public async Task Consume_returns_state_mismatch_when_states_differ()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        var (cookie, _, err, _) = await f.Svc.ConsumeCallbackAsync(
            f.OrgId, "code", "bad-state", "expected-state", "nonce1", "verifier1", CancellationToken.None);

        err.Should().Be(SsoError.OidcStateMismatch);
        cookie.Should().BeNull();
    }

    [Fact]
    public async Task Consume_returns_oidc_invalid_id_token_when_handler_rejects_signature()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeOidcValidationMode.InvalidIdToken;

        var (cookie, _, err, _) = await f.Svc.ConsumeCallbackAsync(
            f.OrgId, "code", "s", "s", "n", "v", CancellationToken.None);

        err.Should().Be(SsoError.OidcInvalidIdToken);
        cookie.Should().BeNull();
    }

    [Fact]
    public async Task Consume_returns_oidc_nonce_mismatch_when_handler_rejects_nonce()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeOidcValidationMode.NonceMismatch;

        var (cookie, _, err, _) = await f.Svc.ConsumeCallbackAsync(
            f.OrgId, "code", "s", "s", "n", "v", CancellationToken.None);

        err.Should().Be(SsoError.OidcNonceMismatch);
        cookie.Should().BeNull();
    }

    [Fact]
    public async Task Consume_returns_user_not_member_when_email_not_in_org()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.SuccessEmail = "unknown@external.com";

        var (cookie, _, err, _) = await f.Svc.ConsumeCallbackAsync(
            f.OrgId, "code", "s", "s", "n", "v", CancellationToken.None);

        err.Should().Be(SsoError.UserNotMember);
        cookie.Should().BeNull();
    }

    [Fact]
    public async Task Consume_writes_audit_entry_on_success_and_failure()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertOidcConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        // Success
        await f.Svc.ConsumeCallbackAsync(f.OrgId, "code", "s", "s", "n", "v", CancellationToken.None);
        var successEntry = await f.Db.OrganizationAuditLog
            .FirstOrDefaultAsync(e => e.OrgId == f.OrgId && e.EventType == "sso.login" && e.Success);
        successEntry.Should().NotBeNull();

        // Failure
        f.FakeHandler.ValidationMode = FakeOidcValidationMode.InvalidIdToken;
        await f.Svc.ConsumeCallbackAsync(f.OrgId, "code", "s", "s", "n", "v", CancellationToken.None);
        var failEntry = await f.Db.OrganizationAuditLog
            .FirstOrDefaultAsync(e => e.OrgId == f.OrgId && e.EventType == "sso.login" && !e.Success);
        failEntry.Should().NotBeNull();
    }

    // ── M15-002 tier-matrix tests ────────────────────────────────────────────

    private static async Task SeedSubscriptionAsync(AppDbContext db, Guid orgId, SubscriptionTier? tier)
    {
        // tier == null seeds NO subscription row (the "default-Free" case).
        if (tier is null) return;
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = tier.Value,
            Status = SubscriptionStatus.Active,
            SeatCount = 1,
            SeatLimit = 5,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddDays(30),
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
    }

    public static IEnumerable<object?[]> NonEnterpriseTiers => new List<object?[]>
    {
        new object?[] { null }, // no-subscription → Free
        new object?[] { SubscriptionTier.Free },
        new object?[] { SubscriptionTier.Professional },
        new object?[] { SubscriptionTier.Team },
    };

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Upsert_returns_tier_ineligible_when_not_enterprise(SubscriptionTier? tier)
    {
        await using var f = await BuildAsync();
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, tier);

        var (dto, err, _, _) = await f.Svc.UpsertOidcConfigAsync(
            f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.TierIneligible);
        dto.Should().BeNull();
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task BuildLogin_returns_tier_ineligible_when_not_enterprise(SubscriptionTier? tier)
    {
        await using var f = await BuildAsync();
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, tier);

        var (url, _, _, _, err, _) = await f.Svc.BuildLoginAsync(f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.TierIneligible);
        url.Should().BeNull();
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task ConsumeCallback_returns_tier_ineligible_when_not_enterprise(SubscriptionTier? tier)
    {
        await using var f = await BuildAsync();
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, tier);

        var (cookie, _, err, _) = await f.Svc.ConsumeCallbackAsync(
            f.OrgId, "code", "s", "s", "n", "v", CancellationToken.None);

        err.Should().Be(SsoError.TierIneligible);
        cookie.Should().BeNull();
    }
}
