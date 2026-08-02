using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Concrete implementation of <see cref="IUserAnonymiser"/> that fulfils a GDPR deletion
/// request by anonymising all PII columns attributed to the deleted user. Replaces the
/// M18-005 <c>StubUserAnonymiser</c>. Per spec v4-6.
///
/// <para>
/// <b>Transaction contract:</b> all writes are wrapped in a single EF transaction.
/// The <c>await using var tx</c> pattern ensures automatic rollback via DisposeAsync on
/// exception — no explicit catch/RollbackAsync is needed.
/// The operation is idempotent — a second call for a user whose <c>AnonymisedAt</c> is
/// already set is a no-op. Callers must snapshot the user's email BEFORE invoking
/// <see cref="AnonymiseAsync"/> if they need it for a post-deletion notification.
/// </para>
/// </summary>
public sealed class UserAnonymiser(
    AppDbContext db,
    TimeProvider clock,
    ILogger<UserAnonymiser> logger) : IUserAnonymiser
{
    /// <inheritdoc/>
    public async Task AnonymiseAsync(Guid userId, CancellationToken ct)
    {
        var user = await db.Users.FindAsync([userId], ct);
        if (user is null)
        {
            logger.LogDebug("UserAnonymiser: user {UserId} not found — skipping", userId);
            return;
        }

        // Idempotency guard — second call after a completed anonymisation is a no-op.
        if (user.AnonymisedAt is not null)
        {
            logger.LogDebug("UserAnonymiser: user {UserId} already anonymised at {At} — skipping",
                userId, user.AnonymisedAt);
            return;
        }

        // `await using` ensures rollback via DisposeAsync if an exception propagates —
        // no explicit catch/RollbackAsync is needed or desirable.
        await using var tx = await db.Database.BeginTransactionAsync(ct);

        // Step 1: Discover all orgs the user is attributable to.
        // Union across ALL attribution sources so that orgs where the user only appears
        // via CreatedBy/UpdatedBy columns (no audit log entries) are also covered.
        // This ensures user.anonymised audit-of-audit rows are emitted for every affected org.
        var auditOrgIds = db.OrganizationAuditLog
            .Where(e => e.ActorId == (Guid?)userId && e.OrgId.HasValue)
            .Select(e => e.OrgId!.Value);

        var scheduleOrgIds = db.Schedules
            .Where(s => s.CreatedBy == (Guid?)userId)
            .Select(s => s.OrgId);

        var customRoleOrgIds = db.OrganizationCustomRoles
            .Where(r => r.CreatedBy == (Guid?)userId)
            .Select(r => r.OrgId);

        var coordinatorJobOrgIds = db.CoordinatorJobs
            .Where(j => j.CreatedBy == (Guid?)userId)
            .Select(j => j.OrgId);

        var teamVaultCreatedOrgIds = db.TeamVaults
            .Where(v => v.CreatedBy == (Guid?)userId)
            .Select(v => v.OrgId);

        var teamVaultUpdatedOrgIds = db.TeamVaults
            .Where(v => v.UpdatedBy == (Guid?)userId)
            .Select(v => v.OrgId);

        var notificationRuleOrgIds = db.NotificationRules
            .Where(r => r.CreatedBy == (Guid?)userId)
            .Select(r => r.OrgId);

        var invitedByOrgIds = db.OrganizationMembers
            .Where(m => m.InvitedBy == (Guid?)userId)
            .Select(m => m.OrgId);

        var affectedOrgIds = await auditOrgIds
            .Union(scheduleOrgIds)
            .Union(customRoleOrgIds)
            .Union(coordinatorJobOrgIds)
            .Union(teamVaultCreatedOrgIds)
            .Union(teamVaultUpdatedOrgIds)
            .Union(notificationRuleOrgIds)
            .Union(invitedByOrgIds)
            .Distinct()
            .ToListAsync(ct);

        // Step 2: Anonymise OrganizationAuditLogEntry rows per affected org.
        foreach (var orgId in affectedOrgIds)
        {
            var token = AnonymisationToken.Compute(userId, orgId);
            await db.OrganizationAuditLog
                .Where(e => e.OrgId == orgId && e.ActorId == (Guid?)userId)
                .ExecuteUpdateAsync(s => s
                    .SetProperty(e => e.ActorId, _ => (Guid?)null)
                    .SetProperty(e => e.ActorEmail, _ => token),
                    ct);
        }

        // Account-level audit events have no organization, but still carry user
        // attribution and must be anonymised during the same transaction.
        var accountAuditToken = AnonymisationToken.Compute(userId, Guid.Empty);
        await db.OrganizationAuditLog
            .Where(e => e.OrgId == null && e.ActorId == (Guid?)userId)
            .ExecuteUpdateAsync(s => s
                .SetProperty(e => e.ActorId, _ => (Guid?)null)
                .SetProperty(e => e.ActorEmail, _ => accountAuditToken),
                ct);

        // Step 3: Anonymise SetNull columns on CreatedBy/UpdatedBy tables.
        await db.OrganizationMembers
            .Where(m => m.InvitedBy == (Guid?)userId)
            .ExecuteUpdateAsync(s => s.SetProperty(m => m.InvitedBy, _ => (Guid?)null), ct);

        await db.NotificationRules
            .Where(r => r.CreatedBy == (Guid?)userId)
            .ExecuteUpdateAsync(s => s.SetProperty(r => r.CreatedBy, _ => (Guid?)null), ct);

        await db.OrganizationCustomRoles
            .Where(r => r.CreatedBy == (Guid?)userId)
            .ExecuteUpdateAsync(s => s.SetProperty(r => r.CreatedBy, _ => (Guid?)null), ct);

        await db.Schedules
            .Where(s => s.CreatedBy == (Guid?)userId)
            .ExecuteUpdateAsync(s => s.SetProperty(x => x.CreatedBy, _ => (Guid?)null), ct);

        await db.CoordinatorJobs
            .Where(j => j.CreatedBy == (Guid?)userId)
            .ExecuteUpdateAsync(s => s.SetProperty(j => j.CreatedBy, _ => (Guid?)null), ct);

        await db.TeamVaults
            .Where(v => v.CreatedBy == (Guid?)userId)
            .ExecuteUpdateAsync(s => s.SetProperty(v => v.CreatedBy, _ => (Guid?)null), ct);

        await db.TeamVaults
            .Where(v => v.UpdatedBy == (Guid?)userId)
            .ExecuteUpdateAsync(s => s.SetProperty(v => v.UpdatedBy, _ => (Guid?)null), ct);

        // Step 4: Hard-delete InDeletionHard rows attributed to the user.
        // Do NOT delete the User row itself — it stays as a tombstone with AnonymisedAt set.
        await db.RefreshTokens
            .Where(r => r.UserId == userId)
            .ExecuteDeleteAsync(ct);

        await db.EmailVerificationTokens
            .Where(t => t.UserId == userId)
            .ExecuteDeleteAsync(ct);

        await db.PasswordResetTokens
            .Where(t => t.UserId == userId)
            .ExecuteDeleteAsync(ct);

        await db.DeletionReauthTokens
            .Where(t => t.UserId == userId)
            .ExecuteDeleteAsync(ct);

        await db.OrganizationMembers
            .Where(m => m.UserId == userId)
            .ExecuteDeleteAsync(ct);

        // Step 5: Scrub PII on the User row itself.
        var emailToken = AnonymisationToken.Compute(userId, Guid.Empty);
        user.Email = emailToken;
        user.PasswordHash = null;
        user.IsAdmin = false;
        user.EmailVerified = false;
        user.PendingDeletionAt = null;
        user.AnonymisedAt = clock.GetUtcNow().UtcDateTime;

        // Step 6: Emit one user.anonymised audit row per affected org.
        // These rows are written directly (not via IAuditWriter) so that ActorId can be null
        // (the audit row must itself be anonymised — no PII in the actor attribution).
        var now = clock.GetUtcNow().UtcDateTime;
        foreach (var orgId in affectedOrgIds)
        {
            var token = AnonymisationToken.Compute(userId, orgId);
            var payloadJson = System.Text.Json.JsonSerializer.Serialize(
                new { anonymisation_token = token },
                new System.Text.Json.JsonSerializerOptions
                {
                    PropertyNamingPolicy = System.Text.Json.JsonNamingPolicy.SnakeCaseLower,
                });
            db.OrganizationAuditLog.Add(new Data.Entities.OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                ActorId = null,        // system event — no attributed user
                ActorEmail = token,    // per-org token so the row is itself anonymised
                EventType = "user.anonymised",
                TargetType = "user",
                TargetId = null,
                PayloadJson = payloadJson,
                Success = true,
                CreatedAt = now,
            });
        }

        await db.SaveChangesAsync(ct);
        await tx.CommitAsync(ct);

        logger.LogInformation(
            "UserAnonymiser: anonymised user {UserId} across {OrgCount} org(s)",
            userId, affectedOrgIds.Count);
    }
}
