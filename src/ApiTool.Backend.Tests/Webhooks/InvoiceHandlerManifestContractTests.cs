// Refs docs/SPECIFICATION.md:8941 (manifest contract enforced at send time).
// These tests prove that every variable name in the handler's EmailMessage payload
// appears in the on-disk manifest (and vice versa), making the contract a CI gate.
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
/// Validates that every variable name emitted by the invoice handlers is present
/// in the corresponding on-disk email template manifest. This is the manifest-contract
/// gate per behavior #6: "the handler fails its unit test when emitting a variable not
/// in the manifest."
/// </summary>
public sealed class InvoiceHandlerManifestContractTests
{
    // Navigate from test binary up to the repo root's templates/email/ folder.
    private static readonly string TemplatesRoot = Path.GetFullPath(
        Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "..", "templates", "email"));

    private static async Task<(TestDbScope scope, AppDbContext db, Guid orgId)> SeedAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = "manifest-test@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "ManifestOrg", Slug = $"mfst-{orgId:N}"[..20],
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
            StripeCustomerId = "cus_manifest_test",
            StripeSubscriptionId = "sub_manifest_test",
            CurrentPeriodStart = DateTime.UtcNow.AddMonths(-1),
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (scope, db, orgId);
    }

    private static StripeInvoiceHandler BuildHandler(AppDbContext db, FakeStripeGateway gateway, RecordingEmailQueue queue)
    {
        var appOptions = Options.Create(new AppOptions { WebAppUrl = "https://app.test" });
        return new StripeInvoiceHandler(
            db, gateway, queue, TimeProvider.System, appOptions,
            NullLogger<StripeInvoiceHandler>.Instance);
    }

    [Fact]
    public async Task BillingReceipt_emitted_variables_exactly_match_manifest()
    {
        var loader = new EmailTemplateLoader(TemplatesRoot);
        var manifest = loader.LoadManifest("billing_receipt");

        var (scope, db, _) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(new Stripe.Invoice
            {
                Id = "in_manifest_receipt_1",
                CustomerId = "cus_manifest_test",
                SubscriptionId = "sub_manifest_test",
                Status = "paid",
                AmountPaid = 4900,
                Total = 4900,
                Currency = "usd",
                HostedInvoiceUrl = "https://invoice.stripe.test/in_manifest_receipt_1",
                PeriodStart = new DateTime(2026, 5, 1, 0, 0, 0, DateTimeKind.Utc),
                PeriodEnd = new DateTime(2026, 6, 1, 0, 0, 0, DateTimeKind.Utc),
            });

            var queue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, queue);
            await handler.HandlePaymentSucceededAsync("in_manifest_receipt_1", default);

            queue.Messages.Should().HaveCount(1, "billing_receipt email must be enqueued");
            var msg = queue.Messages[0];
            msg.TemplateSlug.Should().Be("billing_receipt");

            // Every variable the handler emits must exist in the manifest.
            msg.Variables.Keys.Should().BeSubsetOf(
                manifest.Variables.Keys,
                "handler must not emit variables absent from the manifest");

            // Every required manifest variable must be present in the handler's output.
            manifest.Variables.Keys.Should().BeSubsetOf(
                msg.Variables.Keys,
                "handler must emit all variables declared in the manifest");
        }
    }

    [Fact]
    public async Task BillingPaymentFailed_emitted_variables_exactly_match_manifest()
    {
        var loader = new EmailTemplateLoader(TemplatesRoot);
        var manifest = loader.LoadManifest("billing_payment_failed");

        var (scope, db, _) = await SeedAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetInvoiceForTest(new Stripe.Invoice
            {
                Id = "in_manifest_failed_1",
                CustomerId = "cus_manifest_test",
                SubscriptionId = "sub_manifest_test",
                Status = "open",
                AmountPaid = 0,
                Total = 4900,
                Currency = "usd",
                PeriodStart = new DateTime(2026, 5, 1, 0, 0, 0, DateTimeKind.Utc),
                PeriodEnd = new DateTime(2026, 6, 1, 0, 0, 0, DateTimeKind.Utc),
                AttemptCount = 2,
            });

            var queue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, queue);
            await handler.HandlePaymentFailedAsync("in_manifest_failed_1", default);

            queue.Messages.Should().HaveCount(1, "billing_payment_failed email must be enqueued");
            var msg = queue.Messages[0];
            msg.TemplateSlug.Should().Be("billing_payment_failed");

            // Every variable the handler emits must exist in the manifest.
            msg.Variables.Keys.Should().BeSubsetOf(
                manifest.Variables.Keys,
                "handler must not emit variables absent from the manifest");

            // Every required manifest variable must be present in the handler's output.
            manifest.Variables.Keys.Should().BeSubsetOf(
                msg.Variables.Keys,
                "handler must emit all variables declared in the manifest");
        }
    }
}
