namespace ApiTool.Backend.Data.Entities;

/// <summary>Lifecycle state of a coordinator job.</summary>
public enum CoordinatorJobStatus
{
    /// <summary>All shards are pending assignment.</summary>
    Pending,

    /// <summary>At least one shard is being processed.</summary>
    Running,

    /// <summary>All shards completed successfully.</summary>
    Completed,

    /// <summary>One or more shards failed.</summary>
    Failed,
}
