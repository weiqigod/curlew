// Tests for PrCheck entity EF model additions: provider discriminator, GitLab FK columns.
// Refs M16-014 plan step 3.
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// EF model tests for the M16-014 pr_checks schema additions: <c>provider</c> discriminator,
/// <c>gitlab_installation_id</c> nullable FK, <c>gitlab_status_id</c> nullable bigint.
/// Named so the filter FullyQualifiedName~PrCheckEntitySchema matches.
/// </summary>
public sealed class PrCheckEntitySchemaTests
{
    [Fact]
    public void Provider_HasDefaultGithub_AndIsRequired()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        var entity = db.Model.FindEntityType(typeof(PrCheck))!;
        var prop = entity.FindProperty(nameof(PrCheck.Provider))!;
        prop.IsNullable.Should().BeFalse();
        prop.GetDefaultValue().Should().Be("github");
    }

    [Fact]
    public void Provider_HasCheckConstraint_GithubOrGitlab()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        // Check constraints require the design-time model (not the read-optimized runtime model).
        var designTimeModel = db.GetService<Microsoft.EntityFrameworkCore.Metadata.IDesignTimeModel>().Model;
        var entity = designTimeModel.FindEntityType(typeof(PrCheck))!;
        var ck = entity.GetCheckConstraints().FirstOrDefault(c => c.Name == "ck_pr_checks_provider");
        ck.Should().NotBeNull();
        ck!.Sql.Should().Contain("'github'").And.Contain("'gitlab'");
    }

    [Fact]
    public void GitLabInstallationId_IsNullable()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        var entity = db.Model.FindEntityType(typeof(PrCheck))!;
        var prop = entity.FindProperty(nameof(PrCheck.GitLabInstallationId))!;
        prop.IsNullable.Should().BeTrue();
    }

    [Fact]
    public void GitLabInstallationId_IsFK_OnDeleteSetNull()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        var entity = db.Model.FindEntityType(typeof(PrCheck))!;
        var fk = entity.GetForeignKeys()
            .FirstOrDefault(f => f.Properties.Any(p => p.Name == nameof(PrCheck.GitLabInstallationId)));
        fk.Should().NotBeNull();
        fk!.DeleteBehavior.Should().Be(DeleteBehavior.SetNull);
    }

    [Fact]
    public void GitLabStatusId_IsNullable()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        var entity = db.Model.FindEntityType(typeof(PrCheck))!;
        var prop = entity.FindProperty(nameof(PrCheck.GitLabStatusId))!;
        prop.IsNullable.Should().BeTrue();
    }

    [Fact]
    public async Task ExistingRow_WithoutProvider_DefaultsToGithub()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var orgId = await SeedOrg(db);

        db.PrChecks.Add(new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "a/b", Pr = 1,
            State = "success", HeadSha = new string('a', 40), ExternalId = Guid.NewGuid(),
            CreatedAt = DateTime.UtcNow
            // Provider NOT set — should default to "github"
        });
        await db.SaveChangesAsync();
        db.ChangeTracker.Clear();
        var loaded = await db.PrChecks.SingleAsync();
        loaded.Provider.Should().Be("github");
    }

    [Fact]
    public async Task GitLabFK_OnDeleteSetNull_ClearsFkOnInstallationHardDelete()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
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
        db.Users.Add(new User { Id = userId, Email = $"u{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "TestOrg", Slug = $"to-{orgId:N}"[..20],
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
