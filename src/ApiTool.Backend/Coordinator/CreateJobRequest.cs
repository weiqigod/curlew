namespace ApiTool.Backend.Coordinator;

/// <summary>Request body for creating a coordinator job.</summary>
public sealed class CreateJobRequest
{
    /// <summary>Opaque collection identifier (e.g. a SHA of the collection file).</summary>
    public string? CollectionSha { get; set; }

    /// <summary>Number of shards to split the collection into.</summary>
    public int? ShardCount { get; set; }
}
