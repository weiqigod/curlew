// Refs docs/SPECIFICATION.md:8501-8511 (CLI state → GitHub conclusion mapping).
namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Maps the CLI's <c>state</c> field to GitHub's <c>conclusion</c> enum
/// per docs/SPECIFICATION.md:8501-8511.
/// </summary>
public static class PrCheckConclusionMapper
{
    /// <summary>The six valid CLI states that map to GitHub conclusions.</summary>
    public static readonly IReadOnlyList<string> ValidStates =
        ["success", "failure", "cancelled", "timed_out", "neutral", "skipped"];

    /// <summary>
    /// Attempts to map a CLI <paramref name="state"/> to a GitHub conclusion string.
    /// Returns <c>true</c> and sets <paramref name="conclusion"/> on success.
    /// Returns <c>false</c> for null, empty, or invalid input (including the banned
    /// <c>action_required</c> value per spec :8512).
    /// </summary>
    public static bool TryMap(string? state, out string conclusion)
    {
        conclusion = string.Empty;
        if (string.IsNullOrWhiteSpace(state)) return false;
        var normalized = state.Trim().ToLowerInvariant();
        // action_required is reserved per :8512 — never emitted in M14.
        if (normalized == "action_required") return false;
        if (!((ICollection<string>)ValidStates).Contains(normalized)) return false;
        conclusion = normalized;
        return true;
    }
}
