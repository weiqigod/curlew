namespace ApiTool.Backend.Health;

/// <summary>Probes the backend's primary database.</summary>
public interface IDbHealthProbe
{
    /// <summary>Returns true if the database is reachable within the probe timeout.</summary>
    Task<bool> IsConnectedAsync(CancellationToken ct);
}
