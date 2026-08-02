namespace ApiTool.Backend.Health;

/// <summary>Probes the Redis cache. May be unconfigured.</summary>
public interface IRedisHealthProbe
{
    /// <summary>Returns true if Redis is configured and PING succeeded; false if configured but unreachable.</summary>
    Task<bool> IsConnectedAsync(CancellationToken ct);

    /// <summary>Returns true when a Redis host is configured.</summary>
    bool IsConfigured { get; }
}
