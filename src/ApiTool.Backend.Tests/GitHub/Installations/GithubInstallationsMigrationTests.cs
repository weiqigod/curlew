// Refs docs/SPECIFICATION.md:8409-8427 (lifecycle), :10006-10025 (schema).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.GitHub.Installations;

/// <summary>
/// Migration smoke tests for the github_installations table + cross-tenant UNIQUE index.
/// Uses a real SQLite in-memory database via TestDb so constraint enforcement is real.
/// </summary>
public sealed class GithubInstallationsMigrationTests
{
    [Fact]
    public async Task Migration_creates_github_installations_table_with_expected_columns()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        // Can query the table without errors
        var count = await db.GithubInstallations.CountAsync();
        count.Should().Be(0);
    }

    [Fact]
    public async Task Migration_creates_partial_unique_index_idx_github_installations_org()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        // Two different orgs can each have one install — should work fine
        var (userId, orgId1) = await SeedOrgAsync(db, "org-idx-1@example.com", "org-idx-1");
        var (_, orgId2) = await SeedOrgAsync(db, "org-idx-2@example.com", "org-idx-2");

        db.GithubInstallations.Add(MakeRow(installationId: 1001, orgId: orgId1));
        db.GithubInstallations.Add(MakeRow(installationId: 1002, orgId: orgId2));
        var act = async () => await db.SaveChangesAsync();
        await act.Should().NotThrowAsync();
    }

    [Fact]
    public async Task Two_active_installations_for_same_org_violates_unique_index()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var (_, orgId) = await SeedOrgAsync(db, "org-dupe@example.com", "org-dupe");

        db.GithubInstallations.Add(MakeRow(installationId: 2001, orgId: orgId));
        await db.SaveChangesAsync();

        db.GithubInstallations.Add(MakeRow(installationId: 2002, orgId: orgId));
        var act = async () => await db.SaveChangesAsync();
        // SQLite raises a unique constraint violation which EF wraps in DbUpdateException
        await act.Should().ThrowAsync<DbUpdateException>();
    }

    [Fact]
    public async Task Two_installations_for_same_org_one_soft_deleted_is_allowed()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var (_, orgId) = await SeedOrgAsync(db, "org-softdel@example.com", "org-softdel");

        // First row: soft-deleted (deleted_at IS NOT NULL) — NOT covered by partial index
        var deleted = MakeRow(installationId: 3001, orgId: orgId);
        deleted.DeletedAt = DateTime.UtcNow.AddDays(-1);
        db.GithubInstallations.Add(deleted);
        await db.SaveChangesAsync();

        // Second row: active (deleted_at IS NULL) — covered by partial index
        db.GithubInstallations.Add(MakeRow(installationId: 3002, orgId: orgId));
        var act = async () => await db.SaveChangesAsync();
        await act.Should().NotThrowAsync();
    }

    [Fact]
    public async Task GithubInstallation_round_trips_through_db()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var (_, orgId) = await SeedOrgAsync(db, "org-rtrip@example.com", "org-rtrip");
        var repoJson = """[{"id":1,"owner":"acme","name":"api"}]""";

        var row = MakeRow(installationId: 4001, orgId: orgId);
        row.RepoSetJson = repoJson;
        db.GithubInstallations.Add(row);
        await db.SaveChangesAsync();

        // Re-fetch from DB
        db.ChangeTracker.Clear();
        var fetched = await db.GithubInstallations.FindAsync(4001L);
        fetched.Should().NotBeNull();
        fetched!.RepoSetJson.Should().Be(repoJson);
        fetched.OrgId.Should().Be(orgId);
        fetched.AccountLogin.Should().Be("acme-corp");
        fetched.RepoSelection.Should().Be("selected");
    }

    [Fact]
    public async Task GithubInstallation_allows_null_org_id_for_webhook_first_row()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        // org_id = NULL (webhook-first path before claim) — no FK needed
        var row = MakeRow(installationId: 5001, orgId: null);
        db.GithubInstallations.Add(row);
        var act = async () => await db.SaveChangesAsync();
        await act.Should().NotThrowAsync();

        db.ChangeTracker.Clear();
        var fetched = await db.GithubInstallations.FindAsync(5001L);
        fetched!.OrgId.Should().BeNull();
    }

    [Fact]
    public async Task Two_webhook_first_rows_with_null_org_both_allowed()
    {
        // Partial index only covers rows where org_id IS NOT NULL, so two NULL-org rows
        // for different installations must both be insertable.
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        db.GithubInstallations.Add(MakeRow(installationId: 6001, orgId: null));
        db.GithubInstallations.Add(MakeRow(installationId: 6002, orgId: null));
        var act = async () => await db.SaveChangesAsync();
        await act.Should().NotThrowAsync();
    }

    // ── Helpers ───────────────────────────────────────────────────────────────────

    private static async Task<(Guid userId, Guid orgId)> SeedOrgAsync(
        AppDbContext db, string email, string slug)
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = email, CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = slug, Slug = slug,
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (userId, orgId);
    }

    private static GithubInstallation MakeRow(long installationId, Guid? orgId) =>
        new()
        {
            InstallationId = installationId,
            AppId = 12345L,
            OrgId = orgId,
            AccountLogin = "acme-corp",
            AccountType = "Organization",
            RepoSelection = "selected",
            RepoSetJson = "[]",
            InstalledAt = DateTime.UtcNow,
            LastReconciledAt = DateTime.UtcNow,
        };
}
