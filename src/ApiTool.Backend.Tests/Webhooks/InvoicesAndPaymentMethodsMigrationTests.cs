// Refs docs/SPECIFICATION.md:6801–6806 (invoice + payment_method tables).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>
/// Migration smoke tests: verifies that the invoices and payment_methods tables
/// are created with the expected columns and soft-delete semantics.
/// </summary>
public sealed class InvoicesAndPaymentMethodsMigrationTests
{
    [Fact]
    public async Task Migration_creates_invoices_table_with_expected_columns()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var columns = await scope.Db.Database
            .SqlQueryRaw<string>("SELECT name FROM pragma_table_info('invoices')")
            .ToListAsync();

        columns.Should().Contain("StripeInvoiceId");
        columns.Should().Contain("HostedInvoiceUrl");
        columns.Should().Contain("OrgId");
        columns.Should().Contain("Status");
        columns.Should().Contain("AmountPaid");
        columns.Should().Contain("AmountTotal");
        columns.Should().Contain("Currency");
        columns.Should().Contain("AttemptCount");
    }

    [Fact]
    public async Task Migration_creates_payment_methods_table_with_soft_delete_column()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var columns = await scope.Db.Database
            .SqlQueryRaw<string>("SELECT name FROM pragma_table_info('payment_methods')")
            .ToListAsync();

        columns.Should().Contain("Last4");
        columns.Should().Contain("DetachedAt");
        columns.Should().Contain("Brand");
        columns.Should().Contain("StripePaymentMethodId");
        columns.Should().Contain("OrgId");
    }

    [Fact]
    public async Task Invoice_round_trips_through_db()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = "invoice-test@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "InvOrg", Slug = $"invorg-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var invoiceId = Guid.NewGuid();
        db.Invoices.Add(new Invoice
        {
            Id = invoiceId,
            OrgId = orgId,
            StripeInvoiceId = "in_round_trip_1",
            StripeCustomerId = "cus_rt_1",
            StripeSubscriptionId = "sub_rt_1",
            Status = "paid",
            AmountPaid = 4900,
            AmountTotal = 4900,
            Currency = "usd",
            HostedInvoiceUrl = "https://invoice.stripe.test/in_rt_1",
            PeriodStart = new DateTime(2026, 5, 1, 0, 0, 0, DateTimeKind.Utc),
            PeriodEnd = new DateTime(2026, 6, 1, 0, 0, 0, DateTimeKind.Utc),
            AttemptCount = 1,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        db.ChangeTracker.Clear();
        var loaded = await db.Invoices.SingleAsync(i => i.StripeInvoiceId == "in_round_trip_1");
        loaded.Status.Should().Be("paid");
        loaded.HostedInvoiceUrl.Should().Be("https://invoice.stripe.test/in_rt_1");
        loaded.AmountPaid.Should().Be(4900);
        loaded.OrgId.Should().Be(orgId);
    }

    [Fact]
    public async Task PaymentMethod_round_trips_with_soft_delete()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = "pm-test@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "PmOrg", Slug = $"pmorg-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var pmId = Guid.NewGuid();
        var attachedAt = DateTime.UtcNow;
        db.PaymentMethods.Add(new PaymentMethod
        {
            Id = pmId,
            OrgId = orgId,
            StripePaymentMethodId = "pm_rt_1",
            StripeCustomerId = "cus_rt_1",
            Brand = "visa",
            Last4 = "4242",
            ExpMonth = 12,
            ExpYear = 2030,
            AttachedAt = attachedAt,
            DetachedAt = null,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        // Soft-delete the payment method
        db.ChangeTracker.Clear();
        var loaded = await db.PaymentMethods.SingleAsync(pm => pm.StripePaymentMethodId == "pm_rt_1");
        loaded.DetachedAt = DateTime.UtcNow;
        loaded.UpdatedAt = DateTime.UtcNow;
        await db.SaveChangesAsync();

        db.ChangeTracker.Clear();
        var updated = await db.PaymentMethods.SingleAsync(pm => pm.StripePaymentMethodId == "pm_rt_1");
        updated.Brand.Should().Be("visa");
        updated.Last4.Should().Be("4242");
        updated.DetachedAt.Should().NotBeNull("soft-delete sets DetachedAt, does not remove the row");
    }
}
