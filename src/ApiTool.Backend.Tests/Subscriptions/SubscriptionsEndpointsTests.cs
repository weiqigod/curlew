using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Full behaviour-level tests for the /api/v1/subscriptions endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class SubscriptionsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;

    public SubscriptionsEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var ownerEmail = $"sub-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, ownerEmail);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
        // M16-003: checkout endpoint is now gated on email_verified.
        // Make a request to trigger user-row upsert, then mark verified.
        await _client.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(_ownerId);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<string> CreateOrgAndGetIdAsync()
    {
        var slug = $"sub-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "SubOrg", slug });
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        return doc.RootElement.GetProperty("id").GetString()!;
    }

    // ── POST /api/v1/subscriptions/checkout ──────────────────────────────────

    [Fact]
    public async Task Checkout_returns_200_with_checkout_url_for_owner()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        var body = new
        {
            org_id = orgId, tier = "team", interval = "month",
            seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", body);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("checkout_url").GetString().Should()
            .StartWith("https://checkout.stripe.test/cs_");
        doc.RootElement.GetProperty("session_id").GetString().Should().StartWith("cs_");
    }

    [Fact]
    public async Task Checkout_returns_403_for_non_owner()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        // Member token (not owner).
        var (memberToken, _) = TestTokens.CreateNew($"member-{Guid.NewGuid():N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var body = new
        {
            org_id = orgId, tier = "team", interval = "month",
            seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        var response = await memberClient.PostAsJsonAsync("/api/v1/subscriptions/checkout", body);

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── GET /api/v1/subscriptions ──────────────────────────────────────────────

    [Fact]
    public async Task Get_returns_null_subscription_and_tier_free_when_no_sub()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        var response = await _client.GetAsync($"/api/v1/subscriptions?org_id={orgId}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("tier").GetString().Should().Be("free");
        // When WhenWritingNull is set, null fields may be omitted; check for absent or null.
        if (doc.RootElement.TryGetProperty("subscription", out var subProp))
            subProp.ValueKind.Should().Be(JsonValueKind.Null);
    }

    // ── PATCH /api/v1/subscriptions/{id} ─────────────────────────────────────

    [Fact]
    public async Task Patch_increases_seats_and_returns_proration_block()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        // First create a subscription.
        var checkoutBody = new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        // Fetch the subscription to get its id.
        var getResp = await _client.GetAsync($"/api/v1/subscriptions?org_id={orgId}");
        var getJson = await getResp.Content.ReadAsStringAsync();
        using var getDoc = JsonDocument.Parse(getJson);
        var subId = getDoc.RootElement.GetProperty("subscription").GetProperty("id").GetString()!;

        // Increase seats.
        var patchBody = new { seat_count = 8 };
        var response = await _client.PatchAsJsonAsync($"/api/v1/subscriptions/{subId}", patchBody);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("subscription").GetProperty("seat_limit").GetInt32().Should().Be(8);
        doc.RootElement.GetProperty("proration").GetProperty("net").GetInt32().Should().BeGreaterThan(0);
    }

    [Fact]
    public async Task Patch_with_downgrade_below_active_members_returns_403_downgrade_blocked()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        // Create a subscription with 1 seat (but there is 1 member already).
        var checkoutBody = new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 1,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var getResp = await _client.GetAsync($"/api/v1/subscriptions?org_id={orgId}");
        var getJson = await getResp.Content.ReadAsStringAsync();
        using var getDoc = JsonDocument.Parse(getJson);
        var subId = getDoc.RootElement.GetProperty("subscription").GetProperty("id").GetString()!;

        // Try to reduce below active seat count (1 member exists, try to reduce to 0).
        var patchBody = new { seat_count = 0 };
        var response = await _client.PatchAsJsonAsync($"/api/v1/subscriptions/{subId}", patchBody);

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("subscription_downgrade_blocked");
    }

    // ── DELETE /api/v1/subscriptions/{id} ─────────────────────────────────────

    [Fact]
    public async Task Delete_marks_cancel_at_period_end()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        var checkoutBody = new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 3,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var getResp = await _client.GetAsync($"/api/v1/subscriptions?org_id={orgId}");
        var getJson = await getResp.Content.ReadAsStringAsync();
        using var getDoc = JsonDocument.Parse(getJson);
        var subId = getDoc.RootElement.GetProperty("subscription").GetProperty("id").GetString()!;

        var response = await _client.DeleteAsync($"/api/v1/subscriptions/{subId}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("subscription").GetProperty("cancel_at_period_end").GetBoolean().Should().BeTrue();
    }

    // ── Interval validation ───────────────────────────────────────────────────

    [Fact]
    public async Task Checkout_with_invalid_interval_returns_400()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        var body = new
        {
            org_id = orgId, tier = "team", interval = "quarter",
            seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_interval");
    }

    [Fact]
    public async Task Patch_with_invalid_interval_returns_400()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        // First create a subscription.
        var checkoutBody = new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var getResp = await _client.GetAsync($"/api/v1/subscriptions?org_id={orgId}");
        var getJson = await getResp.Content.ReadAsStringAsync();
        using var getDoc = JsonDocument.Parse(getJson);
        var subId = getDoc.RootElement.GetProperty("subscription").GetProperty("id").GetString()!;

        // Attempt to update with an invalid interval.
        var patchBody = new { interval = "weekly" };
        var response = await _client.PatchAsJsonAsync($"/api/v1/subscriptions/{subId}", patchBody);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_interval");
    }

    [Fact]
    public async Task Patch_with_invalid_tier_returns_400()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        // First create a subscription.
        var checkoutBody = new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var getResp = await _client.GetAsync($"/api/v1/subscriptions?org_id={orgId}");
        var getJson = await getResp.Content.ReadAsStringAsync();
        using var getDoc = JsonDocument.Parse(getJson);
        var subId = getDoc.RootElement.GetProperty("subscription").GetProperty("id").GetString()!;

        // Attempt to update with an invalid tier (should return 400, not silently succeed).
        var patchBody = new { tier = "quantum" };
        var response = await _client.PatchAsJsonAsync($"/api/v1/subscriptions/{subId}", patchBody);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_tier");
    }

    // ── price_id path behaviours ──────────────────────────────────────────────

    /// <summary>Behaviour #4: invalid price_id returns HTTP 400 with code invalid_price_id.</summary>
    [Fact]
    public async Task Checkout_with_invalid_price_id_returns_400_invalid_price_id()
    {
        var body = new
        {
            price_id = "price_unknown_thing",
            success_url = "http://localhost/ok",
            cancel_url = "http://localhost/no",
        };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_price_id");
    }

    /// <summary>Behaviour #6: org_id in request body is ignored when price_id is supplied; the bearer's org is billed.</summary>
    [Fact]
    public async Task Checkout_price_id_path_ignores_body_org_id_and_uses_bearer_org()
    {
        // This owner already owns an org created in CreateOrgAndGetIdAsync.
        var ownerOrgId = await CreateOrgAndGetIdAsync();

        // POST with a different (fake) org_id in the body alongside price_id.
        var fakeOrgId = $"org_{Guid.NewGuid():N}"[..20]; // won't match any real org
        var body = new
        {
            price_id = "price_test_team_monthly",
            org_id = fakeOrgId,   // should be ignored
            success_url = "http://localhost/ok",
            cancel_url = "http://localhost/no",
        };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", body);

        // The service resolves org from the bearer token owner; the fake org_id is ignored.
        // Because the bearer's org exists and the user is owner → checkout succeeds.
        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("checkout_url").GetString().Should().Contain("cs_test_");
    }

    /// <summary>Behaviour #5: Stripe rate-limit maps to HTTP 503 STRIPE_RATE_LIMITED.</summary>
    [Fact]
    public async Task Checkout_price_id_path_returns_503_when_stripe_rate_limits()
    {
        // Build a one-off client backed by a factory variant whose IStripeGateway throws StripeRateLimitedException.
        await using var rateLimitFactory = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IStripeGateway>();
                services.AddSingleton<IStripeGateway, ThrowingFakeStripeGateway>();
            }));

        var ownerId = Guid.NewGuid();
        var ownerEmail = $"rl-owner-{ownerId:N}@example.com";
        var token = TestTokens.Create(ownerId, ownerEmail);
        var client = rateLimitFactory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        // Seed the owner's org via the rate-limit factory's own DB scope.
        using (var scope = rateLimitFactory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            var orgId = Guid.NewGuid();
            // M16-003: EmailVerified = true required to pass RequireVerifiedEmail filter on checkout.
            db.Users.Add(new User { Id = ownerId, Email = ownerEmail, CreatedAt = DateTime.UtcNow, EmailVerified = true });
            db.Organizations.Add(new Organization
            {
                Id = orgId, Name = "RLOrg", Slug = $"rlorg-{orgId:N}"[..20],
                OwnerId = ownerId, Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId, UserId = ownerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var body = new
        {
            price_id = "price_test_team_monthly",
            success_url = "http://localhost/ok",
            cancel_url = "http://localhost/no",
        };
        var response = await client.PostAsJsonAsync("/api/v1/subscriptions/checkout", body);

        response.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("stripe_rate_limited");
    }

    // ── POST /api/v1/subscriptions/portal ─────────────────────────────────────

    [Fact]
    public async Task Portal_returns_portal_url()
    {
        var orgId = await CreateOrgAndGetIdAsync();

        var body = new { org_id = orgId, return_url = "http://localhost/return" };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/portal", body);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("portal_url").GetString().Should()
            .StartWith("https://billing.stripe.test/");
    }

    // ── POST /api/v1/subscriptions/billing-portal ────────────────────────────

    /// <summary>Behaviour #1 — empty body, return_url defaults to App:WebAppUrl + /billing, response has .url field.</summary>
    [Fact]
    public async Task BillingPortal_with_empty_body_returns_url_using_default_return_url()
    {
        var orgId = await CreateOrgAndGetIdAsync();
        // Seed a Subscription row with a stripe_customer_id so the no_billing_setup guard does not trip.
        using (var scope = _factory.Services.CreateScope())
        {
            OrgId.TryParse(orgId, out var rawOrgId);
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = rawOrgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                StripeCustomerId = "cus_test_seed_billing",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/billing-portal", new { });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("url").GetString().Should().StartWith("https://billing.stripe.test/");
        response.Headers.GetValues("X-Stripe-Idempotency-Key").Should().NotBeEmpty();
    }

    /// <summary>Behaviour #2 — org with no stripe_customer_id returns 409 no_billing_setup with no Stripe call.</summary>
    [Fact]
    public async Task BillingPortal_with_no_stripe_customer_id_returns_409_no_billing_setup()
    {
        await CreateOrgAndGetIdAsync(); // org exists but no Subscription seeded

        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/billing-portal", new { });

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("no_billing_setup");
    }

    /// <summary>Behaviour #5 — member-role user gets 403 permission_denied.</summary>
    [Fact]
    public async Task BillingPortal_with_member_role_returns_403_permission_denied()
    {
        await CreateOrgAndGetIdAsync();
        var memberEmail = $"member-{Guid.NewGuid():N}@example.com";
        var (memberToken, memberId) = TestTokens.CreateNew(memberEmail);
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            var rawOrgId = db.OrganizationMembers
                .Where(m => m.UserId == _ownerId)
                .Select(m => m.OrgId)
                .FirstOrDefault();
            db.Users.Add(new User { Id = memberId, Email = memberEmail, CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = rawOrgId, UserId = memberId,
                Role = OrgRole.Member, JoinedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", memberToken);

        var response = await memberClient.PostAsJsonAsync("/api/v1/subscriptions/billing-portal", new { });
        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("permission_denied");
    }

    /// <summary>Behaviour: admin-role user can access the portal (relaxed from owner-only).</summary>
    [Fact]
    public async Task BillingPortal_with_admin_role_returns_url()
    {
        var orgId = await CreateOrgAndGetIdAsync();
        var adminEmail = $"admin-{Guid.NewGuid():N}@example.com";
        var (adminToken, adminId) = TestTokens.CreateNew(adminEmail);
        using (var scope = _factory.Services.CreateScope())
        {
            OrgId.TryParse(orgId, out var rawOrgId);
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            db.Users.Add(new User { Id = adminId, Email = adminEmail, CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = rawOrgId, UserId = adminId,
                Role = OrgRole.Admin, JoinedAt = DateTime.UtcNow,
            });
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = rawOrgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                StripeCustomerId = "cus_test_admin_role",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var adminClient = _factory.CreateClient();
        adminClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", adminToken);

        var response = await adminClient.PostAsJsonAsync("/api/v1/subscriptions/billing-portal", new { });
        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    /// <summary>Stripe rate-limit on billing-portal returns HTTP 503 with code stripe_rate_limited.</summary>
    [Fact]
    public async Task BillingPortal_with_stripe_rate_limit_returns_503()
    {
        await using var rateLimitFactory = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IStripeGateway>();
                services.AddSingleton<IStripeGateway, ThrowingRateLimitedPortalGateway>();
            }));

        var ownerId = Guid.NewGuid();
        var ownerEmail = $"rl-portal-owner-{ownerId:N}@example.com";
        var token = TestTokens.Create(ownerId, ownerEmail);
        var client = rateLimitFactory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        using (var scope = rateLimitFactory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            var orgId = Guid.NewGuid();
            db.Users.Add(new User { Id = ownerId, Email = ownerEmail, CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = orgId, Name = "RLPortalOrg", Slug = $"rlp-{orgId:N}"[..20],
                OwnerId = ownerId, Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId, UserId = ownerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
            });
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                StripeCustomerId = "cus_test_rl_portal",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var response = await client.PostAsJsonAsync("/api/v1/subscriptions/billing-portal", new { });

        response.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("stripe_rate_limited");
    }

    /// <summary>Behaviour #4 — STRIPE_UNAVAILABLE 502 with request_id preserved.</summary>
    [Fact]
    public async Task BillingPortal_with_stripe_5xx_returns_502_STRIPE_UNAVAILABLE_with_request_id()
    {
        await using var unavailableFactory = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IStripeGateway>();
                services.AddSingleton<IStripeGateway, ThrowingUnavailableFakeStripeGateway>();
            }));

        var ownerId = Guid.NewGuid();
        var ownerEmail = $"unavail-owner-{ownerId:N}@example.com";
        var token = TestTokens.Create(ownerId, ownerEmail);
        var client = unavailableFactory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        using (var scope = unavailableFactory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            var orgId = Guid.NewGuid();
            db.Users.Add(new User { Id = ownerId, Email = ownerEmail, CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = orgId, Name = "UnavailOrg", Slug = $"unavail-{orgId:N}"[..20],
                OwnerId = ownerId, Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId, UserId = ownerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
            });
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                StripeCustomerId = "cus_test_unavail",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var response = await client.PostAsJsonAsync("/api/v1/subscriptions/billing-portal", new { });
        response.StatusCode.Should().Be(HttpStatusCode.BadGateway);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("STRIPE_UNAVAILABLE");
        doc.RootElement.GetProperty("stripe_request_id").GetString().Should().Be("req_test_5xx_simulated");
    }

    // ── POST /api/v1/subscriptions/preview-proration ──────────────────────────

    [Fact]
    public async Task PreviewProration_returns_409_no_active_subscription_when_org_has_no_sub()
    {
        await CreateOrgAndGetIdAsync();

        var body = new { new_price_id = "price_test_team_monthly" };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/preview-proration", body);

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("no_active_subscription");
    }

    [Fact]
    public async Task PreviewProration_returns_400_invalid_price_id_for_unknown_price()
    {
        await CreateOrgAndGetIdAsync();
        var body = new { new_price_id = "price_unknown_thing" };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/preview-proration", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_price_id");
    }

    [Fact]
    public async Task PreviewProration_returns_200_with_amount_due_now_zero_for_no_op_swap()
    {
        await CreateOrgAndGetIdAsync();
        // Create a Team monthly subscription via the existing checkout path.
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", new
        {
            price_id = "price_test_team_monthly",
            success_url = "http://localhost/ok",
            cancel_url  = "http://localhost/no",
        });

        var body = new { new_price_id = "price_test_team_monthly" };
        var response = await _client.PostAsJsonAsync("/api/v1/subscriptions/preview-proration", body);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("amount_due_now").GetInt32().Should().Be(0);
        doc.RootElement.GetProperty("renewal_date").ValueKind.Should().NotBe(JsonValueKind.Null);
    }

    [Fact]
    public async Task PreviewProration_echoes_idempotency_key_header()
    {
        await CreateOrgAndGetIdAsync();
        var key = Guid.NewGuid().ToString();
        using var req = new HttpRequestMessage(HttpMethod.Post, "/api/v1/subscriptions/preview-proration")
        {
            Content = JsonContent.Create(new { new_price_id = "price_test_team_monthly" }),
        };
        req.Headers.Add("Idempotency-Key", key);

        var response = await _client.SendAsync(req);
        response.Headers.GetValues("X-Stripe-Idempotency-Key").Should().Contain(key);
    }

    /// <summary>Stripe rate-limit on preview-proration returns HTTP 503 stripe_rate_limited.</summary>
    [Fact]
    public async Task PreviewProration_with_stripe_rate_limit_returns_503()
    {
        await using var rateLimitFactory = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IStripeGateway>();
                services.AddSingleton<IStripeGateway, ThrowingRateLimitedProrationGateway>();
            }));

        var ownerId = Guid.NewGuid();
        var ownerEmail = $"rl-proration-{ownerId:N}@example.com";
        var token = TestTokens.Create(ownerId, ownerEmail);
        var client = rateLimitFactory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        using (var scope = rateLimitFactory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            var orgId = Guid.NewGuid();
            db.Users.Add(new User { Id = ownerId, Email = ownerEmail, CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = orgId, Name = "RLProrationOrg", Slug = $"rlpr-{orgId:N}"[..20],
                OwnerId = ownerId, Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId, UserId = ownerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
            });
            // Seed a Professional sub so the no_active_subscription guard doesn't trip.
            // Use different tier (Professional) so (tier,interval,seats) mismatch with the
            // requested price_test_team_monthly forces an actual gateway call.
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Professional, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 1, SeatLimit = 1,
                StripeSubscriptionId = "sub_test_rl_proration",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var response = await client.PostAsJsonAsync("/api/v1/subscriptions/preview-proration",
            new { new_price_id = "price_test_team_monthly" });

        response.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("stripe_rate_limited");
    }

    /// <summary>Stripe 5xx on preview-proration returns HTTP 502 STRIPE_UNAVAILABLE with request_id.</summary>
    [Fact]
    public async Task PreviewProration_with_stripe_5xx_returns_502_STRIPE_UNAVAILABLE()
    {
        await using var unavailableFactory = _factory.WithWebHostBuilder(b =>
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IStripeGateway>();
                services.AddSingleton<IStripeGateway, ThrowingUnavailableProrationGateway>();
            }));

        var ownerId = Guid.NewGuid();
        var ownerEmail = $"unavail-proration-{ownerId:N}@example.com";
        var token = TestTokens.Create(ownerId, ownerEmail);
        var client = unavailableFactory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        using (var scope = unavailableFactory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            var orgId = Guid.NewGuid();
            db.Users.Add(new User { Id = ownerId, Email = ownerEmail, CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = orgId, Name = "UnavailProrationOrg", Slug = $"uvpr-{orgId:N}"[..20],
                OwnerId = ownerId, Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId, UserId = ownerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
            });
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Professional, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 1, SeatLimit = 1,
                StripeSubscriptionId = "sub_test_unavail_proration",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var response = await client.PostAsJsonAsync("/api/v1/subscriptions/preview-proration",
            new { new_price_id = "price_test_team_monthly" });

        response.StatusCode.Should().Be(HttpStatusCode.BadGateway);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("STRIPE_UNAVAILABLE");
        doc.RootElement.GetProperty("stripe_request_id").GetString().Should().Be("req_test_proration_5xx");
    }
}

