namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Raw telemetry event emitted by the CLI and stored server-side.
/// Retention: 90 days (hard delete by <c>TelemetryPurgeHost</c>).
/// Aggregated daily by <c>TelemetryAggregatorHost</c> before purge.
/// Refs: docs/SPECIFICATION.md — Telemetry Phase 3 Implementation Pipeline.
/// </summary>
public sealed class TelemetryEvent
{
    /// <summary>Surrogate PK (UUID v4, client-generated or server-assigned).</summary>
    public Guid Id { get; set; }

    /// <summary>Anonymous identifier of the CLI installation that emitted this event.</summary>
    public Guid InstallId { get; set; }

    /// <summary>Dot-namespaced event type (e.g. <c>run.completed</c>). Open-ended for forward-compat.</summary>
    public string EventType { get; set; } = string.Empty;

    /// <summary>
    /// Opaque JSON payload supplied by the CLI. Stored as <c>jsonb</c> on Postgres,
    /// plain TEXT on SQLite. Not projected into columns — raw shape preserved.
    /// </summary>
    public string EventPayloadJson { get; set; } = "{}";

    /// <summary>Server-side timestamp when the event was received (UTC). Not supplied by the CLI.</summary>
    public DateTime ReceivedAt { get; set; }

    /// <summary>
    /// Client-supplied idempotency key (UUID string). UNIQUE constraint enforces
    /// at-most-once delivery — duplicate inserts are detected and converted to 202 no-ops.
    /// </summary>
    public string IdempotencyKey { get; set; } = string.Empty;
}
