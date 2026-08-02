// Spec refs: docs/SPECIFICATION.md:6856–6870 (three-layer test strategy), :7322–7345 (POST /api/v1/subscriptions/portal), :7980 (force-validation triggers), :6860 (Stripe Test Strategy — known limitation: stripe-mock cannot exercise real proration math).
using System.Net;
using ApiTool.Backend.Data.Entities;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using Stripe;
using Stripe.BillingPortal;
using Stripe.Checkout;

namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Live implementation of <see cref="IStripeGateway"/> backed by the Stripe.Net 43.x SDK.
/// Requires <c>Stripe:Mode = live</c> and a valid <c>Stripe:ApiKey</c>.
/// Point <c>Stripe:ApiBase</c> at stripe-mock in CI (e.g. http://localhost:12111).
/// </summary>
public sealed class StripeGateway : IStripeGateway
{
    private readonly StripeClient _client;
    private readonly ILogger<StripeGateway> _log;

    /// <summary>Constructs a live Stripe gateway, validating that ApiKey is set.</summary>
    public StripeGateway(IOptions<StripeOptions> options, ILogger<StripeGateway> log)
    {
        var opts = options.Value;
        if (string.IsNullOrEmpty(opts.ApiKey))
            throw new InvalidOperationException(
                "ApiTool:Stripe:Mode=live requires APITOOL__STRIPE__APIKEY to be set.");

        _client = string.IsNullOrEmpty(opts.ApiBase)
            ? new StripeClient(opts.ApiKey)
            : new StripeClient(opts.ApiKey, apiBase: opts.ApiBase);
        _log = log;
    }

    /// <summary>
    /// Constructor that accepts a pre-built <see cref="StripeClient"/>.
    /// Useful for unit tests: inject a stub <see cref="IHttpClient"/> via <c>new StripeClient(apiKey, httpClient: stub)</c>.
    /// Internal — accessible to <c>ApiTool.Backend.Tests</c> via InternalsVisibleTo.
    /// </summary>
    internal StripeGateway(IOptions<StripeOptions> options, ILogger<StripeGateway> log, StripeClient client)
    {
        _ = options; // validated by caller
        _client = client;
        _log = log;
    }

    /// <inheritdoc/>
    public async Task<CheckoutSession> CreateCheckoutSessionAsync(
        Guid userId, Guid orgId, SubscriptionTier tier, string interval, int seatCount,
        string successUrl, string cancelUrl,
        string? priceId = null,
        string? existingCustomerId = null,
        string? idempotencyKey = null,
        CancellationToken ct = default)
    {
        var requestOptions = new RequestOptions
        {
            IdempotencyKey = idempotencyKey,
        };

        // Truncate idempotency key to Stripe's 255-byte limit (defensive; UUID is 36 bytes).
        if (requestOptions.IdempotencyKey?.Length > 255)
            requestOptions.IdempotencyKey = requestOptions.IdempotencyKey[..255];

        // 1) Resolve customer id.
        string customerId;
        if (!string.IsNullOrEmpty(existingCustomerId))
        {
            customerId = existingCustomerId;
        }
        else
        {
            try
            {
                var customer = await new CustomerService(_client).CreateAsync(
                    new CustomerCreateOptions
                    {
                        Metadata = new Dictionary<string, string> { ["org_id"] = orgId.ToString("D") },
                    },
                    requestOptions, ct);
                customerId = customer.Id;
            }
            catch (StripeException ex) when (ex.HttpStatusCode == HttpStatusCode.TooManyRequests)
            {
                _log.LogWarning(
                    "Stripe rate-limited during customer.create; idempotency_key={IdempotencyKey}",
                    idempotencyKey);
                throw new StripeRateLimitedException(idempotencyKey, ex);
            }
        }

        // 2) Build the checkout session line item.
        if (string.IsNullOrEmpty(priceId))
            throw new InvalidOperationException(
                "StripeGateway.CreateCheckoutSessionAsync requires a non-empty priceId; " +
                "SubscriptionsService must resolve tier → price before calling the gateway.");

        var sessionOptions = new Stripe.Checkout.SessionCreateOptions
        {
            Mode = "subscription",
            Customer = customerId,
            SuccessUrl = successUrl,
            CancelUrl = cancelUrl,
            LineItems =
            [
                new SessionLineItemOptions { Price = priceId, Quantity = seatCount },
            ],
            Metadata = new Dictionary<string, string>
            {
                ["org_id"] = orgId.ToString("D"),
                ["user_id"] = userId.ToString("D"),
            },
        };

        try
        {
            var session = await new Stripe.Checkout.SessionService(_client).CreateAsync(sessionOptions, requestOptions, ct);
            return new CheckoutSession(session.Id, session.Url, customerId);
        }
        catch (StripeException ex) when (ex.HttpStatusCode == HttpStatusCode.TooManyRequests)
        {
            _log.LogWarning(
                "Stripe rate-limited during session.create; idempotency_key={IdempotencyKey}",
                idempotencyKey);
            throw new StripeRateLimitedException(idempotencyKey, ex);
        }
    }

