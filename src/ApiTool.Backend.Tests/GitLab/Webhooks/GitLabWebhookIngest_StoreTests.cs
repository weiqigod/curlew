// Refs docs/SPECIFICATION.md:10922-10943 (schema), :9261 (idempotency contract).
// Tests for GitLabWebhookStore using SQLite in-memory DB.
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.GitLab.Webhooks;

/// <summary>
/// Unit tests for <see cref="GitLabWebhookStore"/> using SQLite in-memory DB.
/// Mirrors the GitHub side's GithubWebhookIngest_StoreTests pattern.
/// </summary>
public sealed class GitLabWebhookIngest_StoreTests
{
    private const string EventType = "Pipeline Hook";
    private const string Payload = """{"object_kind":"pipeline"}""";
    private const string EventUuid = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";

    private static GitLabWebhookStore CreateStore(AppDbContext db)
        => new(db, TimeProvider.System);

    /// <summary>Seeds the required Organization and GitLabInstallation rows so FK is satisfied.</summary>
    private static async Task<Guid> SeedInstallationAsync(AppDbContext db)
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"u{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "TestOrg", Slug = $"t{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        var instId = Guid.NewGuid();
        db.GitLabInstallations.Add(new GitLabInstallation
        {
            Id = instId, OrgId = orgId, ProjectId = 1L, ProjectPath = "g/p",
            AccessTokenCiphertext = [1, 2, 3],
            AccessTokenKid = "fake-kid",
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return instId;
    }

    [Fact]
    public async Task TryInsertPending_returns_inserted_true_for_new_event_uuid()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        var result = await store.TryInsertPendingAsync(
            EventUuid, EventType, installId, Payload, CancellationToken.None);

        result.Inserted.Should().BeTrue();
        result.ExistingReceivedAt.Should().BeNull();
        result.ExistingQuarantined.Should().BeFalse();
    }

    [Fact]
    public async Task TryInsertPending_returns_inserted_false_for_duplicate()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);
        var result = await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);

        result.Inserted.Should().BeFalse();
        result.ExistingReceivedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task MarkProcessed_sets_processed_at()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);
        await store.MarkProcessedAsync(EventUuid, CancellationToken.None);

        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        row.Should().NotBeNull();
        row!.ProcessedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task RecordFailure_increments_failure_count_below_threshold()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);
        var count = await store.RecordFailureAsync(EventUuid, "Something failed", CancellationToken.None);

        count.Should().Be(1);
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        row!.FailureCount.Should().Be(1);
        row.QuarantinedAt.Should().BeNull("not yet at threshold");
    }

    [Fact]
    public async Task RecordFailure_sets_quarantined_at_at_fifth_attempt()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);

        int count = 0;
        for (var i = 0; i < GitLabWebhookStore.QuarantineThreshold; i++)
            count = await store.RecordFailureAsync(EventUuid, $"Error {i + 1}", CancellationToken.None);

        count.Should().Be(GitLabWebhookStore.QuarantineThreshold);
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        row!.FailureCount.Should().Be(GitLabWebhookStore.QuarantineThreshold);
        row.QuarantinedAt.Should().NotBeNull("should be quarantined at threshold");
    }

    [Fact]
    public async Task DeleteOlderThan_removes_processed_rows()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);
        await store.MarkProcessedAsync(EventUuid, CancellationToken.None);

        // Set received_at to old date
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        row!.ReceivedAt = DateTime.UtcNow.AddDays(-40);
        await scope.Db.SaveChangesAsync();

        var cutoff = DateTime.UtcNow.AddDays(-30);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(1);
        scope.Db.ChangeTracker.Clear();
        var found = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        found.Should().BeNull();
    }

    [Fact]
    public async Task DeleteOlderThan_retains_quarantined_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);

        // Quarantine by recording 5 failures
        for (var i = 0; i < GitLabWebhookStore.QuarantineThreshold; i++)
            await store.RecordFailureAsync(EventUuid, "Error", CancellationToken.None);

        // Set old received_at
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        row!.ReceivedAt = DateTime.UtcNow.AddDays(-100);
        await scope.Db.SaveChangesAsync();

        var cutoff = DateTime.UtcNow.AddDays(-30);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0, "quarantined rows must be retained regardless of age");
        scope.Db.ChangeTracker.Clear();
        var stillThere = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        stillThere.Should().NotBeNull();
    }

    [Fact]
    public async Task DeleteOlderThan_retains_pending_rows_regardless_of_age()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var installId = await SeedInstallationAsync(scope.Db);

        var store = CreateStore(scope.Db);
        await store.TryInsertPendingAsync(EventUuid, EventType, installId, Payload, CancellationToken.None);

        // Set old received_at but leave as pending
        scope.Db.ChangeTracker.Clear();
        var row = await scope.Db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == EventUuid);
        row!.ReceivedAt = DateTime.UtcNow.AddDays(-100);
        await scope.Db.SaveChangesAsync();

        var cutoff = DateTime.UtcNow.AddDays(-30);
        var deleted = await store.DeleteOlderThanAsync(cutoff, CancellationToken.None);

        deleted.Should().Be(0, "pending rows must not be deleted");
    }
}
