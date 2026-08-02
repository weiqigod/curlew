namespace ApiTool.Backend.Coordinator;

/// <summary>Wire DTO for a coordinator job (header + shards).</summary>
/// <param name="JobId">Wire-format job id (<c>job_…</c>).</param>
/// <param name="OrgId">Organization id.</param>
/// <param name="CollectionSha">Opaque collection identifier.</param>
/// <param name="ShardCount">Total number of shards.</param>
/// <param name="State">Current state of the job.</param>
/// <param name="CreatedAt">When the job was created.</param>
/// <param name="CompletedAt">When the job completed, if applicable.</param>
/// <param name="AggregateResultId">Wire-format result id once job completes.</param>
/// <param name="Shards">List of shards.</param>
public sealed record CoordinatorJobDto(
    string JobId,
    Guid OrgId,
    string CollectionSha,
    int ShardCount,
    string State,
    DateTime CreatedAt,
    DateTime? CompletedAt,
    string? AggregateResultId,
    IReadOnlyList<CoordinatorShardDto> Shards);
