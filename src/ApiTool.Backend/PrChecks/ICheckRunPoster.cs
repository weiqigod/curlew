namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Posts check runs to the GitHub Checks API on behalf of an organisation's
/// GitHub App installation.
/// Refs docs/SPECIFICATION.md:8429-8550.
/// </summary>
public interface ICheckRunPoster
{
    /// <summary>
    /// Posts a check run for the given <c>pr_checks</c> row. Updates the row in place
    /// (<c>posting_started_at</c>, <c>posted_at</c>, <c>check_run_id</c>, <c>status</c>,
    /// <c>last_error</c>, <c>attempt_count</c>).
    /// </summary>
    Task<CheckRunPostResult> PostAsync(Guid prCheckId, CancellationToken ct);
}