/// <summary>
/// Test double for <see cref="IStripeGateway"/> that always throws <see cref="StripeRateLimitedException"/>
/// on <see cref="CreateCheckoutSessionAsync"/>. Used to assert the HTTP 503 mapping in endpoint tests.
/// </summary>
internal sealed class ThrowingFakeStripeGateway : IStripeGateway
{
    public Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null, string? existingCustomerId = null, string? idempotencyKey = null,
        CancellationToken ct = default)
        => throw new StripeRateLimitedException(idempotencyKey, new Exception("simulated 429"));

    public Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null, string? idempotencyKey = null, CancellationToken ct = default)
        => throw new NotImplementedException();

    public ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats, string interval)
        => throw new NotImplementedException();

    public Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId, string newPriceId, int newQuantity, DateTimeOffset prorationDate,
        string? idempotencyKey = null, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default)
        => throw new NotImplementedException();
}

/// <summary>
/// Test double that throws <see cref="StripeUnavailableException"/> for portal calls.
/// Used to assert the HTTP 502 STRIPE_UNAVAILABLE mapping.
/// </summary>
internal sealed class ThrowingUnavailableFakeStripeGateway : IStripeGateway
{
    public Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null, string? existingCustomerId = null, string? idempotencyKey = null,
        CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null, string? idempotencyKey = null, CancellationToken ct = default)
        => throw new StripeUnavailableException(idempotencyKey, "req_test_5xx_simulated", new Exception("simulated 503"));

    public ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats, string interval)
        => throw new NotImplementedException();

    public Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId, string newPriceId, int newQuantity, DateTimeOffset prorationDate,
        string? idempotencyKey = null, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default)
        => throw new NotImplementedException();
}

