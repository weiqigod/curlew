namespace ApiTool.Backend.Data.Entities;

/// <summary>A single shard belonging to a distributed execution job.</summary>
public sealed class CoordinatorShard
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>Parent job id.</summary>
    public Guid JobId { get; set; }

    /// <summary>Zero-based shard index within the job.</summary>
    public int ShardIndex { get; set; }

    /// <summary>Current lifecycle state of this shard.</summary>
    public CoordinatorShardStatus Status { get; set; } = CoordinatorShardStatus.Pending;

    /// <summary>Worker id that claimed this shard, if any.</summary>
    public string? AssignedWorker { get; set; }

    /// <summary>When the shard was claimed by a worker.</summary>
    public DateTime? ClaimedAt { get; set; }

    /// <summary>When the last heartbeat was received from the worker.</summary>
    public DateTime? LastHeartbeatAt { get; set; }

    /// <summary>When the shard was completed.</summary>
    public DateTime? CompletedAt { get; set; }

    /// <summary>JSON array of HTTP request specs for this shard.</summary>
    public string RequestsJson { get; set; } = "[]";

    /// <summary>JSON array of per-request outcomes submitted by the worker on completion.</summary>
    public string? ResultJson { get; set; }

    /// <summary>Number of passing requests in this shard's results.</summary>
    public int PassCount { get; set; }

    /// <summary>Number of failing requests in this shard's results.</summary>
    public int FailCount { get; set; }

    /// <summary>Total duration in milliseconds for this shard.</summary>
    public long DurationMs { get; set; }
}
