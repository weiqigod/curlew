namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data.Entities;
using System.Diagnostics;

/// <summary>
/// Per-feature adapter for schedule-executor endpoints. Requires Team tier or above.
/// Spec: docs/SPECIFICATION.md "Tier-Gate Generic Abstraction (v4.3)".
/// </summary>
public static class ScheduleExecutorTierGate
{
    /// <summary>The minimum subscription tier required to access schedule-executor endpoints.</summary>
    public const SubscriptionTier RequiredMinimum = SubscriptionTier.Team;

    /// <summary>
    /// Delegates to <see cref="ITierGate.EnsureAsync"/> with <see cref="RequiredMinimum"/>
    /// and maps the result to <see cref="ScheduleExecutorError"/>.
    /// </summary>
    public static async Task<ScheduleExecutorError> EnsureTeamOrAboveAsync(
        ITierGate gate, Guid orgId, CancellationToken ct) =>
        await gate.EnsureAsync(orgId, RequiredMinimum, ct) switch
        {
            TierGateResult.Allowed        => ScheduleExecutorError.None,
            TierGateResult.OrgNotFound    => ScheduleExecutorError.OrgNotFound,
            TierGateResult.TierIneligible => ScheduleExecutorError.TierIneligible,
            _ => throw new UnreachableException(),
        };
}
