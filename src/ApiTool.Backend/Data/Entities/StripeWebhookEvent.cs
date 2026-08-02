namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// One row per Stripe webhook event delivery — idempotency store and audit trail.
/// Refs docs/SPECIFICATION.md:9985-10004.
/// </summary>
public sealed class StripeWebhookEvent
{
    /// <summary>Stripe's event id (e.g. <c>evt_1NXp…</c>). Primary key.</summary>
    public string EventId { get; set; } = string.Empty;

    /// <summary>Event type, e.g. <c>customer.subscription.created</c>.</summary>
    public string EventType { get; set; } = string.Empty;

    /// <summary>UTC moment we received the delivery (DB default <c>NOW()</c>).</summary>
    public DateTime ReceivedAt { get; set; }

    /// <summary>UTC moment processing succeeded; null while pending or after a failure.</summary>
    public DateTime? ProcessedAt { get; set; }

    /// <summary>One of <c>pending</c>, <c>processed</c>, <c>quarantined</c>.</summary>
    public string Status { get; set; } = "pending";

    /// <summary>Raw JSON event payload — TEXT on SQLite, jsonb on Postgres.</summary>
    public string PayloadJson { get; set; } = "{}";

    /// <summary>Number of failed handler attempts. Increments to 5 then quarantines.</summary>
    public int AttemptCount { get; set; }

    /// <summary>Last error message; null if never failed.</summary>
    public string? LastError { get; set; }

    /// <summary>UTC moment the most recent failure was recorded.</summary>
    public DateTime? LastErrorAt { get; set; }
}
