using System.Text.Json.Serialization;

namespace ApiTool.Backend.Results.Dashboard;

/// <summary>Response envelope for <c>GET /results/stats</c>.</summary>
public sealed record StatsResponse(
    [property: JsonPropertyName("window")]       string Window,
    [property: JsonPropertyName("window_start")] DateTime WindowStart,
    [property: JsonPropertyName("window_end")]   DateTime WindowEnd,
    [property: JsonPropertyName("totals")]       StatsTotals Totals,
    [property: JsonPropertyName("trend")]        IReadOnlyList<TrendEntry> Trend);

/// <summary>Aggregated totals across all runs in the window.</summary>
public sealed record StatsTotals(
    [property: JsonPropertyName("runs")]              int Runs,
    [property: JsonPropertyName("pass_count")]        int PassCount,
    [property: JsonPropertyName("fail_count")]        int FailCount,
    [property: JsonPropertyName("skipped_count")]     int SkippedCount,
    [property: JsonPropertyName("pass_rate")]         double PassRate,
    [property: JsonPropertyName("avg_duration_ms")]   long AvgDurationMs,
    [property: JsonPropertyName("p50_duration_ms")]   long P50DurationMs,
    [property: JsonPropertyName("p95_duration_ms")]   long P95DurationMs);

/// <summary>Per-day trend entry. Sparse — only days with at least one run are included.</summary>
public sealed record TrendEntry(
    [property: JsonPropertyName("date")]           string Date,
    [property: JsonPropertyName("runs")]           int Runs,
    [property: JsonPropertyName("pass_count")]     int PassCount,
    [property: JsonPropertyName("fail_count")]     int FailCount,
    [property: JsonPropertyName("pass_rate")]      double PassRate,
    [property: JsonPropertyName("avg_duration_ms")] long AvgDurationMs);
