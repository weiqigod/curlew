// Refs docs/SPECIFICATION.md:9233-9242 (lossy state-mapping table: 6 CLI states → 4 GitLab states).
namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Maps the CLI's <c>state</c> field to GitLab's commit-status state per spec :9233-9242.
/// The mapping is lossy: GitLab has 4 states, the CLI has 6; <c>timed_out</c> collapses
/// to <c>failed</c> with a description marker; <c>neutral</c>/<c>skipped</c> collapse to
/// <c>success</c> with description markers.
/// </summary>
public static class GitLabStateMapper
{
    /// <summary>Maximum length of GitLab's description field (hard-capped server-side).</summary>
    public const int MaxDescriptionLength = 255;

    private const string TruncationMarker = "…(truncated)";

    /// <summary>Attempts to map CLI state to GitLab commit-status state. Returns true on success.</summary>
    public static bool TryMap(string? cliState, out string gitlabState)
    {
        gitlabState = string.Empty;
        if (string.IsNullOrWhiteSpace(cliState)) return false;
        var n = cliState.Trim().ToLowerInvariant();
        gitlabState = n switch
        {
            "success"   => "success",
            "failure"   => "failed",
            "cancelled" => "canceled",
            "timed_out" => "failed",
            "neutral"   => "success",
            "skipped"   => "success",
            _           => string.Empty,
        };
        return gitlabState.Length > 0;
    }

    /// <summary>
    /// Builds the GitLab description: prepends a marker for lossy states, then truncates
    /// to <see cref="MaxDescriptionLength"/> with a <c>…(truncated)</c> suffix when needed.
    /// Control characters (\r, \n) are replaced with spaces to avoid unexpected rendering.
    /// </summary>
    public static string BuildDescription(string? cliState, string? originalDescription)
    {
        var n = (cliState ?? string.Empty).Trim().ToLowerInvariant();
        var prefix = n switch
        {
            "timed_out" => "[timed out] ",
            "neutral"   => "neutral: ",
            "skipped"   => "skipped: ",
            _           => string.Empty,
        };
        var desc = (originalDescription ?? string.Empty).Replace('\r', ' ').Replace('\n', ' ');
        var combined = prefix + desc;
        if (combined.Length <= MaxDescriptionLength) return combined;
        return string.Concat(
            combined.AsSpan(0, MaxDescriptionLength - TruncationMarker.Length),
            TruncationMarker);
    }
}