/// <summary>
/// Test double that throws <see cref="StripeRateLimitedException"/> for portal calls.
/// Used to assert the HTTP 503 rate-limit mapping for the billing-portal endpoint.
/// </summary>
internal sealed class ThrowingRateLimitedPortalGateway : IStripeGateway
{
    public Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null, string? existingCustomerId = null, string? idempotencyKey = null,
        CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null, string? idempotencyKey = null, CancellationToken ct = default)
        => throw new StripeRateLimitedException(idempotencyKey, new Exception("simulated 429"));

    public ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats, string interval)
        => throw new NotImplementedException();

    public Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId, string newPriceId, int newQuantity, DateTimeOffset prorationDate,
        string? idempotencyKey = null, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default)
        => throw new NotImplementedException();
}

/// <summary>
/// Test double that throws <see cref="StripeRateLimitedException"/> on <see cref="ComputeProrationAsync"/>.
/// Used to assert the HTTP 503 rate-limit mapping for the preview-proration endpoint.
/// </summary>
internal sealed class ThrowingRateLimitedProrationGateway : IStripeGateway
{
    public Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null, string? existingCustomerId = null, string? idempotencyKey = null,
        CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null, string? idempotencyKey = null, CancellationToken ct = default)
        => throw new NotImplementedException();

    public ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats, string interval)
        => throw new NotImplementedException();

    public Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId, string newPriceId, int newQuantity, DateTimeOffset prorationDate,
        string? idempotencyKey = null, CancellationToken ct = default)
        => throw new StripeRateLimitedException(idempotencyKey, new Exception("simulated proration 429"));

    public Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default)
        => throw new NotImplementedException();
}

/// <summary>
/// Test double that throws <see cref="StripeUnavailableException"/> on <see cref="ComputeProrationAsync"/>.
/// Used to assert the HTTP 502 STRIPE_UNAVAILABLE mapping for the preview-proration endpoint.
/// </summary>
internal sealed class ThrowingUnavailableProrationGateway : IStripeGateway
{
    public Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null, string? existingCustomerId = null, string? idempotencyKey = null,
        CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null, string? idempotencyKey = null, CancellationToken ct = default)
        => throw new NotImplementedException();

    public ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats, string interval)
        => throw new NotImplementedException();

    public Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId, string newPriceId, int newQuantity, DateTimeOffset prorationDate,
        string? idempotencyKey = null, CancellationToken ct = default)
        => throw new StripeUnavailableException(idempotencyKey, "req_test_proration_5xx", new Exception("simulated proration 503"));

    public Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default)
        => throw new NotImplementedException();

    public Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default)
        => throw new NotImplementedException();
}
