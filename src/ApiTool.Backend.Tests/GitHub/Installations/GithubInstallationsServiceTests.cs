// Refs docs/SPECIFICATION.md:8409-8427 (github_installations lifecycle).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub.Installations;

/// <summary>
/// Unit tests for GithubInstallationsService using a real SQLite in-memory DB
/// so FK constraints and the partial unique index are enforced.
/// </summary>
public sealed class GithubInstallationsServiceTests
{
    private const long AppId = 12345L;

    // ── Helpers ───────────────────────────────────────────────────────────────────

    private static GithubInstallationsService CreateService(AppDbContext db)
    {
        var clock = new FakeClock(DateTimeOffset.UtcNow);
        return new GithubInstallationsService(
            db, clock, new FakeInstallationTokenCache(),
            NullLogger<GithubInstallationsService>.Instance);
    }

    private static async Task<(AppDbContext db, Guid orgId)> SeedOrgAsync(TestDbScope scope)
    {
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"u-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Acme", Slug = $"acme-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (db, orgId);
    }

    private static async Task<GithubInstallation> SeedInstallAsync(
        AppDbContext db, long installationId, Guid? orgId, DateTime? deletedAt = null)
    {
        var row = new GithubInstallation
        {
            InstallationId = installationId,
            AppId = AppId,
            OrgId = orgId,
            AccountLogin = "acme-corp",
            AccountType = "Organization",
            RepoSelection = "selected",
            RepoSetJson = "[]",
            InstalledAt = DateTime.UtcNow,
            DeletedAt = deletedAt,
            LastReconciledAt = DateTime.UtcNow,
        };
        db.GithubInstallations.Add(row);
        await db.SaveChangesAsync();
        return row;
    }

    // ── ClaimAsync ────────────────────────────────────────────────────────────────

    [Fact]
    public async Task ClaimAsync_inserts_new_row_when_no_existing_install()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        var svc = CreateService(db);

        var err = await svc.ClaimAsync(9001L, orgId, AppId, CancellationToken.None);

