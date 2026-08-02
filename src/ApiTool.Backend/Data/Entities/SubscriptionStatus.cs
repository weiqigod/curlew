namespace ApiTool.Backend.Data.Entities;

/// <summary>The lifecycle status of a subscription.</summary>
public enum SubscriptionStatus
{
    /// <summary>Subscription is current and paid.</summary>
    Active,

    /// <summary>Payment is overdue but the subscription is still active.</summary>
    PastDue,

    /// <summary>Subscription has been explicitly canceled.</summary>
    Canceled,

    /// <summary>Subscription has passed its period without renewal.</summary>
    Expired,

    /// <summary>Subscription has been created but payment not yet confirmed.</summary>
    Incomplete,

    /// <summary>
    /// Stripe reported the underlying subscription as deleted (404) and the local row could
    /// not be reconciled. Manual investigation required; retries are halted.
    /// Refs docs/SPECIFICATION.md:6843, :6845–6848.
    /// </summary>
    Quarantined,
}
