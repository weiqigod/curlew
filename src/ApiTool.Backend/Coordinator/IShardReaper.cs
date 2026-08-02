namespace ApiTool.Backend.Coordinator;

/// <summary>Contract for the shard reaper that reclaims stale running shards.</summary>
public interface IShardReaper
{
    /// <summary>
    /// Reclaims shards whose heartbeat has not been received within the timeout window.
    /// </summary>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>The number of shards reclaimed.</returns>
    Task<int> ReapStaleShardsAsync(CancellationToken ct);
}
