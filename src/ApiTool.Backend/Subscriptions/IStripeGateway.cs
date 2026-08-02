using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Abstraction over the Stripe API, enabling deterministic fake implementations in tests.
/// </summary>
public interface IStripeGateway
{
    /// <summary>Creates a Stripe Checkout session for a new or upgraded subscription.</summary>
    /// <param name="userId">The user initiating the checkout.</param>
    /// <param name="orgId">The organization being subscribed.</param>
    /// <param name="tier">The target subscription tier.</param>
    /// <param name="interval">Billing interval: <c>month</c> or <c>year</c>.</param>
    /// <param name="seatCount">Number of seats to purchase.</param>
    /// <param name="successUrl">URL to redirect on successful payment.</param>
    /// <param name="cancelUrl">URL to redirect if the user cancels.</param>
    /// <param name="priceId">Optional Stripe price id; when supplied, takes precedence over tier/interval/seatCount for the checkout line item.</param>
    /// <param name="existingCustomerId">Optional existing Stripe customer id to reuse instead of creating a new customer.</param>
    /// <param name="idempotencyKey">Optional idempotency key to pass to the Stripe API for safe retries.</param>
    /// <param name="ct">Cancellation token.</param>
    Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null,
        string? existingCustomerId = null,
        string? idempotencyKey = null,
        CancellationToken ct = default);

    /// <summary>Creates a Stripe Billing Portal session for subscription management.</summary>
    /// <param name="userId">The user accessing the portal.</param>
    /// <param name="orgId">The organization whose billing is being managed.</param>
    /// <param name="returnUrl">URL to redirect after the portal session ends.</param>
    /// <param name="customerId">Stripe customer id (<c>cus_*</c>). Required by the live gateway; ignored by the fake.</param>
    /// <param name="idempotencyKey">Optional idempotency key for safe retries.</param>
    /// <param name="ct">Cancellation token.</param>
    Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null,
        string? idempotencyKey = null,
        CancellationToken ct = default);

    /// <summary>Computes a proration amount when changing a subscription.</summary>
    /// <param name="fromTier">The current tier.</param>
    /// <param name="fromSeats">The current seat count.</param>
    /// <param name="toTier">The target tier.</param>
    /// <param name="toSeats">The target seat count.</param>
    /// <param name="interval">Billing interval: <c>month</c> or <c>year</c>.</param>
    ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats,
        string interval);

    /// <summary>
    /// Computes a proration preview by asking Stripe what the upcoming invoice would look like
    /// if the subscription's items were swapped to <paramref name="newPriceId"/>. This is Stripe's
    /// server-side proration model — it operates on Stripe subscription/price ids, not on local
    /// tier/seat abstractions.
    /// </summary>
    /// <remarks>
    /// The synchronous <see cref="ComputeProration"/> method on this interface is retained as the
    /// legacy local-math path used by <c>SubscriptionsService.UpdateAsync</c>. New surfaces (the
    /// preview-proration endpoint) call this async method which talks to Stripe's
    /// <c>invoices/upcoming</c> endpoint. The fake gateway's implementations of both methods
    /// share the same simple-math local approximation; the live gateway implements the async
    /// method against Stripe and falls back to the same simple math for the sync method (used
    /// only by the apply path until that is migrated in a later slice).
    /// </remarks>
    /// <param name="subscriptionId">The active Stripe subscription id (<c>sub_*</c>).</param>
    /// <param name="newPriceId">The target Stripe price id to swap to.</param>
    /// <param name="newQuantity">The target quantity (seat count).</param>
    /// <param name="prorationDate">UTC moment at which proration is computed; passed to Stripe as <c>subscription_proration_date</c> in epoch seconds.</param>
    /// <param name="idempotencyKey">Optional idempotency key forwarded to Stripe.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// A tuple of (proration result with credit/charge/net in cents, optional renewal date taken
    /// from the upcoming invoice's next payment attempt or period end, null when Stripe reports
    /// no upcoming invoice).
    /// </returns>
    Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId,
        string newPriceId,
        int newQuantity,
        DateTimeOffset prorationDate,
        string? idempotencyKey = null,
        CancellationToken ct = default);

    /// <summary>
    /// Fetches the live Stripe Subscription by id. Returns <see langword="null"/> when Stripe
    /// reports 404 — the underlying object was deleted and the local row should be quarantined
    /// per docs/SPECIFICATION.md:6845–6848. Other failures bubble up as
    /// <see cref="Stripe.StripeException"/>.
    /// </summary>
    /// <param name="subscriptionId">The Stripe subscription id (<c>sub_*</c>).</param>
    /// <param name="ct">Cancellation token.</param>
    Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default);

    /// <summary>
    /// Fetches the live Stripe Customer by id. Returns <see langword="null"/> on 404;
    /// other failures bubble up as <see cref="Stripe.StripeException"/>.
    /// </summary>
    /// <param name="customerId">The Stripe customer id (<c>cus_*</c>).</param>
    /// <param name="ct">Cancellation token.</param>
    Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default);

    /// <summary>
    /// Fetches the live Stripe Invoice by id. Returns <see langword="null"/> when Stripe
    /// reports 404. Other failures bubble up as <see cref="Stripe.StripeException"/>.
    /// Refs docs/SPECIFICATION.md:6845–6848 (re-fetch pattern).
    /// </summary>
    /// <param name="invoiceId">The Stripe invoice id (<c>in_*</c>).</param>
    /// <param name="ct">Cancellation token.</param>
    Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default);

    /// <summary>
    /// Fetches the live Stripe PaymentMethod by id. Returns <see langword="null"/> on 404;
    /// other failures bubble up as <see cref="Stripe.StripeException"/>.
    /// </summary>
    /// <param name="paymentMethodId">The Stripe payment method id (<c>pm_*</c>).</param>
    /// <param name="ct">Cancellation token.</param>
    Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default);
}
