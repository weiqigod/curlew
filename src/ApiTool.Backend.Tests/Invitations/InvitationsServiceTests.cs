using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Invitations;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Invitations;

/// <summary>Verifies InvitationsService business rules against in-memory SQLite.</summary>
public sealed class InvitationsServiceTests
{
    private static async Task<(TestDbScope scope, AppDbContext db, InvitationsService svc, Guid userId, Guid orgId)>
        BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = "owner@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Acme", Slug = $"acme-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        // Give the org a subscription with 5 seats.
        db.Subscriptions.Add(new ApiTool.Backend.Data.Entities.Subscription
        {
            Id = Guid.NewGuid(), OrgId = orgId,
            Tier = ApiTool.Backend.Data.Entities.SubscriptionTier.Team,
            Status = ApiTool.Backend.Data.Entities.SubscriptionStatus.Active,
            Interval = "month", SeatCount = 5, SeatLimit = 5,
            CurrentPeriodStart = DateTime.UtcNow, CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
        var roleResolver = new RoleResolver(db);
        var svc = new InvitationsService(db, TimeProvider.System, auditWriter, roleResolver);
        return (scope, db, svc, userId, orgId);
    }

    [Fact]
    public async Task CreateAsync_writes_member_invited_audit_log_row()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, rawToken, err, _) = await svc.CreateAsync(
                userId, orgId, "newuser@example.com", "member", default);

            err.Should().Be(InvitationError.None);
            dto.Should().NotBeNull();
            rawToken.Should().NotBeNullOrEmpty();

            // Assert the audit log row was written.
            var auditEntry = await db.OrganizationAuditLog
                .FirstOrDefaultAsync(e => e.OrgId == orgId && e.EventType == "member.invited");
            auditEntry.Should().NotBeNull();
            auditEntry!.ActorId.Should().Be(userId);
        }
    }

    [Fact]
    public async Task CountSeatsAsync_equals_members_plus_pending_invitations()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;

            // Add a pending invitation.
            db.OrganizationInvitations.Add(new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "pending@example.com", EmailNormalized = "pending@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('a', 64),
                ExpiresAt = now.AddDays(7), CreatedAt = now, LastSentAt = now, SendCount = 1,
            });
            await db.SaveChangesAsync();

            var count = await svc.CountSeatsAsync(orgId, now, default);

            // 1 member (owner) + 1 pending invitation = 2.
            count.Should().Be(2);
        }
    }

    [Fact]
    public async Task ResendAsync_within_cooldown_returns_ResendCooldown()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;

            // Seed an invitation whose LastSentAt is within the 24-hour cooldown window.
            var invitation = new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "cooldown@example.com", EmailNormalized = "cooldown@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('d', 64),
                ExpiresAt = now.AddDays(6), CreatedAt = now.AddHours(-1),
                LastSentAt = now.AddMinutes(-30), // within 24-hour cooldown
                SendCount = 1,
            };
            db.OrganizationInvitations.Add(invitation);
            await db.SaveChangesAsync();

            var (dto, err, msg) = await svc.ResendAsync(userId, orgId, invitation.Id, default);

            err.Should().Be(InvitationError.ResendCooldown);
            dto.Should().BeNull();
            msg.Should().Contain("24");
        }
    }

    [Fact]
    public async Task ResendAsync_after_cooldown_succeeds()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;

            // Seed an invitation whose LastSentAt is beyond the 24-hour cooldown.
            var invitation = new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "cooldown2@example.com", EmailNormalized = "cooldown2@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('e', 64),
                ExpiresAt = now.AddDays(6), CreatedAt = now.AddDays(-2),
                LastSentAt = now.AddHours(-25), // cooldown window elapsed
                SendCount = 1,
            };
            db.OrganizationInvitations.Add(invitation);
            await db.SaveChangesAsync();

            var (dto, err, _) = await svc.ResendAsync(userId, orgId, invitation.Id, default);

            err.Should().Be(InvitationError.None);
            dto.Should().NotBeNull();
        }
    }

    [Fact]
    public async Task CountSeatsAsync_excludes_expired_and_revoked_invitations()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;

            // Expired.
            db.OrganizationInvitations.Add(new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "expired@example.com", EmailNormalized = "expired@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('b', 64),
                ExpiresAt = now.AddDays(-1), CreatedAt = now.AddDays(-8), LastSentAt = now.AddDays(-8), SendCount = 1,
            });
            // Revoked.
            db.OrganizationInvitations.Add(new OrganizationInvitation
            {
                Id = Guid.NewGuid(), OrgId = orgId,
                Email = "revoked@example.com", EmailNormalized = "revoked@example.com",
                Role = OrgRole.Member, InvitedBy = userId,
                TokenHash = new string('c', 64),
                ExpiresAt = now.AddDays(7), CreatedAt = now, LastSentAt = now, SendCount = 1,
                RevokedAt = now.AddHours(-1), RevokedBy = userId,
            });
            await db.SaveChangesAsync();

            var count = await svc.CountSeatsAsync(orgId, now, default);

            // Only 1 owner member; expired and revoked invitations excluded.
            count.Should().Be(1);
        }
    }
}
