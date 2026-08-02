using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Data;

/// <summary>Verifies the extended <c>organization_audit_log</c> schema introduced in M5-004.</summary>
public sealed class OrganizationAuditLogSchemaTests
{
    [Fact]
    public async Task Audit_log_table_has_extended_columns()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var orgId = Guid.NewGuid();
        var ownerId = Guid.NewGuid();
        scope.Db.Users.Add(new User { Id = ownerId, Email = $"owner-{ownerId:N}@test.com", CreatedAt = DateTime.UtcNow });
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "AuditSchemaOrg",
            Slug = $"aud{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        // Insert a row with all the new M5-004 columns populated.
        scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            ActorId = ownerId,
            EventType = "member.invited",
            TargetType = "invitation",
            TargetId = Guid.NewGuid(),
            PreviousStateJson = "{}",
            NewStateJson = """{"role":"member"}""",
            IpAddress = "192.0.2.1",
            UserAgent = "curl/8",
            Success = true,
            FailureReason = null,
            CreatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();
    }

    [Fact]
    public async Task Audit_log_entry_defaults_success_to_true()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var orgId = Guid.NewGuid();
        var ownerId = Guid.NewGuid();
        scope.Db.Users.Add(new User { Id = ownerId, Email = $"def-{ownerId:N}@test.com", CreatedAt = DateTime.UtcNow });
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "DefaultSuccessOrg",
            Slug = $"def{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        var entry = new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            ActorId = ownerId,
            EventType = "org.created",
            CreatedAt = DateTime.UtcNow,
        };
        scope.Db.OrganizationAuditLog.Add(entry);
        await scope.Db.SaveChangesAsync();

        entry.Success.Should().BeTrue();
    }

    [Fact]
    public async Task Organizations_table_has_audit_log_retention_days_column_with_default_365()
    {
        // M18-002: verifies the EF migration added audit_log_retention_days NOT NULL DEFAULT 365
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var ownerId = Guid.NewGuid();
        scope.Db.Users.Add(new User { Id = ownerId, Email = $"ret-{ownerId:N}@test.com", CreatedAt = DateTime.UtcNow });

        var org = new Organization
        {
            Id = Guid.NewGuid(),
            Name = "RetentionOrg",
            Slug = $"ret{ownerId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };
        scope.Db.Organizations.Add(org);
        await scope.Db.SaveChangesAsync();

        var fetched = await scope.Db.Organizations.FindAsync(org.Id);
        fetched!.AuditLogRetentionDays.Should().Be(365);
    }

    [Fact]
    public async Task Audit_log_entry_allows_null_optional_columns()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var orgId = Guid.NewGuid();
        var ownerId = Guid.NewGuid();
        scope.Db.Users.Add(new User { Id = ownerId, Email = $"null-{ownerId:N}@test.com", CreatedAt = DateTime.UtcNow });
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "NullColumnsOrg",
            Slug = $"nul{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            ActorId = ownerId,
            EventType = "org.created",
            TargetType = null,
            TargetId = null,
            PreviousStateJson = null,
            NewStateJson = null,
            IpAddress = null,
            UserAgent = null,
            FailureReason = null,
            CreatedAt = DateTime.UtcNow,
        });

        // Should not throw
        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().NotThrowAsync();
    }

    [Fact]
    public async Task Audit_log_entry_allows_null_org_for_account_level_events()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        scope.Db.Users.Add(new User
        {
            Id = userId,
            Email = $"account-audit-{userId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            OrgId = null,
            ActorId = userId,
            EventType = "account.deletion_cancelled",
            TargetType = "user",
            TargetId = userId,
            CreatedAt = DateTime.UtcNow,
        });

        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().NotThrowAsync();
    }
}
