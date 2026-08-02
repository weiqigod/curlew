using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Rbac;

/// <summary>Verifies RoleResolver effective permission computation.</summary>
public sealed class RoleResolverTests
{
    private static async Task<(TestDbScope scope, RoleResolver resolver, Guid orgId, Guid userId)>
        BuildAsync(OrgRole role = OrgRole.Member, Guid? customRoleId = null, string[]? directOverrides = null)
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"test-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"slug-{orgId:N}"[..20],
            OwnerId = userId,
            SettingsJson = "{}",
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var overridesJson = directOverrides is null ? "[]" : JsonSerializer.Serialize(directOverrides);
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = role,
            JoinedAt = DateTime.UtcNow,
            PermissionsJson = overridesJson,
            RoleId = customRoleId,
        });
        await db.SaveChangesAsync();

        var resolver = new RoleResolver(db);
        return (scope, resolver, orgId, userId);
    }

    private static async Task<Guid> AddCustomRoleAsync(
        ApiTool.Backend.Data.AppDbContext db,
        Guid orgId,
        Guid createdBy,
        string[] permissions)
    {
        var roleId = Guid.NewGuid();
        db.OrganizationCustomRoles.Add(new CustomRole
        {
            Id = roleId,
            OrgId = orgId,
            Name = $"custom-{roleId:N}"[..20],
            PermissionsJson = JsonSerializer.Serialize(permissions),
            CreatedAt = DateTime.UtcNow,
            CreatedBy = createdBy,
        });
        await db.SaveChangesAsync();
        return roleId;
    }

    [Fact]
    public async Task BuiltIn_owner_gets_owner_permission_set()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Owner);
        await using (scope)
        {
            var perms = await resolver.GetEffectivePermissionsAsync(userId, orgId, default);
            perms.Should().BeEquivalentTo(Permissions.BuiltInOwner);
        }
    }

    [Fact]
    public async Task BuiltIn_admin_gets_admin_permission_set()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Admin);
        await using (scope)
        {
            var perms = await resolver.GetEffectivePermissionsAsync(userId, orgId, default);
            perms.Should().BeEquivalentTo(Permissions.BuiltInAdmin);
        }
    }

    [Fact]
    public async Task BuiltIn_member_gets_member_permission_set()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Member);
        await using (scope)
        {
            var perms = await resolver.GetEffectivePermissionsAsync(userId, orgId, default);
            perms.Should().BeEquivalentTo(Permissions.BuiltInMember);
        }
    }

    [Fact]
    public async Task Custom_role_returns_exactly_its_permissions()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Member);
        await using (scope)
        {
            var db = scope.Db;
            var roleId = await AddCustomRoleAsync(db, orgId, userId, ["results.upload", "dashboard.view"]);

            // Assign the custom role to the member.
            var member = await db.OrganizationMembers.FindAsync(orgId, userId);
            member!.RoleId = roleId;
            await db.SaveChangesAsync();

            var perms = await resolver.GetEffectivePermissionsAsync(userId, orgId, default);
            perms.Should().BeEquivalentTo(new[] { "results.upload", "dashboard.view" });
        }
    }

    [Fact]
    public async Task Custom_role_plus_direct_override_returns_union()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Member, directOverrides: ["members.view"]);
        await using (scope)
        {
            var db = scope.Db;
            var roleId = await AddCustomRoleAsync(db, orgId, userId, ["results.upload"]);
            var member = await db.OrganizationMembers.FindAsync(orgId, userId);
            member!.RoleId = roleId;
            await db.SaveChangesAsync();

            var perms = await resolver.GetEffectivePermissionsAsync(userId, orgId, default);
            perms.Should().Contain("results.upload");
            perms.Should().Contain("members.view");
        }
    }

    [Fact]
    public async Task Custom_role_id_pointing_to_deleted_role_falls_back_to_builtin()
    {
        var deletedRoleId = Guid.NewGuid(); // never inserted
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Admin, customRoleId: deletedRoleId);
        await using (scope)
        {
            var perms = await resolver.GetEffectivePermissionsAsync(userId, orgId, default);
            // Falls back to built-in admin set.
            perms.Should().BeEquivalentTo(Permissions.BuiltInAdmin);
        }
    }

    [Fact]
    public async Task Non_member_returns_empty_set()
    {
        var (scope, resolver, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var outsiderId = Guid.NewGuid();
            var perms = await resolver.GetEffectivePermissionsAsync(outsiderId, orgId, default);
            perms.Should().BeEmpty();
        }
    }

    [Fact]
    public async Task HasPermission_returns_true_for_member_with_custom_role_containing_key()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Member);
        await using (scope)
        {
            var db = scope.Db;
            var roleId = await AddCustomRoleAsync(db, orgId, userId, ["results.upload"]);
            var member = await db.OrganizationMembers.FindAsync(orgId, userId);
            member!.RoleId = roleId;
            await db.SaveChangesAsync();

            var result = await resolver.HasPermissionAsync(userId, orgId, Permissions.ResultsUpload, default);
            result.Should().BeTrue();
        }
    }

    [Fact]
    public async Task HasPermission_returns_false_for_key_not_in_custom_role()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Member);
        await using (scope)
        {
            var db = scope.Db;
            var roleId = await AddCustomRoleAsync(db, orgId, userId, ["results.view"]);
            var member = await db.OrganizationMembers.FindAsync(orgId, userId);
            member!.RoleId = roleId;
            await db.SaveChangesAsync();

            var result = await resolver.HasPermissionAsync(userId, orgId, Permissions.MembersInvite, default);
            result.Should().BeFalse();
        }
    }

    [Fact]
    public async Task Custom_role_in_different_org_is_ignored_falls_back_to_builtin()
    {
        var (scope, resolver, orgId, userId) = await BuildAsync(OrgRole.Admin);
        await using (scope)
        {
            // Create a custom role in a different org and point the member's RoleId at it.
            var otherOrgId = Guid.NewGuid();
            var db = scope.Db;

            db.Organizations.Add(new Organization
            {
                Id = otherOrgId,
                Name = "OtherOrg",
                Slug = $"other-{otherOrgId:N}"[..20],
                OwnerId = userId,
                SettingsJson = "{}",
                CreatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var roleId = await AddCustomRoleAsync(db, otherOrgId, userId, ["results.upload"]);
            var member = await db.OrganizationMembers.FindAsync(orgId, userId);
            member!.RoleId = roleId;
            await db.SaveChangesAsync();

            var perms = await resolver.GetEffectivePermissionsAsync(userId, orgId, default);
            // Falls back to built-in admin (cross-org role is ignored).
            perms.Should().BeEquivalentTo(Permissions.BuiltInAdmin);
        }
    }
}
