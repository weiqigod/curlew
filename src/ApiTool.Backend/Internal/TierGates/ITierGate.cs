namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data.Entities;

/// <summary>
/// Single canonical tier-gate seam (v4.3). Per-feature adapters
/// (<c>SsoTierGate</c>, <c>VaultConfigTierGate</c>, <c>ScheduleExecutorTierGate</c>,
/// <c>DashboardTierGate</c>) all delegate here so the EF subscription-tier query
/// exists in exactly one place.
/// </summary>
public interface ITierGate
{
    /// <summary>
    /// Returns <see cref="TierGateResult.Allowed"/> when the organisation's current
    /// subscription tier is greater-than-or-equal to <paramref name="requiredMinimumTier"/>;
    /// <see cref="TierGateResult.OrgNotFound"/> when the org does not exist;
    /// <see cref="TierGateResult.TierIneligible"/> otherwise.
    /// A missing subscription row is treated as <see cref="SubscriptionTier.Free"/>.
    /// </summary>
    Task<TierGateResult> EnsureAsync(
        Guid orgId,
        SubscriptionTier requiredMinimumTier,
        CancellationToken ct);
}
