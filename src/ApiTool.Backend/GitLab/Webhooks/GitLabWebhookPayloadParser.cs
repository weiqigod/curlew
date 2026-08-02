// Refs M16-015 plan Step 3; mirrors GithubWebhookPayloadParser pattern.
using System.Text.Json;

namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Static helpers that extract strongly-typed payloads from a <see cref="JsonDocument"/>
/// for each handled GitLab webhook event type. Mirrors <c>GithubWebhookPayloadParser</c>.
/// </summary>
public static class GitLabWebhookPayloadParser
{
    /// <summary>
    /// Extracts a <see cref="GitLabPipelineHookPayload"/> from a Pipeline Hook event.
    /// Throws <see cref="InvalidOperationException"/> when required fields are absent.
    /// </summary>
    public static GitLabPipelineHookPayload ParsePipelineHook(JsonDocument doc)
    {
        var root = doc.RootElement;

        if (!root.TryGetProperty("object_attributes", out var oa))
            throw new InvalidOperationException("Pipeline Hook payload missing 'object_attributes'");

        if (!root.TryGetProperty("project", out var proj))
            throw new InvalidOperationException("Pipeline Hook payload missing 'project'");

        if (!oa.TryGetProperty("id", out var idEl) || !idEl.TryGetInt64(out var pipelineId))
            throw new InvalidOperationException("Pipeline Hook payload missing 'object_attributes.id'");

        if (!oa.TryGetProperty("status", out var statusEl) || statusEl.GetString() is not { } status)
            throw new InvalidOperationException("Pipeline Hook payload missing 'object_attributes.status'");

        if (!oa.TryGetProperty("sha", out var shaEl) || shaEl.GetString() is not { } sha)
            throw new InvalidOperationException("Pipeline Hook payload missing 'object_attributes.sha'");

        if (!proj.TryGetProperty("id", out var projectIdEl) || !projectIdEl.TryGetInt64(out var projectId))
            throw new InvalidOperationException("Pipeline Hook payload missing 'project.id'");

        return new GitLabPipelineHookPayload(pipelineId, status, sha, projectId);
    }
}
