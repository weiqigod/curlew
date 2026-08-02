using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Data;

/// <summary>
/// Shared seat-counting logic used by both <c>SubscriptionsService</c> and
/// <c>InvitationsService</c> to ensure consistent enforcement of the rule:
/// <para>seat_count = active members + pending invitations
/// (not accepted, not revoked, not expired).</para>
/// </summary>
public static class SeatCounter
{
    /// <summary>
    /// Returns the current seat usage for the given organization.
    /// </summary>
    /// <param name="db">The application database context.</param>
    /// <param name="orgId">The organization identifier.</param>
    /// <param name="now">The reference UTC timestamp for expiry comparisons.</param>
    /// <param name="ct">Cancellation token.</param>
    public static async Task<int> CountAsync(AppDbContext db, Guid orgId, DateTime now, CancellationToken ct)
    {
        var memberCount = await db.OrganizationMembers.CountAsync(m => m.OrgId == orgId, ct);
        var pendingInvites = await db.OrganizationInvitations.CountAsync(
            i => i.OrgId == orgId
              && i.AcceptedAt == null
              && i.RevokedAt == null
              && i.ExpiresAt > now,
            ct);
        return memberCount + pendingInvites;
    }
}
