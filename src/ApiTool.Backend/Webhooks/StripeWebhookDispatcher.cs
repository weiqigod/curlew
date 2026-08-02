// Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
// M14-013: invoice.* and payment_method.* event routing added.
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.Extensions.Logging;
using Stripe;

namespace ApiTool.Backend.Webhooks;

/// <summary>
/// Routes verified Stripe events to their type-specific handlers.
/// Unknown types are logged and dropped — Stripe still receives a 200 so it stops retrying.
/// Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
/// </summary>
public sealed class StripeWebhookDispatcher(
    StripeSubscriptionHandler subs,
    StripeCustomerUpdatedHandler customers,
    StripeInvoiceHandler invoices,
    StripePaymentMethodHandler paymentMethods,
    ILogger<StripeWebhookDispatcher> log) : IStripeWebhookDispatcher
{
    /// <inheritdoc/>
    public async Task DispatchAsync(Event @event, CancellationToken ct)
    {
        switch (@event.Type)
        {
            case Events.CustomerSubscriptionCreated:
            {
                var sub = (Subscription)@event.Data.Object;
                await subs.HandleCreatedOrUpdatedAsync(sub.Id, isCreated: true, ct);
                break;
            }
            case Events.CustomerSubscriptionUpdated:
            {
                var sub = (Subscription)@event.Data.Object;
                await subs.HandleCreatedOrUpdatedAsync(sub.Id, isCreated: false, ct);
                break;
            }
            case Events.CustomerSubscriptionDeleted:
            {
                var sub = (Subscription)@event.Data.Object;
                await subs.HandleDeletedAsync(sub.Id, ct);
                break;
            }
            case Events.CustomerUpdated:
            {
                var customer = (Customer)@event.Data.Object;
                await customers.HandleAsync(customer.Id, ct);
                break;
            }
            case Events.InvoicePaymentSucceeded:
            {
                var inv = (Invoice)@event.Data.Object;
                await invoices.HandlePaymentSucceededAsync(inv.Id, ct);
                break;
            }
            case Events.InvoicePaymentFailed:
            {
                var inv = (Invoice)@event.Data.Object;
                await invoices.HandlePaymentFailedAsync(inv.Id, ct);
                break;
            }
            case Events.InvoiceFinalized:
            {
                var inv = (Invoice)@event.Data.Object;
                await invoices.HandleFinalizedAsync(inv.Id, ct);
                break;
            }
            case Events.PaymentMethodAttached:
            {
                var pm = (PaymentMethod)@event.Data.Object;
                await paymentMethods.HandleAttachedAsync(pm.Id, ct);
                break;
            }
            case Events.PaymentMethodDetached:
            {
                var pm = (PaymentMethod)@event.Data.Object;
                await paymentMethods.HandleDetachedAsync(pm.Id, ct);
                break;
            }
            default:
                // Unknown event types are logged and dropped. The upstream idempotency store
                // marks the event processed so Stripe stops retrying.
                log.LogInformation(
                    "stripe_webhook_unhandled_type event_type={EventType} event_id={EventId}",
                    @event.Type, @event.Id);
                break;
        }
    }
}
