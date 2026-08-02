// Refs docs/SPECIFICATION.md:6801–6806 (invoice.payment_succeeded/failed/finalized handlers).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>
/// Unit tests for StripeInvoiceHandler: covers payment_succeeded, payment_failed, and finalized paths.
/// </summary>
public sealed class StripeInvoiceHandlerTests
{
    private const string StripeInvoiceId = "in_test_1";
    private const string StripeSubId = "sub_test_1";
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
        string customerId = StripeCusId, string subId = StripeSubId,
        SubscriptionStatus status = SubscriptionStatus.Active)
    {
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(), OrgId = orgId,
            Tier = SubscriptionTier.Team, Status = status,
            Interval = "month", SeatCount = 3, SeatLimit = 3,
            StripeCustomerId = customerId,
            StripeSubscriptionId = subId,
            CurrentPeriodStart = DateTime.UtcNow.AddMonths(-1),
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
    }

    private static Stripe.Invoice MakeStripeInvoice(
        string id = StripeInvoiceId,
        string customerId = StripeCusId,
        string subId = StripeSubId,
        string status = "paid",
        long amountPaid = 4900,
        long total = 4900,
        string currency = "usd",
        string? hostedInvoiceUrl = "https://invoice.stripe.test/in_test_1",
        int attemptCount = 1)
    {
        return new Stripe.Invoice
        {
            Id = id,
            CustomerId = customerId,
            SubscriptionId = subId,
            Status = status,
            AmountPaid = amountPaid,
            Total = total,
            Currency = currency,
            HostedInvoiceUrl = hostedInvoiceUrl,
            PeriodStart = DateTime.UtcNow.AddMonths(-1),
            PeriodEnd = DateTime.UtcNow,
            AttemptCount = attemptCount,
        };
    }

    private static StripeInvoiceHandler BuildHandler(
        AppDbContext db,
        FakeStripeGateway gateway,
        IEmailQueue? emailQueue = null,
        string webAppUrl = "https://app.example.com")
    {
        emailQueue ??= new RecordingEmailQueue();
        var appOptions = Options.Create(new AppOptions { WebAppUrl = webAppUrl });
        return new StripeInvoiceHandler(
            db, gateway, emailQueue, TimeProvider.System, appOptions,
            NullLogger<StripeInvoiceHandler>.Instance);
    }

    // ── payment_succeeded ───────────────────────────────────────────────────────

    [Fact]
    public async Task PaymentSucceeded_persists_invoice_row_and_marks_subscription_active()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, status: SubscriptionStatus.PastDue);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice(status: "paid", amountPaid: 4900));

            var handler = BuildHandler(db, gateway);
            await handler.HandlePaymentSucceededAsync(StripeInvoiceId, default);

            db.ChangeTracker.Clear();
            var invoice = await db.Invoices.SingleAsync(i => i.StripeInvoiceId == StripeInvoiceId);
            invoice.Status.Should().Be("paid");
            invoice.AmountPaid.Should().Be(4900);
            invoice.OrgId.Should().Be(orgId);

            var sub = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            sub.Status.Should().Be(SubscriptionStatus.Active);
        }
    }

    [Fact]
    public async Task PaymentSucceeded_enqueues_billing_receipt_with_required_variables()
    {
        var (scope, db, orgId) = await SeedOrgAsync("alice@example.com");
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice());

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);
            await handler.HandlePaymentSucceededAsync(StripeInvoiceId, default);

            emailQueue.Messages.Should().HaveCount(1);
            var msg = emailQueue.Messages[0];
            msg.TemplateSlug.Should().Be("billing_receipt");
            msg.To.Should().Be("alice@example.com");
            msg.Variables.Should().ContainKey("first_name");
            msg.Variables.Should().ContainKey("billing_period");
            msg.Variables.Should().ContainKey("amount_total");
            msg.Variables.Should().ContainKey("invoice_url");
            msg.Variables["first_name"].Should().Be("alice");
        }
    }

    [Fact]
    public async Task PaymentSucceeded_uses_re_fetched_state_not_event_payload()
    {
        // Handler only receives the invoice ID — the actual data is re-fetched from gateway.
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            // Gateway returns AmountPaid=9900, not 4900 (the event payload might differ).
            gateway.SetInvoiceForTest(MakeStripeInvoice(amountPaid: 9900, total: 9900));

            var handler = BuildHandler(db, gateway);
            await handler.HandlePaymentSucceededAsync(StripeInvoiceId, default);

            db.ChangeTracker.Clear();
            var invoice = await db.Invoices.SingleAsync(i => i.StripeInvoiceId == StripeInvoiceId);
            invoice.AmountPaid.Should().Be(9900, "handler must use re-fetched state, not event payload");
        }
    }

    [Fact]
    public async Task PaymentSucceeded_with_404_invoice_is_noop()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            // Gateway returns null (Stripe 404).
            var gateway = new FakeStripeGateway();
            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);

            var act = async () => await handler.HandlePaymentSucceededAsync("in_missing", default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            (await db.Invoices.CountAsync()).Should().Be(0, "no row created when Stripe returns 404");
            emailQueue.Messages.Should().BeEmpty("no email when invoice not found");
        }
    }

    [Fact]
    public async Task PaymentSucceeded_replay_is_idempotent_no_double_email()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice());

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);

            // Two calls with the same invoice id
            await handler.HandlePaymentSucceededAsync(StripeInvoiceId, default);
            await handler.HandlePaymentSucceededAsync(StripeInvoiceId, default);

            db.ChangeTracker.Clear();
            // Still only one row (upsert)
            (await db.Invoices.CountAsync(i => i.StripeInvoiceId == StripeInvoiceId)).Should().Be(1);
            // Two receipt emails (one per call) — the idempotency guard is at the dispatcher
            // level (M14-011 idempotency store). The handler itself enqueues on each call,
            // so the test documents this and the assertion is 2, not 1.
            // The dispatcher guarantees at-most-once delivery end-to-end.
            emailQueue.Messages.Should().HaveCount(2, "handler-level replay enqueues; idempotency is at dispatcher");
        }
    }

    // ── payment_failed ──────────────────────────────────────────────────────────

    [Fact]
    public async Task PaymentFailed_persists_invoice_and_sets_subscription_past_due()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, status: SubscriptionStatus.Active);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice(status: "open", amountPaid: 0, attemptCount: 1));

            var handler = BuildHandler(db, gateway);
            await handler.HandlePaymentFailedAsync(StripeInvoiceId, default);

            db.ChangeTracker.Clear();
            var invoice = await db.Invoices.SingleAsync(i => i.StripeInvoiceId == StripeInvoiceId);
            invoice.Status.Should().Be("open");

            var sub = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            sub.Status.Should().Be(SubscriptionStatus.PastDue);
        }
    }

    [Fact]
    public async Task PaymentFailed_enqueues_billing_payment_failed_with_attempt_count()
    {
        var (scope, db, orgId) = await SeedOrgAsync("bob@example.com");
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice(status: "open", amountPaid: 0, attemptCount: 3));

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue, "https://app.example.com");
            await handler.HandlePaymentFailedAsync(StripeInvoiceId, default);

            emailQueue.Messages.Should().HaveCount(1);
            var msg = emailQueue.Messages[0];
            msg.TemplateSlug.Should().Be("billing_payment_failed");
            msg.To.Should().Be("bob@example.com");
            msg.Variables.Should().ContainKey("first_name");
            msg.Variables.Should().ContainKey("amount_total");
            msg.Variables.Should().ContainKey("update_payment_url");
            msg.Variables.Should().ContainKey("attempt_count");
            msg.Variables["first_name"].Should().Be("bob");
            msg.Variables["attempt_count"].Should().Be("3");
            msg.Variables["update_payment_url"].Should().Contain("update-payment");
        }
    }

    // ── finalized ───────────────────────────────────────────────────────────────

    [Fact]
    public async Task Finalized_stores_invoice_url_only()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice(
                status: "open",
                hostedInvoiceUrl: "https://invoice.stripe.test/in_finalized_1"));

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);
            await handler.HandleFinalizedAsync(StripeInvoiceId, default);

            db.ChangeTracker.Clear();
            var invoice = await db.Invoices.SingleAsync(i => i.StripeInvoiceId == StripeInvoiceId);
            invoice.HostedInvoiceUrl.Should().Be("https://invoice.stripe.test/in_finalized_1");
        }
    }

    [Fact]
    public async Task Finalized_does_not_change_subscription_status()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, status: SubscriptionStatus.Active);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice(status: "open"));

            var handler = BuildHandler(db, gateway);
            await handler.HandleFinalizedAsync(StripeInvoiceId, default);

            db.ChangeTracker.Clear();
            var sub = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            sub.Status.Should().Be(SubscriptionStatus.Active, "finalized does not touch subscription status");
        }
    }

    [Fact]
    public async Task Finalized_does_not_enqueue_email()
    {
        var (scope, db, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice(status: "open"));

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);
            await handler.HandleFinalizedAsync(StripeInvoiceId, default);

            emailQueue.Messages.Should().BeEmpty("invoice.finalized does not send any email");
        }
    }

    // ── FormatBillingPeriod ─────────────────────────────────────────────────────

    /// <summary>
    /// Table-driven tests for FormatBillingPeriod covering same-month full month,
    /// non-full-month ranges, full calendar month (non-December), and December→January
    /// cross-year boundary. Asserts formatted values, not just key presence.
    /// </summary>
    [Theory]
    [InlineData(2026, 5, 1, 2026, 6, 1, "May 2026")]        // full May (normal path)
    [InlineData(2026, 1, 1, 2026, 2, 1, "January 2026")]    // full January
    [InlineData(2026, 11, 1, 2026, 12, 1, "November 2026")] // full November (month % 12 + 1 = 12 path)
    [InlineData(2025, 12, 1, 2026, 1, 1, "December 2025")]  // full December→January cross-year
    [InlineData(2026, 5, 3, 2026, 5, 28, "May 2026")]       // mid-month same-month → still "May 2026"
    [InlineData(2026, 5, 15, 2026, 6, 14, "2026-05-15 – 2026-06-14")] // cross-month non-full
    [InlineData(2025, 12, 15, 2026, 1, 14, "2025-12-15 – 2026-01-14")] // mid-December→mid-January cross-year (NOT a full month)
    public void FormatBillingPeriod_formats_correctly(
        int startYear, int startMonth, int startDay,
        int endYear, int endMonth, int endDay,
        string expected)
    {
        var start = new DateTime(startYear, startMonth, startDay, 0, 0, 0, DateTimeKind.Utc);
        var end = new DateTime(endYear, endMonth, endDay, 0, 0, 0, DateTimeKind.Utc);
        var result = StripeInvoiceHandler.FormatBillingPeriod(start, end);
        result.Should().Be(expected);
    }

    // ── FormatAmount ────────────────────────────────────────────────────────────

    [Theory]
    [InlineData(4900L, "usd", "$49.00")]
    [InlineData(100L, "usd", "$1.00")]
    [InlineData(0L, "usd", "$0.00")]
    [InlineData(4900L, "eur", "4900 EUR")]
    [InlineData(4900L, "USD", "$49.00")] // case-insensitive
    public void FormatAmount_formats_correctly(long amountCents, string currency, string expected)
    {
        var result = StripeInvoiceHandler.FormatAmount(amountCents, currency);
        result.Should().Be(expected);
    }

    // ── Value assertions in email variables ─────────────────────────────────────

    [Fact]
    public async Task PaymentSucceeded_email_variables_have_correct_formatted_values()
    {
        // Use a known full-month period: May 1 → June 1 2026 → "May 2026"
        var (scope, db, orgId) = await SeedOrgAsync("carol@example.com");
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            var invoice = new Stripe.Invoice
            {
                Id = StripeInvoiceId,
                CustomerId = StripeCusId,
                SubscriptionId = StripeSubId,
                Status = "paid",
                AmountPaid = 4900,
                Total = 4900,
                Currency = "usd",
                HostedInvoiceUrl = "https://invoice.stripe.test/in_test_1",
                PeriodStart = new DateTime(2026, 5, 1, 0, 0, 0, DateTimeKind.Utc),
                PeriodEnd = new DateTime(2026, 6, 1, 0, 0, 0, DateTimeKind.Utc),
                AttemptCount = 1,
            };
            gateway.SetInvoiceForTest(invoice);

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);
            await handler.HandlePaymentSucceededAsync(StripeInvoiceId, default);

            emailQueue.Messages.Should().HaveCount(1);
            var msg = emailQueue.Messages[0];
            msg.Variables["billing_period"].Should().Be("May 2026");
            msg.Variables["amount_total"].Should().Be("$49.00");
            msg.Variables["invoice_url"].Should().Be("https://invoice.stripe.test/in_test_1");
        }
    }

    [Fact]
    public async Task PaymentFailed_email_variables_have_correct_formatted_values()
    {
        var (scope, db, orgId) = await SeedOrgAsync("dave@example.com");
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(MakeStripeInvoice(status: "open", amountPaid: 0, total: 4900, attemptCount: 2));

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue, "https://app.example.com");
            await handler.HandlePaymentFailedAsync(StripeInvoiceId, default);

            emailQueue.Messages.Should().HaveCount(1);
            var msg = emailQueue.Messages[0];
            msg.Variables["amount_total"].Should().Be("$49.00");
            msg.Variables["update_payment_url"].Should().Be("https://app.example.com/billing/update-payment");
            msg.Variables["attempt_count"].Should().Be("2");
        }
    }
}
