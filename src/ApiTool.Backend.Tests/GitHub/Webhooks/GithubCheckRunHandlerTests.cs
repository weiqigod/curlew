// Refs docs/SPECIFICATION.md:8565 (check_run.rerequested).
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubCheckRunHandler.rerequested routing.
/// </summary>
public sealed class GithubCheckRunHandlerTests
{
    /// <summary>
    /// Test double for <see cref="IRerunJobQueue"/> that records all enqueue calls.
    /// </summary>
    private sealed class RecordingRerunJobQueue : IRerunJobQueue
    {
        public List<(Guid ExternalId, string HeadSha)> Calls { get; } = new();

        public Task EnqueueAsync(Guid externalId, string headSha, CancellationToken ct)
        {
            Calls.Add((externalId, headSha));
            return Task.CompletedTask;
        }
    }

    [Fact]
    public async Task rerequested_with_known_external_id_enqueues_rerun_job()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var externalId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"cr-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "CROrg", Slug = $"cr-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.PrChecks.Add(new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "acme/api", Pr = 42,
            State = "success", HeadSha = "deadbeef", ExternalId = externalId,
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var logger = new RecordingLogger<GithubCheckRunHandler>();
        var queue = new RecordingRerunJobQueue();
        var handler = new GithubCheckRunHandler(db, queue, logger);

        await handler.HandleRerequestedAsync(
            new GithubCheckRunRerequestedPayload(externalId.ToString(), "deadbeef"),
            CancellationToken.None);

        // Verify that an enqueue was issued with the correct identifiers.
        queue.Calls.Should().HaveCount(1);
        queue.Calls[0].ExternalId.Should().Be(externalId);
        queue.Calls[0].HeadSha.Should().Be("deadbeef");

        // Structured log is also emitted for observability.
        logger.Messages.Should().ContainMatch("*github_check_run_rerequested*");
        logger.Messages.Should().ContainMatch($"*{externalId}*");
    }

    [Fact]
    public async Task rerequested_with_unknown_external_id_logs_warning_does_not_enqueue()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var logger = new RecordingLogger<GithubCheckRunHandler>();
        var queue = new RecordingRerunJobQueue();
        var handler = new GithubCheckRunHandler(db, queue, logger);
        var unknownId = Guid.NewGuid();

        // Should not throw
        await handler.HandleRerequestedAsync(
            new GithubCheckRunRerequestedPayload(unknownId.ToString(), "abc"),
            CancellationToken.None);

        logger.Messages.Should().ContainMatch("*github_check_run_rerequested_unknown_pr_check*");
        queue.Calls.Should().BeEmpty();
    }

    [Fact]
    public async Task rerequested_with_malformed_external_id_logs_warning_does_not_enqueue()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var logger = new RecordingLogger<GithubCheckRunHandler>();
        var queue = new RecordingRerunJobQueue();
        var handler = new GithubCheckRunHandler(db, queue, logger);

        // Should not throw
        await handler.HandleRerequestedAsync(
            new GithubCheckRunRerequestedPayload("not-a-guid", "abc"),
            CancellationToken.None);

        logger.Messages.Should().ContainMatch("*github_check_run_rerequested_unknown_external_id*");
        queue.Calls.Should().BeEmpty();
    }
}
