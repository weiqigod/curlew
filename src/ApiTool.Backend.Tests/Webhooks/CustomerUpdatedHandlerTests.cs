// Refs docs/SPECIFICATION.md:6796–6804 (customer.updated event), :6845–6848 (re-fetch pattern).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>Unit tests for StripeCustomerUpdatedHandler behavior.</summary>
public sealed class CustomerUpdatedHandlerTests
{
    private static async Task<(TestDbScope scope, AppDbContext db, Guid userId, Guid orgId)>
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

        return (scope, db, userId, orgId);
    }

    private static StripeCustomerUpdatedHandler BuildHandler(AppDbContext db, IStripeGateway gateway)
        => new(db, gateway, TimeProvider.System, NullLogger<StripeCustomerUpdatedHandler>.Instance);

    // ── Test 9: Customer_updated_mirrors_email_into_subscription_row ─────────

    [Fact]
    public async Task Customer_updated_mirrors_email_into_subscription_row()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 3, SeatLimit = 3,
                StripeCustomerId = "cus_test_owner",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetCustomerForTest(new Stripe.Customer
            {
                Id = "cus_test_owner",
                Email = "new-billing@acme.com",
            });

            var handler = BuildHandler(db, gateway);
            await handler.HandleAsync("cus_test_owner", default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.StripeCustomerEmail.Should().Be("new-billing@acme.com");
        }
    }

    // ── Test 10: Customer_updated_with_unknown_customer_is_noop ─────────────

    [Fact]
    public async Task Customer_updated_with_unknown_customer_is_noop()
    {
        var (scope, db, _, _) = await SeedOrgAsync();
        await using (scope)
        {
            // No subscription rows at all.
            var gateway = new FakeStripeGateway();
            gateway.SetCustomerForTest(new Stripe.Customer { Id = "cus_unknown", Email = "x@y.com" });

            var handler = BuildHandler(db, gateway);

            // Should complete without throwing.
            var act = async () => await handler.HandleAsync("cus_unknown", default);
            await act.Should().NotThrowAsync();

            (await db.Subscriptions.CountAsync()).Should().Be(0);
        }
    }

    // ── Test Customer_updated_returns_when_stripe_customer_is_404 ────────────

    [Fact]
    public async Task Customer_updated_returns_when_stripe_customer_is_404()
    {
        // FakeStripeGateway has no customer registered for "cus_missing" →
        // GetCustomerAsync returns null (simulates Stripe 404). The handler
        // must log and return without throwing; no subscription rows are touched.
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            // Seed a subscription row so we can assert it is untouched.
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                Interval = "month", SeatCount = 3, SeatLimit = 3,
                StripeCustomerId = "cus_missing",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            // No customer scripted → GetCustomerAsync returns null.
            var gateway = new FakeStripeGateway();

            var handler = BuildHandler(db, gateway);

            var act = async () => await handler.HandleAsync("cus_missing", default);
            await act.Should().NotThrowAsync();

            // Subscription row must be untouched (StripeCustomerEmail still null).
            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.StripeCustomerEmail.Should().BeNull("handler returned early on customer 404");
        }
    }
}
