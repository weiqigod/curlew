namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// One row per (day, event_type) produced nightly by <c>TelemetryAggregatorHost</c>.
/// Persisted indefinitely — these are the long-lived analytics surface.
/// The composite PK <c>(Day, EventType)</c> ensures idempotent re-aggregation.
/// Refs: docs/SPECIFICATION.md — Telemetry Phase 3 Implementation Pipeline.
/// </summary>
public sealed class TelemetryDailyAggregate
{
    /// <summary>The UTC calendar day this aggregate covers (date-only).</summary>
    public DateOnly Day { get; set; }

    /// <summary>The event type being aggregated (e.g. <c>run.completed</c>).</summary>
    public string EventType { get; set; } = string.Empty;

    /// <summary>Total number of raw events received on this day for this event type.</summary>
    public long EventCount { get; set; }

    /// <summary>Count of distinct <c>install_id</c> values that emitted this event type on this day.</summary>
    public long DistinctInstallCount { get; set; }

    /// <summary>
    /// JSON object summarising numeric fields from the event payload.
    /// Shape: <c>{ "duration_ms": {"sum": 12345, "min": 1, "max": 999, "avg": 41.2, "n": 300}, ... }</c>.
    /// Only a fixed whitelist of known numeric keys is aggregated; unknown keys are ignored.
    /// Stored as <c>jsonb</c> on Postgres, TEXT on SQLite.
    /// </summary>
    public string NumericSumsJson { get; set; } = "{}";
}
