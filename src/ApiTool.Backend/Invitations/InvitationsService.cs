using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Rbac.CustomRoles;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Invitations;

/// <summary>Business logic for managing organization invitations.</summary>
public sealed class InvitationsService(AppDbContext db, TimeProvider clock, IAuditWriter audit, RoleResolver roleResolver)
{
    private const int ExpiryDays = 7;
    private const int MaxResends = 3;
    private const int ResendCooldownHours = 24;

    /// <summary>
    /// Returns the current seat usage:
    /// active members + pending invitations (not accepted, not revoked, not expired).
    /// Delegates to <see cref="SeatCounter.CountAsync"/> so both services stay in sync.
    /// </summary>
    public Task<int> CountSeatsAsync(Guid orgId, DateTime now, CancellationToken ct)
        => SeatCounter.CountAsync(db, orgId, now, ct);

    /// <summary>Creates a new invitation for the given email address.</summary>
    public async Task<(InvitationDto? dto, string? rawToken, InvitationError err, string? msg)>
        CreateAsync(Guid inviterUserId, Guid orgId, string email, string roleStr, CancellationToken ct)
    {
        // Validate email.
        if (string.IsNullOrWhiteSpace(email))
            return (null, null, InvitationError.InvalidEmail, "Email must not be empty.");

        // Validate role.
        if (!Enum.TryParse<OrgRole>(roleStr, ignoreCase: true, out var role)
            || role == OrgRole.Owner)
            return (null, null, InvitationError.PermissionDenied,
                "Role must be 'admin' or 'member'.");

        // Verify inviter has the members.invite permission (respects custom roles).
        var inviterMember = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == inviterUserId, ct);
        if (inviterMember is null)
            return (null, null, InvitationError.PermissionDenied, "Only admins and owners can invite members.");
        if (!await roleResolver.HasPermissionAsync(inviterUserId, orgId, Permissions.MembersInvite, ct))
            return (null, null, InvitationError.PermissionDenied, "Only admins and owners can invite members.");

        var emailNormalized = email.Trim().ToLowerInvariant();
        var now = clock.GetUtcNow().UtcDateTime;

        // Check if invitee is already a member.
        var existingUser = await db.Users.FirstOrDefaultAsync(u => u.Email == emailNormalized, ct);
        if (existingUser is not null)
        {
            var alreadyMember = await db.OrganizationMembers
                .AnyAsync(m => m.OrgId == orgId && m.UserId == existingUser.Id, ct);
            if (alreadyMember)
                return (null, null, InvitationError.AlreadyMember, "This user is already a member of the organization.");
        }

        // Check for duplicate pending invitation.
        var pending = await db.OrganizationInvitations
            .AnyAsync(i => i.OrgId == orgId
                && i.EmailNormalized == emailNormalized
                && i.AcceptedAt == null
                && i.RevokedAt == null
                && i.ExpiresAt > now, ct);
        if (pending)
            return (null, null, InvitationError.InvitationPending,
                $"A pending invitation for {email} already exists.");