    /// <inheritdoc/>
    // Spec refs: docs/SPECIFICATION.md:7322–7345 (POST /api/v1/subscriptions/portal).
    public async Task<PortalSession> CreatePortalSessionAsync(
        Guid userId, Guid orgId, string returnUrl,
        string? customerId = null,
        string? idempotencyKey = null,
        CancellationToken ct = default)
    {
        if (string.IsNullOrEmpty(customerId))
            throw new InvalidOperationException(
                "StripeGateway.CreatePortalSessionAsync requires a non-empty customerId; " +
                "SubscriptionsService must resolve the org's stripe_customer_id before calling the gateway.");

        var requestOptions = new RequestOptions { IdempotencyKey = idempotencyKey };

        // Truncate idempotency key to Stripe's 255-byte limit (defensive; UUID is 36 bytes).
        if (requestOptions.IdempotencyKey?.Length > 255)
            requestOptions.IdempotencyKey = requestOptions.IdempotencyKey[..255];

        var options = new Stripe.BillingPortal.SessionCreateOptions
        {
            Customer = customerId,
            ReturnUrl = returnUrl,
        };

        try
        {
            var session = await new Stripe.BillingPortal.SessionService(_client)
                .CreateAsync(options, requestOptions, ct);
            return new PortalSession(session.Url);
        }
        catch (StripeException ex) when (ex.HttpStatusCode == HttpStatusCode.TooManyRequests)
        {
            _log.LogWarning(
                "Stripe rate-limited during billing portal session.create; idempotency_key={IdempotencyKey}",
                idempotencyKey);
            throw new StripeRateLimitedException(idempotencyKey, ex);
        }
        catch (StripeException ex) when ((int)ex.HttpStatusCode >= 500)
        {
            _log.LogWarning(
                "Stripe 5xx during billing portal session.create; idempotency_key={IdempotencyKey} request_id={RequestId} status={Status}",
                idempotencyKey, ex.StripeResponse?.RequestId, (int)ex.HttpStatusCode);
            throw new StripeUnavailableException(idempotencyKey, ex.StripeResponse?.RequestId, ex);
        }
    }

    /// <inheritdoc/>
    /// <remarks>
    /// Live gateway delegates the sync method to simple-math for the apply path (PATCH).
    /// The new <see cref="ComputeProrationAsync"/> talks to Stripe directly for the preview path.
    /// Migrating PATCH to the async path is a future slice.
    /// </remarks>
    public ProrationResult ComputeProration(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats,
        string interval) =>
        StripeProrationLocalMath.Compute(fromTier, fromSeats, toTier, toSeats, interval);

