namespace ApiTool.Backend.Data.Entities;

/// <summary>Lifecycle state of a coordinator shard.</summary>
public enum CoordinatorShardStatus
{
    /// <summary>Shard is waiting to be claimed by a worker.</summary>
    Pending,

    /// <summary>Shard has been claimed and is being processed.</summary>
    Running,

    /// <summary>Shard completed successfully.</summary>
    Completed,

    /// <summary>Shard processing failed.</summary>
    Failed,
}
