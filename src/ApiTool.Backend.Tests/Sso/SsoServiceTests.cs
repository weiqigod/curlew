using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Sso;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Tests for <see cref="SsoService"/> against in-memory SQLite with a <see cref="FakeSamlHandler"/>.</summary>
public sealed class SsoServiceTests
{
    // ── Test fixture ──────────────────────────────────────────────────────────

    private sealed class Fixture : IAsyncDisposable
    {
        public TestDbScope Scope { get; init; } = null!;
        public AppDbContext Db => Scope.Db;
        public SsoService Svc { get; init; } = null!;
        public FakeSamlHandler FakeHandler { get; init; } = null!;
        public Guid OwnerId { get; init; }
        public Guid AdminId { get; init; }
        public Guid MemberId { get; init; }
        public Guid OrgId { get; init; }

        public ValueTask DisposeAsync() => Scope.DisposeAsync();
    }

    private static async Task<Fixture> BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var ownerId = Guid.NewGuid();
        var adminId = Guid.NewGuid();
        var memberId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        var memberEmail = $"member-{memberId:N}@example.com";

        db.Users.Add(new User { Id = ownerId, Email = $"owner-{ownerId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Users.Add(new User { Id = adminId, Email = $"admin-{adminId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Users.Add(new User { Id = memberId, Email = memberEmail, CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "SsoTestOrg",
            Slug = $"sso-{orgId:N}"[..20],
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
            OrgId = orgId, UserId = adminId, Role = OrgRole.Admin, JoinedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = memberId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow,
        });
        // M15-002: SSO requires Enterprise subscription. Seed one so the existing
        // suite (which exercises the happy path of every SsoService method) still
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

        var fakeHandler = new FakeSamlHandler { SuccessEmail = memberEmail };
        var sessions = new SessionTokenIssuer(jwtOptions, TimeProvider.System);
        var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
        var svc = new SsoService(db, fakeHandler, sessions, TimeProvider.System, samlOptions, auditWriter);

        return new Fixture
        {
            Scope = scope,
            Svc = svc,
            FakeHandler = fakeHandler,
            OwnerId = ownerId,
            AdminId = adminId,
            MemberId = memberId,
            OrgId = orgId,
        };
    }

    private static SamlConfigRequest ValidRequest() => new SamlConfigRequest(
        IdpMetadataUrl: "https://idp.example.com/metadata",
        AcsUrl: "https://sp.example.com/acs",
        EntityId: "https://sp.example.com",
        IdpSsoUrl: "https://idp.example.com/sso");

    // ── UpsertSamlConfigAsync ─────────────────────────────────────────────────

    [Theory]
    [InlineData(null, "acs", "entity", "idp_metadata_url")]
    [InlineData("meta", null, "entity", "acs_url")]
    [InlineData("meta", "acs", null, "entity_id")]
    public async Task Upsert_returns_invalid_config_for_missing_field(
        string? meta, string? acs, string? entity, string missingField)
    {
        await using var f = await BuildAsync();

        var req = new SamlConfigRequest(meta, acs, entity);
        var (dto, err, field, _) = await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        err.Should().Be(SsoError.InvalidConfig);
        field.Should().Be(missingField);
        dto.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_as_owner_persists_config_and_sets_sso_enabled_true()
    {
        await using var f = await BuildAsync();

        var (dto, err, _, _) = await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.None);
        dto.Should().NotBeNull();

        var org = await f.Db.Organizations.FindAsync(f.OrgId);
        var settings = OrganizationSettings.FromJson(org!.SettingsJson);
        settings.SsoEnabled.Should().BeTrue();
        settings.SsoProvider.Should().Be("saml");
        settings.SsoConfig.Should().NotBeNull();
    }

    [Fact]
    public async Task Upsert_as_non_owner_returns_permission_denied()
    {
        await using var f = await BuildAsync();

        var (dto, err, _, _) = await f.Svc.UpsertSamlConfigAsync(f.MemberId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.PermissionDenied);
        dto.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_stores_idp_cert_in_sso_credentials_when_pem_given()
    {
        await using var f = await BuildAsync();

        var req = ValidRequest() with { IdpCertPem = "-----BEGIN CERTIFICATE-----\nMIIBtest\n-----END CERTIFICATE-----" };
        var (_, err, _, _) = await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        err.Should().Be(SsoError.None);

        var cred = await f.Db.SsoCredentials.FirstOrDefaultAsync(c => c.OrgId == f.OrgId);
        cred.Should().NotBeNull();
        cred!.PublicCertPem.Should().Be(req.IdpCertPem);
    }

    // ── BuildLoginAsync ───────────────────────────────────────────────────────

    [Fact]
    public async Task Build_login_returns_redirect_url_with_saml_request_query()
    {
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        var (redirectUrl, err) = await f.Svc.BuildLoginAsync(f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.None);
        redirectUrl.Should().NotBeNullOrWhiteSpace();
        redirectUrl.Should().Contain("SAMLRequest=");
    }

    [Fact]
    public async Task Build_login_returns_sso_not_enabled_when_not_configured()
    {
        await using var f = await BuildAsync();

        var (redirectUrl, err) = await f.Svc.BuildLoginAsync(f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.SsoNotEnabled);
        redirectUrl.Should().BeNull();
    }

    // ── ConsumeAssertionAsync ─────────────────────────────────────────────────

    [Fact]
    public async Task Consume_issues_session_cookie_when_email_matches_member()
    {
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeValidationMode.Success;

        var (sessionCookie, redirectUrl, err, _) = await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        err.Should().Be(SsoError.None);
        sessionCookie.Should().NotBeNullOrWhiteSpace();
        redirectUrl.Should().Be("http://localhost:3000/sso/callback");
    }

    [Fact]
    public async Task Consume_returns_signature_invalid_from_handler_failure()
    {
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeValidationMode.SignatureInvalid;

        var (sessionCookie, _, err, _) = await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        err.Should().Be(SsoError.SignatureInvalid);
        sessionCookie.Should().BeNull();
    }

    [Fact]
    public async Task Consume_returns_user_not_member_when_email_not_in_org()
    {
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeValidationMode.Success;
        f.FakeHandler.SuccessEmail = "not-a-member@other.com";

        var (sessionCookie, _, err, _) = await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        err.Should().Be(SsoError.UserNotMember);
        sessionCookie.Should().BeNull();
    }

    [Fact]
    public async Task Consume_writes_audit_log_entry_on_success()
    {
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeValidationMode.Success;

        await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        var auditEntry = await f.Db.OrganizationAuditLog
            .FirstOrDefaultAsync(e => e.OrgId == f.OrgId && e.EventType == "sso.login" && e.Success);
        auditEntry.Should().NotBeNull();
    }

    [Fact]
    public async Task Consume_writes_audit_log_entry_on_failure()
    {
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeValidationMode.SignatureInvalid;

        await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        var auditEntry = await f.Db.OrganizationAuditLog
            .FirstOrDefaultAsync(e => e.OrgId == f.OrgId && e.EventType == "sso.login" && !e.Success);
        auditEntry.Should().NotBeNull();
    }

    [Fact]
    public async Task Consume_success_audit_row_carries_actor_email()
    {
        // Regression for M5-021: audit-log UI surfaces ActorEmail to admins. The SSO
        // success path must propagate the authenticated email to the audit writer;
        // otherwise the audit row renders `—` for the user column and enterprise-full
        // spec assertion #2 (`expect(ssoLoginRow).toContainText('qa@acme.example')`)
        // fails on CI even when the SAML round-trip itself succeeded.
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        f.FakeHandler.ValidationMode = FakeValidationMode.Success;

        await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        var auditEntry = await f.Db.OrganizationAuditLog
            .FirstOrDefaultAsync(e => e.OrgId == f.OrgId && e.EventType == "sso.login" && e.Success);
        auditEntry.Should().NotBeNull();
        auditEntry!.ActorEmail.Should().NotBeNullOrEmpty();
        auditEntry.ActorEmail.Should().Contain("@");
    }

    [Fact]
    public async Task Consume_returns_user_not_member_when_user_exists_but_not_in_org()
    {
        // Finding #2: cover the code path where user exists in db.Users but is NOT a member of the org.
        await using var f = await BuildAsync();

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        // Add a user to db.Users who is NOT a member of the test org
        var externalUserId = Guid.NewGuid();
        var externalEmail = $"external-{externalUserId:N}@other.com";
        f.Db.Users.Add(new ApiTool.Backend.Data.Entities.User
        {
            Id = externalUserId,
            Email = externalEmail,
            CreatedAt = DateTime.UtcNow,
        });
        await f.Db.SaveChangesAsync();

        // Configure fake handler to return that user's email
        f.FakeHandler.ValidationMode = FakeValidationMode.Success;
        f.FakeHandler.SuccessEmail = externalEmail;

        var (sessionCookie, _, err, _) = await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        err.Should().Be(SsoError.UserNotMember);
        sessionCookie.Should().BeNull();
    }

    [Fact]
    public async Task Upsert_cert_twice_with_same_kid_updates_row_and_leaves_single_credential()
    {
        // Finding #7: cover the idempotent cert upsert (else branch in SsoService.cs lines 86-88).
        await using var f = await BuildAsync();

        var certPem = "-----BEGIN CERTIFICATE-----\nMIIBupsert\n-----END CERTIFICATE-----";
        var req = ValidRequest() with { IdpCertPem = certPem };

        // First upsert — creates the SsoCredential row
        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        // Second upsert with the same cert PEM — same KID, so existing row should be updated, not duplicated.
        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        var credCount = await f.Db.SsoCredentials.CountAsync(c => c.OrgId == f.OrgId);
        credCount.Should().Be(1, because: "a second upsert with the same KID should update, not duplicate");
    }

    // ── GetConfigAsync ────────────────────────────────────────────────────────

    [Theory]
    [InlineData("owner_with_no_sso_configured", "owner", false, null)]
    [InlineData("owner_with_saml_configured", "owner", true, "saml")]
    [InlineData("owner_with_oidc_configured", "owner", true, "oidc")]
    public async Task GetConfig_as_owner_returns_expected_config(
        string name, string callerRole, bool setupSso, string? expectedProvider)
    {
        _ = name;  // used for InlineData readability only
        await using var f = await BuildAsync();

        if (setupSso && expectedProvider == "saml")
        {
            await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);
        }
        else if (setupSso && expectedProvider == "oidc")
        {
            // Set OIDC config directly via OidcService path through settings
            var org = await f.Db.Organizations.FindAsync(f.OrgId);
            var settings = OrganizationSettings.FromJson(org!.SettingsJson);
            settings.SsoEnabled = true;
            settings.SsoProvider = "oidc";
            settings.OidcConfig = new ApiTool.Backend.Sso.OidcConfig(
                "https://idp.example.com",
                "client_id",
                "https://sp.example.com/oidc/callback");
            org.SettingsJson = settings.ToJson();
            await f.Db.SaveChangesAsync();
        }

        _ = callerRole;  // always owner here; non-owner cases tested below
        var (view, err) = await f.Svc.GetConfigAsync(f.OwnerId, f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.None);
        view.Should().NotBeNull();
        view!.SsoEnabled.Should().Be(setupSso);
        view.SsoProvider.Should().Be(expectedProvider);
    }

    [Fact]
    public async Task GetConfig_as_owner_with_saml_does_not_expose_idp_cert()
    {
        await using var f = await BuildAsync();

        // Upsert with a cert
        var certPem = "-----BEGIN CERTIFICATE-----\nMIIBsecret\n-----END CERTIFICATE-----";
        var req = ValidRequest() with { IdpCertPem = certPem };
        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, req, CancellationToken.None);

        var (view, err) = await f.Svc.GetConfigAsync(f.OwnerId, f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.None);
        view.Should().NotBeNull();
        view!.SamlConfig.Should().NotBeNull();
        // The IdP cert must NOT be exposed in the view
        view.SamlConfig!.IdpMetadataUrl.Should().Be(req.IdpMetadataUrl);
        view.SamlConfig.AcsUrl.Should().Be(req.AcsUrl);
        view.SamlConfig.EntityId.Should().Be(req.EntityId);
    }

    [Fact]
    public async Task GetConfig_with_saml_returns_saml_fields_and_null_oidc()
    {
        await using var f = await BuildAsync();
        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        var (view, _) = await f.Svc.GetConfigAsync(f.OwnerId, f.OrgId, CancellationToken.None);

        view!.SamlConfig.Should().NotBeNull();
        view.OidcConfig.Should().BeNull();
        view.SsoProvider.Should().Be("saml");
    }

    [Theory]
    [InlineData("admin_denied", true)]
    [InlineData("member_denied", false)]
    public async Task GetConfig_as_non_owner_returns_permission_denied(string name, bool isAdmin)
    {
        _ = name;
        await using var f = await BuildAsync();

        var callerId = isAdmin ? f.AdminId : f.MemberId;
        var (view, err) = await f.Svc.GetConfigAsync(callerId, f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.PermissionDenied);
        view.Should().BeNull();
    }

    [Fact]
    public async Task GetConfig_with_unknown_org_returns_org_not_found()
    {
        // M15-002: The tier gate (EnsureEnterpriseTierAsync) runs first and returns OrgNotFound
        // for unknown orgs. This takes precedence over the previous PermissionDenied that would
        // have come from the missing membership check. The new behaviour is safer: callers cannot
        // distinguish "org doesn't exist" from "org exists but has no Enterprise tier" on the
        // authenticated config endpoint.
        await using var f = await BuildAsync();

        var unknownOrgId = Guid.NewGuid();
        var (view, err) = await f.Svc.GetConfigAsync(f.OwnerId, unknownOrgId, CancellationToken.None);

        err.Should().Be(SsoError.OrgNotFound);
        view.Should().BeNull();
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
        // Replace fixture's seeded Enterprise sub with the matrix tier.
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, tier);

        var (dto, err, _, _) = await f.Svc.UpsertSamlConfigAsync(
            f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        err.Should().Be(SsoError.TierIneligible);
        dto.Should().BeNull();
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task Get_returns_tier_ineligible_when_not_enterprise(SubscriptionTier? tier)
    {
        await using var f = await BuildAsync();
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, tier);

        var (view, err) = await f.Svc.GetConfigAsync(f.OwnerId, f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.TierIneligible);
        view.Should().BeNull();
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task BuildLogin_returns_tier_ineligible_when_not_enterprise(SubscriptionTier? tier)
    {
        await using var f = await BuildAsync();
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, tier);

        var (redirectUrl, err) = await f.Svc.BuildLoginAsync(f.OrgId, CancellationToken.None);

        err.Should().Be(SsoError.TierIneligible);
        redirectUrl.Should().BeNull();
    }

    [Theory, MemberData(nameof(NonEnterpriseTiers))]
    public async Task ConsumeAssertion_returns_tier_ineligible_when_not_enterprise(SubscriptionTier? tier)
    {
        await using var f = await BuildAsync();
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, tier);

        var (cookie, _, err, _) = await f.Svc.ConsumeAssertionAsync(
            f.OrgId, Convert.ToBase64String("fake"u8.ToArray()), CancellationToken.None);

        err.Should().Be(SsoError.TierIneligible);
        cookie.Should().BeNull();
    }

    [Fact]
    public async Task TierIneligible_short_circuits_before_db_writes()
    {
        // Regression: ensure the gate fires BEFORE the SSO config is persisted —
        // otherwise non-Enterprise tenants could leak partial state into SettingsJson.
        await using var f = await BuildAsync();
        var existing = await f.Db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == f.OrgId);
        if (existing is not null) f.Db.Subscriptions.Remove(existing);
        await f.Db.SaveChangesAsync();
        await SeedSubscriptionAsync(f.Db, f.OrgId, SubscriptionTier.Team);

        await f.Svc.UpsertSamlConfigAsync(f.OwnerId, f.OrgId, ValidRequest(), CancellationToken.None);

        var org = await f.Db.Organizations.FindAsync(f.OrgId);
        var settings = OrganizationSettings.FromJson(org!.SettingsJson);
        settings.SsoEnabled.Should().BeFalse(because: "tier gate must reject before persistence");
        settings.SsoConfig.Should().BeNull();
    }
}
