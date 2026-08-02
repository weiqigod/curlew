using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Organizations;

/// <summary>
/// Resolves an organization's internal GUID from either a wire ID (<c>org_&lt;hex&gt;</c>)
/// or a URL slug. Used by endpoints that accept org slugs from CLI clients.
/// </summary>
public static class OrgResolver
{
    /// <summary>
    /// Tries to resolve <paramref name="orgIdOrSlug"/> to an internal org GUID.
    /// First attempts wire-ID parsing (<c>org_&lt;hex&gt;</c>); falls back to slug lookup.
    /// </summary>
    /// <returns>The org GUID, or <see langword="null"/> if not found.</returns>
    public static async Task<Guid?> ResolveAsync(
        string orgIdOrSlug,
        AppDbContext db,
        CancellationToken ct)
    {
        if (OrgId.TryParse(orgIdOrSlug, out var guid))
            return guid;

        return await db.Organizations
            .Where(o => o.Slug == orgIdOrSlug)
            .Select(o => (Guid?)o.Id)
            .FirstOrDefaultAsync(ct);
    }
}
