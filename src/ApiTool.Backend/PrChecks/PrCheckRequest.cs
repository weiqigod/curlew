namespace ApiTool.Backend.PrChecks;

/// <summary>Request body for POST /api/v1/organizations/{orgId}/pr-checks.
/// Matches the CLI <c>PrCheckPayload</c> shape (snake_case JSON).</summary>
public sealed class PrCheckRequest
{
    /// <summary>Repo slug (e.g. <c>owner/name</c>). Required.</summary>
    public string? Repo { get; set; }

    /// <summary>Pull-request number. Must be positive.</summary>
    public int? Pr { get; set; }

    /// <summary>Check state. Must be <c>success</c> or <c>failure</c>.</summary>
    public string? State { get; set; }

    /// <summary>Optional wire-format result id (e.g. <c>res_abc123…</c>).</summary>
    public string? ResultId { get; set; }
}
