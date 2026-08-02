namespace ApiTool.Backend.Coordinator;

/// <summary>Request body for sending a worker heartbeat to keep a shard alive.</summary>
public sealed class HeartbeatRequest
{
    /// <summary>Worker id sending the heartbeat (must match the claiming worker).</summary>
    public string? WorkerId { get; set; }
}
