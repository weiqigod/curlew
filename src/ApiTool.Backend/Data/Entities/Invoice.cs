namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// A Stripe invoice mirrored to the local DB by webhook handlers.
/// Refs docs/SPECIFICATION.md:6801–6806 (invoice.payment_succeeded handler), :6803 (store invoice URL).
/// </summary>
public sealed class Invoice
{
    /// <summary>Local surrogate primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>The organization this invoice belongs to.</summary>
    public Guid OrgId { get; set; }

    /// <summary>The Stripe invoice id (in_*).</summary>
    public string StripeInvoiceId { get; set; } = string.Empty;

    /// <summary>The Stripe customer id (cus_*) associated with this invoice.</summary>
    public string StripeCustomerId { get; set; } = string.Empty;

    /// <summary>The Stripe subscription id (sub_*), if this invoice is subscription-related.</summary>
    public string? StripeSubscriptionId { get; set; }

    /// <summary>Stripe invoice status: paid|open|void|uncollectible|draft.</summary>
    public string Status { get; set; } = string.Empty;

    /// <summary>Amount paid in smallest currency unit (cents for USD).</summary>
    public long AmountPaid { get; set; }

    /// <summary>Total invoice amount in smallest currency unit.</summary>
    public long AmountTotal { get; set; }

    /// <summary>ISO 4217 currency code (3 chars, e.g. "usd").</summary>
    public string Currency { get; set; } = "usd";

    /// <summary>Stripe-hosted invoice PDF/page URL, null until invoice is finalized.</summary>
    public string? HostedInvoiceUrl { get; set; }

    /// <summary>Billing period start (UTC).</summary>
    public DateTime PeriodStart { get; set; }

    /// <summary>Billing period end (UTC).</summary>
    public DateTime PeriodEnd { get; set; }

    /// <summary>Number of payment attempts Stripe has made for this invoice.</summary>
    public int AttemptCount { get; set; }

    /// <summary>Row creation timestamp (UTC).</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>Row last-updated timestamp (UTC).</summary>
    public DateTime UpdatedAt { get; set; }
}
