using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Organizations;

/// <summary>Business logic for creating and querying organizations.</summary>
public sealed class OrganizationService(
	AppDbContext db,
	TimeProvider clock,
	IAuditWriter audit,
	IOrganizationTierReader tierReader)
{
    /// <summary>Seat limit for the free tier: owner only.</summary>
    public const int FreeTierSeatLimit = 1;

    /// <summary>Maximum seats for the Team tier (kept for compatibility).</summary>
    public const int TeamTierDefaultSeatLimit = 10;

    /// <summary>
    /// Returns the list of organizations the given user is a member of.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<IReadOnlyList<OrganizationDto>> ListForUserAsync(Guid userId, CancellationToken ct)
    {
        // Single query: join memberships → organizations → seat counts via subquery.
        var rows = await db.OrganizationMembers
            .Where(m => m.UserId == userId)
            .Join(db.Organizations,
                  m => m.OrgId,
                  o => o.Id,
                  (m, o) => new
                  {
                      m.Role,
                      Org = o,
                      SeatCount = db.OrganizationMembers.Count(x => x.OrgId == o.Id),
                      SeatLimit = db.Subscriptions
                          .Where(s => s.OrgId == o.Id)
                          .Select(s => (int?)s.SeatLimit)
                          .FirstOrDefault(),
                  })
            .ToListAsync(ct);

		var tiers = await tierReader.GetCurrentTiersAsync(rows.Select(r => r.Org.Id).ToArray(), ct);
		return rows.Select(r => ToDto(
			r.Org,
			r.Role,
			r.SeatCount,
			r.SeatLimit ?? FreeTierSeatLimit,
			tiers[r.Org.Id])).ToList();
    }

    /// <summary>
    /// Creates a new organization, inserting member and audit-log rows in the same transaction.
    /// </summary>
    /// <param name="userId">The creating user — becomes the owner.</param>
    /// <param name="name">Display name.</param>
    /// <param name="slug">URL-safe slug.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// A tuple of (dto, error, message). On success <paramref name="error"/> is
    /// <see cref="OrgError.None"/> and <c>dto</c> is populated. On failure <c>dto</c> is
    /// <see langword="null"/> and <paramref name="error"/> indicates the reason.
    /// </returns>
    public async Task<(OrganizationDto? dto, OrgError error, string? message)>
        CreateAsync(Guid userId, string name, string slug, CancellationToken ct)
    {
        var trimmedName = (name ?? string.Empty).Trim();
        if (trimmedName.Length == 0)
            return (null, OrgError.InvalidName, "Name must not be empty.");
        if (trimmedName.Length > 100)
            return (null, OrgError.InvalidName, "Name must not exceed 100 characters.");

        if (!SlugValidator.TryValidate(slug, out var slugError))
            return (null, OrgError.InvalidSlug, slugError);

        var slugTaken = await db.Organizations.AnyAsync(o => o.Slug == slug, ct);
        if (slugTaken)
            return (null, OrgError.SlugTaken, $"The slug '{slug}' is already taken.");

        var now = clock.GetUtcNow().UtcDateTime;
        var orgId = Guid.NewGuid();

        var org = new Organization
        {
            Id = orgId,
            Name = trimmedName,
            Slug = slug,
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = now,
            UpdatedAt = now,
        };

        var member = new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = OrgRole.Owner,
            JoinedAt = now,
        };

        db.Organizations.Add(org);
        db.OrganizationMembers.Add(member);
        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "org.created",
            Payload: new { name = trimmedName, slug }));

        try
        {
            await db.SaveChangesAsync(ct);
        }
        catch (DbUpdateException ex) when (IsUniqueConstraintViolation(ex))
        {
            // Concurrent-insert safety net: two requests that both pass the AnyAsync check
            // above can race to SaveChangesAsync; the loser hits the UNIQUE constraint on
            // organizations.slug and lands here. This path cannot be exercised by the current
            // in-memory SQLite test infrastructure (a single shared connection serialises
            // writes), so it is intentionally left without automated test coverage.
            db.ChangeTracker.Clear();
            return (null, OrgError.SlugTaken, $"The slug '{slug}' is already taken.");
        }

		return (ToDto(org, OrgRole.Owner, 1, FreeTierSeatLimit, SubscriptionTier.Free), OrgError.None, null);
    }

    /// <summary>
    /// Retrieves an organization by id for a specific user.
    /// Returns <see cref="OrgError.NotFound"/> for both "org doesn't exist" and
    /// "user is not a member" to avoid enumeration.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">The organization's internal id.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(OrganizationDto? dto, OrgError error)>
        GetByIdForUserAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var row = await db.OrganizationMembers
            .Where(m => m.OrgId == orgId && m.UserId == userId)
            .Join(db.Organizations,
                  m => m.OrgId,
                  o => o.Id,
                  (m, o) => new { m, o })
            .SingleOrDefaultAsync(ct);

        if (row is null)
            return (null, OrgError.NotFound);

        var seatCount = await db.OrganizationMembers.CountAsync(x => x.OrgId == orgId, ct);
		var seatLimit = await db.Subscriptions
            .Where(s => s.OrgId == orgId)
            .Select(s => (int?)s.SeatLimit)
			.FirstOrDefaultAsync(ct) ?? FreeTierSeatLimit;
		var tier = await tierReader.GetCurrentTierAsync(orgId, ct);
		return (ToDto(row.o, row.m.Role, seatCount, seatLimit, tier), OrgError.None);
	}

	private static OrganizationDto ToDto(
		Organization org,
		OrgRole role,
		int seatCount,
		int seatLimit,
		SubscriptionTier tier) =>
		new(
            Id: OrgId.Format(org.Id),
            Name: org.Name,
			Slug: org.Slug,
			Role: role.ToString().ToLowerInvariant(),
			Tier: tier.ToString().ToLowerInvariant(),
            SeatCount: seatCount,
            SeatLimit: seatLimit,
            Status: ToStatusString(org.Status),
            CreatedAt: org.CreatedAt
        );

    /// <summary>Converts an <see cref="OrgStatus"/> to its snake_case wire representation.</summary>
    internal static string ToStatusString(OrgStatus status) => status switch
    {
        OrgStatus.Creating => "creating",
        OrgStatus.Active => "active",
        OrgStatus.PendingDeletion => "pending_deletion",
        OrgStatus.Deleted => "deleted",
        _ => status.ToString().ToLowerInvariant(),
    };

    /// <summary>
    /// Returns <see langword="true"/> when a <see cref="DbUpdateException"/> is caused by a
    /// UNIQUE constraint violation (SQLite extended error code 19 — SQLITE_CONSTRAINT).
    /// </summary>
    private static bool IsUniqueConstraintViolation(DbUpdateException ex) =>
        ex.InnerException?.Message.Contains("UNIQUE constraint failed", StringComparison.OrdinalIgnoreCase) == true
        || ex.InnerException?.Message.Contains("UNIQUE", StringComparison.OrdinalIgnoreCase) == true;
}
