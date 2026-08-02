using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac.CustomRoles;
using TeamVaultEntity = ApiTool.Backend.Data.Entities.TeamVault;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.VaultConfig;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.VaultConfig;

/// <summary>Unit tests for <see cref="VaultConfigService"/> against in-memory SQLite.</summary>
public sealed class VaultConfigServiceTests
{
    // ── Helpers ─────────────────────────────────────────────────────────────

    private const string ValidYaml = """
        team_secrets:
          provider: aws-secrets-manager
          keys:
            api_key: prod/api-key
        """;

    private const string SuspiciousYaml = """
        team_secrets:
          password: abcdefghij1234567890
        """;

    private static async Task<(
        TestDbScope scope,
        VaultConfigService svc,
        Guid userId,
        Guid orgId)>
        BuildAsync(string validatorMode = "reject", OrgRole role = OrgRole.Admin)
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"user-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"testorg-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = role,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var svc = BuildService(db, validatorMode);
        return (scope, svc, userId, orgId);
    }

    private static VaultConfigService BuildService(AppDbContext db, string validatorMode = "reject")
    {
        var roles = new RoleResolver(db);
        var auditCtx = new AuditContext();
        var auditWriter = new AuditWriter(db, auditCtx, TimeProvider.System);
        var opts = Options.Create(new VaultConfigOptions { ValidatorMode = validatorMode });
        return new VaultConfigService(
            db,
            roles,
            auditWriter,
            TimeProvider.System,
            opts,
            new FakeTeamVaultKeyProvider(),
            NullLogger<VaultConfigService>.Instance);
    }

    // ── GetAsync ──────────────────────────────────────────────────────────────

    [Fact]
    public async Task Get_returns_NotFound_when_no_row()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, err, _) = await svc.GetAsync(userId, orgId, default);
            err.Should().Be(VaultConfigServiceError.NotFound);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Get_returns_dto_for_member_with_vault_config_view()
    {
        var (scope, svc, userId, orgId) = await BuildAsync(role: OrgRole.Member);
        await using (scope)
        {
            var db = scope.Db;
            var now = DateTime.UtcNow;
            db.TeamVaults.Add(new TeamVaultEntity
            {
                OrgId = orgId,
                TemplateYaml = ValidYaml,
                TemplateJson = "{}",
                Version = 1,
                CreatedAt = now,
                CreatedBy = userId,
                UpdatedAt = now,
                UpdatedBy = userId,
            });
            await db.SaveChangesAsync();

            var (dto, err, _) = await svc.GetAsync(userId, orgId, default);
            err.Should().Be(VaultConfigServiceError.None);
            dto.Should().NotBeNull();
            dto!.Version.Should().Be(1);
            dto.Template.Should().Be(ValidYaml);
        }
    }

    [Fact]
    public async Task Get_without_vault_config_view_returns_PermissionDenied()
    {
        // User exists but is NOT a member of the org — RoleResolver returns empty permission set
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var ownerId = Guid.NewGuid();
        var nonMemberId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = ownerId, Email = $"owner-{ownerId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Users.Add(new User { Id = nonMemberId, Email = $"non-{nonMemberId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"testorg-{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        // nonMemberId is NOT added to OrganizationMembers — no permission
        await db.SaveChangesAsync();
        await using (scope)
        {
            var svc = BuildService(db);
            var (dto, err, _) = await svc.GetAsync(nonMemberId, orgId, default);
            err.Should().Be(VaultConfigServiceError.PermissionDenied);
        }
    }

    // ── UpsertAsync ───────────────────────────────────────────────────────────

    [Fact]
    public async Task Upsert_inserts_new_row_with_version_1()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var db = scope.Db;
            var (dto, err, _, warnings) = await svc.UpsertAsync(userId, orgId, ValidYaml, default);

            err.Should().Be(VaultConfigServiceError.None);
            dto.Should().NotBeNull();
            dto!.Version.Should().Be(1);
            warnings.Should().BeEmpty();

            var row = await db.TeamVaults.FindAsync(orgId);
            row.Should().NotBeNull();
            row!.Version.Should().Be(1);
        }
    }

    [Fact]
    public async Task Upsert_existing_row_increments_version_and_updates_updated_by()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var db = scope.Db;
            await svc.UpsertAsync(userId, orgId, ValidYaml, default);

            // Second upsert — version should be 2
            var (dto, err, _, _) = await svc.UpsertAsync(userId, orgId, ValidYaml, default);
            err.Should().Be(VaultConfigServiceError.None);
            dto!.Version.Should().Be(2);

            var row = await db.TeamVaults.FindAsync(orgId);
            row!.Version.Should().Be(2);
            row.UpdatedBy.Should().Be(userId);
        }
    }

    [Fact]
    public async Task Upsert_reject_mode_returns_SuspiciousValue_for_literal_secret()
    {
        var (scope, svc, userId, orgId) = await BuildAsync(validatorMode: "reject");
        await using (scope)
        {
            var (dto, err, _, _) = await svc.UpsertAsync(userId, orgId, SuspiciousYaml, default);
            err.Should().Be(VaultConfigServiceError.SuspiciousValue);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Upsert_warn_mode_succeeds_and_returns_warnings_for_literal_secret()
    {
        var (scope, svc, userId, orgId) = await BuildAsync(validatorMode: "warn");
        await using (scope)
        {
            var (dto, err, _, warnings) = await svc.UpsertAsync(userId, orgId, SuspiciousYaml, default);
            err.Should().Be(VaultConfigServiceError.None);
            dto.Should().NotBeNull();
            warnings.Should().NotBeEmpty();
        }
    }

    [Fact]
    public async Task Upsert_rejects_invalid_yaml_with_InvalidYaml_error()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, err, _, _) = await svc.UpsertAsync(userId, orgId, ": invalid: yaml: {{", default);
            err.Should().Be(VaultConfigServiceError.InvalidYaml);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Upsert_empty_yaml_returns_InvalidYaml()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, err, _, _) = await svc.UpsertAsync(userId, orgId, string.Empty, default);
            err.Should().Be(VaultConfigServiceError.InvalidYaml);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Upsert_without_vault_config_manage_returns_PermissionDenied()
    {
        // Member role has vault_config.view but NOT vault_config.manage
        var (scope, svc, userId, orgId) = await BuildAsync(role: OrgRole.Member);
        await using (scope)
        {
            var (dto, err, _, _) = await svc.UpsertAsync(userId, orgId, ValidYaml, default);
            err.Should().Be(VaultConfigServiceError.PermissionDenied);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Upsert_emits_audit_event_vault_config_upserted()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var db = scope.Db;
            await svc.UpsertAsync(userId, orgId, ValidYaml, default);

            var auditEntry = await db.OrganizationAuditLog
                .FirstOrDefaultAsync(e => e.EventType == "vault_config.upserted");
            auditEntry.Should().NotBeNull();
            auditEntry!.ActorId.Should().Be(userId);
            auditEntry.OrgId.Should().Be(orgId);
        }
    }

    // ── DeleteAsync ───────────────────────────────────────────────────────────

    [Fact]
    public async Task Delete_emits_vault_config_deleted_audit_event()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var db = scope.Db;
            await svc.UpsertAsync(userId, orgId, ValidYaml, default);
            await svc.DeleteAsync(userId, orgId, default);

            var auditEntry = await db.OrganizationAuditLog
                .FirstOrDefaultAsync(e => e.EventType == "vault_config.deleted");
            auditEntry.Should().NotBeNull();
        }
    }

    [Fact]
    public async Task Delete_returns_NotFound_when_no_row()
    {
        var (scope, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var err = await svc.DeleteAsync(userId, orgId, default);
            err.Should().Be(VaultConfigServiceError.NotFound);
        }
    }

    [Fact]
    public async Task Delete_without_vault_config_manage_returns_PermissionDenied()
    {
        var (scope, svc, userId, orgId) = await BuildAsync(role: OrgRole.Member);
        await using (scope)
        {
            var err = await svc.DeleteAsync(userId, orgId, default);
            err.Should().Be(VaultConfigServiceError.PermissionDenied);
        }
    }
}
