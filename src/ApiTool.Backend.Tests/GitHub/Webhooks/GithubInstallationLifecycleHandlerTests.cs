// Refs docs/SPECIFICATION.md:8560-8562 (installation lifecycle: suspend, unsuspend, delete).
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubInstallationLifecycleHandler.
/// </summary>
public sealed class GithubInstallationLifecycleHandlerTests
{
    private static async Task<(ApiTool.Backend.Data.AppDbContext db, long installId, Guid orgId)> SeedAsync(
        TestDbScope scope)
    {
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"lh-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "LifecycleH", Slug = $"lh-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        const long installId = 60001L;
        db.GithubInstallations.Add(new GithubInstallation
        {
            InstallationId = installId, AppId = 111L, OrgId = orgId,
            AccountLogin = "acme", AccountType = "Organization",
            RepoSelection = "selected", RepoSetJson = "[]",
            InstalledAt = DateTime.UtcNow, LastReconciledAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (db, installId, orgId);
    }

    private static (GithubInstallationLifecycleHandler handler, FakeInstallationTokenCache cache)
        CreateHandler(ApiTool.Backend.Data.AppDbContext db)
    {
        var cache = new FakeInstallationTokenCache();
        var clock = new FakeClock(DateTimeOffset.UtcNow);
        var svc = new GithubInstallationsService(
            db, clock, cache, NullLogger<GithubInstallationsService>.Instance);
        return (new GithubInstallationLifecycleHandler(svc), cache);
    }

    [Fact]
    public async Task installation_deleted_evicts_token_and_marks_pr_checks()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, installId, orgId) = await SeedAsync(scope);

        // Add an open pr_check
        db.PrChecks.Add(new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/api", Pr = 1,
            State = "success", Status = "pending", CreatedAt = DateTime.UtcNow,
            ExternalId = Guid.NewGuid(),
        });
        await db.SaveChangesAsync();

        var (handler, cache) = CreateHandler(db);
        await handler.HandleDeletedAsync(new GithubInstallationLifecyclePayload(installId), CancellationToken.None);

        db.ChangeTracker.Clear();
        var install = await db.GithubInstallations.FindAsync(installId);
        install!.DeletedAt.Should().NotBeNull();
        cache.Evictions.Should().Contain(installId);

        var check = await db.PrChecks.FirstAsync(p => p.Repo == "acme/api");
        check.State.Should().Be("INSTALLATION_DELETED");
    }

    [Fact]
    public async Task installation_suspend_evicts_token()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, installId, _) = await SeedAsync(scope);
        var (handler, cache) = CreateHandler(db);

        await handler.HandleSuspendedAsync(new GithubInstallationLifecyclePayload(installId), CancellationToken.None);

        db.ChangeTracker.Clear();
        var install = await db.GithubInstallations.FindAsync(installId);
        install!.SuspendedAt.Should().NotBeNull();
        cache.Evictions.Should().Contain(installId);
    }

    [Fact]
    public async Task installation_unsuspend_clears_suspended_at()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, installId, _) = await SeedAsync(scope);

        // Pre-suspend
        var install = await db.GithubInstallations.FindAsync(installId);
        install!.SuspendedAt = DateTime.UtcNow;
        await db.SaveChangesAsync();

        var (handler, _) = CreateHandler(db);
        await handler.HandleUnsuspendedAsync(new GithubInstallationLifecyclePayload(installId), CancellationToken.None);

        db.ChangeTracker.Clear();
        var updated = await db.GithubInstallations.FindAsync(installId);
        updated!.SuspendedAt.Should().BeNull();
    }
}
