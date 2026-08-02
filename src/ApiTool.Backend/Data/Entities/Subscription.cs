namespace ApiTool.Backend.Data.Entities;

/// <summary>Represents an organization's billing subscription.</summary>
public sealed class Subscription
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>The organization this subscription belongs to.</summary>
    public Guid OrgId { get; set; }

    /// <summary>The pricing tier.</summary>
    public SubscriptionTier Tier { get; set; }

    /// <summary>The lifecycle status.</summary>
    public SubscriptionStatus Status { get; set; }

    /// <summary>Billing interval: <c>month</c> or <c>year</c>.</summary>
    public string Interval { get; set; } = "month";

    /// <summary>Number of seats currently being billed.</summary>
    public int SeatCount { get; set; }

    /// <summary>Maximum seats ceiling purchased.</summary>
    public int SeatLimit { get; set; }

    /// <summary>Start of the current billing period (UTC).</summary>
    public DateTime CurrentPeriodStart { get; set; }

    /// <summary>End of the current billing period (UTC).</summary>
    public DateTime CurrentPeriodEnd { get; set; }

    /// <summary>Whether the subscription cancels at the end of the current period.</summary>
    public bool CancelAtPeriodEnd { get; set; }

    /// <summary>Stripe customer identifier, if available.</summary>
    public string? StripeCustomerId { get; set; }

    /// <summary>Stripe subscription identifier, if available.</summary>
    public string? StripeSubscriptionId { get; set; }

    /// <summary>
    /// The email address on the Stripe Customer record, mirrored here by the
    /// <c>customer.updated</c> webhook handler. Nullable — not set until the first webhook arrives.
    /// Refs docs/SPECIFICATION.md:6796–6804 (customer.updated event).
    /// </summary>
    public string? StripeCustomerEmail { get; set; }

    /// <summary>UTC timestamp when the subscription was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp of the last update.</summary>
    public DateTime UpdatedAt { get; set; }

    /// <summary>UTC timestamp when the subscription was canceled, if applicable.</summary>
    public DateTime? CanceledAt { get; set; }
}
