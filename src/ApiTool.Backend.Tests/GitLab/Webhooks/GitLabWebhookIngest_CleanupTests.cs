// Refs docs/SPECIFICATION.md:10943 (30-day retention for GitLab), plan Decision H.
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.GitLab.Webhooks;

/// <summary>
/// Verifies the 30-day cleanup retention policy via <see cref="GitLabWebhookStore.DeleteOlderThanAsync"/>.
/// Mirrors <c>GithubWebhookIngest_CleanupTests</c>.
/// </summary>
public sealed class GitLabWebhookIngest_CleanupTests
{
    private static GitLabWebhookStore CreateStore(TestDbScope scope, DateTimeOffset? now = null)
    {
        var clock = new FakeClock(now ?? new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero));
        return new GitLabWebhookStore(scope.Db, clock);
    }

    /// <summary>Seeds the required Organization and GitLabInstallation rows so FK is satisfied.</summary>
    private static async Task<Guid> SeedInstallationAsync(ApiTool.Backend.Data.AppDbContext db)
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"u{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "CleanupOrg", Slug = $"c{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        var instId = Guid.NewGuid();
        db.GitLabInstallations.Add(new GitLabInstallation
        {
            Id = instId, OrgId = orgId, ProjectId = 2L, ProjectPath = "g/q",
            AccessTokenCiphertext = [1, 2, 3],
            AccessTokenKid = "fake-kid",
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return instId;
    }

    [Fact]
    public async Task DeleteOlderThan_removes_processed_rows_older_than_cutoff()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope);
        var eventUuid = Guid.NewGuid().ToString();
        await store.TryInsertPendingAsync(eventUuid, "Pipeline Hook", installId, "{}", CancellationToken.None);
        await store.MarkProcessedAsync(eventUuid, CancellationToken.None);

        // Cutoff far in the future — should remove the processed row
        var cutoff = new DateTime(2027, 1, 1, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(1);
        scope.Db.ChangeTracker.Clear();
        (await scope.Db.GitLabWebhookEvents.CountAsync()).Should().Be(0);
    }

    [Fact]
    public async Task DeleteOlderThan_retains_quarantined_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope);
        var eventUuid = Guid.NewGuid().ToString();
        await store.TryInsertPendingAsync(eventUuid, "Pipeline Hook", installId, "{}", CancellationToken.None);
        for (var i = 0; i < GitLabWebhookStore.QuarantineThreshold; i++)
            await store.RecordFailureAsync(eventUuid, "err", CancellationToken.None);

        var cutoff = new DateTime(2030, 12, 31, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0, "quarantined rows must be retained regardless of age");
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GitLabWebhookEvents.FirstAsync();
        row.QuarantinedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task DeleteOlderThan_retains_pending_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope);
        var eventUuid = Guid.NewGuid().ToString();
        await store.TryInsertPendingAsync(eventUuid, "Pipeline Hook", installId, "{}", CancellationToken.None);

        var cutoff = new DateTime(2030, 12, 31, 0, 0, 0, DateTimeKind.Utc);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0, "pending rows (ProcessedAt IS NULL) must not be deleted");
        scope.Db.ChangeTracker.Clear();
        (await scope.Db.GitLabWebhookEvents.CountAsync()).Should().Be(1);
    }
}
