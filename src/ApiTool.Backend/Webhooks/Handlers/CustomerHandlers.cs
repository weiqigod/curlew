// Refs docs/SPECIFICATION.md:6796–6804 (customer.updated event), :6845–6848 (re-fetch pattern).
using ApiTool.Backend.Data;
using ApiTool.Backend.Subscriptions;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Webhooks.Handlers;

/// <summary>
/// Handles <c>customer.updated</c> webhook events by re-fetching the Stripe Customer and
/// mirroring the email/metadata changes into the local <c>subscriptions.StripeCustomerEmail</c>
/// column. Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
/// </summary>
public sealed class StripeCustomerUpdatedHandler(
    AppDbContext db,
    IStripeGateway gateway,
    TimeProvider clock,
    ILogger<StripeCustomerUpdatedHandler> log)
{
    /// <summary>Re-fetches the Stripe Customer and mirrors the email into the local subscription row(s).</summary>
    public async Task HandleAsync(string customerId, CancellationToken ct)
    {
        var stripeCustomer = await gateway.GetCustomerAsync(customerId, ct);
        if (stripeCustomer is null)
        {
            log.LogInformation("stripe_customer_404 customer_id={CustomerId}", customerId);
            return;
        }

        var rows = await db.Subscriptions
            .Where(s => s.StripeCustomerId == customerId)
            .ToListAsync(ct);

        if (rows.Count == 0)
        {
            log.LogInformation(
                "stripe_customer_updated_no_local_row customer_id={CustomerId}", customerId);
            return;
        }

        var now = clock.GetUtcNow().UtcDateTime;
        foreach (var row in rows)
        {
            row.StripeCustomerEmail = stripeCustomer.Email;
            row.UpdatedAt = now;
        }
        await db.SaveChangesAsync(ct);
    }
}
