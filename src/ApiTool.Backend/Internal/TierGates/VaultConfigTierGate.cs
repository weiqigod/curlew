namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data.Entities;
using System.Diagnostics;

/// <summary>
/// Per-feature adapter for shared-vault template endpoints. Requires Team tier or above.
/// Spec: docs/SPECIFICATION.md "Tier-Gate Generic Abstraction (v4.3)".
/// </summary>
public static class VaultConfigTierGate
{
    /// <summary>The minimum subscription tier required to access vault-config endpoints.</summary>
    public const SubscriptionTier RequiredMinimum = SubscriptionTier.Team;

    /// <summary>
    /// Delegates to <see cref="ITierGate.EnsureAsync"/> with <see cref="RequiredMinimum"/>
    /// and maps the result to <see cref="VaultConfigError"/>.
    /// </summary>
    public static async Task<VaultConfigError> EnsureTeamOrAboveAsync(
        ITierGate gate, Guid orgId, CancellationToken ct) =>
        await gate.EnsureAsync(orgId, RequiredMinimum, ct) switch
        {
            TierGateResult.Allowed        => VaultConfigError.None,
            TierGateResult.OrgNotFound    => VaultConfigError.OrgNotFound,
            TierGateResult.TierIneligible => VaultConfigError.TierIneligible,
            _ => throw new UnreachableException(),
        };
}
