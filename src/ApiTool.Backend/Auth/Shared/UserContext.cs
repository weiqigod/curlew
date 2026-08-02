// Shared user-context resolution used by both /auth/refresh and /api/v1/trials/{feature}
// to re-mint License + Access tokens. Extracted to avoid duplication per M16-007 review finding #5.
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Auth.Shared;

/// <summary>
/// Resolves tier, org, orgRole, and email for a user. Used when re-minting tokens
/// so that the LicenseTokenIssuer receives an accurate user context.
/// Falls back to <c>("free", null, null, "")</c> for users with no org membership.
/// </summary>
internal static class UserContext
{
    /// <summary>
    /// Resolves tier, org, orgRole, and email for the given user.
    /// Falls back to <c>("free", null, null, "")</c> for users with no org membership.
    /// Picks the first (oldest) org membership by JoinedAt ASC.
    /// </summary>
    public static async Task<(string Tier, Guid? OrgId, string? OrgRole, string Email)>
        ResolveAsync(AppDbContext db, Guid userId, CancellationToken ct)
    {
        var user  = await db.Users.FindAsync([userId], ct);
        var email = user?.Email ?? string.Empty;

        var membership = await db.OrganizationMembers
            .Where(m => m.UserId == userId)
            .OrderBy(m => m.JoinedAt)
            .ThenBy(m => m.OrgId)
            .FirstOrDefaultAsync(ct);

        if (membership is null)
            return ("free", null, null, email);

        var subscription = await db.Subscriptions
            .FirstOrDefaultAsync(s => s.OrgId == membership.OrgId, ct);

        var tier    = subscription?.Tier.ToString().ToLowerInvariant() ?? "free";
        var orgRole = membership.Role.ToString().ToLowerInvariant();

        return (tier, membership.OrgId, orgRole, email);
    }
}
