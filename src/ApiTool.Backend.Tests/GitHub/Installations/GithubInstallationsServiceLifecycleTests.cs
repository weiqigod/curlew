// Refs docs/SPECIFICATION.md:8560-8562 (installation lifecycle: suspend, unsuspend, delete).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub.Installations;

/// <summary>
/// Unit tests for GithubInstallationsService lifecycle methods:
/// MarkSuspendedAsync, MarkUnsuspendedAsync, MarkDeletedAsync.
/// </summary>
public sealed class GithubInstallationsServiceLifecycleTests
{
    private const long AppId = 99001L;

    private static GithubInstallationsService CreateService(
        AppDbContext db, IInstallationTokenCache? cache = null)
    {
        var clock = new FakeClock(DateTimeOffset.UtcNow);
        return new GithubInstallationsService(
            db, clock, cache ?? new FakeInstallationTokenCache(),
            NullLogger<GithubInstallationsService>.Instance);
    }

    private static async Task<(AppDbContext db, Guid orgId)> SeedOrgAsync(TestDbScope scope)
    {
        var db = scope.Db;
        await db.Database.MigrateAsync();
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"lc-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Lifecycle", Slug = $"lc-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (db, orgId);
    }

    private static async Task<GithubInstallation> SeedInstallAsync(
        AppDbContext db, long installationId, Guid? orgId)
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
            LastReconciledAt = DateTime.UtcNow,
        };
        db.GithubInstallations.Add(row);
        await db.SaveChangesAsync();
        return row;
    }

    // ── MarkSuspendedAsync ────────────────────────────────────────────────────

    [Fact]
    public async Task MarkSuspended_sets_suspended_at_and_evicts_token()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 50001L, orgId);
        var cache = new FakeInstallationTokenCache();
        var svc = CreateService(db, cache);

        await svc.MarkSuspendedAsync(50001L, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(50001L);
        row!.SuspendedAt.Should().NotBeNull();
        cache.Evictions.Should().Contain(50001L);
    }

    [Fact]
    public async Task MarkSuspended_unknown_installation_logs_and_returns()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, _) = await SeedOrgAsync(scope);
        var cache = new FakeInstallationTokenCache();
        var svc = CreateService(db, cache);

        // Should not throw
        await svc.MarkSuspendedAsync(99999L, CancellationToken.None);
        cache.Evictions.Should().BeEmpty(); // no eviction for unknown install
    }

    // ── MarkUnsuspendedAsync ──────────────────────────────────────────────────

    [Fact]
    public async Task MarkUnsuspended_clears_suspended_at()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        var install = await SeedInstallAsync(db, 50010L, orgId);
        install.SuspendedAt = DateTime.UtcNow;
        await db.SaveChangesAsync();

        var svc = CreateService(db);

        await svc.MarkUnsuspendedAsync(50010L, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(50010L);
        row!.SuspendedAt.Should().BeNull();
    }

    // ── MarkDeletedAsync ──────────────────────────────────────────────────────

    [Fact]
    public async Task MarkDeleted_sets_deleted_at_evicts_token_and_marks_open_pr_checks()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 50020L, orgId);

        // Seed an open pr_check (not 'posted' or 'failed') that should be marked
        var openCheck = new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/api", Pr = 1,
            State = "success", Status = "queued", CreatedAt = DateTime.UtcNow,
            ExternalId = Guid.NewGuid(),
        };
        // Seed a posted pr_check that should NOT be marked
        var postedCheck = new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/sdk", Pr = 2,
            State = "success", Status = "posted", CreatedAt = DateTime.UtcNow,
            ExternalId = Guid.NewGuid(),
        };
        db.PrChecks.AddRange(openCheck, postedCheck);
        await db.SaveChangesAsync();

        var cache = new FakeInstallationTokenCache();
        var svc = CreateService(db, cache);

        await svc.MarkDeletedAsync(50020L, CancellationToken.None);

        db.ChangeTracker.Clear();
        var install = await db.GithubInstallations.FindAsync(50020L);
        install!.DeletedAt.Should().NotBeNull();
        cache.Evictions.Should().Contain(50020L);

        var markedCheck = await db.PrChecks.FindAsync(openCheck.Id);
        markedCheck!.State.Should().Be("INSTALLATION_DELETED");

        var unchangedCheck = await db.PrChecks.FindAsync(postedCheck.Id);
        unchangedCheck!.State.Should().Be("success"); // posted — not marked
    }

    [Fact]
    public async Task MarkDeleted_does_not_mark_already_posted_pr_checks()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, orgId) = await SeedOrgAsync(scope);
        await SeedInstallAsync(db, 50030L, orgId);

        var failedCheck = new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/api", Pr = 1,
            State = "failure", Status = "failed", CreatedAt = DateTime.UtcNow,
            ExternalId = Guid.NewGuid(),
        };
        db.PrChecks.Add(failedCheck);
        await db.SaveChangesAsync();

        var svc = CreateService(db);
        await svc.MarkDeletedAsync(50030L, CancellationToken.None);

        db.ChangeTracker.Clear();
        var check = await db.PrChecks.FindAsync(failedCheck.Id);
        check!.State.Should().Be("failure"); // 'failed' status — should NOT be marked
    }

    [Fact]
    public async Task MarkDeleted_when_orgid_null_skips_pr_checks()
    {
        await using var scope = TestDb.CreateOpen();
        var (db, _) = await SeedOrgAsync(scope);
        // Webhook-first install with org_id = NULL
        await SeedInstallAsync(db, 50040L, orgId: null);

        var svc = CreateService(db);
        // Should not throw, should just set deleted_at
        await svc.MarkDeletedAsync(50040L, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.GithubInstallations.FindAsync(50040L);
        row!.DeletedAt.Should().NotBeNull();
    }
}
