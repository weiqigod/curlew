namespace ApiTool.Backend.Telemetry;

/// <summary>Configuration for <see cref="TelemetryAggregatorHost"/>.</summary>
public sealed class TelemetryAggregatorOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:Telemetry:Aggregator";

    /// <summary>How often to run the aggregator tick. Defaults to 24 hours.</summary>
    public TimeSpan TickInterval { get; set; } = TimeSpan.FromHours(24);

    /// <summary>UTC hour at which the daily tick fires (0–23). Defaults to 4 (04:00 UTC).</summary>
    public int RunHourUtc { get; set; } = 4;
}
