namespace ApiTool.Backend.Coordinator;

/// <summary>Request body for claiming a shard from a coordinator job.</summary>
public sealed class ClaimRequest
{
    /// <summary>Identifier of the requesting worker.</summary>
    public string? WorkerId { get; set; }

    /// <summary>Worker capability flags (e.g. <c>["http"]</c>).</summary>
    public IReadOnlyList<string>? Capabilities { get; set; }
}
