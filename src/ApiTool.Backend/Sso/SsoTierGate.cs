using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;

namespace ApiTool.Backend.Sso;

/// <summary>
/// Adapter (v4.3) over the canonical <see cref="ITierGate"/>. The existing
/// static signature is preserved so every <see cref="SsoService"/> and
/// <see cref="OidcService"/> call site compiles unchanged.
/// </summary>
internal static class SsoTierGate
{
    /// <summary>
    /// Delegates to <c>new TierGate(db).EnsureAsync(orgId, SubscriptionTier.Enterprise, ct)</c>
    /// and translates <see cref="TierGateResult"/> into <see cref="SsoError"/>.
    /// </summary>
    public static async Task<SsoError> EnsureEnterpriseAsync(AppDbContext db, Guid orgId, CancellationToken ct)
    {
        var result = await new TierGate(db).EnsureAsync(orgId, SubscriptionTier.Enterprise, ct);
        return result switch
        {
            TierGateResult.Allowed        => SsoError.None,
            TierGateResult.OrgNotFound    => SsoError.OrgNotFound,
            TierGateResult.TierIneligible => SsoError.TierIneligible,
            _ => throw new System.Diagnostics.UnreachableException(),
        };
    }
}
