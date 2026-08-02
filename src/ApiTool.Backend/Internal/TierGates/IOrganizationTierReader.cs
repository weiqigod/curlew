namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data.Entities;

/// <summary>
/// Read-only tier projection seam for response DTOs. Tier enforcement remains
/// on <see cref="ITierGate"/>; both interfaces share the canonical
/// <see cref="TierGate"/> implementation so subscription-tier queries stay in
/// one package.
/// </summary>
public interface IOrganizationTierReader
{
    /// <summary>Returns the current tier, defaulting a missing subscription to Free.</summary>
    Task<SubscriptionTier> GetCurrentTierAsync(Guid orgId, CancellationToken ct);

    /// <summary>
    /// Batch form used by organization listings to avoid one query per row.
    /// Every requested id is present in the returned dictionary.
    /// </summary>
    Task<IReadOnlyDictionary<Guid, SubscriptionTier>> GetCurrentTiersAsync(
        IReadOnlyCollection<Guid> orgIds,
        CancellationToken ct);
}
