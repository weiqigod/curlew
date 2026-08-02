// Refs docs/SPECIFICATION.md:6801–6806 (invoice.payment_succeeded/failed/finalized, payment_method handlers).
// :8941 (manifest contract enforced at send time by SendGridSmtpSender).
// Handlers re-fetch the underlying object from Stripe — never trusting the event payload's
// snapshot, which is order-dependent. Idempotency is provided upstream by StripeWebhookStore.
using System.Globalization;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Subscriptions;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Webhooks.Handlers;

/// <summary>
/// Handles <c>invoice.payment_succeeded</c>, <c>invoice.payment_failed</c>, and
/// <c>invoice.finalized</c> Stripe webhook events by re-fetching the invoice from Stripe,
/// persisting the result to the local <c>invoices</c> table, updating subscription status,
/// and enqueuing the appropriate transactional email.
/// Refs docs/SPECIFICATION.md:6801–6806, :6845–6848 (re-fetch pattern).
/// </summary>
public sealed class StripeInvoiceHandler(
    AppDbContext db,
    IStripeGateway gateway,
    IEmailQueue emailQueue,
    TimeProvider clock,
    IOptions<AppOptions> appOptions,
    ILogger<StripeInvoiceHandler> log)
{
    /// <summary>
    /// Handles <c>invoice.payment_succeeded</c>: upserts invoice row, marks subscription Active,
    /// and enqueues a <c>billing_receipt</c> email to the org owner.
    /// </summary>
    public async Task HandlePaymentSucceededAsync(string invoiceId, CancellationToken ct)
    {
        var stripeInvoice = await gateway.GetInvoiceAsync(invoiceId, ct);
        if (stripeInvoice is null)
        {
            log.LogWarning("stripe_invoice_not_found invoice_id={InvoiceId} event=payment_succeeded", invoiceId);
            return;
        }

        var local = await UpsertInvoiceFromStripeAsync(stripeInvoice, ct);
        if (local is null) return; // org not found, already logged

        // Mark subscription Active on successful payment.
        var sub = await FindSubscriptionAsync(stripeInvoice, ct);
        if (sub is not null)
        {
            sub.Status = SubscriptionStatus.Active;
            sub.UpdatedAt = clock.GetUtcNow().UtcDateTime;
        }

        await EnqueueReceiptAsync(local, stripeInvoice, ct);
        await db.SaveChangesAsync(ct);
    }

    /// <summary>
    /// Handles <c>invoice.payment_failed</c>: upserts invoice row, marks subscription PastDue,
    /// and enqueues a <c>billing_payment_failed</c> email.
    /// </summary>
    public async Task HandlePaymentFailedAsync(string invoiceId, CancellationToken ct)
    {
        var stripeInvoice = await gateway.GetInvoiceAsync(invoiceId, ct);
        if (stripeInvoice is null)
        {
            log.LogWarning("stripe_invoice_not_found invoice_id={InvoiceId} event=payment_failed", invoiceId);
            return;
        }

        var local = await UpsertInvoiceFromStripeAsync(stripeInvoice, ct);
        if (local is null) return;

        // Mark subscription PastDue — dunning grace timer is data-only in M14 (M16 adds cron).
        var sub = await FindSubscriptionAsync(stripeInvoice, ct);
        if (sub is not null)
        {
            sub.Status = SubscriptionStatus.PastDue;
            sub.UpdatedAt = clock.GetUtcNow().UtcDateTime;
        }

        await EnqueuePaymentFailedAsync(local, stripeInvoice, ct);
        await db.SaveChangesAsync(ct);
    }

    /// <summary>
    /// Handles <c>invoice.finalized</c>: upserts invoice row (persisting <c>HostedInvoiceUrl</c>).
    /// No email, no subscription status change.
    /// </summary>
    public async Task HandleFinalizedAsync(string invoiceId, CancellationToken ct)
    {
        var stripeInvoice = await gateway.GetInvoiceAsync(invoiceId, ct);
        if (stripeInvoice is null)
        {
            log.LogWarning("stripe_invoice_not_found invoice_id={InvoiceId} event=finalized", invoiceId);
            return;
        }

        await UpsertInvoiceFromStripeAsync(stripeInvoice, ct);
        await db.SaveChangesAsync(ct);
    }

    // ── Private helpers ─────────────────────────────────────────────────────────

    /// <summary>
    /// Upserts a local <see cref="Invoice"/> row from the re-fetched Stripe invoice.
    /// Returns null (and logs) when no org can be resolved for the invoice's customer id.
    /// </summary>
    private async Task<Invoice?> UpsertInvoiceFromStripeAsync(Stripe.Invoice stripe, CancellationToken ct)
    {
        // Resolve OrgId via subscription or customer id.
        var orgId = await ResolveOrgIdAsync(stripe.SubscriptionId, stripe.CustomerId, ct);
        if (orgId is null)
        {
            log.LogWarning(
                "stripe_invoice_no_org invoice_id={InvoiceId} customer_id={CustomerId} — skipping upsert",
                stripe.Id, stripe.CustomerId);
            return null;
        }

        var now = clock.GetUtcNow().UtcDateTime;
        var existing = await db.Invoices.FirstOrDefaultAsync(i => i.StripeInvoiceId == stripe.Id, ct);

        if (existing is null)
        {
            existing = new Invoice
            {
                Id = Guid.NewGuid(),
                OrgId = orgId.Value,
                StripeInvoiceId = stripe.Id,
                CreatedAt = now,
            };
            db.Invoices.Add(existing);
        }

        existing.StripeCustomerId = stripe.CustomerId ?? string.Empty;
        existing.StripeSubscriptionId = stripe.SubscriptionId;
        existing.Status = stripe.Status ?? string.Empty;
        existing.AmountPaid = stripe.AmountPaid;
        existing.AmountTotal = stripe.Total;
        existing.Currency = stripe.Currency ?? "usd";
        existing.HostedInvoiceUrl = stripe.HostedInvoiceUrl;
        existing.PeriodStart = stripe.PeriodStart;
        existing.PeriodEnd = stripe.PeriodEnd;
        existing.AttemptCount = (int)stripe.AttemptCount;
        existing.UpdatedAt = now;

        return existing;
    }

    private async Task<Guid?> ResolveOrgIdAsync(string? stripeSubscriptionId, string? stripeCustomerId, CancellationToken ct)
    {
        if (!string.IsNullOrEmpty(stripeSubscriptionId))
        {
            var sub = await db.Subscriptions
                .Where(s => s.StripeSubscriptionId == stripeSubscriptionId)
                .Select(s => (Guid?)s.OrgId)
                .FirstOrDefaultAsync(ct);
            if (sub.HasValue) return sub;
        }

        if (!string.IsNullOrEmpty(stripeCustomerId))
        {
            var sub = await db.Subscriptions
                .Where(s => s.StripeCustomerId == stripeCustomerId)
                .Select(s => (Guid?)s.OrgId)
                .FirstOrDefaultAsync(ct);
            if (sub.HasValue) return sub;
        }

        return null;
    }

    private async Task<Subscription?> FindSubscriptionAsync(Stripe.Invoice stripe, CancellationToken ct)
    {
        if (!string.IsNullOrEmpty(stripe.SubscriptionId))
        {
            var sub = await db.Subscriptions
                .FirstOrDefaultAsync(s => s.StripeSubscriptionId == stripe.SubscriptionId, ct);
            if (sub is not null) return sub;
        }

        if (!string.IsNullOrEmpty(stripe.CustomerId))
        {
            return await db.Subscriptions
                .FirstOrDefaultAsync(s => s.StripeCustomerId == stripe.CustomerId, ct);
        }

        return null;
    }

    private async Task EnqueueReceiptAsync(Invoice local, Stripe.Invoice stripe, CancellationToken ct)
    {
        var ownerEmail = await FindOwnerEmailAsync(local.OrgId, ct);
        if (ownerEmail is null)
        {
            log.LogWarning(
                "stripe_invoice_payment_succeeded_no_owner org_id={OrgId} invoice_id={InvoiceId} — receipt not sent",
                local.OrgId, local.StripeInvoiceId);
            return;
        }

        var firstName = ExtractFirstName(ownerEmail);
        var invoiceUrl = local.HostedInvoiceUrl
            ?? $"{appOptions.Value.WebAppUrl}/billing/invoices/{stripe.Id}";

        await emailQueue.EnqueueAsync(new EmailMessage(
            To: ownerEmail,
            TemplateSlug: "billing_receipt",
            Variables: new Dictionary<string, string>
            {
                ["first_name"] = firstName,
                ["billing_period"] = FormatBillingPeriod(stripe.PeriodStart, stripe.PeriodEnd),
                ["amount_total"] = FormatAmount(stripe.Total, stripe.Currency ?? "usd"),
                ["invoice_url"] = invoiceUrl,
            },
            EnqueuedAt: clock.GetUtcNow()), ct);
    }

    private async Task EnqueuePaymentFailedAsync(Invoice local, Stripe.Invoice stripe, CancellationToken ct)
    {
        var ownerEmail = await FindOwnerEmailAsync(local.OrgId, ct);
        if (ownerEmail is null)
        {
            log.LogWarning(
                "stripe_invoice_payment_failed_no_owner org_id={OrgId} invoice_id={InvoiceId} — email not sent",
                local.OrgId, local.StripeInvoiceId);
            return;
        }

        var firstName = ExtractFirstName(ownerEmail);
        var updatePaymentUrl = $"{appOptions.Value.WebAppUrl}/billing/update-payment";
        var attemptCount = (int)stripe.AttemptCount;

        await emailQueue.EnqueueAsync(new EmailMessage(
            To: ownerEmail,
            TemplateSlug: "billing_payment_failed",
            Variables: new Dictionary<string, string>
            {
                ["first_name"] = firstName,
                ["amount_total"] = FormatAmount(stripe.Total, stripe.Currency ?? "usd"),
                ["update_payment_url"] = updatePaymentUrl,
                ["attempt_count"] = attemptCount.ToString(CultureInfo.InvariantCulture),
            },
            EnqueuedAt: clock.GetUtcNow()), ct);
    }

    private async Task<string?> FindOwnerEmailAsync(Guid orgId, CancellationToken ct)
    {
        return await db.Users
            .Where(u => u.Id == db.OrganizationMembers
                .Where(m => m.OrgId == orgId && m.Role == OrgRole.Owner)
                .Select(m => m.UserId)
                .FirstOrDefault())
            .Select(u => u.Email)
            .FirstOrDefaultAsync(ct);
    }

    /// <summary>
    /// Formats the billing period as a human-readable string.
    /// Single calendar months: "May 2026". Date ranges: "2026-05-01 – 2026-05-15".
    /// </summary>
    internal static string FormatBillingPeriod(DateTime start, DateTime end)
    {
        // Check if this is a full calendar month (first day to first day of next month).
        // The date-boundary guard (start/end == first of their respective months) MUST apply
        // to both the normal-month and the December→January cross-year sub-expressions.
        // Without explicit parentheses around the || operands, && binds tighter and the
        // cross-year sub-expression escapes the boundary guard (operator-precedence bug).
        var startMonthFirst = new DateTime(start.Year, start.Month, 1);
        var endMonthFirst = new DateTime(end.Year, end.Month, 1);
        if (start.Date == startMonthFirst && end.Date == endMonthFirst
            && (end.Month == (start.Month % 12) + 1
                || (start.Month == 12 && end.Month == 1 && end.Year == start.Year + 1)))
        {
            return start.ToString("MMMM yyyy", CultureInfo.GetCultureInfo("en-US"));
        }

        // Single month (first to last day of same month)
        if (start.Year == end.Year && start.Month == end.Month)
        {
            return start.ToString("MMMM yyyy", CultureInfo.GetCultureInfo("en-US"));
        }

        return $"{start:yyyy-MM-dd} – {end:yyyy-MM-dd}";
    }

    /// <summary>
    /// Formats an amount in the smallest currency unit (cents for USD).
    /// USD: "$49.00". Others: "4900 EUR".
    /// </summary>
    internal static string FormatAmount(long amountCents, string currency)
    {
        if (string.Equals(currency, "usd", StringComparison.OrdinalIgnoreCase))
        {
            var dollars = amountCents / 100m;
            return dollars.ToString("$0.00", CultureInfo.InvariantCulture);
        }

        return $"{amountCents} {currency.ToUpperInvariant()}";
    }

    private static string ExtractFirstName(string email)
    {
        var at = email.IndexOf('@', StringComparison.Ordinal);
        return at > 0 ? email[..at] : email;
    }
}

