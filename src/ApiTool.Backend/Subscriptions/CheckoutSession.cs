namespace ApiTool.Backend.Subscriptions;

/// <summary>Result from creating a Stripe Checkout session.</summary>
/// <param name="SessionId">The Stripe session identifier (e.g. <c>cs_&lt;guid&gt;</c>).</param>
/// <param name="CheckoutUrl">The URL to redirect the user to for payment.</param>
/// <param name="CustomerId">The Stripe customer id, if one was created or reused during checkout.</param>
public sealed record CheckoutSession(string SessionId, string CheckoutUrl, string? CustomerId = null);
