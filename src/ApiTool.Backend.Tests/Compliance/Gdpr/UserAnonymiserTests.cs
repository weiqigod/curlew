using System.Text.Json;
using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// Unit tests for <see cref="UserAnonymiser"/>.
/// Uses SQLite in-memory for realistic EF Core query behaviour — the same pattern
/// used in <see cref="UserDeletionFinalizerHostTests"/>.
/// </summary>
public sealed class UserAnonymiserTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly FakeClock _clock;

    private static readonly DateTimeOffset Now = new DateTimeOffset(2026, 5, 18, 3, 0, 0, TimeSpan.Zero);

    public UserAnonymiserTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        _clock = new FakeClock(Now);

        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    public async ValueTask DisposeAsync()
    {
        await _conn.DisposeAsync();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    private UserAnonymiser BuildAnonymiser(AppDbContext db)
        => new UserAnonymiser(db, _clock, NullLogger<UserAnonymiser>.Instance);

    private async Task<(Guid userId, Guid orgId)> SeedUserAndOrgAsync()
    {
        using var scope = OpenScope();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        scope.Db.Users.Add(new User
        {
            Id = userId,
            Email = $"user-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = Now.UtcDateTime.AddDays(-31),
        });
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"org-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();
        return (userId, orgId);
    }

    private async Task SeedAuditLogEntryAsync(Guid orgId, Guid actorId, string eventType = "test.event")
    {
        using var scope = OpenScope();
        scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            ActorId = actorId,
            ActorEmail = $"{actorId:N}@example.com",
            EventType = eventType,
            CreatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();
    }

    // ── Audit log anonymisation ─────────────────────────────────────────────

    [Fact]
    public async Task Anonymise_sets_audit_log_actor_id_to_null_for_users_rows()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        await SeedAuditLogEntryAsync(orgId, userId);

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var entries = await scope.Db.OrganizationAuditLog
            .Where(e => e.OrgId == orgId && e.EventType == "test.event")
            .ToListAsync();
        entries.Should().ContainSingle();
        entries[0].ActorId.Should().BeNull(because: "ActorId must be set to NULL after anonymisation");
    }

    [Fact]
    public async Task Anonymise_sets_audit_log_actor_email_to_deleted_user_token()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        await SeedAuditLogEntryAsync(orgId, userId);

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var entry = await scope.Db.OrganizationAuditLog
            .Where(e => e.OrgId == orgId && e.EventType == "test.event")
            .FirstAsync();
        var expectedToken = AnonymisationToken.Compute(userId, orgId);
        entry.ActorEmail.Should().Be(expectedToken,
            because: "ActorEmail must be replaced with the per-(user,org) token");
    }

    [Fact]
    public async Task Anonymise_uses_distinct_per_org_tokens_for_cross_org_user()
    {
        // Seed user and two orgs
        using var setupScope = OpenScope();
        var userId = Guid.NewGuid();
        var orgA = Guid.NewGuid();
        var orgB = Guid.NewGuid();

        setupScope.Db.Users.Add(new User
        {
            Id = userId,
            Email = $"cross-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = Now.UtcDateTime.AddDays(-31),
        });
        var ownerId = Guid.NewGuid();
        setupScope.Db.Users.Add(new User { Id = ownerId, Email = "owner@x.com", CreatedAt = DateTime.UtcNow });
        setupScope.Db.Organizations.Add(new Organization
        {
            Id = orgA, Name = "OrgA", Slug = $"orga{orgA:N}"[..20],
            OwnerId = ownerId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        setupScope.Db.Organizations.Add(new Organization
        {
            Id = orgB, Name = "OrgB", Slug = $"orgb{orgB:N}"[..20],
            OwnerId = ownerId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        // Add membership so user is attributed in both orgs
        setupScope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgA, UserId = userId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow,
        });
        setupScope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgB, UserId = userId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow,
        });
        // Audit entries in both orgs
        setupScope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(), OrgId = orgA, ActorId = userId, ActorEmail = "u@x.com",
            EventType = "test.event", CreatedAt = DateTime.UtcNow,
        });
        setupScope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(), OrgId = orgB, ActorId = userId, ActorEmail = "u@x.com",
            EventType = "test.event", CreatedAt = DateTime.UtcNow,
        });
        await setupScope.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var entryA = await scope.Db.OrganizationAuditLog
            .Where(e => e.OrgId == orgA && e.EventType == "test.event")
            .FirstAsync();
        var entryB = await scope.Db.OrganizationAuditLog
            .Where(e => e.OrgId == orgB && e.EventType == "test.event")
            .FirstAsync();
        entryA.ActorEmail.Should().NotBe(entryB.ActorEmail, because: "per-org tokens must differ");
        entryA.ActorEmail.Should().Be(AnonymisationToken.Compute(userId, orgA));
        entryB.ActorEmail.Should().Be(AnonymisationToken.Compute(userId, orgB));
    }

    [Fact]
    public async Task Anonymise_leaves_other_users_audit_rows_untouched()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();

        // Seed another user's audit entry
        using var otherScope = OpenScope();
        var otherUserId = Guid.NewGuid();
        otherScope.Db.Users.Add(new User { Id = otherUserId, Email = "other@x.com", CreatedAt = DateTime.UtcNow });
        await otherScope.Db.SaveChangesAsync();
        await SeedAuditLogEntryAsync(orgId, otherUserId, "other.event");
        await SeedAuditLogEntryAsync(orgId, userId, "my.event");

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var otherEntry = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "other.event")
            .FirstAsync();
        otherEntry.ActorId.Should().Be(otherUserId, because: "other users rows must not be touched");
    }

    // ── Hard-delete tables ──────────────────────────────────────────────────

    [Fact]
    public async Task Anonymise_hard_deletes_refresh_tokens()
    {
        var (userId, _) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.RefreshTokens.Add(new RefreshToken
        {
            Id = Guid.NewGuid(), UserId = userId, TokenHash = "hash1"u8.ToArray(),
            DeviceId = Guid.NewGuid(), FamilyId = Guid.NewGuid(),
            IssuedAt = DateTime.UtcNow, ExpiresAt = DateTime.UtcNow.AddHours(1),
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var count = await scope.Db.RefreshTokens.Where(r => r.UserId == userId).CountAsync();
        count.Should().Be(0, because: "refresh_tokens must be hard-deleted for the user");
    }

    [Fact]
    public async Task Anonymise_hard_deletes_email_verification_tokens()
    {
        var (userId, _) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id = Guid.NewGuid(), UserId = userId, TokenHash = "evhash1"u8.ToArray(),
            IssuedAt = DateTime.UtcNow, ExpiresAt = DateTime.UtcNow.AddHours(24),
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var count = await scope.Db.EmailVerificationTokens.Where(t => t.UserId == userId).CountAsync();
        count.Should().Be(0);
    }

    [Fact]
    public async Task Anonymise_hard_deletes_password_reset_tokens()
    {
        var (userId, _) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.PasswordResetTokens.Add(new PasswordResetToken
        {
            Id = Guid.NewGuid(), UserId = userId, TokenHash = "prhash1"u8.ToArray(),
            IssuedAt = DateTime.UtcNow, ExpiresAt = DateTime.UtcNow.AddMinutes(30),
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var count = await scope.Db.PasswordResetTokens.Where(t => t.UserId == userId).CountAsync();
        count.Should().Be(0);
    }

    [Fact]
    public async Task Anonymise_hard_deletes_deletion_reauth_tokens()
    {
        var (userId, _) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.DeletionReauthTokens.Add(new DeletionReauthToken
        {
            Id = Guid.NewGuid(), UserId = userId, TokenHash = "drhash1"u8.ToArray(),
            IssuedAt = DateTime.UtcNow, ExpiresAt = DateTime.UtcNow.AddMinutes(5),
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var count = await scope.Db.DeletionReauthTokens.Where(t => t.UserId == userId).CountAsync();
        count.Should().Be(0);
    }

    [Fact]
    public async Task Anonymise_hard_deletes_organization_member_rows_for_user()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        // userId is already a member via OwnerId; add explicit membership row
        using var setup = OpenScope();
        setup.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var count = await scope.Db.OrganizationMembers.Where(m => m.UserId == userId).CountAsync();
        count.Should().Be(0, because: "organization_members for the user must be hard-deleted");
    }

    // ── Anonymisable columns ────────────────────────────────────────────────

    [Fact]
    public async Task Anonymise_sets_organization_member_invited_by_null_for_other_members()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        var otherUserId = Guid.NewGuid();
        setup.Db.Users.Add(new User { Id = otherUserId, Email = "invited@x.com", CreatedAt = DateTime.UtcNow });
        setup.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        setup.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = otherUserId, Role = OrgRole.Member,
            JoinedAt = DateTime.UtcNow, InvitedBy = userId, // userId invited otherUser
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var otherMember = await scope.Db.OrganizationMembers
            .Where(m => m.UserId == otherUserId)
            .FirstAsync();
        otherMember.InvitedBy.Should().BeNull(
            because: "InvitedBy must be NULLed when the inviter is anonymised");
    }

    [Fact]
    public async Task Anonymise_sets_notification_rule_created_by_null()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.NotificationRules.Add(new NotificationRule
        {
            Id = Guid.NewGuid(), OrgId = orgId, Channel = NotificationChannel.Email,
            Target = "ops@example.com", OnEvents = "run_failed",
            CreatedBy = userId, CreatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var rule = await scope.Db.NotificationRules.Where(r => r.OrgId == orgId).FirstAsync();
        rule.CreatedBy.Should().BeNull();
    }

    [Fact]
    public async Task Anonymise_sets_custom_role_created_by_null()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.OrganizationCustomRoles.Add(new CustomRole
        {
            Id = Guid.NewGuid(), OrgId = orgId, Name = "testrole",
            CreatedBy = userId, CreatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var role = await scope.Db.OrganizationCustomRoles.Where(r => r.OrgId == orgId).FirstAsync();
        role.CreatedBy.Should().BeNull();
    }

    [Fact]
    public async Task Anonymise_sets_schedule_created_by_null()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.Schedules.Add(new Schedule
        {
            Id = Guid.NewGuid(), OrgId = orgId, Name = "nightly",
            CronExpression = "0 2 * * *", CollectionRef = "smoke.yaml",
            CreatedBy = userId, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var sched = await scope.Db.Schedules.Where(s => s.OrgId == orgId).FirstAsync();
        sched.CreatedBy.Should().BeNull();
    }

    [Fact]
    public async Task Anonymise_sets_coordinator_job_created_by_null()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.CoordinatorJobs.Add(new CoordinatorJob
        {
            Id = Guid.NewGuid(), OrgId = orgId, CollectionSha = "abc123",
            ShardCount = 2, CreatedBy = userId,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var job = await scope.Db.CoordinatorJobs.Where(j => j.OrgId == orgId).FirstAsync();
        job.CreatedBy.Should().BeNull();
    }

    [Fact]
    public async Task Anonymise_sets_team_vault_created_by_and_updated_by_null()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.TeamVaults.Add(new TeamVault
        {
            OrgId = orgId, TemplateYaml = "vars: []", TemplateJson = "{}",
            CreatedBy = userId, UpdatedBy = userId,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var vault = await scope.Db.TeamVaults.Where(v => v.OrgId == orgId).FirstAsync();
        vault.CreatedBy.Should().BeNull();
        vault.UpdatedBy.Should().BeNull();
    }

    // ── User row scrub ──────────────────────────────────────────────────────

    [Fact]
    public async Task Anonymise_scrubs_user_email_to_deleted_user_token()
    {
        var (userId, _) = await SeedUserAndOrgAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var user = await scope.Db.Users.FindAsync(userId);
        var expectedToken = AnonymisationToken.Compute(userId, Guid.Empty);
        user!.Email.Should().Be(expectedToken);
    }

    [Fact]
    public async Task Anonymise_clears_user_password_hash()
    {
        var (userId, _) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        var user = await setup.Db.Users.FindAsync(userId);
        user!.PasswordHash = "$argon2id$v=19$m=65536,t=3,p=4$test";
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var updated = await scope.Db.Users.FindAsync(userId);
        updated!.PasswordHash.Should().BeNull();
    }

    [Fact]
    public async Task Anonymise_sets_user_anonymised_at()
    {
        var (userId, _) = await SeedUserAndOrgAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var user = await scope.Db.Users.FindAsync(userId);
        user!.AnonymisedAt.Should().Be(Now.UtcDateTime);
    }

    [Fact]
    public async Task Anonymise_clears_user_pending_deletion_at()
    {
        var (userId, _) = await SeedUserAndOrgAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var user = await scope.Db.Users.FindAsync(userId);
        user!.PendingDeletionAt.Should().BeNull();
    }

    [Fact]
    public async Task Anonymise_clears_user_email_verified_and_is_admin()
    {
        var (userId, _) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        var user = await setup.Db.Users.FindAsync(userId);
        user!.EmailVerified = true;
        user.IsAdmin = true;
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var updated = await scope.Db.Users.FindAsync(userId);
        updated!.EmailVerified.Should().BeFalse();
        updated.IsAdmin.Should().BeFalse();
    }

    // ── Audit-of-audit ──────────────────────────────────────────────────────

    [Fact]
    public async Task Anonymise_emits_user_anonymised_audit_row_per_affected_org()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        await SeedAuditLogEntryAsync(orgId, userId);

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var auditRows = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .ToListAsync();
        auditRows.Should().ContainSingle(because: "one user.anonymised row per affected org");
    }

    [Fact]
    public async Task User_anonymised_audit_row_carries_token_in_payload()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        await SeedAuditLogEntryAsync(orgId, userId);

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var auditRow = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .FirstAsync();
        var payload = JsonDocument.Parse(auditRow.PayloadJson);
        payload.RootElement.GetProperty("anonymisation_token").GetString()
            .Should().Be(AnonymisationToken.Compute(userId, orgId));
    }

    [Fact]
    public async Task User_anonymised_audit_row_is_itself_anonymised()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        await SeedAuditLogEntryAsync(orgId, userId);

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var auditRow = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .FirstAsync();
        // audit-of-audit row must not be attributable to the deleted user
        auditRow.ActorId.Should().BeNull(
            because: "user.anonymised audit row must have ActorId=null (system event)");
    }

    // ── Audit-of-audit coverage for CreatedBy-only orgs ────────────────────

    /// <summary>
    /// Regression test for Finding #1 from review M18-006.
    /// When the user has CreatedBy rows in an org but NO audit-log entries there,
    /// the anonymiser must still emit a user.anonymised audit row for that org.
    /// </summary>
    [Fact]
    public async Task Anonymise_emits_user_anonymised_audit_row_for_org_with_only_created_by_rows()
    {
        // Arrange: user seeded with an org they only appear in as schedule.CreatedBy
        using var setupScope = OpenScope();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        var ownerId = Guid.NewGuid();
        setupScope.Db.Users.Add(new User { Id = ownerId, Email = "owner-createdby@x.com", CreatedAt = DateTime.UtcNow });
        setupScope.Db.Users.Add(new User
        {
            Id = userId,
            Email = $"createdby-only-{userId:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = Now.UtcDateTime.AddDays(-31),
        });
        setupScope.Db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "CreatedByOrg", Slug = $"cbog{orgId:N}"[..20],
            OwnerId = ownerId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        // No audit log entries for userId in this org
        setupScope.Db.Schedules.Add(new Schedule
        {
            Id = Guid.NewGuid(), OrgId = orgId, Name = "nightly-cb",
            CronExpression = "0 3 * * *", CollectionRef = "smoke.yaml",
            CreatedBy = userId, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await setupScope.Db.SaveChangesAsync();

        // Act
        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        // Assert: user.anonymised audit row must exist for the org the user appeared in via CreatedBy
        scope.Db.ChangeTracker.Clear();
        var auditRows = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .ToListAsync();
        auditRows.Should().ContainSingle(
            because: "user.anonymised must be emitted for orgs the user appears in only via CreatedBy columns");
    }

    [Fact]
    public async Task Anonymise_emits_user_anonymised_for_notification_rule_created_by_org()
    {
        using var setupScope = OpenScope();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        var ownerId = Guid.NewGuid();
        setupScope.Db.Users.Add(new User { Id = ownerId, Email = $"owner-nr@x.com", CreatedAt = DateTime.UtcNow });
        setupScope.Db.Users.Add(new User
        {
            Id = userId, Email = $"nr-only-{userId:N}@example.com", CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = Now.UtcDateTime.AddDays(-31),
        });
        setupScope.Db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "NROrg", Slug = $"nrog{orgId:N}"[..20],
            OwnerId = ownerId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        setupScope.Db.NotificationRules.Add(new NotificationRule
        {
            Id = Guid.NewGuid(), OrgId = orgId, Channel = NotificationChannel.Email,
            Target = "ops@example.com", OnEvents = "run_failed",
            CreatedBy = userId, CreatedAt = DateTime.UtcNow,
        });
        await setupScope.Db.SaveChangesAsync();

        using var scope = OpenScope();
        await BuildAnonymiser(scope.Db).AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var rows = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .ToListAsync();
        rows.Should().ContainSingle(
            because: "user.anonymised must be emitted for orgs appearing only via NotificationRule.CreatedBy");
    }

    // ── Audit-of-audit row emitted for schedule/custom_role/coordinator/team_vault tests ──

    [Fact]
    public async Task Anonymise_sets_schedule_created_by_null_and_emits_audit_row()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.Schedules.Add(new Schedule
        {
            Id = Guid.NewGuid(), OrgId = orgId, Name = "nightly-audit",
            CronExpression = "0 2 * * *", CollectionRef = "smoke.yaml",
            CreatedBy = userId, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        await BuildAnonymiser(scope.Db).AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var sched = await scope.Db.Schedules.Where(s => s.OrgId == orgId).FirstAsync();
        sched.CreatedBy.Should().BeNull();
        var auditRows = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .ToListAsync();
        auditRows.Should().ContainSingle(because: "user.anonymised must be emitted for schedule org");
    }

    [Fact]
    public async Task Anonymise_sets_custom_role_created_by_null_and_emits_audit_row()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.OrganizationCustomRoles.Add(new CustomRole
        {
            Id = Guid.NewGuid(), OrgId = orgId, Name = "testrole-audit",
            CreatedBy = userId, CreatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        await BuildAnonymiser(scope.Db).AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var role = await scope.Db.OrganizationCustomRoles.Where(r => r.OrgId == orgId).FirstAsync();
        role.CreatedBy.Should().BeNull();
        var auditRows = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .ToListAsync();
        auditRows.Should().ContainSingle(because: "user.anonymised must be emitted for custom_role org");
    }

    [Fact]
    public async Task Anonymise_sets_coordinator_job_created_by_null_and_emits_audit_row()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.CoordinatorJobs.Add(new CoordinatorJob
        {
            Id = Guid.NewGuid(), OrgId = orgId, CollectionSha = "abc123-audit",
            ShardCount = 2, CreatedBy = userId,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        await BuildAnonymiser(scope.Db).AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var job = await scope.Db.CoordinatorJobs.Where(j => j.OrgId == orgId).FirstAsync();
        job.CreatedBy.Should().BeNull();
        var auditRows = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .ToListAsync();
        auditRows.Should().ContainSingle(because: "user.anonymised must be emitted for coordinator_job org");
    }

    [Fact]
    public async Task Anonymise_sets_team_vault_created_by_and_updated_by_null_and_emits_audit_row()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        using var setup = OpenScope();
        setup.Db.TeamVaults.Add(new TeamVault
        {
            OrgId = orgId, TemplateYaml = "vars: []", TemplateJson = "{}",
            CreatedBy = userId, UpdatedBy = userId,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await setup.Db.SaveChangesAsync();

        using var scope = OpenScope();
        await BuildAnonymiser(scope.Db).AnonymiseAsync(userId, default);

        scope.Db.ChangeTracker.Clear();
        var vault = await scope.Db.TeamVaults.Where(v => v.OrgId == orgId).FirstAsync();
        vault.CreatedBy.Should().BeNull();
        vault.UpdatedBy.Should().BeNull();
        var auditRows = await scope.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .ToListAsync();
        auditRows.Should().ContainSingle(because: "user.anonymised must be emitted for team_vault org");
    }

    // ── Idempotency / safety ────────────────────────────────────────────────

    [Fact]
    public async Task Anonymise_is_idempotent_second_call_no_op()
    {
        var (userId, orgId) = await SeedUserAndOrgAsync();
        await SeedAuditLogEntryAsync(orgId, userId);

        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        await anonymiser.AnonymiseAsync(userId, default);

        // Second call — should not throw and should not produce duplicate audit rows
        scope.Db.ChangeTracker.Clear();
        var scope2Db = TestDb.CreateOpenFromConnection(_conn);
        var anonymiser2 = BuildAnonymiser(scope2Db.Db);
        var act = async () => await anonymiser2.AnonymiseAsync(userId, default);
        await act.Should().NotThrowAsync();

        scope2Db.Db.ChangeTracker.Clear();
        var auditRows = await scope2Db.Db.OrganizationAuditLog
            .Where(e => e.EventType == "user.anonymised" && e.OrgId == orgId)
            .CountAsync();
        auditRows.Should().Be(1, because: "idempotent second call must not emit duplicate audit rows");
        await scope2Db.DisposeAsync();
    }

    [Fact]
    public async Task Anonymise_on_unknown_user_id_no_op()
    {
        using var scope = OpenScope();
        var anonymiser = BuildAnonymiser(scope.Db);
        var unknownId = Guid.NewGuid();

        var act = async () => await anonymiser.AnonymiseAsync(unknownId, default);
        await act.Should().NotThrowAsync(because: "unknown userId must be treated as a no-op");
    }
}
