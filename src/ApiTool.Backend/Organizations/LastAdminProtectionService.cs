using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Organizations;

/// <summary>
/// Classifies organizations owned by a user into "blocking" and "cascade" categories
/// to support the GDPR account-deletion last-admin protection (spec v4-7).
/// <para>
/// <b>Blocking:</b> the user is the sole <see cref="OrgRole.Owner"/> of the org AND
/// other members (non-owner) are present. The user must transfer ownership or remove
/// all other members before the deletion can proceed.
/// </para>
/// <para>
/// <b>Cascade:</b> the user is the sole <see cref="OrgRole.Owner"/> of the org AND
/// no other members exist. The org can be moved to <see cref="OrgStatus.PendingDeletion"/>
/// atomically with the user's deletion request.
/// </para>
/// <para>
/// Orgs where another Owner also exists are <b>neither</b> blocking nor cascade — another
/// owner remains, so the user can leave without consequence.
/// </para>
/// </summary>
public sealed class LastAdminProtectionService(AppDbContext db)
{
    /// <summary>
    /// Classifies orgs the user owns into blocking vs cascade.
    /// Returns <c>(Blocking, CascadeOrgIds)</c>.
    /// </summary>
    public async Task<(IReadOnlyList<BlockingOrg> Blocking, IReadOnlyList<Guid> CascadeOrgIds)>
        ClassifyOwnedOrgsAsync(Guid userId, CancellationToken ct)
    {
        // Find all orgs where this user is an Owner.
        var ownedOrgIds = await db.OrganizationMembers
            .Where(m => m.UserId == userId && m.Role == OrgRole.Owner)
            .Select(m => m.OrgId)
            .ToListAsync(ct);

        if (ownedOrgIds.Count == 0)
            return ([], []);

        // For each owned org, count total members and count of other owners.
        var orgStats = await db.OrganizationMembers
            .Where(m => ownedOrgIds.Contains(m.OrgId))
            .GroupBy(m => m.OrgId)
            .Select(g => new
            {
                OrgId = g.Key,
                TotalMembers = g.Count(),
                OtherOwners = g.Count(m => m.Role == OrgRole.Owner && m.UserId != userId),
            })
            .ToListAsync(ct);

        // Separate into blocking (sole owner + other members) vs cascade (sole owner + zero others).
        var blockingOrgIds = orgStats
            .Where(s => s.OtherOwners == 0 && s.TotalMembers > 1)
            .Select(s => s.OrgId)
            .ToList();

        var cascadeOrgIds = orgStats
            .Where(s => s.OtherOwners == 0 && s.TotalMembers == 1)
            .Select(s => s.OrgId)
            .ToList();

        if (blockingOrgIds.Count == 0)
            return ([], cascadeOrgIds);

        // Fetch slug + name for the blocking orgs so callers can surface them to the user.
        var blockingOrgs = await db.Organizations
            .Where(o => blockingOrgIds.Contains(o.Id))
            .Select(o => new BlockingOrg(o.Slug, o.Name))
            .ToListAsync(ct);

        return (blockingOrgs, cascadeOrgIds);
    }
}
