namespace ApiTool.Backend.Health;

/// <summary>Result of a /health probe.</summary>
public sealed record HealthReport(string Status, string Db, string Redis)
{
    /// <summary>Status value when all probes succeed.</summary>
    public const string StatusHealthy = "healthy";

    /// <summary>Status value when at least one probe fails.</summary>
    public const string StatusUnhealthy = "unhealthy";

    /// <summary>Probe value when the service is reachable.</summary>
    public const string Connected = "connected";

    /// <summary>Probe value when the service is unreachable.</summary>
    public const string Disconnected = "disconnected";

    /// <summary>Probe value when the service is not configured (e.g., Redis host unset).</summary>
    public const string NotConfigured = "not_configured";
}
