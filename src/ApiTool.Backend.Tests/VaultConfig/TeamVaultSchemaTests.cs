using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.VaultConfig;

/// <summary>Verifies the team_vaults schema round-trips via SQLite migrations.</summary>
public sealed class TeamVaultSchemaTests
{
    private static async Task<(TestDbScope scope, Guid userId, Guid orgId)> SeedBaseAsync()
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
        await db.SaveChangesAsync();
        return (scope, userId, orgId);
    }

    [Fact]
    public async Task TeamVault_round_trips_through_sqlite()
    {
        var (scope, userId, orgId) = await SeedBaseAsync();
        await using (scope)
        {
            var db = scope.Db;
            var now = DateTime.UtcNow;
            db.TeamVaults.Add(new TeamVault
            {
                OrgId = orgId,
                TemplateYaml = "team_secrets:\n  provider: aws-secrets-manager\n",
                TemplateJson = """{"team_secrets":{"provider":"aws-secrets-manager"}}""",
                Version = 1,
                CreatedAt = now,
                CreatedBy = userId,
                UpdatedAt = now,
                UpdatedBy = userId,
            });
            await db.SaveChangesAsync();

            db.ChangeTracker.Clear();
            var fetched = await db.TeamVaults.FindAsync(orgId);
            fetched.Should().NotBeNull();
            fetched!.TemplateYaml.Should().Contain("team_secrets");
            fetched.Version.Should().Be(1);
            fetched.CreatedBy.Should().Be(userId);
            fetched.UpdatedBy.Should().Be(userId);
        }
    }

    [Fact]
    public async Task TeamVault_org_id_is_primary_key_unique_per_org()
    {
        var (scope, userId, orgId) = await SeedBaseAsync();
        await using (scope)
        {
            var db = scope.Db;
            var now = DateTime.UtcNow;
            db.TeamVaults.Add(new TeamVault
            {
                OrgId = orgId,
                TemplateYaml = "team_secrets: {}",
                TemplateJson = "{}",
                Version = 1,
                CreatedAt = now,
                CreatedBy = userId,
                UpdatedAt = now,
                UpdatedBy = userId,
            });
            await db.SaveChangesAsync();

            db.ChangeTracker.Clear();
            db.TeamVaults.Add(new TeamVault
            {
                OrgId = orgId,  // Same org_id — should fail PK constraint
                TemplateYaml = "team_secrets: {}",
                TemplateJson = "{}",
                Version = 2,
                CreatedAt = now,
                CreatedBy = userId,
                UpdatedAt = now,
                UpdatedBy = userId,
            });
            var act = async () => await db.SaveChangesAsync();
            await act.Should().ThrowAsync<DbUpdateException>();
        }
    }
}
