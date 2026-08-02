namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data.Entities;
using System.Diagnostics;

/// <summary>
/// Per-feature adapter for dashboard endpoints. Requires Team tier or above.
/// Spec: docs/SPECIFICATION.md "Tier-Gate Generic Abstraction (v4.3)".
/// </summary>
public static class DashboardTierGate
{
    /// <summary>The minimum subscription tier required to access dashboard endpoints.</summary>
    public const SubscriptionTier RequiredMinimum = SubscriptionTier.Team;

    /// <summary>
    /// Delegates to <see cref="ITierGate.EnsureAsync"/> with <see cref="RequiredMinimum"/>
    /// and maps the result to <see cref="DashboardError"/>.
    /// </summary>
    public static async Task<DashboardError> EnsureTeamOrAboveAsync(
        ITierGate gate, Guid orgId, CancellationToken ct) =>
        await gate.EnsureAsync(orgId, RequiredMinimum, ct) switch
        {
            TierGateResult.Allowed        => DashboardError.None,
            TierGateResult.OrgNotFound    => DashboardError.OrgNotFound,
            TierGateResult.TierIneligible => DashboardError.TierIneligible,
            _ => throw new UnreachableException(),
        };
}
