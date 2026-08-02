namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Computes the JWT-facing trial state for a user, given their effective tier.
/// Refs docs/SPECIFICATION.md:5806-5816 (transition table).
/// </summary>
/// <remarks>
/// Multi-org users see preemption for all trials when ANY of their orgs upgrades.
/// This is per spec — trials are a user-level concept; tier resolution is per-JWT.
/// </remarks>
public interface ITrialStateResolver
{
    /// <summary>
    /// Resolves the trial state for the given user.
    /// </summary>
    /// <param name="userId">User whose trials are being queried.</param>
    /// <param name="tier">
    /// User's effective tier for the current JWT (from <c>AuthRefreshEndpoints.ResolveUserContextAsync</c>).
    /// Any tier other than <c>free</c> short-circuits to <c>none</c> per spec row "Active subscription (Solo+)".
    /// </param>
    /// <param name="ct">Cancellation token.</param>
    Task<TrialStateResult> ResolveAsync(Guid userId, string tier, CancellationToken ct = default);
}