        // Check seat limit.
        var sub = await db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == orgId, ct);
        var seatLimit = sub?.SeatLimit ?? 1;
        var currentSeats = await CountSeatsAsync(orgId, now, ct);
        if (currentSeats >= seatLimit)
            return (null, null, InvitationError.SeatLimitReached,
                $"Seat limit of {seatLimit} has been reached.");

        // Generate token.
        var rawToken = InvitationTokenGenerator.GenerateRawToken();
        var tokenHash = InvitationTokenGenerator.HashToken(rawToken);

        var invitation = new OrganizationInvitation
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Email = email.Trim(),
            EmailNormalized = emailNormalized,
            Role = role,
            InvitedBy = inviterUserId,
            TokenHash = tokenHash,
            ExpiresAt = now.AddDays(ExpiryDays),
            CreatedAt = now,
            LastSentAt = now,
            SendCount = 1,
        };
        db.OrganizationInvitations.Add(invitation);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: inviterUserId,
            EventType: "member.invited",
            TargetType: "invitation",
            TargetId: invitation.Id,
            Payload: new { email = emailNormalized, role = roleStr.ToLowerInvariant() },
            NewState: new { email = emailNormalized, role = roleStr.ToLowerInvariant() }));

        await db.SaveChangesAsync(ct);

        return (ToDto(invitation), rawToken, InvitationError.None, null);
    }

    /// <summary>Lists all pending invitations for the given organization.</summary>
    public async Task<(IReadOnlyList<InvitationDto> invitations, InvitationError err)>
        ListPendingAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null)
            return ([], InvitationError.OrganizationNotFound);

        var now = clock.GetUtcNow().UtcDateTime;
        var invitations = await db.OrganizationInvitations
            .Where(i => i.OrgId == orgId && i.AcceptedAt == null && i.RevokedAt == null && i.ExpiresAt > now)
            .OrderByDescending(i => i.CreatedAt)
            .ToListAsync(ct);

        return (invitations.Select(ToDto).ToList(), InvitationError.None);
    }

    /// <summary>Resends an existing invitation.</summary>
    public async Task<(InvitationDto? dto, InvitationError err, string? msg)>
        ResendAsync(Guid userId, Guid orgId, Guid invitationId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null || member.Role == OrgRole.Member)
            return (null, InvitationError.PermissionDenied, "Only admins and owners can resend invitations.");

        var invitation = await db.OrganizationInvitations
            .FirstOrDefaultAsync(i => i.Id == invitationId && i.OrgId == orgId, ct);
        if (invitation is null)
            return (null, InvitationError.InvitationNotFound, "Invitation not found.");

        if (invitation.SendCount > MaxResends)
            return (null, InvitationError.ResendLimitReached,
                $"This invitation has been sent {invitation.SendCount} times (limit: {MaxResends}).");

        var now = clock.GetUtcNow().UtcDateTime;

        if (invitation.LastSentAt.AddHours(ResendCooldownHours) > now)
            return (null, InvitationError.ResendCooldown,
                $"Please wait {ResendCooldownHours} hours between resends.");

        invitation.SendCount++;
        invitation.LastSentAt = now;
        await db.SaveChangesAsync(ct);

        return (ToDto(invitation), InvitationError.None, null);
    }

    /// <summary>Revokes (deletes) an invitation, freeing the seat.</summary>
    public async Task<InvitationError> RevokeAsync(Guid userId, Guid orgId, Guid invitationId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null || member.Role == OrgRole.Member)
            return InvitationError.PermissionDenied;

        var invitation = await db.OrganizationInvitations
            .FirstOrDefaultAsync(i => i.Id == invitationId && i.OrgId == orgId, ct);
        if (invitation is null)
            return InvitationError.InvitationNotFound;

        var now = clock.GetUtcNow().UtcDateTime;
        invitation.RevokedAt = now;
        invitation.RevokedBy = userId;

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "member.invitation_revoked",
            TargetType: "invitation",
            TargetId: invitationId));

        await db.SaveChangesAsync(ct);
        return InvitationError.None;
    }

    /// <summary>Accepts an invitation using the raw token.</summary>
    public async Task<(InvitationDto? dto, OrgRole? role, InvitationError err, string? msg)>
        AcceptAsync(Guid acceptingUserId, string rawToken, CancellationToken ct)
    {
        var tokenHash = InvitationTokenGenerator.HashToken(rawToken);
        var invitation = await db.OrganizationInvitations
            .FirstOrDefaultAsync(i => i.TokenHash == tokenHash, ct);

        if (invitation is null)
            return (null, null, InvitationError.InvitationNotFound, "Invitation not found.");

        var now = clock.GetUtcNow().UtcDateTime;

        if (invitation.ExpiresAt <= now)
            return (null, null, InvitationError.InvitationExpired, "This invitation has expired.");

        // Check if user is already a member.
        var alreadyMember = await db.OrganizationMembers
            .AnyAsync(m => m.OrgId == invitation.OrgId && m.UserId == acceptingUserId, ct);
        if (alreadyMember)
        {
            // Mark accepted to clean up the pending row.
            invitation.AcceptedAt = now;
            await db.SaveChangesAsync(ct);
            return (null, null, InvitationError.AlreadyMember, "You are already a member of this organization.");
        }

        // Add membership.
        var member = new OrganizationMember
        {
            OrgId = invitation.OrgId,
            UserId = acceptingUserId,
            Role = invitation.Role,
            JoinedAt = now,
            InvitedBy = invitation.InvitedBy,
        };
        db.OrganizationMembers.Add(member);
        invitation.AcceptedAt = now;

        audit.Append(new AuditEvent(
            OrgId: invitation.OrgId,
            ActorId: acceptingUserId,
            EventType: "member.invitation_accepted",
            TargetType: "invitation",
            TargetId: invitation.Id,
            Payload: new { role = invitation.Role.ToString().ToLowerInvariant() }));

        await db.SaveChangesAsync(ct);
        return (ToDto(invitation), invitation.Role, InvitationError.None, null);
    }

    private static InvitationDto ToDto(OrganizationInvitation i) => new(
        Id: InvitationId.Format(i.Id),
        OrgId: OrgId.Format(i.OrgId),
        Email: i.Email,
        Role: i.Role.ToString().ToLowerInvariant(),
        ExpiresAt: i.ExpiresAt,
        CreatedAt: i.CreatedAt,
        AcceptedAt: i.AcceptedAt,
        RevokedAt: i.RevokedAt);
}
