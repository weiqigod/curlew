namespace ApiTool.Backend.Coordinator;

/// <summary>Request body for submitting the result of a completed shard.</summary>
public sealed class SubmitResultRequest
{
    /// <summary>Worker id submitting the result (must match the claiming worker).</summary>
    public string? WorkerId { get; set; }

    /// <summary>Number of passing requests in this shard.</summary>
    public int PassCount { get; set; }

    /// <summary>Number of failing requests in this shard.</summary>
    public int FailCount { get; set; }

    /// <summary>Total execution duration in milliseconds.</summary>
    public long DurationMs { get; set; }

    /// <summary>Per-request outcome items as a JSON-serializable array.</summary>
    public IReadOnlyList<object>? Items { get; set; }
}
