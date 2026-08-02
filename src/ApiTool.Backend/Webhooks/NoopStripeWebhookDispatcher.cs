namespace ApiTool.Backend.Webhooks;

/// <summary>
/// M14-011 stub. Returns immediately so the ingest pipeline can mark the row
/// as processed and the integration test exercises end-to-end flow. M14-012
/// replaces this registration with the real router.
/// </summary>
public sealed class NoopStripeWebhookDispatcher : IStripeWebhookDispatcher
{
    /// <inheritdoc/>
    public Task DispatchAsync(Stripe.Event @event, CancellationToken ct) => Task.CompletedTask;
}
