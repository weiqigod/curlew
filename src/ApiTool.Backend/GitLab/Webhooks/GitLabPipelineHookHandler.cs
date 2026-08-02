// Refs M16-015 plan Decision D (status reconciliation via installation+head_sha).
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Reconciles <c>pr_checks.status</c> rows when a GitLab Pipeline Hook event arrives.
/// Lookup is by <c>(provider='gitlab', gitlab_installation_id, head_sha)</c> per plan Decision D.
/// Multiple matching rows (same commit, multiple test collections) are all updated.
/// Unknown GitLab pipeline states log a warning and leave the row unchanged.
/// </summary>
/// <remarks>
/// The class is non-sealed so the dispatcher tests can override <see cref="HandleAsync"/>
/// with a tracking stub without introducing a separate interface.
/// </remarks>
public class GitLabPipelineHookHandler(
    AppDbContext db,
    ILogger<GitLabPipelineHookHandler> log)
{
    /// <summary>
    /// Processes a parsed <see cref="GitLabPipelineHookPayload"/> for the given installation.
    /// </summary>
    public virtual async Task HandleAsync(
        GitLabPipelineHookPayload payload, Guid installationId, CancellationToken ct)
    {
        var (newStatus, lastError) = MapPipelineStatus(payload.Status);

        if (newStatus is null)
        {
            log.LogWarning(
                "gitlab_pipeline_hook_unknown_state pipeline_id={PipelineId} status={Status} installation_id={InstallationId}",
                payload.PipelineId, payload.Status, installationId);
            return;
        }

        var rows = await db.PrChecks
            .Where(x => x.Provider == "gitlab"
                        && x.GitLabInstallationId == installationId
                        && x.HeadSha == payload.Sha
                        && x.HeadSha != string.Empty)
            .ToListAsync(ct);

        if (rows.Count == 0)
        {
            log.LogInformation(
                "gitlab_pipeline_hook_no_match pipeline_id={PipelineId} sha={Sha} installation_id={InstallationId}",
                payload.PipelineId, payload.Sha, installationId);
            return;
        }

        foreach (var row in rows)
        {
            row.Status = newStatus;
            if (lastError is not null)
                row.LastError = lastError;
        }

        await db.SaveChangesAsync(ct);

        log.LogInformation(
            "gitlab_pipeline_hook_reconciled pipeline_id={PipelineId} sha={Sha} status={Status} rows_updated={Count}",
            payload.PipelineId, payload.Sha, newStatus, rows.Count);
    }

    /// <summary>
    /// Maps a GitLab pipeline state string to a <c>pr_checks.status</c> value and optional last_error.
    /// Returns <c>(null, null)</c> for unknown states (caller logs warning and skips update).
    /// </summary>
    private static (string? Status, string? LastError) MapPipelineStatus(string gitLabState) =>
        gitLabState switch
        {
            "success" => ("posted", null),
            "failed" => ("posted", null),
            "running" => ("posting", null),
            "pending" => ("queued", null),
            "canceled" => ("failed", "Pipeline cancelled in GitLab UI"),
            "skipped" => ("posted", null),
            _ => (null, null),
        };
}
