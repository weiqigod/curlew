namespace ApiTool.Backend.Telemetry;

/// <summary>Configuration for <see cref="TelemetryPurgeHost"/>.</summary>
public sealed class TelemetryPurgeOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:Telemetry:Purge";

    /// <summary>How often to run the purge tick. Defaults to 24 hours.</summary>
    public TimeSpan TickInterval { get; set; } = TimeSpan.FromHours(24);

    /// <summary>Hard-delete telemetry_events rows older than this many days. Defaults to 90.</summary>
    public int RetentionDays { get; set; } = 90;
}
