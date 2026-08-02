// Refs docs/SPECIFICATION.md:9216-9246 (outbound GitLab Commit Status API).
namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Posts commit statuses to GitLab on behalf of an organisation's <c>gitlab_installations</c> row.
/// Refs docs/SPECIFICATION.md:9216-9246.
/// </summary>
public interface IGitLabCheckPoster
{
    /// <summary>
    /// Posts a commit status for the given <c>pr_checks</c> row. Updates the row in place
    /// (<c>posting_started_at</c>, <c>posted_at</c>, <c>gitlab_status_id</c>, <c>status</c>,
    /// <c>last_error</c>, <c>attempt_count</c>).
    /// </summary>
    Task<CheckRunPostResult> PostAsync(Guid prCheckId, CancellationToken ct);
}
