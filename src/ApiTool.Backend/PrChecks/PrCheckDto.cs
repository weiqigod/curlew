namespace ApiTool.Backend.PrChecks;

/// <summary>Wire DTO for a PR check (snake_case via global JSON options).</summary>
/// <param name="Id">Wire-format pr-check id (e.g. <c>prc_abc123…</c>).</param>
/// <param name="Repo">Repo slug (e.g. <c>owner/name</c>).</param>
/// <param name="Pr">Pull-request number.</param>
/// <param name="State">Check state: <c>success</c> or <c>failure</c>.</param>
/// <param name="ResultId">Optional wire-format result id.</param>
/// <param name="CreatedAt">Server-side creation timestamp (UTC).</param>
/// <param name="PostedAt">Timestamp when posting to GitHub Checks API completed successfully. Null when not yet posted.</param>
/// <param name="CheckRunId">GitHub check_run id returned by POST /repos/{o}/{r}/check-runs. Null when not yet posted.</param>
public sealed record PrCheckDto(
    string Id,
    string Repo,
    int Pr,
    string State,
    string? ResultId,
    DateTime CreatedAt,
    DateTime? PostedAt,
    long? CheckRunId);
