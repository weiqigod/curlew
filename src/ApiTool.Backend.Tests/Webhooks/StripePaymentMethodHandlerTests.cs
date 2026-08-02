// Refs docs/SPECIFICATION.md:6801–6806 (payment_method.attached/detached handlers).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>
/// Unit tests for StripePaymentMethodHandler: covers attached and detached event paths.
/// </summary>
public sealed class StripePaymentMethodHandlerTests
{
    private const string StripePmId = "pm_test_visa_1";
    private const string StripeCusId = "cus_test_owner";

    // ── Fixture helpers ─────────────────────────────────────────────────────────

    private static async Task<(TestDbScope scope, AppDbContext db, Guid orgId)>
        SeedOrgAsync(string ownerEmail = "owner@example.com")
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = ownerEmail, CreatedAt = DateTime.UtcNow });
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
        return (scope, db, orgId);
    }

    private static void SeedSubscription(AppDbContext db, Guid orgId,
        string customerId = StripeCusId)
    {
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(), OrgId = orgId,
            Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
            Interval = "month", SeatCount = 3, SeatLimit = 3,
            StripeCustomerId = customerId,
            StripeSubscriptionId = "sub_test_pm_1",
            CurrentPeriodStart = DateTime.UtcNow.AddMonths(-1),
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
    }

    private static Stripe.PaymentMethod MakeStripePaymentMethod(
        string id = StripePmId,
        string customerId = StripeCusId,
        string brand = "visa",
        string last4 = "4242",
        long expMonth = 12,
        long expYear = 2030)
    {
        return new Stripe.PaymentMethod
        {
            Id = id,
            CustomerId = customerId,
            Type = "card",
            Card = new Stripe.PaymentMethodCard
            {
                Brand = brand,
                Last4 = last4,
                ExpMonth = expMonth,
                ExpYear = expYear,
            },
        };
    }

    private static StripePaymentMethodHandler BuildHandler(AppDbContext db, FakeStripeGateway gateway)
    {
        return new StripePaymentMethodHandler(
            db, gateway, TimeProvider.System,
            NullLogger<StripePaymentMethodHandler>.Instance);
    }

    // ── attached ────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Attached_upserts_payment_method_with_brand_and_last4()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetPaymentMethodForTest(MakeStripePaymentMethod(brand: "mastercard", last4: "5555"));

            var handler = BuildHandler(db, gateway);
            await handler.HandleAttachedAsync(StripePmId, default);

            db.ChangeTracker.Clear();
            var pm = await db.PaymentMethods.SingleAsync(p => p.StripePaymentMethodId == StripePmId);
            pm.Brand.Should().Be("mastercard");
            pm.Last4.Should().Be("5555");
            pm.OrgId.Should().Be(orgId);
            pm.DetachedAt.Should().BeNull("freshly attached payment method has no detach date");
        }
    }

    [Fact]
    public async Task Attached_with_no_org_for_customer_logs_and_skips()
    {
        var (scope, db, _) = await SeedOrgAsync();
        await using (scope)
        {
            // No subscription row — customer not associated with any org.
            var gateway = new FakeStripeGateway();
            gateway.SetPaymentMethodForTest(MakeStripePaymentMethod(customerId: "cus_no_org"));

            var handler = BuildHandler(db, gateway);

            var act = async () => await handler.HandleAttachedAsync(StripePmId, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            (await db.PaymentMethods.CountAsync()).Should().Be(0, "no row when org not found");
        }
    }

    [Fact]
    public async Task Attached_with_404_payment_method_is_noop()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            // Gateway returns null (Stripe 404).
            var gateway = new FakeStripeGateway();
            var handler = BuildHandler(db, gateway);

            var act = async () => await handler.HandleAttachedAsync("pm_missing", default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            (await db.PaymentMethods.CountAsync()).Should().Be(0, "no row when Stripe returns 404");
        }
    }

    [Fact]
    public async Task Attached_replay_is_idempotent_no_duplicate_row()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetPaymentMethodForTest(MakeStripePaymentMethod());

            var handler = BuildHandler(db, gateway);

            // Two calls with the same payment method id
            await handler.HandleAttachedAsync(StripePmId, default);
            await handler.HandleAttachedAsync(StripePmId, default);

            db.ChangeTracker.Clear();
            (await db.PaymentMethods.CountAsync(p => p.StripePaymentMethodId == StripePmId)).Should().Be(1,
                "upsert prevents duplicate rows on replay");
        }
    }

    // ── detached ────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Detached_marks_existing_row_detached_at()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            // Seed a payment method row (as if attached earlier)
            db.PaymentMethods.Add(new PaymentMethod
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                StripePaymentMethodId = StripePmId,
                StripeCustomerId = StripeCusId,
                Brand = "visa", Last4 = "4242",
                ExpMonth = 12, ExpYear = 2030,
                AttachedAt = DateTime.UtcNow.AddMinutes(-5),
                DetachedAt = null,
                CreatedAt = DateTime.UtcNow.AddMinutes(-5),
                UpdatedAt = DateTime.UtcNow.AddMinutes(-5),
            });
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetPaymentMethodForTest(MakeStripePaymentMethod());

            var handler = BuildHandler(db, gateway);
            await handler.HandleDetachedAsync(StripePmId, default);

            db.ChangeTracker.Clear();
            var pm = await db.PaymentMethods.SingleAsync(p => p.StripePaymentMethodId == StripePmId);
            pm.DetachedAt.Should().NotBeNull("detach event sets DetachedAt");
        }
    }

    [Fact]
    public async Task Detached_with_no_local_row_is_noop()
    {
        var (scope, db, _) = await SeedOrgAsync();
        await using (scope)
        {
            // No payment method row in DB.
            var gateway = new FakeStripeGateway();
            var handler = BuildHandler(db, gateway);

            var act = async () => await handler.HandleDetachedAsync("pm_not_local", default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            (await db.PaymentMethods.CountAsync()).Should().Be(0);
        }
    }

    [Fact]
    public async Task Detached_does_not_physically_delete()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            db.PaymentMethods.Add(new PaymentMethod
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                StripePaymentMethodId = StripePmId,
                StripeCustomerId = StripeCusId,
                Brand = "visa", Last4 = "4242",
                ExpMonth = 12, ExpYear = 2030,
                AttachedAt = DateTime.UtcNow.AddMinutes(-5),
                DetachedAt = null,
                CreatedAt = DateTime.UtcNow.AddMinutes(-5),
                UpdatedAt = DateTime.UtcNow.AddMinutes(-5),
            });
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetPaymentMethodForTest(MakeStripePaymentMethod());

            var handler = BuildHandler(db, gateway);
            await handler.HandleDetachedAsync(StripePmId, default);

            db.ChangeTracker.Clear();
            // Row still exists (soft-delete)
            var pm = await db.PaymentMethods.SingleOrDefaultAsync(p => p.StripePaymentMethodId == StripePmId);
            pm.Should().NotBeNull("soft-delete preserves the row for audit");
            pm!.Brand.Should().Be("visa", "card data preserved after soft-delete");
        }
    }
}
