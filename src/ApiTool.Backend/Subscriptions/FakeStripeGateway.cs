using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// In-memory implementation of <see cref="IStripeGateway"/> for tests and development.
/// Returns deterministic URLs with a <c>cs_&lt;guid&gt;</c> session id format.
/// </summary>
public sealed class FakeStripeGateway : IStripeGateway
{
    private readonly Dictionary<string, Stripe.Subscription> _subscriptions = [];
    private readonly Dictionary<string, Stripe.Customer> _customers = [];
    private readonly Dictionary<string, Stripe.Invoice> _invoices = [];
    private readonly Dictionary<string, Stripe.PaymentMethod> _paymentMethods = [];

    /// <summary>
    /// Registers a scripted <see cref="Stripe.Subscription"/> that
    /// <see cref="GetSubscriptionAsync"/> will return when asked for its id.
    /// </summary>
    public void SetSubscriptionForTest(Stripe.Subscription subscription)
    {
        ArgumentNullException.ThrowIfNull(subscription);
        _subscriptions[subscription.Id] = subscription;
    }

    /// <summary>
    /// Registers a scripted <see cref="Stripe.Customer"/> that
    /// <see cref="GetCustomerAsync"/> will return when asked for its id.
    /// </summary>
    public void SetCustomerForTest(Stripe.Customer customer)
    {
        ArgumentNullException.ThrowIfNull(customer);
        _customers[customer.Id] = customer;
    }
    /// <summary>
    /// Registers a scripted <see cref="Stripe.Invoice"/> that
    /// <see cref="GetInvoiceAsync"/> will return when asked for its id.
    /// </summary>
    public void SetInvoiceForTest(Stripe.Invoice invoice)
    {
        ArgumentNullException.ThrowIfNull(invoice);
        _invoices[invoice.Id] = invoice;
    }

    /// <summary>
    /// Registers a scripted <see cref="Stripe.PaymentMethod"/> that
    /// <see cref="GetPaymentMethodAsync"/> will return when asked for its id.
    /// </summary>
    public void SetPaymentMethodForTest(Stripe.PaymentMethod pm)
    {
        ArgumentNullException.ThrowIfNull(pm);
        _paymentMethods[pm.Id] = pm;
    }

    /// <inheritdoc/>
    public Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null,
        string? existingCustomerId = null,
        string? idempotencyKey = null,
        CancellationToken ct = default)
    {
        var sessionGuid = Guid.NewGuid().ToString("N");
        // When priceId is supplied, prefix the session id with cs_test_ to distinguish the price-id path.
        var sessionId = priceId is not null ? $"cs_test_{sessionGuid}" : $"cs_{sessionGuid}";
        var checkoutUrl = $"https://checkout.stripe.test/{sessionId}";
        // Pass through an existing customer id when supplied so callers can assert reuse behaviour.
        return Task.FromResult(new CheckoutSession(sessionId, checkoutUrl, existingCustomerId));
    }

    /// <inheritdoc/>
    public Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null,
        string? idempotencyKey = null,
        CancellationToken ct = default)
    {
        var portalGuid = Guid.NewGuid().ToString("N");
        var portalUrl = $"https://billing.stripe.test/p_{portalGuid}";
        return Task.FromResult(new PortalSession(portalUrl));
    }

    /// <inheritdoc/>
    public ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats,
        string interval) =>
        StripeProrationLocalMath.Compute(fromTier, fromSeats, toTier, toSeats, interval);

    /// <inheritdoc/>
    public Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default)
    {
        _subscriptions.TryGetValue(subscriptionId, out var sub);
        return Task.FromResult<Stripe.Subscription?>(sub);
    }

    /// <inheritdoc/>
    public Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default)
    {
        _customers.TryGetValue(customerId, out var customer);
        return Task.FromResult<Stripe.Customer?>(customer);
    }

    /// <inheritdoc/>
    /// <remarks>
    /// The fake gateway uses a deterministic full-month-delta approximation that intentionally
    /// diverges from Stripe's day-prorated math. The baseline "from" state is always
    /// <see cref="SubscriptionTier.Free"/> with zero seats — the fake has no live subscription
    /// context, so any non-Free target produces a non-zero charge. Unit tests assert
    /// <em>shape</em> (non-negative net) only; integration tests against stripe-mock exercise
    /// wire-protocol correctness; real-Stripe smoke tests cover cents-accuracy.
    /// Do NOT add cents-identical assertions on this path.
    /// </remarks>
    public Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId,
        string newPriceId,
        int newQuantity,
        DateTimeOffset prorationDate,
        string? idempotencyKey = null,
        CancellationToken ct = default)
    {
        // Map the target price id to (tier, interval) and compute the delta from a Free-tier
        // baseline. Using Free/0 as "from" ensures any paid target returns a positive charge,
        // making the result non-trivially verifiable in unit tests.
        var (toTier, _, interval) = StripePriceParser.Parse(newPriceId);
        var result = ComputeProration(SubscriptionTier.Free, 0, toTier, newQuantity, interval);
        // Renewal date: 30 days out from the proration date (deterministic for tests).
        var renewal = (DateTime?)prorationDate.UtcDateTime.AddDays(30);
        return Task.FromResult((result, renewal));
    }

    /// <inheritdoc/>
    public Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default)
    {
        _invoices.TryGetValue(invoiceId, out var inv);
        return Task.FromResult<Stripe.Invoice?>(inv);
    }

    /// <inheritdoc/>
    public Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default)
    {
        _paymentMethods.TryGetValue(paymentMethodId, out var pm);
        return Task.FromResult<Stripe.PaymentMethod?>(pm);
    }
}
