namespace ApiTool.Backend.Webhooks;

/// <summary>
/// Routes a verified Stripe event to its type-specific handler. M14-011 ships a
/// no-op implementation; M14-012 introduces real subscription/customer routing
/// and M14-013 introduces invoice and payment-method routing.
/// </summary>
public interface IStripeWebhookDispatcher
{
    /// <summary>Process a single webhook delivery.</summary>
    Task DispatchAsync(Stripe.Event @event, CancellationToken ct);
}
