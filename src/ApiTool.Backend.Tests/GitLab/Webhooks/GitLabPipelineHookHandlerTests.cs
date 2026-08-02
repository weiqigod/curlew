// Tests for GitLabPipelineHookHandler — status reconciliation into pr_checks.
// Refs plan Decision D (lookup by installation+head_sha, not gitlab_status_id).
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitLab.Webhooks;

/// <summary>
/// Unit tests for <see cref="GitLabPipelineHookHandler"/> status reconciliation.
/// Each test uses an in-memory EF DB (no SQLite FK enforcement needed since we seed required rows).
/// </summary>
public sealed class GitLabPipelineHookHandlerTests
{
    private static readonly Guid InstallationId = Guid.NewGuid();

    private static GitLabPipelineHookHandler CreateHandler(ApiTool.Backend.Data.AppDbContext db)
        => new(db, NullLogger<GitLabPipelineHookHandler>.Instance);

    private static ApiTool.Backend.Data.AppDbContext CreateInMemoryDb()
    {
        var opts = new DbContextOptionsBuilder<ApiTool.Backend.Data.AppDbContext>()
            .UseInMemoryDatabase($"pipeline_handler_{Guid.NewGuid():N}")
            .Options;
        return new ApiTool.Backend.Data.AppDbContext(opts);
    }

    private static PrCheck MakePrCheck(Guid installationId, string sha, string status = "posting") =>
        new()
        {
            Id = Guid.NewGuid(),
            OrgId = Guid.NewGuid(),
            Provider = "gitlab",
            GitLabInstallationId = installationId,
            HeadSha = sha,
            Status = status,
            Repo = "g/p", Pr = 1, State = "success", CreatedAt = DateTime.UtcNow,
            ExternalId = Guid.NewGuid(),
        };

    [Fact]
    public async Task Handle_matching_pr_check_row_updates_status_to_posted_on_success()
    {
        await using var db = CreateInMemoryDb();
        var sha = "abc123def4567890abc123def4567890abc12345";
        db.PrChecks.Add(MakePrCheck(InstallationId, sha));
        await db.SaveChangesAsync();

        var handler = CreateHandler(db);
        var payload = new GitLabPipelineHookPayload(4242L, "success", sha, 31337L);
        await handler.HandleAsync(payload, InstallationId, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FirstAsync(x => x.HeadSha == sha);
        row.Status.Should().Be("posted");
    }

    [Fact]
    public async Task Handle_pipeline_state_running_updates_status_to_posting()
    {
        await using var db = CreateInMemoryDb();
        var sha = "aabbccddeeff11223344556677889900aabbccdd";
        db.PrChecks.Add(MakePrCheck(InstallationId, sha, "queued"));
        await db.SaveChangesAsync();

        var handler = CreateHandler(db);
        var payload = new GitLabPipelineHookPayload(1L, "running", sha, 1L);
        await handler.HandleAsync(payload, InstallationId, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FirstAsync(x => x.HeadSha == sha);
        row.Status.Should().Be("posting");
    }

    [Fact]
    public async Task Handle_pipeline_state_pending_updates_status_to_queued()
    {
        await using var db = CreateInMemoryDb();
        var sha = "11111111111111111111111111111111aaaaaaaa";
        db.PrChecks.Add(MakePrCheck(InstallationId, sha, "posting"));
        await db.SaveChangesAsync();

        var handler = CreateHandler(db);
        var payload = new GitLabPipelineHookPayload(1L, "pending", sha, 1L);
        await handler.HandleAsync(payload, InstallationId, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FirstAsync(x => x.HeadSha == sha);
        row.Status.Should().Be("queued");
    }

    [Fact]
    public async Task Handle_pipeline_state_canceled_updates_status_and_last_error()
    {
        await using var db = CreateInMemoryDb();
        var sha = "22222222222222222222222222222222bbbbbbbb";
        db.PrChecks.Add(MakePrCheck(InstallationId, sha, "posting"));
        await db.SaveChangesAsync();

        var handler = CreateHandler(db);
        var payload = new GitLabPipelineHookPayload(1L, "canceled", sha, 1L);
        await handler.HandleAsync(payload, InstallationId, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FirstAsync(x => x.HeadSha == sha);
        row.Status.Should().Be("failed");
        row.LastError.Should().Be("Pipeline cancelled in GitLab UI");
    }

    [Fact]
    public async Task Handle_no_matching_pr_check_logs_no_match_returns_success()
    {
        await using var db = CreateInMemoryDb();
        var handler = CreateHandler(db);
        var payload = new GitLabPipelineHookPayload(1L, "success", "nonexistentsha000000000000000000000000", 1L);

        // Must not throw
        await handler.HandleAsync(payload, InstallationId, CancellationToken.None);
    }

    [Fact]
    public async Task Handle_multiple_matching_rows_updates_all()
    {
        await using var db = CreateInMemoryDb();
        var sha = "33333333333333333333333333333333cccccccc";
        db.PrChecks.Add(MakePrCheck(InstallationId, sha, "posting"));
        db.PrChecks.Add(MakePrCheck(InstallationId, sha, "posting"));
        await db.SaveChangesAsync();

        var handler = CreateHandler(db);
        var payload = new GitLabPipelineHookPayload(1L, "success", sha, 1L);
        await handler.HandleAsync(payload, InstallationId, CancellationToken.None);

        db.ChangeTracker.Clear();
        var rows = await db.PrChecks.Where(x => x.HeadSha == sha).ToListAsync();
        rows.Should().HaveCount(2);
        rows.Should().AllSatisfy(r => r.Status.Should().Be("posted"));
    }

    [Fact]
    public async Task Handle_unknown_pipeline_state_logs_warning_no_update()
    {
        await using var db = CreateInMemoryDb();
        var sha = "44444444444444444444444444444444dddddddd";
        db.PrChecks.Add(MakePrCheck(InstallationId, sha, "posting"));
        await db.SaveChangesAsync();

        var handler = CreateHandler(db);
        var payload = new GitLabPipelineHookPayload(1L, "unknown_state_xyz", sha, 1L);

        // Must not throw
        await handler.HandleAsync(payload, InstallationId, CancellationToken.None);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FirstAsync(x => x.HeadSha == sha);
        row.Status.Should().Be("posting", "unknown state must not change the status");
    }
}
