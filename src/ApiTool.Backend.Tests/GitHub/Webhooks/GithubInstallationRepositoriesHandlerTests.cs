// Refs docs/SPECIFICATION.md:8419-8421 (installation_repositories.added/removed).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubInstallationRepositoriesHandler: repo-set add/remove webhooks.
/// </summary>
public sealed class GithubInstallationRepositoriesHandlerTests
{
    private static async Task<(AppDbContext db, Guid orgId, long installationId)> SeedAsync(
        TestDbScope scope)
    {
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        const long installId = 88001L;

        db.Users.Add(new User { Id = userId, Email = $"wh-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Wh", Slug = $"wh-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.GithubInstallations.Add(new GithubInstallation
        {
            InstallationId = installId,
            AppId = 12345L,
            OrgId = orgId,
            AccountLogin = "acme-corp",
            AccountType = "Organization",
            RepoSelection = "selected",
            RepoSetJson = "[]",
            InstalledAt = DateTime.UtcNow,
            ClaimedAt = DateTime.UtcNow,
            LastReconciledAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (db, orgId, installId);
    }

    [Fact]
    public async Task installation_repositories_added_unions_repo_set()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, _, installId) = await SeedAsync(scope);
        var clock = new FakeClock(DateTimeOffset.UtcNow);
        var svc = new GithubInstallationsService(db, clock, new FakeInstallationTokenCache(), NullLogger<GithubInstallationsService>.Instance);
        var handler = new GithubInstallationRepositoriesHandler(svc);

        var payload = new GithubInstallationRepositoriesPayload(
            InstallationId: installId,
            Repositories: [new RepoRef(101, "acme", "api"), new RepoRef(102, "acme", "sdk")]);

        await handler.HandleAddedAsync(payload, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(installId);
        var list = System.Text.Json.JsonSerializer.Deserialize<List<RepoRef>>(
            row!.RepoSetJson,
            new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
        list.Should().HaveCount(2);
    }

    [Fact]
    public async Task installation_repositories_removed_subtracts_and_marks_pr_checks()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId, installId) = await SeedAsync(scope);
        var clock = new FakeClock(DateTimeOffset.UtcNow);
        var svc = new GithubInstallationsService(db, clock, new FakeInstallationTokenCache(), NullLogger<GithubInstallationsService>.Instance);
        var handler = new GithubInstallationRepositoriesHandler(svc);

        // First add two repos
        var addPayload = new GithubInstallationRepositoriesPayload(
            InstallationId: installId,
            Repositories: [new RepoRef(201, "acme", "api"), new RepoRef(202, "acme", "sdk")]);
        await handler.HandleAddedAsync(addPayload, CancellationToken.None);

        // Seed a pr_check for acme/api
        var check = new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/api", Pr = 1,
            State = "success", CreatedAt = DateTime.UtcNow,
        };
        db.PrChecks.Add(check);
        await db.SaveChangesAsync();

        // Remove acme/api
        var removePayload = new GithubInstallationRepositoriesPayload(
            InstallationId: installId,
            Repositories: [new RepoRef(201, "acme", "api")]);
        await handler.HandleRemovedAsync(removePayload, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(installId);
        var list = System.Text.Json.JsonSerializer.Deserialize<List<RepoRef>>(
            row!.RepoSetJson,
            new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
        list.Should().HaveCount(1);
        list![0].Id.Should().Be(202);

        var markedCheck = await db.PrChecks.FindAsync(check.Id);
        markedCheck!.State.Should().Be("REPO_NOT_COVERED");
    }
}
