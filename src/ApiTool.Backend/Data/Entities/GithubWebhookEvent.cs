namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// One row per GitHub webhook event delivery — idempotency store and audit trail.
/// Refs docs/SPECIFICATION.md:10031-10049 (schema), :8586-8595 (idempotency contract).
/// </summary>
public sealed class GithubWebhookEvent
{
    /// <summary>Value of the X-GitHub-Delivery header (a UUID GitHub assigns per delivery). Primary key.</summary>
    public Guid DeliveryId { get; set; }

    /// <summary>Event type, e.g. <c>installation</c>, <c>check_run</c>, <c>installation_repositories</c>.</summary>
    public string EventType { get; set; } = string.Empty;

    /// <summary>Action, e.g. <c>created</c>, <c>deleted</c>, <c>rerequested</c>. Nullable; some events have none.</summary>
    public string? Action { get; set; }

    /// <summary>UTC moment we received the delivery.</summary>
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
