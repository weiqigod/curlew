using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Rbac.CustomRoles;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Organizations;

/// <summary>Business logic for managing organization members, roles, and org lifecycle.</summary>
public sealed class MembersService(
    AppDbContext db,
    TimeProvider clock,
    IAuditWriter audit,
    ITierGate tierGate,
    IOrganizationTierReader tierReader)
{
    /// <summary>Lists all members of the given organization.</summary>
    public async Task<(IReadOnlyList<MemberDto> members, MemberError err)>
        ListMembersAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var requestor = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (requestor is null)
            return ([], MemberError.OrganizationNotFound);

        // Project to a tuple first; formatting custom role ids is done client-side so
        // the projection stays translatable across EF providers (SQLite's query translator
        // rejects Guid.ToString("N") even though InMemory accepts it).
        var rows = await db.OrganizationMembers
            .Where(m => m.OrgId == orgId)
            .OrderBy(m => m.JoinedAt)
            .Select(m => new { m.UserId, m.Role, m.JoinedAt, m.RoleId })
            .ToListAsync(ct);

        var members = rows
            .Select(r => new MemberDto(
                r.UserId.ToString("N"),
                r.Role.ToString().ToLowerInvariant(),
                r.JoinedAt,
                r.RoleId is null ? null : RoleId.Format(r.RoleId.Value)))
            .ToList();

        return (members, MemberError.None);
    }

    /// <summary>
    /// Updates a member's built-in role and/or custom role id. Owner only.
    /// When <paramref name="roleStr"/> is non-null, the built-in role is changed.
    /// When <paramref name="roleId"/> is non-null, the custom role FK is set.
    /// Emits <c>member.role_changed</c> (built-in change) or <c>role.changed</c> (custom role FK assignment).
    /// </summary>
    public async Task<(MemberDto? member, MemberError err)>
        UpdateMemberAsync(
            Guid requestorId, Guid orgId, Guid targetUserId,
            string? roleStr, Guid? roleId,
            CancellationToken ct)
    {
        var requestor = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == requestorId, ct);
        if (requestor is null || requestor.Role != OrgRole.Owner)
            return (null, MemberError.PermissionDenied);

        var target = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == targetUserId, ct);
        if (target is null)
            return (null, MemberError.MemberNotFound);

        if (target.Role == OrgRole.Owner)
            return (null, MemberError.CannotChangeOwnerRole);

        if (roleStr is not null)
        {
            if (!Enum.TryParse<OrgRole>(roleStr, ignoreCase: true, out var newRole) || newRole == OrgRole.Owner)
                return (null, MemberError.PermissionDenied);

            target.Role = newRole;

            audit.Append(new AuditEvent(
                OrgId: orgId,
                ActorId: requestorId,
                EventType: "member.role_changed",
                TargetType: "member",
                TargetId: targetUserId,
                Payload: new { role = roleStr.ToLowerInvariant() }));
        }

        if (roleId is { } rid)
        {
            // Validate the custom role belongs to the same org.
            var customRole = await db.OrganizationCustomRoles
                .FirstOrDefaultAsync(r => r.Id == rid && r.OrgId == orgId, ct);
            if (customRole is null)
                return (null, MemberError.PermissionDenied);

            target.RoleId = rid;

            audit.Append(new AuditEvent(
                OrgId: orgId,
                ActorId: requestorId,
                EventType: "role.changed",
                TargetType: "member",
                TargetId: targetUserId,
                Payload: new { role_id = RoleId.Format(rid) }));
        }

        await db.SaveChangesAsync(ct);
        return (new MemberDto(
            target.UserId.ToString("N"),
            target.Role.ToString().ToLowerInvariant(),
            target.JoinedAt,
            target.RoleId is null ? null : RoleId.Format(target.RoleId.Value)), MemberError.None);
    }

    /// <summary>Removes a member from the organization.</summary>
    public async Task<MemberError> RemoveMemberAsync(Guid requestorId, Guid orgId, Guid targetUserId, CancellationToken ct)
    {
        var requestor = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == requestorId, ct);
        if (requestor is null || requestor.Role == OrgRole.Member)
            return MemberError.PermissionDenied;

        var target = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == targetUserId, ct);
        if (target is null)
            return MemberError.MemberNotFound;

        if (target.Role == OrgRole.Owner)
            return MemberError.CannotRemoveOwner;

        var now = clock.GetUtcNow().UtcDateTime;
        db.OrganizationMembers.Remove(target);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: requestorId,
            EventType: "member.removed",
            TargetType: "member",
            TargetId: targetUserId));

        await db.SaveChangesAsync(ct);
        return MemberError.None;
    }

    /// <summary>Transfers ownership to an admin member.</summary>
    public async Task<MemberError> TransferOwnershipAsync(Guid requestorId, Guid orgId, Guid newOwnerId, CancellationToken ct)
    {
        var requestor = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == requestorId, ct);
        if (requestor is null || requestor.Role != OrgRole.Owner)
            return MemberError.PermissionDenied;

        var target = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == newOwnerId, ct);
        if (target is null || target.Role == OrgRole.Member)
            return MemberError.InvalidTransferTarget;

        var now = clock.GetUtcNow().UtcDateTime;
        requestor.Role = OrgRole.Admin;
        target.Role = OrgRole.Owner;

        // Update org.OwnerId.
        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is not null)
        {
            org.OwnerId = newOwnerId;
            org.UpdatedAt = now;
        }

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: requestorId,
            EventType: "org.ownership_transferred",
            TargetType: "member",
            TargetId: newOwnerId));

        await db.SaveChangesAsync(ct);
        return MemberError.None;
    }

    /// <summary>Allows a non-owner member to leave the organization.</summary>
    public async Task<MemberError> LeaveAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null)
            return MemberError.OrganizationNotFound;

        if (member.Role == OrgRole.Owner)
            return MemberError.OwnerCannotLeave;

        var now = clock.GetUtcNow().UtcDateTime;
        db.OrganizationMembers.Remove(member);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "member.left",
            TargetType: "member",
            TargetId: userId));

        await db.SaveChangesAsync(ct);
        return MemberError.None;
    }

    /// <summary>
    /// Updates the organization's name and/or audit-log retention settings.
    /// Non-Enterprise orgs cannot set <c>auditLogRetentionDays</c> &gt; 365 (v4-2 cap).
    /// </summary>
    public async Task<(OrganizationDto? dto, MemberError err)>
        UpdateOrgAsync(Guid requestorId, Guid orgId, string? name, int? auditLogRetentionDays, CancellationToken ct)
    {
        var requestor = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == requestorId, ct);
        if (requestor is null || requestor.Role == OrgRole.Member)
            return (null, MemberError.PermissionDenied);

        // Validate retention days before touching the org row.
        if (auditLogRetentionDays.HasValue)
        {
            if (auditLogRetentionDays.Value <= 0)
                return (null, MemberError.RetentionDaysInvalid);

            if (auditLogRetentionDays.Value > 365)
            {
                var gateResult = await tierGate.EnsureAsync(orgId, SubscriptionTier.Enterprise, ct);
                if (gateResult != TierGateResult.Allowed)
                    return (null, MemberError.RetentionDaysExceedsCap);
            }
        }

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, MemberError.OrganizationNotFound);

        var now = clock.GetUtcNow().UtcDateTime;
        var prevName = org.Name;
        var prevRetentionDays = org.AuditLogRetentionDays;
        if (name is not null)
        {
            org.Name = name.Trim();
            org.UpdatedAt = now;
        }

        if (auditLogRetentionDays.HasValue)
        {
            org.AuditLogRetentionDays = auditLogRetentionDays.Value;
            org.UpdatedAt = now;
        }

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: requestorId,
            EventType: "org.settings.updated",
            PreviousState: new { name = prevName, audit_log_retention_days = prevRetentionDays },
            NewState: new { name = org.Name, audit_log_retention_days = org.AuditLogRetentionDays }));

        await db.SaveChangesAsync(ct);

        var seatCount = await db.OrganizationMembers.CountAsync(m => m.OrgId == orgId, ct);
        var seatLimit = await db.Subscriptions
            .Where(s => s.OrgId == orgId)
            .Select(s => (int?)s.SeatLimit)
            .FirstOrDefaultAsync(ct) ?? OrganizationService.FreeTierSeatLimit;
		var tier = await tierReader.GetCurrentTierAsync(orgId, ct);

		return (ToOrgDto(org, requestor.Role, seatCount, seatLimit, tier), MemberError.None);
    }

    /// <summary>Marks an organization as pending deletion.</summary>
    public async Task<(OrganizationDto? dto, MemberError err)>
        DeleteOrgAsync(Guid requestorId, Guid orgId, CancellationToken ct)
    {
        var requestor = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == requestorId, ct);
        if (requestor is null || requestor.Role != OrgRole.Owner)
            return (null, MemberError.PermissionDenied);

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, MemberError.OrganizationNotFound);

        var now = clock.GetUtcNow().UtcDateTime;
        org.Status = OrgStatus.PendingDeletion;
        org.UpdatedAt = now;

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: requestorId,
            EventType: "org.deletion_scheduled"));

        await db.SaveChangesAsync(ct);

        var seatCount = await db.OrganizationMembers.CountAsync(m => m.OrgId == orgId, ct);
        var seatLimit = await db.Subscriptions
            .Where(s => s.OrgId == orgId)
            .Select(s => (int?)s.SeatLimit)
            .FirstOrDefaultAsync(ct) ?? OrganizationService.FreeTierSeatLimit;
		var tier = await tierReader.GetCurrentTierAsync(orgId, ct);
		return (ToOrgDto(org, OrgRole.Owner, seatCount, seatLimit, tier), MemberError.None);
    }

    /// <summary>Cancels a pending deletion, restoring the organization to active.</summary>
    public async Task<(OrganizationDto? dto, MemberError err)>
        CancelDeletionAsync(Guid requestorId, Guid orgId, CancellationToken ct)
    {
        var requestor = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == requestorId, ct);
        if (requestor is null || requestor.Role != OrgRole.Owner)
            return (null, MemberError.PermissionDenied);

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, MemberError.OrganizationNotFound);

        if (org.Status != OrgStatus.PendingDeletion)
            return (null, MemberError.InvalidState);

        var now = clock.GetUtcNow().UtcDateTime;
        org.Status = OrgStatus.Active;
        org.UpdatedAt = now;

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: requestorId,
            EventType: "org.deletion_canceled"));

        await db.SaveChangesAsync(ct);

        var seatCount = await db.OrganizationMembers.CountAsync(m => m.OrgId == orgId, ct);
        var seatLimit = await db.Subscriptions
            .Where(s => s.OrgId == orgId)
            .Select(s => (int?)s.SeatLimit)
            .FirstOrDefaultAsync(ct) ?? OrganizationService.FreeTierSeatLimit;
		var tier = await tierReader.GetCurrentTierAsync(orgId, ct);
		return (ToOrgDto(org, OrgRole.Owner, seatCount, seatLimit, tier), MemberError.None);
	}

	private static OrganizationDto ToOrgDto(
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
            Status: OrganizationService.ToStatusString(org.Status),
            CreatedAt: org.CreatedAt);
}