    /// <inheritdoc/>
    public async Task<(ProrationResult Result, DateTime? RenewalDate)> ComputeProrationAsync(
        string subscriptionId,
        string newPriceId,
        int newQuantity,
        DateTimeOffset prorationDate,
        string? idempotencyKey = null,
        CancellationToken ct = default)
    {
        var requestOptions = new RequestOptions { IdempotencyKey = idempotencyKey };
        // Truncate idempotency key to Stripe's 255-byte limit (defensive; UUID is 36 bytes).
        if (requestOptions.IdempotencyKey?.Length > 255)
            requestOptions.IdempotencyKey = requestOptions.IdempotencyKey[..255];

        var options = new UpcomingInvoiceOptions
        {
            Subscription = subscriptionId,
            SubscriptionItems =
            [
                new InvoiceSubscriptionItemOptions { Price = newPriceId, Quantity = newQuantity },
            ],
            SubscriptionProrationDate = prorationDate.UtcDateTime,
            SubscriptionProrationBehavior = "create_prorations",
        };

        Stripe.Invoice invoice;
        try
        {
            invoice = await new InvoiceService(_client).UpcomingAsync(options, requestOptions, ct);
        }
        catch (StripeException ex) when (ex.HttpStatusCode == HttpStatusCode.TooManyRequests)
        {
            _log.LogWarning(
                "Stripe rate-limited during invoice.upcoming; idempotency_key={IdempotencyKey}",
                idempotencyKey);
            throw new StripeRateLimitedException(idempotencyKey, ex);
        }
        catch (StripeException ex) when (ex.StripeError?.Code == "invoice_upcoming_none")
        {
            // Behaviour #5: no upcoming invoice → 200 OK with zeroed result (not a client error).
            return (new ProrationResult(Credit: 0, Charge: 0, Net: 0), null);
        }
        catch (StripeException ex) when ((int)ex.HttpStatusCode >= 500)
        {
            _log.LogWarning(
                "Stripe 5xx during invoice.upcoming; idempotency_key={IdempotencyKey} request_id={RequestId} status={Status}",
                idempotencyKey, ex.StripeResponse?.RequestId, (int)ex.HttpStatusCode);
            throw new StripeUnavailableException(idempotencyKey, ex.StripeResponse?.RequestId, ex);
        }

        // Sum line-item amounts: positive amounts are charges, negative are credits.
        long charge = 0, credit = 0;
        if (invoice.Lines is not null)
        {
            foreach (var line in invoice.Lines)
            {
                if (line.Amount >= 0) charge += line.Amount;
                else credit += -line.Amount;
            }
        }

        // checked: throws OverflowException rather than silently wrapping if the invoice total
        // exceeds int.MaxValue (~$21M in cents). Extremely unlikely for normal subscriptions.
        var net = checked((int)(charge - credit));
        var renewal = invoice.NextPaymentAttempt ?? invoice.PeriodEnd;
        return (new ProrationResult(checked((int)credit), checked((int)charge), net), renewal);
    }

    /// <inheritdoc/>
    /// <remarks>
    /// Returns <see langword="null"/> when Stripe reports 404 — the underlying subscription
    /// was deleted. Other exceptions bubble up so the caller can decide on retry semantics.
    /// Refs docs/SPECIFICATION.md:6845–6848 (re-fetch pattern).
    /// </remarks>
    public async Task<Stripe.Subscription?> GetSubscriptionAsync(string subscriptionId, CancellationToken ct = default)
    {
        try
        {
            return await new SubscriptionService(_client).GetAsync(subscriptionId, cancellationToken: ct);
        }
        catch (StripeException ex) when (ex.HttpStatusCode == System.Net.HttpStatusCode.NotFound)
        {
            return null;
        }
    }

    /// <inheritdoc/>
    /// <remarks>
    /// Returns <see langword="null"/> when Stripe reports 404. Other exceptions bubble up.
    /// </remarks>
    public async Task<Stripe.Customer?> GetCustomerAsync(string customerId, CancellationToken ct = default)
    {
        try
        {
            return await new CustomerService(_client).GetAsync(customerId, cancellationToken: ct);
        }
        catch (StripeException ex) when (ex.HttpStatusCode == HttpStatusCode.NotFound)
        {
            return null;
        }
    }

    /// <inheritdoc/>
    /// <remarks>
    /// Returns <see langword="null"/> when Stripe reports 404 (invoice deleted/not found).
    /// Other exceptions bubble up. Refs docs/SPECIFICATION.md:6845–6848 (re-fetch pattern).
    /// </remarks>
    public async Task<Stripe.Invoice?> GetInvoiceAsync(string invoiceId, CancellationToken ct = default)
    {
        try
        {
            return await new InvoiceService(_client).GetAsync(invoiceId, cancellationToken: ct);
        }
        catch (StripeException ex) when (ex.HttpStatusCode == HttpStatusCode.NotFound)
        {
            return null;
        }
    }

    /// <inheritdoc/>
    /// <remarks>
    /// Returns <see langword="null"/> when Stripe reports 404. Other exceptions bubble up.
    /// </remarks>
    public async Task<Stripe.PaymentMethod?> GetPaymentMethodAsync(string paymentMethodId, CancellationToken ct = default)
    {
        try
        {
            return await new PaymentMethodService(_client).GetAsync(paymentMethodId, cancellationToken: ct);
        }
        catch (StripeException ex) when (ex.HttpStatusCode == HttpStatusCode.NotFound)
        {
            return null;
        }
    }
}

