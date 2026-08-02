namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Thrown when the Stripe API returns an unrecoverable 5xx response or the request
/// fails to reach Stripe (network failure). Maps to RFC 7807 STRIPE_UNAVAILABLE 502.
/// </summary>
public sealed class StripeUnavailableException : Exception
{
    /// <summary>The idempotency key from the failed request, for correlation in logs.</summary>
    public string? IdempotencyKey { get; }

    /// <summary>The Stripe-Request-Id header value from the failing response, when available.</summary>
    public string? RequestId { get; }

    /// <summary>Initialises a new <see cref="StripeUnavailableException"/>.</summary>
    public StripeUnavailableException(string? idempotencyKey, string? requestId, Exception inner)
        : base("Stripe is unavailable (HTTP 5xx or network failure).", inner)
    {
        IdempotencyKey = idempotencyKey;
        RequestId = requestId;
    }
}