/// <summary>
/// Handles <c>payment_method.attached</c> and <c>payment_method.detached</c> Stripe webhook
/// events by re-fetching the payment method from Stripe and upserting/soft-deleting the local row.
/// Refs docs/SPECIFICATION.md:6801–6806 (payment_method handlers).
/// </summary>
public sealed class StripePaymentMethodHandler(
    AppDbContext db,
    IStripeGateway gateway,
    TimeProvider clock,
    ILogger<StripePaymentMethodHandler> log)
{
    /// <summary>
    /// Handles <c>payment_method.attached</c>: re-fetches the payment method from Stripe,
    /// resolves the org via the customer's subscription row, and upserts the local row.
    /// Only card-type payment methods are persisted; non-card types are logged and skipped.
    /// </summary>
    public async Task HandleAttachedAsync(string paymentMethodId, CancellationToken ct)
    {
        var stripePm = await gateway.GetPaymentMethodAsync(paymentMethodId, ct);
        if (stripePm is null)
        {
            log.LogWarning("stripe_payment_method_not_found pm_id={PmId} event=attached", paymentMethodId);
            return;
        }

        if (stripePm.Card is null)
        {
            log.LogWarning(
                "stripe_payment_method_unsupported_type pm_id={PmId} type={Type} — only card type supported",
                paymentMethodId, stripePm.Type);
            return;
        }

        var customerId = stripePm.CustomerId;
        if (string.IsNullOrEmpty(customerId))
        {
            log.LogWarning("stripe_payment_method_no_customer pm_id={PmId} — skipping", paymentMethodId);
            return;
        }

        var orgId = await db.Subscriptions
            .Where(s => s.StripeCustomerId == customerId)
            .Select(s => (Guid?)s.OrgId)
            .FirstOrDefaultAsync(ct);

        if (orgId is null)
        {
            log.LogWarning(
                "stripe_payment_method_no_org pm_id={PmId} customer_id={CustomerId} — skipping",
                paymentMethodId, customerId);
            return;
        }

        var now = clock.GetUtcNow().UtcDateTime;
        var existing = await db.PaymentMethods
            .FirstOrDefaultAsync(pm => pm.StripePaymentMethodId == paymentMethodId, ct);

        if (existing is null)
        {
            existing = new PaymentMethod
            {
                Id = Guid.NewGuid(),
                OrgId = orgId.Value,
                StripePaymentMethodId = paymentMethodId,
                CreatedAt = now,
            };
            db.PaymentMethods.Add(existing);
        }

        existing.StripeCustomerId = customerId;
        existing.Brand = stripePm.Card.Brand ?? string.Empty;
        existing.Last4 = stripePm.Card.Last4 ?? string.Empty;
        existing.ExpMonth = stripePm.Card.ExpMonth;
        existing.ExpYear = stripePm.Card.ExpYear;
        existing.AttachedAt = now;
        existing.DetachedAt = null; // clear soft-delete on re-attach
        existing.UpdatedAt = now;

        await db.SaveChangesAsync(ct);
    }

    /// <summary>
    /// Handles <c>payment_method.detached</c>: soft-deletes the local row by setting
    /// <see cref="PaymentMethod.DetachedAt"/>. Does not physically remove the row for audit reasons.
    /// </summary>
    public async Task HandleDetachedAsync(string paymentMethodId, CancellationToken ct)
    {
        // Re-fetch — Stripe still serves a recently-detached PM for a brief window.
        // We don't use the result here, but the re-fetch pattern is required by spec.
        _ = await gateway.GetPaymentMethodAsync(paymentMethodId, ct);

        var existing = await db.PaymentMethods
            .FirstOrDefaultAsync(pm => pm.StripePaymentMethodId == paymentMethodId, ct);

        if (existing is null)
        {
            log.LogInformation(
                "stripe_payment_method_detached_no_local_row pm_id={PmId} — nothing to soft-delete",
                paymentMethodId);
            return;
        }

        var now = clock.GetUtcNow().UtcDateTime;
        existing.DetachedAt = now;
        existing.UpdatedAt = now;
        await db.SaveChangesAsync(ct);
    }
}
