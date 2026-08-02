namespace ApiTool.Backend.Coordinator;

/// <summary>Wire DTO for a coordinator shard.</summary>
/// <param name="ShardId">Wire-format shard id (<c>shd_…</c>).</param>
/// <param name="JobId">Wire-format job id (<c>job_…</c>).</param>
/// <param name="ShardIndex">Zero-based shard index.</param>
/// <param name="State">Current state of the shard.</param>
/// <param name="AssignedWorker">Worker id that claimed this shard, if any.</param>
/// <param name="RequestsJson">JSON array of HTTP request specs for this shard.</param>
public sealed record CoordinatorShardDto(
    string ShardId,
    string JobId,
    int ShardIndex,
    string State,
    string? AssignedWorker,
    string RequestsJson);
