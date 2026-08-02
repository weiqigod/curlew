using ApiTool.Backend.Audit;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Rbac;

/// <summary>Unit tests for <see cref="CustomRolesService"/> business rules.</summary>
public sealed class CustomRolesServiceTests
{
    private sealed class SpyAuditWriter : IAuditWriter
    {
        public List<AuditEvent> Events { get; } = new();
        public void Append(AuditEvent evt) => Events.Add(evt);
    }

    private static async Task<(TestDbScope scope, CustomRolesService svc, SpyAuditWriter spy, Guid orgId, Guid ownerId)>
        BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var ownerId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = ownerId, Email = $"owner-{ownerId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"svc-{orgId:N}"[..20],
            OwnerId = ownerId,
            SettingsJson = "{}",
            CreatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = ownerId,
            Role = OrgRole.Owner,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var spy = new SpyAuditWriter();
        var svc = new CustomRolesService(db, TimeProvider.System, spy);
        return (scope, svc, spy, orgId, ownerId);
    }

    private static async Task<Guid> AddMemberAsync(ApiTool.Backend.Data.AppDbContext db, Guid orgId, OrgRole role)
    {
        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"member-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = role,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return userId;
    }

    // ── Create ───────────────────────────────────────────────────────────────

    [Fact]
    public async Task Create_with_valid_permissions_persists_and_returns_dto()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateRoleRequest("results-only", ["results.view", "results.upload"]);
            var (dto, err, _) = await svc.CreateAsync(ownerId, orgId, req, default);

            err.Should().Be(CustomRoleError.None);
            dto.Should().NotBeNull();
            dto!.Id.Should().StartWith("role_");
            dto.Name.Should().Be("results-only");
            dto.Permissions.Should().BeEquivalentTo(["results.view", "results.upload"]);
            dto.IsBuiltin.Should().BeFalse();
            dto.CreatedAt.Should().NotBeNull();
        }
    }

    [Fact]
    public async Task Create_rejects_unknown_permission_key_with_InvalidPermission()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateRoleRequest("bad-role", ["results.view", "nonexistent.perm"]);
            var (dto, err, message) = await svc.CreateAsync(ownerId, orgId, req, default);

            err.Should().Be(CustomRoleError.InvalidPermission);
            dto.Should().BeNull();
            message.Should().Contain("nonexistent.perm");
        }
    }

    [Fact]
    public async Task Create_rejects_empty_name_with_InvalidName()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateRoleRequest("  ", ["results.view"]);
            var (dto, err, _) = await svc.CreateAsync(ownerId, orgId, req, default);

            err.Should().Be(CustomRoleError.InvalidName);
            dto.Should().BeNull();
        }
    }

    [Theory]
    [InlineData("owner")]
    [InlineData("admin")]
    [InlineData("member")]
    [InlineData("Owner")]
    [InlineData("ADMIN")]
    public async Task Create_rejects_built_in_name_with_InvalidName(string builtInName)
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateRoleRequest(builtInName, ["results.view"]);
            var (dto, err, _) = await svc.CreateAsync(ownerId, orgId, req, default);

            err.Should().Be(CustomRoleError.InvalidName);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_rejects_duplicate_name_with_RoleNameTaken()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateRoleRequest("results-only", ["results.view"]);
            await svc.CreateAsync(ownerId, orgId, req, default);

            var (dto, err, _) = await svc.CreateAsync(ownerId, orgId, req, default);

            err.Should().Be(CustomRoleError.RoleNameTaken);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_by_admin_returns_PermissionDenied()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var adminId = await AddMemberAsync(scope.Db, orgId, OrgRole.Admin);
            var req = new CreateRoleRequest("my-role", ["results.view"]);
            var (dto, err, _) = await svc.CreateAsync(adminId, orgId, req, default);

            err.Should().Be(CustomRoleError.PermissionDenied);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_by_member_returns_PermissionDenied()
    {
        var (scope, svc, _, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var memberId = await AddMemberAsync(scope.Db, orgId, OrgRole.Member);
            var req = new CreateRoleRequest("my-role", ["results.view"]);
            var (dto, err, _) = await svc.CreateAsync(memberId, orgId, req, default);

            err.Should().Be(CustomRoleError.PermissionDenied);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_by_non_member_returns_OrganizationNotFound()
    {
        var (scope, svc, _, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var outsiderId = Guid.NewGuid();
            var req = new CreateRoleRequest("my-role", ["results.view"]);
            var (dto, err, _) = await svc.CreateAsync(outsiderId, orgId, req, default);

            err.Should().Be(CustomRoleError.OrganizationNotFound);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_emits_role_created_audit_event()
    {
        var (scope, svc, spy, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateRoleRequest("audit-role", ["results.view"]);
            await svc.CreateAsync(ownerId, orgId, req, default);

            spy.Events.Should().ContainSingle(e => e.EventType == "role.created");
        }
    }

    // ── List ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task List_contains_three_built_ins_plus_custom()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(ownerId, orgId, new CreateRoleRequest("custom-r", ["results.view"]), default);

            var (roles, err) = await svc.ListAsync(ownerId, orgId, default);

            err.Should().Be(CustomRoleError.None);
            roles.Count.Should().Be(4);
            roles.Where(r => r.IsBuiltin).Should().HaveCount(3);
            roles.Select(r => r.Name).Should().Contain("custom-r");
        }
    }

    [Fact]
    public async Task List_by_member_succeeds()
    {
        var (scope, svc, _, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var memberId = await AddMemberAsync(scope.Db, orgId, OrgRole.Member);
            var (roles, err) = await svc.ListAsync(memberId, orgId, default);

            err.Should().Be(CustomRoleError.None);
            roles.Should().HaveCount(3); // three built-ins
        }
    }

    // ── Delete ───────────────────────────────────────────────────────────────

    [Fact]
    public async Task Delete_blocks_when_members_reference_role_with_RoleInUse_count()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var (dto, _, _) = await svc.CreateAsync(ownerId, orgId, new CreateRoleRequest("block-me", ["results.view"]), default);
            RoleId.TryParse(dto!.Id, out var roleGuid);

            // Assign the custom role to a member.
            var memberId = await AddMemberAsync(scope.Db, orgId, OrgRole.Member);
            var member = await scope.Db.OrganizationMembers.FindAsync(orgId, memberId);
            member!.RoleId = roleGuid;
            await scope.Db.SaveChangesAsync();

            var (err, count) = await svc.DeleteAsync(ownerId, orgId, roleGuid, default);

            err.Should().Be(CustomRoleError.RoleInUse);
            count.Should().Be(1);
        }
    }

    [Fact]
    public async Task Delete_succeeds_when_no_members_reference_role()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var (dto, _, _) = await svc.CreateAsync(ownerId, orgId, new CreateRoleRequest("delete-me", ["results.view"]), default);
            RoleId.TryParse(dto!.Id, out var roleGuid);

            var (err, count) = await svc.DeleteAsync(ownerId, orgId, roleGuid, default);

            err.Should().Be(CustomRoleError.None);
            count.Should().Be(0);

            var stillExists = await scope.Db.OrganizationCustomRoles
                .AnyAsync(r => r.Id == roleGuid);
            stillExists.Should().BeFalse();
        }
    }

    [Fact]
    public async Task Delete_by_admin_returns_PermissionDenied()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var (dto, _, _) = await svc.CreateAsync(ownerId, orgId, new CreateRoleRequest("no-admin-del", ["results.view"]), default);
            RoleId.TryParse(dto!.Id, out var roleGuid);

            var adminId = await AddMemberAsync(scope.Db, orgId, OrgRole.Admin);
            var (err, _) = await svc.DeleteAsync(adminId, orgId, roleGuid, default);

            err.Should().Be(CustomRoleError.PermissionDenied);
        }
    }

    [Fact]
    public async Task Create_allows_empty_permissions_array()
    {
        var (scope, svc, _, orgId, ownerId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateRoleRequest("no-perms", []);
            var (dto, err, _) = await svc.CreateAsync(ownerId, orgId, req, default);

            err.Should().Be(CustomRoleError.None);
            dto.Should().NotBeNull();
            dto!.Permissions.Should().BeEmpty();
        }
    }
}