        err.Should().Be(InstallClaimError.None);
        var row = await db.GithubInstallations.FindAsync(9001L);
        row.Should().NotBeNull();
        row!.OrgId.Should().Be(orgId);
        row.ClaimedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task ClaimAsync_returns_AlreadyLinked_when_org_already_has_different_install()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 9010L, orgId);
        var svc = CreateService(db);

        // Different installation_id, same org → UNIQUE index violation
        var err = await svc.ClaimAsync(9011L, orgId, AppId, CancellationToken.None);

        err.Should().Be(InstallClaimError.AlreadyLinked);
    }

    [Fact]
    public async Task ClaimAsync_is_idempotent_when_called_with_same_org_and_install()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        var svc = CreateService(db);

        var err1 = await svc.ClaimAsync(9020L, orgId, AppId, CancellationToken.None);
        var err2 = await svc.ClaimAsync(9020L, orgId, AppId, CancellationToken.None);

        err1.Should().Be(InstallClaimError.None);
        err2.Should().Be(InstallClaimError.None);
    }

    [Fact]
    public async Task ClaimAsync_reclaims_a_webhook_first_row_with_null_org_id()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        // Webhook-first row: org_id = NULL
        await SeedInstallAsync(db, 9030L, orgId: null);
        var svc = CreateService(db);

        var err = await svc.ClaimAsync(9030L, orgId, AppId, CancellationToken.None);

        err.Should().Be(InstallClaimError.None);
        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(9030L);
        row!.OrgId.Should().Be(orgId);
        row.ClaimedAt.Should().NotBeNull();
    }

    // ── UpsertFromWebhookAsync ─────────────────────────────────────────────────

    [Fact]
    public async Task UpsertFromWebhookAsync_inserts_row_with_null_org_id_when_no_pending_claim()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, _) = await SeedOrgAsync(scope);
        var svc = CreateService(db);

        await svc.UpsertFromWebhookAsync(
            8001L, AppId, "acme-corp", "Organization", "selected",
            [], CancellationToken.None);

        var row = await db.GithubInstallations.FindAsync(8001L);
        row.Should().NotBeNull();
        row!.OrgId.Should().BeNull();
        row.ClaimedAt.Should().BeNull();
    }

    [Fact]
    public async Task UpsertFromWebhookAsync_is_idempotent_on_repeated_delivery()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, _) = await SeedOrgAsync(scope);
        var svc = CreateService(db);

        await svc.UpsertFromWebhookAsync(8010L, AppId, "acme-corp", "Organization", "selected", [], CancellationToken.None);
        await svc.UpsertFromWebhookAsync(8010L, AppId, "acme-corp", "Organization", "selected", [], CancellationToken.None);

        var count = await db.GithubInstallations.CountAsync(x => x.InstallationId == 8010L);
        count.Should().Be(1);
    }

    // ── ApplyRepoAddedAsync ────────────────────────────────────────────────────

    [Fact]
    public async Task ApplyRepoAddedAsync_unions_repo_set()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 7001L, orgId);
        var svc = CreateService(db);

        var repos = new[] { new RepoRef(101, "acme", "api"), new RepoRef(102, "acme", "sdk") };
        await svc.ApplyRepoAddedAsync(7001L, repos, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(7001L);
        var list = System.Text.Json.JsonSerializer.Deserialize<List<RepoRef>>(row!.RepoSetJson,
            new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
        list.Should().HaveCount(2);
        list!.Select(r => r.Id).Should().Contain(101).And.Contain(102);
    }

    [Fact]
    public async Task ApplyRepoAddedAsync_is_idempotent_when_repos_already_present()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 7010L, orgId);
        var svc = CreateService(db);

        var repos = new[] { new RepoRef(201, "acme", "api") };
        await svc.ApplyRepoAddedAsync(7010L, repos, CancellationToken.None);
        await svc.ApplyRepoAddedAsync(7010L, repos, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(7010L);
        var list = System.Text.Json.JsonSerializer.Deserialize<List<RepoRef>>(row!.RepoSetJson,
            new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
        list.Should().HaveCount(1); // idempotent — no duplicates
    }

    // ── ApplyRepoRemovedAsync ──────────────────────────────────────────────────

    [Fact]
    public async Task ApplyRepoRemovedAsync_subtracts_repo_set()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 7020L, orgId);
        var svc = CreateService(db);

        var initialRepos = new[] { new RepoRef(301, "acme", "api"), new RepoRef(302, "acme", "sdk") };
        await svc.ApplyRepoAddedAsync(7020L, initialRepos, CancellationToken.None);

        await svc.ApplyRepoRemovedAsync(7020L, [new RepoRef(301, "acme", "api")], CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(7020L);
        var list = System.Text.Json.JsonSerializer.Deserialize<List<RepoRef>>(row!.RepoSetJson,
            new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
        list.Should().HaveCount(1);
        list![0].Id.Should().Be(302);
    }

    [Fact]
    public async Task ApplyRepoRemovedAsync_marks_in_flight_pr_checks_REPO_NOT_COVERED()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 7030L, orgId);

        // Seed pr_checks for acme/api (should be marked) and acme/sdk (should be unchanged)
        var prCheckMarked = new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/api", Pr = 1,
            State = "success", CreatedAt = DateTime.UtcNow, ExternalId = Guid.NewGuid(),
        };
        var prCheckUnchanged = new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/sdk", Pr = 2,
            State = "success", CreatedAt = DateTime.UtcNow, ExternalId = Guid.NewGuid(),
        };
        db.PrChecks.AddRange(prCheckMarked, prCheckUnchanged);
        await db.SaveChangesAsync();

        // Setup repo_set to include both repos
        await CreateService(db).ApplyRepoAddedAsync(7030L,
            [new RepoRef(401, "acme", "api"), new RepoRef(402, "acme", "sdk")],
            CancellationToken.None);

        // Remove acme/api
        await CreateService(db).ApplyRepoRemovedAsync(7030L,
            [new RepoRef(401, "acme", "api")],
            CancellationToken.None);

        db.ChangeTracker.Clear();
        var marked = await db.PrChecks.FindAsync(prCheckMarked.Id);
        var unchanged = await db.PrChecks.FindAsync(prCheckUnchanged.Id);

        marked!.State.Should().Be("REPO_NOT_COVERED");
        unchanged!.State.Should().Be("success");
    }
}
