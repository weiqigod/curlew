namespace ApiTool.Backend.Subscriptions;

/// <summary>Well-known error codes for subscription operations.</summary>
public enum SubscriptionError
{
    /// <summary>No error.</summary>
    None,

    /// <summary>The organization already has an active subscription.</summary>
    AlreadySubscribed,

    /// <summary>The requesting user is not authorized to perform this action.</summary>
    PermissionDenied,

    /// <summary>The organization was not found.</summary>
    OrganizationNotFound,

    /// <summary>The subscription was not found.</summary>
    SubscriptionNotFound,

    /// <summary>
    /// The requested seat count reduction would leave fewer seats than current active members
    /// plus pending invitations.
    /// </summary>
    DowngradeBlocked,

    /// <summary>The subscription is not in a state that allows reactivation.</summary>
    NotCancelable,

    /// <summary>The provided billing interval is not valid (must be "month" or "year").</summary>
    InvalidInterval,

    /// <summary>The provided subscription tier string is not a recognised tier value.</summary>
    InvalidTier,

    /// <summary>The provided Stripe price id is not in the documented allowlist.</summary>
    InvalidPriceId,

    /// <summary>The Stripe API returned a rate-limit (HTTP 429) response; the caller should retry with the same idempotency key.</summary>
    StripeRateLimited,

    /// <summary>
    /// The organization has no Stripe customer id on file; checkout must complete before
    /// billing portal access is possible. Maps to HTTP 409 with code <c>no_billing_setup</c>.
    /// </summary>
    NoBillingSetup,

    /// <summary>
    /// The organization has no active Stripe subscription on file; preview-proration cannot
    /// be computed because there is nothing to prorate. Maps to HTTP 409 with code
    /// <c>no_active_subscription</c>.
    /// </summary>
    NoActiveSubscription,
}
