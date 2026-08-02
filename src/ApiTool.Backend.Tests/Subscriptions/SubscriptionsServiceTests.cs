using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Verifies SubscriptionsService business rules against in-memory SQLite.</summary>
public sealed class SubscriptionsServiceTests
{
    private static async Task<(TestDbScope scope, AppDbContext db, SubscriptionsService svc, Guid userId, Guid orgId)>
        BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = "owner@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Acme", Slug = $"acme-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
        var svc = new SubscriptionsService(db, new FakeStripeGateway(), TimeProvider.System, auditWriter);
        return (scope, db, svc, userId, orgId);
    }

    [Fact]
    public async Task Seat_count_equals_members_plus_pending_invitations()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;

            // Add a pending invitation.
            db.OrganizationInvitations.Add(new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "pending@example.com", EmailNormalized = "pending@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('a', 64),
                ExpiresAt = now.AddDays(7), CreatedAt = now, LastSentAt = now, SendCount = 1,
            });
            await db.SaveChangesAsync();

            var count = await svc.CountSeatsAsync(orgId, now, default);

            // 1 member (owner) + 1 pending invitation
            count.Should().Be(2);
        }
    }

    [Fact]
    public async Task CountSeatsAsync_excludes_expired_and_revoked_invitations()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;

            // Expired invitation (ExpiresAt in the past).
            db.OrganizationInvitations.Add(new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "expired@example.com", EmailNormalized = "expired@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('b', 64),
                ExpiresAt = now.AddDays(-1), CreatedAt = now.AddDays(-8), LastSentAt = now.AddDays(-8), SendCount = 1,
            });

            // Revoked invitation.
            db.OrganizationInvitations.Add(new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "revoked@example.com", EmailNormalized = "revoked@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('c', 64),
                ExpiresAt = now.AddDays(7), CreatedAt = now, LastSentAt = now, SendCount = 1,
                RevokedAt = now.AddHours(-1), RevokedBy = userId,
            });
            await db.SaveChangesAsync();

            var count = await svc.CountSeatsAsync(orgId, now, default);

            // Only the 1 owner member — both invitations are excluded.
            count.Should().Be(1);
        }
    }

    [Fact]
    public async Task CreateCheckoutByPriceAsync_with_unknown_price_id_returns_InvalidPriceId()
    {
        var (scope, _, svc, userId, _) = await BuildAsync();
        await using (scope)
        {
            var (url, _, err, _) = await svc.CreateCheckoutByPriceAsync(
                userId, "price_unknown_thing", "http://ok", "http://no",
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);
            err.Should().Be(SubscriptionError.InvalidPriceId);
            url.Should().BeEmpty();
        }
    }

    [Fact]
    public async Task CreateCheckoutByPriceAsync_succeeds_for_owner_with_allowed_price()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (url, sessionId, err, _) = await svc.CreateCheckoutByPriceAsync(
                userId, "price_test_team_monthly", "http://ok", "http://no",
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);
            err.Should().Be(SubscriptionError.None);
            url.Should().Contain("cs_test_");
            sessionId.Should().StartWith("cs_test_");

            // Persisted with the right tier.
            var sub = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            sub.Tier.Should().Be(SubscriptionTier.Team);
        }
    }

    [Fact]
    public async Task CreateCheckoutByPriceAsync_rejects_when_user_owns_no_org()
    {
        var (scope, db, _, _, _) = await BuildAsync();
        await using (scope)
        {
            var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
            var svc = new SubscriptionsService(db, new FakeStripeGateway(), TimeProvider.System, auditWriter);
            var (_, _, err, _) = await svc.CreateCheckoutByPriceAsync(
                Guid.NewGuid(), // unknown user
                "price_test_team_monthly", "http://ok", "http://no",
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);
            err.Should().Be(SubscriptionError.PermissionDenied);
        }
    }

    // ── CreateBillingPortalAsync ──────────────────────────────────────────────

    [Fact]
    public async Task CreateBillingPortalAsync_returns_NoBillingSetup_when_org_has_no_stripe_customer_id()
    {
        var (scope, _, svc, userId, _) = await BuildAsync();
        await using (scope)
        {
            // No subscription row exists for the org → no stripe_customer_id.
            var (url, err, _) = await svc.CreateBillingPortalAsync(
                userId, "https://app/billing", Guid.NewGuid().ToString(), default);

            err.Should().Be(SubscriptionError.NoBillingSetup);
            url.Should().BeEmpty();
        }
    }

    [Fact]
    public async Task CreateBillingPortalAsync_returns_PermissionDenied_for_member_role()
    {
        var (scope, db, _, _, orgId) = await BuildAsync();
        await using (scope)
        {
            // Add a Member-role user (not admin/owner).
            var memberId = Guid.NewGuid();
            db.Users.Add(new User { Id = memberId, Email = "member@example.com", CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId, UserId = memberId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
            var svc = new SubscriptionsService(db, new FakeStripeGateway(), TimeProvider.System, auditWriter);

            var (_, err, _) = await svc.CreateBillingPortalAsync(
                memberId, "https://app/billing", Guid.NewGuid().ToString(), default);

            err.Should().Be(SubscriptionError.PermissionDenied);
        }
    }

    [Fact]
    public async Task CreateBillingPortalAsync_succeeds_for_admin_role_with_customer_id()
    {
        var (scope, db, _, _, orgId) = await BuildAsync();
        await using (scope)
        {
            // Add an Admin-role user.
            var adminId = Guid.NewGuid();
            db.Users.Add(new User { Id = adminId, Email = "admin@example.com", CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId, UserId = adminId, Role = OrgRole.Admin, JoinedAt = DateTime.UtcNow,
            });
            // Seed a subscription with a customer id.
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                StripeCustomerId = "cus_test_seed",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
            var svc = new SubscriptionsService(db, new FakeStripeGateway(), TimeProvider.System, auditWriter);

            var (url, err, _) = await svc.CreateBillingPortalAsync(
                adminId, "https://app/billing", Guid.NewGuid().ToString(), default);

            err.Should().Be(SubscriptionError.None);
            url.Should().StartWith("https://billing.stripe.test/");
        }
    }

    [Fact]
    public async Task UpdateAsync_interval_only_change_emits_subscription_updated_audit_event()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Seed an active subscription.
            var subId = Guid.NewGuid();
            db.Subscriptions.Add(new Subscription
            {
                Id = subId, OrgId = orgId,
                Tier = SubscriptionTier.Team,
                Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            // Update interval only (month → year); no tier or seat change.
            var (sub, _, err, _) = await svc.UpdateAsync(userId, subId, null, null, "year", default);

            err.Should().Be(SubscriptionError.None);
            sub.Should().NotBeNull();

            // The audit log must have "subscription.updated" — not "upgraded" or "downgraded".
            var auditEntry = await db.OrganizationAuditLog
                .OrderByDescending(e => e.CreatedAt)
                .FirstAsync();
            auditEntry.EventType.Should().Be("subscription.updated");
        }
    }

    // ── PreviewProrationAsync ────────────────────────────────────────────────

    [Fact]
    public async Task PreviewProrationAsync_returns_NoActiveSubscription_when_org_has_no_sub()
    {
        var (scope, _, svc, userId, _) = await BuildAsync();
        await using (scope)
        {
            var (amount, renewal, credit, charge, err, msg) = await svc.PreviewProrationAsync(
                userId, "price_test_team_monthly", newSeatCount: 5,
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);

            err.Should().Be(SubscriptionError.NoActiveSubscription);
            amount.Should().Be(0);
            renewal.Should().BeNull();
        }
    }

    [Fact]
    public async Task PreviewProrationAsync_short_circuits_zero_when_new_price_equals_current()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Seed an active Team monthly 3-seat subscription.
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 3, SeatLimit = 3,
                StripeSubscriptionId = "sub_test_seed",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var (amount, renewal, _, _, err, _) = await svc.PreviewProrationAsync(
                userId, "price_test_team_monthly", newSeatCount: 3,
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);

            err.Should().Be(SubscriptionError.None);
            amount.Should().Be(0);
            renewal.Should().NotBeNull();
        }
    }

    [Fact]
    public async Task PreviewProrationAsync_returns_InvalidPriceId_for_unknown_price()
    {
        var (scope, _, svc, userId, _) = await BuildAsync();
        await using (scope)
        {
            var (_, _, _, _, err, _) = await svc.PreviewProrationAsync(
                userId, "price_unknown_thing", null,
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);
            err.Should().Be(SubscriptionError.InvalidPriceId);
        }
    }

    [Fact]
    public async Task PreviewProrationAsync_returns_PermissionDenied_for_user_with_no_owned_org()
    {
        var (scope, db, _, _, _) = await BuildAsync();
        await using (scope)
        {
            var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
            var svc = new SubscriptionsService(db, new FakeStripeGateway(), TimeProvider.System, auditWriter);
            var (_, _, _, _, err, _) = await svc.PreviewProrationAsync(
                Guid.NewGuid(), "price_test_team_monthly", 5,
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);
            err.Should().Be(SubscriptionError.PermissionDenied);
        }
    }

    // ── GetForOrgAsync tier resolution ───────────────────────────────────────

    [Fact]
    public async Task GetForOrgAsync_returns_free_when_subscription_is_canceled()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Canceled,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var (_, tier) = await svc.GetForOrgAsync(userId, orgId, default);

            tier.Should().Be("free");
        }
    }

    [Fact]
    public async Task GetForOrgAsync_returns_free_when_subscription_is_quarantined()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Quarantined,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var (_, tier) = await svc.GetForOrgAsync(userId, orgId, default);

            tier.Should().Be("free");
        }
    }

    [Fact]
    public async Task GetForOrgAsync_returns_team_when_subscription_is_active()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 5, SeatLimit = 5,
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var (_, tier) = await svc.GetForOrgAsync(userId, orgId, default);

            tier.Should().Be("team");
        }
    }

    /// <summary>
    /// Behaviour #1: when the new price differs from the current subscription (different tier),
    /// the service calls the gateway and returns the gateway's result. Verifies the non-short-circuit
    /// code path flows the gateway result through correctly.
    /// </summary>
    [Fact]
    public async Task PreviewProrationAsync_calls_gateway_and_returns_result_for_tier_change()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Seed a Professional monthly subscription — different tier than the target price_test_team_monthly.
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Professional, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 1, SeatLimit = 1,
                StripeSubscriptionId = "sub_test_pro_to_team",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            // Request upgrade to Team tier (different tier — short-circuit will NOT fire).
            var (amount, renewal, credit, charge, err, _) = await svc.PreviewProrationAsync(
                userId, "price_test_team_monthly", newSeatCount: 3,
                idempotencyKey: Guid.NewGuid().ToString(), ct: default);

            // FakeStripeGateway uses Free/0-baseline math: upgrading to Team/3 monthly yields
            // a positive net (4900 * 3 = 14700 cents charge, 0 credit). This proves the gateway
            // result flows through the service correctly — the assertion is non-trivially true.
            err.Should().Be(SubscriptionError.None);
            amount.Should().BeGreaterThan(0);
            charge.Should().BeGreaterThan(0);
            credit.Should().Be(0);
            // Fake gateway always returns a renewal date (~30 days out).
            renewal.Should().NotBeNull();
        }
    }
}
