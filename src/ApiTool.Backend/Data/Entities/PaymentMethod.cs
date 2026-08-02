namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// A Stripe payment method attached to an org's customer.
/// Soft-deleted via <see cref="DetachedAt"/> on payment_method.detached events
/// to preserve audit history and simplify idempotent replay.
/// Refs docs/SPECIFICATION.md:6801–6806 (payment_method handlers).
/// </summary>
public sealed class PaymentMethod
{
    /// <summary>Local surrogate primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>The organization this payment method belongs to.</summary>
    public Guid OrgId { get; set; }

    /// <summary>The Stripe payment method id (pm_*).</summary>
    public string StripePaymentMethodId { get; set; } = string.Empty;

    /// <summary>The Stripe customer id (cus_*) this payment method is attached to.</summary>
    public string StripeCustomerId { get; set; } = string.Empty;

    /// <summary>Card network brand (e.g. visa, mastercard, amex).</summary>
    public string Brand { get; set; } = string.Empty;

    /// <summary>Last 4 digits of the card number.</summary>
    public string Last4 { get; set; } = string.Empty;

    /// <summary>Card expiry month (1–12).</summary>
    public long ExpMonth { get; set; }

    /// <summary>Card expiry year (4-digit).</summary>
    public long ExpYear { get; set; }

    /// <summary>When the payment method was attached to the customer (UTC).</summary>
    public DateTime AttachedAt { get; set; }

    /// <summary>
    /// Soft-delete marker. Set to the detached timestamp when payment_method.detached fires.
    /// Null means the payment method is still active.
    /// </summary>
    public DateTime? DetachedAt { get; set; }

    /// <summary>Row creation timestamp (UTC).</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>Row last-updated timestamp (UTC).</summary>
    public DateTime UpdatedAt { get; set; }
}
