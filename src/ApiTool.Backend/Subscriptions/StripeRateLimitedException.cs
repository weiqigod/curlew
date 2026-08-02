namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Thrown when the Stripe API returns HTTP 429 (rate limited).
/// Callers should retry with the same <see cref="IdempotencyKey"/>.
/// </summary>
public sealed class StripeRateLimitedException : Exception
{
    /// <summary>The idempotency key from the failed request, for safe retry.</summary>
    public string? IdempotencyKey { get; }

    /// <summary>Initialises a new <see cref="StripeRateLimitedException"/>.</summary>
    public StripeRateLimitedException(string? idempotencyKey, Exception inner)
        : base("Stripe rate-limited (HTTP 429); retry with the same idempotency key.", inner)
    {
        IdempotencyKey = idempotencyKey;
    }
}
