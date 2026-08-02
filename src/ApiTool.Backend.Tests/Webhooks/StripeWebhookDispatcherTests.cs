// Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks;
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>
/// Unit tests for StripeWebhookDispatcher: verifies routing to the correct handler
/// and graceful drop of unknown event types.
/// </summary>
public sealed class StripeWebhookDispatcherTests
{
    private static async Task<(TestDbScope scope, AppDbContext db, Guid orgId)> SeedAsync()
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
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(), OrgId = orgId,
            Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
            Interval = "month", SeatCount = 3, SeatLimit = 3,
            StripeCustomerId = "cus_disp_test",
            StripeSubscriptionId = "sub_disp_test",
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (scope, db, orgId);
    }

    private static StripeWebhookDispatcher BuildDispatcher(AppDbContext db, FakeStripeGateway gateway, IEmailQueue? emailQueue = null)
    {
        emailQueue ??= new RecordingEmailQueue();
        var subHandler = new StripeSubscriptionHandler(
            db, gateway, emailQueue, TimeProvider.System,
            NullLogger<StripeSubscriptionHandler>.Instance);
        var customerHandler = new StripeCustomerUpdatedHandler(
            db, gateway, TimeProvider.System,
            NullLogger<StripeCustomerUpdatedHandler>.Instance);
        var appOptions = Options.Create(new AppOptions { WebAppUrl = "https://app.test" });
        var invoiceHandler = new StripeInvoiceHandler(
            db, gateway, emailQueue, TimeProvider.System, appOptions,
            NullLogger<StripeInvoiceHandler>.Instance);
        var pmHandler = new StripePaymentMethodHandler(
            db, gateway, TimeProvider.System,
            NullLogger<StripePaymentMethodHandler>.Instance);
        return new StripeWebhookDispatcher(
            subHandler, customerHandler, invoiceHandler, pmHandler,
            NullLogger<StripeWebhookDispatcher>.Instance);
    }

    private static Stripe.Event MakeEvent(string type, string subId = "sub_disp_test")
    {
        var @event = new Stripe.Event
        {
            Type = type,
            Data = new Stripe.EventData
            {
                Object = new Stripe.Subscription { Id = subId, CustomerId = "cus_disp_test" },
            },
        };
        return @event;
    }

    private static Stripe.Event MakeCustomerEvent(string customerId = "cus_disp_test")
    {
        return new Stripe.Event
        {
            Type = Stripe.Events.CustomerUpdated,
            Data = new Stripe.EventData
            {
                Object = new Stripe.Customer { Id = customerId, Email = "updated@acme.com" },
            },
        };
    }

    [Fact]
    public async Task Routes_customer_subscription_created_to_subscription_handler()
    {
        var (scope, db, orgId) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(new Stripe.Subscription
            {
                Id = "sub_disp_test",
                CustomerId = "cus_disp_test",
                Status = "active",
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            });

            var dispatcher = BuildDispatcher(db, gateway);
            await dispatcher.DispatchAsync(MakeEvent(Stripe.Events.CustomerSubscriptionCreated), default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.StripeSubscriptionId.Should().Be("sub_disp_test");
        }
    }

    [Fact]
    public async Task Routes_customer_updated_to_customer_handler()
    {
        var (scope, db, orgId) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetCustomerForTest(new Stripe.Customer { Id = "cus_disp_test", Email = "updated@acme.com" });

            var dispatcher = BuildDispatcher(db, gateway);
            await dispatcher.DispatchAsync(MakeCustomerEvent(), default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.StripeCustomerEmail.Should().Be("updated@acme.com");
        }
    }

    [Fact]
    public async Task Ignores_unknown_event_type_without_throwing()
    {
        // Uses an event type that is defined by Stripe but not handled by the dispatcher.
        // payment_method.attached is now routed — use charge.dispute.created instead.
        var (scope, db, _) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            var dispatcher = BuildDispatcher(db, gateway);

            var unknownEvent = new Stripe.Event
            {
                Type = "charge.dispute.created",
                Data = new Stripe.EventData { Object = new Stripe.Charge { Id = "ch_test" } },
            };

            var act = async () => await dispatcher.DispatchAsync(unknownEvent, default);
            await act.Should().NotThrowAsync();
        }
    }

    // ── M14-013 routing tests ────────────────────────────────────────────────

    [Fact]
    public async Task Routes_invoice_payment_succeeded_to_invoice_handler()
    {
        var (scope, db, orgId) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(new Stripe.Invoice
            {
                Id = "in_disp_test_1",
                CustomerId = "cus_disp_test",
                SubscriptionId = "sub_disp_test",
                Status = "paid",
                AmountPaid = 4900,
                Total = 4900,
                Currency = "usd",
                PeriodStart = DateTime.UtcNow.AddMonths(-1),
                PeriodEnd = DateTime.UtcNow,
            });

            var dispatcher = BuildDispatcher(db, gateway);
            var invoiceEvent = new Stripe.Event
            {
                Type = Stripe.Events.InvoicePaymentSucceeded,
                Data = new Stripe.EventData { Object = new Stripe.Invoice { Id = "in_disp_test_1" } },
            };

            var act = async () => await dispatcher.DispatchAsync(invoiceEvent, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            // Invoice row should be persisted.
            var invoice = await db.Invoices.FirstOrDefaultAsync(i => i.StripeInvoiceId == "in_disp_test_1");
            invoice.Should().NotBeNull("payment_succeeded route writes invoice row");
        }
    }

    [Fact]
    public async Task Routes_invoice_payment_failed_to_invoice_handler()
    {
        var (scope, db, orgId) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(new Stripe.Invoice
            {
                Id = "in_disp_failed_1",
                CustomerId = "cus_disp_test",
                SubscriptionId = "sub_disp_test",
                Status = "open",
                Total = 4900,
                Currency = "usd",
                PeriodStart = DateTime.UtcNow.AddMonths(-1),
                PeriodEnd = DateTime.UtcNow,
            });

            var dispatcher = BuildDispatcher(db, gateway);
            var invoiceEvent = new Stripe.Event
            {
                Type = Stripe.Events.InvoicePaymentFailed,
                Data = new Stripe.EventData { Object = new Stripe.Invoice { Id = "in_disp_failed_1" } },
            };

            var act = async () => await dispatcher.DispatchAsync(invoiceEvent, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            var sub = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            sub.Status.Should().Be(SubscriptionStatus.PastDue, "payment_failed marks subscription past_due");
        }
    }

    [Fact]
    public async Task Routes_invoice_finalized_to_invoice_handler()
    {
        var (scope, db, orgId) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(new Stripe.Invoice
            {
                Id = "in_disp_final_1",
                CustomerId = "cus_disp_test",
                SubscriptionId = "sub_disp_test",
                Status = "open",
                Total = 4900,
                Currency = "usd",
                HostedInvoiceUrl = "https://invoice.stripe.test/in_disp_final_1",
                PeriodStart = DateTime.UtcNow.AddMonths(-1),
                PeriodEnd = DateTime.UtcNow,
            });

            var dispatcher = BuildDispatcher(db, gateway);
            var invoiceEvent = new Stripe.Event
            {
                Type = Stripe.Events.InvoiceFinalized,
                Data = new Stripe.EventData { Object = new Stripe.Invoice { Id = "in_disp_final_1" } },
            };

            var act = async () => await dispatcher.DispatchAsync(invoiceEvent, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            var invoice = await db.Invoices.FirstOrDefaultAsync(i => i.StripeInvoiceId == "in_disp_final_1");
            invoice.Should().NotBeNull();
            invoice!.HostedInvoiceUrl.Should().Be("https://invoice.stripe.test/in_disp_final_1");
        }
    }

    [Fact]
    public async Task Routes_payment_method_attached_to_payment_method_handler()
    {
        var (scope, db, _) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetPaymentMethodForTest(new Stripe.PaymentMethod
            {
                Id = "pm_disp_test_1",
                CustomerId = "cus_disp_test",
                Type = "card",
                Card = new Stripe.PaymentMethodCard { Brand = "visa", Last4 = "4242", ExpMonth = 12, ExpYear = 2030 },
            });

            var dispatcher = BuildDispatcher(db, gateway);
            var pmEvent = new Stripe.Event
            {
                Type = Stripe.Events.PaymentMethodAttached,
                Data = new Stripe.EventData { Object = new Stripe.PaymentMethod { Id = "pm_disp_test_1" } },
            };

            var act = async () => await dispatcher.DispatchAsync(pmEvent, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            var pm = await db.PaymentMethods.FirstOrDefaultAsync(p => p.StripePaymentMethodId == "pm_disp_test_1");
            pm.Should().NotBeNull("payment_method.attached route writes payment_methods row");
            pm!.Brand.Should().Be("visa");
        }
    }

    [Fact]
    public async Task Routes_payment_method_detached_to_payment_method_handler()
    {
        var (scope, db, orgId) = await SeedAsync();
        await using (scope)
        {
            // Pre-seed a payment method row.
            db.PaymentMethods.Add(new PaymentMethod
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                StripePaymentMethodId = "pm_disp_detach_1",
                StripeCustomerId = "cus_disp_test",
                Brand = "visa", Last4 = "4242", ExpMonth = 12, ExpYear = 2030,
                AttachedAt = DateTime.UtcNow.AddMinutes(-5),
                CreatedAt = DateTime.UtcNow.AddMinutes(-5),
                UpdatedAt = DateTime.UtcNow.AddMinutes(-5),
            });
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetPaymentMethodForTest(new Stripe.PaymentMethod { Id = "pm_disp_detach_1" });

            var dispatcher = BuildDispatcher(db, gateway);
            var pmEvent = new Stripe.Event
            {
                Type = Stripe.Events.PaymentMethodDetached,
                Data = new Stripe.EventData { Object = new Stripe.PaymentMethod { Id = "pm_disp_detach_1" } },
            };

            var act = async () => await dispatcher.DispatchAsync(pmEvent, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            var pm = await db.PaymentMethods.SingleAsync(p => p.StripePaymentMethodId == "pm_disp_detach_1");
            pm.DetachedAt.Should().NotBeNull("payment_method.detached route soft-deletes the row");
        }
    }
}
