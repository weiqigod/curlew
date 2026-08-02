// Tests for the AddPrCheckProviderDiscriminator migration.
// Refs M16-014 plan step 4. Named so the filter FullyQualifiedName~PrCheckProvider matches.
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// Migration smoke tests for the M16-014 pr_checks provider discriminator migration.
/// Uses a real SQLite in-memory database so constraint enforcement is real.
/// Named so the filter FullyQualifiedName~PrCheckProvider matches.
/// </summary>
public sealed class PrCheckProviderMigrationTests
{
    [Fact]
    public async Task ExistingRow_WithoutProvider_DefaultsToGithub()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var orgId = await SeedOrg(db);

        // Insert without setting Provider; rely on default
        db.PrChecks.Add(new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "a/b", Pr = 1,
            State = "success", HeadSha = new string('a', 40), ExternalId = Guid.NewGuid(),
            CreatedAt = DateTime.UtcNow
        });
        await db.SaveChangesAsync();
        db.ChangeTracker.Clear();
        var loaded = await db.PrChecks.SingleAsync();
        loaded.Provider.Should().Be("github");
    }

    [Fact]
    public async Task GitLab_Provider_RoundTrips()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var (orgId, glId) = await SeedOrgAndGitLabInstallation(db);
        var rowId = Guid.NewGuid();

        db.PrChecks.Add(new PrCheck
        {
            Id = rowId, OrgId = orgId, Repo = "g/p", Pr = 1,
            Provider = "gitlab", State = "success", HeadSha = new string('b', 40),
            ExternalId = Guid.NewGuid(), CreatedAt = DateTime.UtcNow,
            GitLabInstallationId = glId, GitLabStatusId = 99999L
        });
        await db.SaveChangesAsync();
        db.ChangeTracker.Clear();

        var loaded = await db.PrChecks.FindAsync(rowId);
        loaded!.Provider.Should().Be("gitlab");
        loaded.GitLabInstallationId.Should().Be(glId);
        loaded.GitLabStatusId.Should().Be(99999L);
    }

    [Fact]
    public async Task GitLabFK_OnDeleteSetNull_ClearsFkOnInstallationHardDelete()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var (orgId, glId) = await SeedOrgAndGitLabInstallation(db);
        var rowId = Guid.NewGuid();
        db.PrChecks.Add(new PrCheck
        {
            Id = rowId, OrgId = orgId, Repo = "g/p", Pr = 1,
            Provider = "gitlab", State = "success", HeadSha = new string('b', 40),
            ExternalId = Guid.NewGuid(), CreatedAt = DateTime.UtcNow,
            GitLabInstallationId = glId
        });
        await db.SaveChangesAsync();

        // Hard-delete the installation
        var inst = await db.GitLabInstallations.FindAsync(glId);
        db.GitLabInstallations.Remove(inst!);
        await db.SaveChangesAsync();
        db.ChangeTracker.Clear();

        var loaded = await db.PrChecks.FindAsync(rowId);
        loaded!.GitLabInstallationId.Should().BeNull();
    }

    // ── Seed helpers ──────────────────────────────────────────────────────────

    private static async Task<Guid> SeedOrg(AppDbContext db)
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"m{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "MigrOrg", Slug = $"mi-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return orgId;
    }

    private static async Task<(Guid orgId, Guid glId)> SeedOrgAndGitLabInstallation(AppDbContext db)
    {
        var orgId = await SeedOrg(db);
        var glId = Guid.NewGuid();
        db.GitLabInstallations.Add(new GitLabInstallation
        {
            Id = glId, OrgId = orgId, ProjectId = 1L, ProjectPath = "g/p",
            GitLabBaseUrl = "https://gitlab.com",
            AccessTokenCiphertext = [0x01],
            AccessTokenKid = "k1",
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (orgId, glId);
    }
}
