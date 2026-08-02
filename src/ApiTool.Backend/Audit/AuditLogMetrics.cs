using System.Diagnostics.Metrics;

namespace ApiTool.Backend.Audit;

/// <summary>
/// OpenTelemetry metrics for the audit log subsystem.
/// Counters are exposed via a <see cref="Meter"/> named <c>ApiTool.Backend.Audit</c>.
/// Scraping wiring (e.g. prometheus-net or OTel exporter) is a future-build concern;
/// emitting the counter here is sufficient for the DoD requirement.
/// </summary>
internal static class AuditLogMetrics
{
    /// <summary>Shared meter for the audit subsystem.</summary>
    public static readonly Meter Meter = new("ApiTool.Backend.Audit", "1.0.0");

    /// <summary>
    /// Total number of audit log rows hard-deleted by <see cref="AuditLogCleanupHost"/>.
    /// Tagged with <c>org_id</c>.
    /// </summary>
    public static readonly Counter<long> RowsDeleted =
        Meter.CreateCounter<long>(
            "audit_log_cleanup_rows_deleted_total",
            description: "Total audit log rows deleted by AuditLogCleanupHost per org.");
}
