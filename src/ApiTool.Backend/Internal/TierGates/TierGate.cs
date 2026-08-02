namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

/// <summary>
/// Canonical <see cref="ITierGate"/> implementation backed by <see cref="AppDbContext"/>.
/// Lifted verbatim from <c>SsoTierGate.EnsureEnterpriseAsync</c> (M15-002) and generalised
/// over the required minimum tier. The single source of truth for tier checks.
/// </summary>
internal sealed class TierGate(AppDbContext db) : ITierGate, IOrganizationTierReader
{
	/// <inheritdoc />
	public async Task<SubscriptionTier> GetCurrentTierAsync(Guid orgId, CancellationToken ct) =>
		await db.Subscriptions
			.Where(s => s.OrgId == orgId)
			.OrderByDescending(s => s.UpdatedAt)
			.Select(s => (SubscriptionTier?)s.Tier)
			.FirstOrDefaultAsync(ct) ?? SubscriptionTier.Free;

	/// <inheritdoc />
	public async Task<IReadOnlyDictionary<Guid, SubscriptionTier>> GetCurrentTiersAsync(
		IReadOnlyCollection<Guid> orgIds,
		CancellationToken ct)
	{
		var ids = orgIds.Distinct().ToArray();
		var result = ids.ToDictionary(id => id, _ => SubscriptionTier.Free);
		if (ids.Length == 0) return result;

		var rows = await db.Subscriptions
			.Where(s => ids.Contains(s.OrgId))
			.OrderByDescending(s => s.UpdatedAt)
			.Select(s => new { s.OrgId, s.Tier })
			.ToListAsync(ct);

		var seen = new HashSet<Guid>();
		foreach (var row in rows)
		{
			if (seen.Add(row.OrgId)) result[row.OrgId] = row.Tier;
		}
		return result;
	}

	/// <inheritdoc />
    public async Task<TierGateResult> EnsureAsync(
        Guid orgId,
        SubscriptionTier requiredMinimumTier,
        CancellationToken ct)
    {
        var orgExists = await db.Organizations.AnyAsync(o => o.Id == orgId, ct);
        if (!orgExists) return TierGateResult.OrgNotFound;

		var current = await GetCurrentTierAsync(orgId, ct);

        return current >= requiredMinimumTier
            ? TierGateResult.Allowed
            : TierGateResult.TierIneligible;
    }
}
