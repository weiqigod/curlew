namespace ApiTool.Backend.Coordinator;

/// <summary>Error codes returned by <see cref="CoordinatorService"/>.</summary>
public enum CoordinatorError
{
    /// <summary>No error — operation succeeded.</summary>
    None,

    /// <summary>The caller does not have permission to perform this action.</summary>
    PermissionDenied,

    /// <summary>The requested shard count is out of the allowed range.</summary>
    InvalidShardCount,

    /// <summary>The request body is missing required fields.</summary>
    InvalidRequest,

    /// <summary>The requested resource was not found.</summary>
    NotFound,

    /// <summary>All shards have been claimed; no pending shards remain.</summary>
    NoShardsAvailable,

    /// <summary>Submit or heartbeat called by a worker that does not own the shard.</summary>
    ShardNotClaimed,

    /// <summary>The operation is invalid for the shard's current state.</summary>
    InvalidState,
}
