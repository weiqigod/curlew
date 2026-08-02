// Refs M16-015 plan Decision D (lookup by installation+head_sha).
namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Parsed shape for a GitLab Pipeline Hook event payload.
/// Fields extracted from <c>object_attributes</c> and <c>project</c>.
/// </summary>
/// <param name="PipelineId">The pipeline id from <c>object_attributes.id</c>.</param>
/// <param name="Status">The pipeline state string (e.g. "success", "running", "failed", "canceled", "pending", "skipped").</param>
/// <param name="Sha">The commit SHA from <c>object_attributes.sha</c>.</param>
/// <param name="ProjectId">The GitLab project id from <c>project.id</c>.</param>
public sealed record GitLabPipelineHookPayload(
    long PipelineId,
    string Status,
    string Sha,
    long ProjectId);
