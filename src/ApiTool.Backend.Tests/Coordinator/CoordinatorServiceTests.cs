using System.Text.Json;
using ApiTool.Backend.Coordinator;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Results;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Coordinator;

/// <summary>Unit tests for <see cref="CoordinatorService"/> using real SQLite via <see cref="TestDb"/>.</summary>
public sealed class CoordinatorServiceTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly FakeTimeProvider _clock;
    private readonly CoordinatorService _service;
    private readonly Guid _ownerId;
    private readonly Guid _orgId;

    public CoordinatorServiceTests()
    {
        _scope = TestDb.CreateOpen();
        _scope.Db.Database.Migrate();

        _clock = new FakeTimeProvider(DateTime.UtcNow);

        var notifier = new NoopResultIngestedNotifier();
        var resultsService = new ResultsService(_scope.Db, _clock, notifier);
        var roleResolver = new RoleResolver(_scope.Db);

        _service = new CoordinatorService(_scope.Db, _clock, resultsService, roleResolver,
            NullLogger<CoordinatorService>.Instance);

        // Seed owner user, org, and membership
        _ownerId = Guid.NewGuid();
        _orgId = Guid.NewGuid();

        _scope.Db.Users.Add(new User
        {
            Id = _ownerId,
            Email = $"owner-{_ownerId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        _scope.Db.Organizations.Add(new Organization
        {
            Id = _orgId,
            Name = "TestOrg",
            Slug = $"testorg-{_ownerId:N}"[..20],
            OwnerId = _ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        _scope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = _orgId,
            UserId = _ownerId,
            Role = OrgRole.Owner,
            PermissionsJson = "[]",
            JoinedAt = DateTime.UtcNow,
        });
        _scope.Db.SaveChanges();
        _scope.Db.ChangeTracker.Clear();
    }

    public async ValueTask DisposeAsync()
    {
        await _scope.DisposeAsync();
    }

    // ── CreateJob ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task CreateJob_persists_job_with_pending_shards()
    {
        var (dto, error, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc123", ShardCount = 4 }, default);

        error.Should().Be(CoordinatorError.None);
        dto.Should().NotBeNull();
        dto!.ShardCount.Should().Be(4);
        dto.State.Should().Be("pending");
        dto.Shards.Should().HaveCount(4);
        dto.Shards.Should().AllSatisfy(s => s.State.Should().Be("pending"));
        dto.JobId.Should().StartWith("job_");
        dto.Shards.Should().AllSatisfy(s => s.ShardId.Should().StartWith("shd_"));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    [InlineData(65)]
    public async Task CreateJob_rejects_invalid_shard_count(int shardCount)
    {
        var (dto, error, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = shardCount }, default);

        error.Should().Be(CoordinatorError.InvalidShardCount);
        dto.Should().BeNull();
    }

    [Fact]
    public async Task CreateJob_returns_permission_denied_for_non_member()
    {
        var nonMemberId = Guid.NewGuid();
        var (dto, error, _) = await _service.CreateJobAsync(
            nonMemberId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 2 }, default);

        error.Should().Be(CoordinatorError.PermissionDenied);
        dto.Should().BeNull();
    }

    [Fact]
    public async Task CreateJob_returns_permission_denied_for_custom_role_without_coordinator_worker()
    {
        // A member whose custom role omits coordinator.worker should be denied.
        var memberId = Guid.NewGuid();
        _scope.Db.Users.Add(new User
        {
            Id = memberId,
            Email = $"restricted-{memberId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });

        // Create a custom role with no coordinator.worker permission.
        var roleId = Guid.NewGuid();
        _scope.Db.OrganizationCustomRoles.Add(new CustomRole
        {
            Id = roleId,
            OrgId = _orgId,
            Name = "RestrictedRole",
            PermissionsJson = JsonSerializer.Serialize(new[] { "results.view" }),
            CreatedBy = _ownerId,
            CreatedAt = DateTime.UtcNow,
        });

        _scope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = _orgId,
            UserId = memberId,
            Role = OrgRole.Member,
            RoleId = roleId,
            PermissionsJson = "[]",
            JoinedAt = DateTime.UtcNow,
        });
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        var (dto, error, _) = await _service.CreateJobAsync(
            memberId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 2 }, default);

        error.Should().Be(CoordinatorError.PermissionDenied,
            because: "a custom role without coordinator.worker should not be allowed to create jobs");
        dto.Should().BeNull();
    }

    // ── Claim ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Claim_transitions_one_shard_to_running()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 4 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);

        var (shardDto, error) = await _service.ClaimAsync(
            _ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);

        error.Should().Be(CoordinatorError.None);
        shardDto.Should().NotBeNull();
        shardDto!.State.Should().Be("running");
        shardDto.AssignedWorker.Should().Be("w1");
    }

    [Fact]
    public async Task Claim_returns_no_shards_available_when_all_claimed()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 4 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);

        // Claim all 4
        for (var i = 0; i < 4; i++)
        {
            var (s, e) = await _service.ClaimAsync(
                _ownerId, _orgId, jobId, new ClaimRequest { WorkerId = $"w{i}" }, default);
            e.Should().Be(CoordinatorError.None, $"claim {i} should succeed");
        }

        // 5th claim should fail
        var (dto5, error5) = await _service.ClaimAsync(
            _ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w5" }, default);

        error5.Should().Be(CoordinatorError.NoShardsAvailable);
        dto5.Should().BeNull();
    }

    [Fact]
    public async Task Claim_round_robins_across_workers()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 4 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);

        var (s1, e1) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "wa" }, default);
        var (s2, e2) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "wb" }, default);

        e1.Should().Be(CoordinatorError.None);
        e2.Should().Be(CoordinatorError.None);
        s1!.AssignedWorker.Should().Be("wa");
        s2!.AssignedWorker.Should().Be("wb");
        s1.ShardId.Should().NotBe(s2.ShardId);
    }

    // ── SubmitResult ──────────────────────────────────────────────────────────

    [Fact]
    public async Task SubmitResult_transitions_shard_to_completed()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 2 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);
        ShardId.TryParse(shardDto!.ShardId, out var shardId);

        var (error, _) = await _service.SubmitResultAsync(
            _ownerId, _orgId, jobId, shardId,
            new SubmitResultRequest { WorkerId = "w1", PassCount = 1, FailCount = 0, DurationMs = 100 },
            default);

        error.Should().Be(CoordinatorError.None);
        var shard = await _scope.Db.CoordinatorShards.FindAsync(shardId);
        shard!.Status.Should().Be(CoordinatorShardStatus.Completed);
    }

    [Fact]
    public async Task SubmitResult_rejects_wrong_worker()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 2 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);
        ShardId.TryParse(shardDto!.ShardId, out var shardId);

        // w2 tries to submit for w1's shard
        var (error, _) = await _service.SubmitResultAsync(
            _ownerId, _orgId, jobId, shardId,
            new SubmitResultRequest { WorkerId = "w2", PassCount = 1, FailCount = 0, DurationMs = 100 },
            default);

        error.Should().Be(CoordinatorError.ShardNotClaimed);
    }

    [Fact]
    public async Task SubmitResult_on_last_shard_creates_aggregate_result()
    {
        var collectionSha = "sha-aggregate-test";
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = collectionSha, ShardCount = 1 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);
        ShardId.TryParse(shardDto!.ShardId, out var shardId);

        var (error, _) = await _service.SubmitResultAsync(
            _ownerId, _orgId, jobId, shardId,
            new SubmitResultRequest { WorkerId = "w1", PassCount = 3, FailCount = 1, DurationMs = 500 },
            default);

        error.Should().Be(CoordinatorError.None);

        // Job should be completed with an aggregate result
        var job = await _scope.Db.CoordinatorJobs.FindAsync(jobId);
        job!.Status.Should().Be(CoordinatorJobStatus.Completed);
        job.AggregateResultId.Should().NotBeNull();

        // Result row should exist with collection_name = collectionSha
        var result = await _scope.Db.Results.FindAsync(job.AggregateResultId!.Value);
        result.Should().NotBeNull();
        result!.CollectionName.Should().Be(collectionSha);
        result.TriggeredBy.Should().Be("coordinator");
    }

    [Fact]
    public async Task SubmitResult_on_last_shard_flattens_shard_items_into_aggregate()
    {
        // Arrange: create a 1-shard job and submit result items to exercise JSON flattening path.
        var collectionSha = "sha-items-test";
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = collectionSha, ShardCount = 1 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);
        ShardId.TryParse(shardDto!.ShardId, out var shardId);

        var items = new List<object>
        {
            new { name = "GET /health", status = "Passed", duration_ms = 50, message = (string?)null },
            new { name = "POST /api/v1/things", status = "Failed", duration_ms = 120, message = "unexpected 500" },
        };

        var (error, _) = await _service.SubmitResultAsync(
            _ownerId, _orgId, jobId, shardId,
            new SubmitResultRequest { WorkerId = "w1", PassCount = 1, FailCount = 1, DurationMs = 170, Items = items },
            default);

        error.Should().Be(CoordinatorError.None);

        var job = await _scope.Db.CoordinatorJobs.FindAsync(jobId);
        job!.Status.Should().Be(CoordinatorJobStatus.Completed);
        job.AggregateResultId.Should().NotBeNull();

        // Aggregate result should exist with the correct totals.
        var result = await _scope.Db.Results.FindAsync(job.AggregateResultId!.Value);
        result.Should().NotBeNull();
        result!.PassCount.Should().Be(1);
        result.FailCount.Should().Be(1);
        result.CollectionName.Should().Be(collectionSha);
    }

    [Fact]
    public async Task SubmitResult_on_last_shard_returns_error_when_ingestion_fails()
    {
        // Arrange: create a 1-shard job then remove the creator from the org so IngestAsync fails.
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "ingest-fail-sha", ShardCount = 1 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);
        ShardId.TryParse(shardDto!.ShardId, out var shardId);

        // Remove creator's membership so IngestAsync returns PermissionDenied.
        var membership = await _scope.Db.OrganizationMembers
            .FirstAsync(m => m.OrgId == _orgId && m.UserId == _ownerId);
        _scope.Db.OrganizationMembers.Remove(membership);
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        var (error, message) = await _service.SubmitResultAsync(
            _ownerId, _orgId, jobId, shardId,
            new SubmitResultRequest { WorkerId = "w1", PassCount = 1, FailCount = 0, DurationMs = 100 },
            default);

        // The error should be surfaced rather than silently swallowed.
        error.Should().NotBe(CoordinatorError.None,
            because: "when IngestAsync fails, SubmitResultAsync should propagate the failure");

        // The job should NOT be marked Completed with a null AggregateResultId.
        var job = await _scope.Db.CoordinatorJobs.FindAsync(jobId);
        job!.Status.Should().NotBe(CoordinatorJobStatus.Completed,
            because: "the job should not be marked completed when aggregation failed");
    }

    // ── Heartbeat ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task Heartbeat_updates_last_heartbeat_at()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 1 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);
        ShardId.TryParse(shardDto!.ShardId, out var shardId);

        // Advance clock
        _clock.Advance(TimeSpan.FromSeconds(5));
        var before = _clock.GetUtcNow().UtcDateTime;

        var (error, _) = await _service.HeartbeatAsync(
            _ownerId, _orgId, jobId, shardId, new HeartbeatRequest { WorkerId = "w1" }, default);

        error.Should().Be(CoordinatorError.None);
        var shard = await _scope.Db.CoordinatorShards.FindAsync(shardId);
        shard!.LastHeartbeatAt.Should().BeOnOrAfter(before);
    }

    // ── ReapStaleShards ──────────────────────────────────────────────────────

    [Fact]
    public async Task ReapStaleShards_reclaims_shards_past_timeout()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 1 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);
        ShardId.TryParse(shardDto!.ShardId, out var shardId);

        // Advance clock past the heartbeat timeout
        _clock.Advance(CoordinatorService.HeartbeatTimeout + TimeSpan.FromSeconds(1));

        var count = await _service.ReapStaleShardsAsync(default);

        count.Should().Be(1);
        var shard = await _scope.Db.CoordinatorShards.FindAsync(shardId);
        shard!.Status.Should().Be(CoordinatorShardStatus.Pending);
        shard.AssignedWorker.Should().BeNull();
    }

    [Fact]
    public async Task ReapStaleShards_leaves_fresh_shards_alone()
    {
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "abc", ShardCount = 1 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        var (shardDto, _) = await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w1" }, default);

        // No time advance — heartbeat is fresh
        var count = await _service.ReapStaleShardsAsync(default);

        count.Should().Be(0);
    }

    [Fact]
    public async Task ReapStaleShards_reclaims_scheduled_runs_with_stale_heartbeat()
    {
        // Arrange: a schedule and a running ScheduledRun with stale heartbeat
        var schedule = await SeedScheduleAsync();
        var runId = Guid.NewGuid();
        var staleHeartbeat = _clock.GetUtcNow().UtcDateTime.AddMinutes(-6);
        _scope.Db.ScheduledRuns.Add(new ScheduledRun
        {
            Id = runId,
            ScheduleId = schedule.Id,
            Status = ScheduledRunStatus.Running,
            ClaimToken = Guid.NewGuid(),
            ClaimedByWorker = "worker-1",
            ClaimedAt = staleHeartbeat,
            StartedAt = staleHeartbeat,
            LastHeartbeatAt = staleHeartbeat,
            ClaimDeadline = staleHeartbeat.AddSeconds(30),
            CreatedAt = staleHeartbeat,
        });
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        var count = await _service.ReapStaleShardsAsync(default);

        count.Should().BeGreaterThanOrEqualTo(1, because: "the stale scheduled_run should be reaped");
        var run = await _scope.Db.ScheduledRuns.FindAsync(runId);
        run!.Status.Should().Be(ScheduledRunStatus.Queued, because: "reaper must reset status to Queued");
        run.ClaimToken.Should().BeNull();
        run.ClaimedByWorker.Should().BeNull();
        run.ClaimedAt.Should().BeNull();
        run.LastHeartbeatAt.Should().BeNull();
        run.ClaimDeadline.Should().BeNull();
        run.StartedAt.Should().BeNull();
    }

    [Fact]
    public async Task ReapStaleShards_does_not_reclaim_fresh_scheduled_run()
    {
        // Arrange: a running ScheduledRun with a heartbeat only 1 minute ago
        var schedule = await SeedScheduleAsync();
        var runId = Guid.NewGuid();
        var recentHeartbeat = _clock.GetUtcNow().UtcDateTime.AddMinutes(-1);
        _scope.Db.ScheduledRuns.Add(new ScheduledRun
        {
            Id = runId,
            ScheduleId = schedule.Id,
            Status = ScheduledRunStatus.Running,
            ClaimToken = Guid.NewGuid(),
            ClaimedByWorker = "worker-fresh",
            ClaimedAt = recentHeartbeat,
            StartedAt = recentHeartbeat,
            LastHeartbeatAt = recentHeartbeat,
            ClaimDeadline = recentHeartbeat.AddSeconds(30),
            CreatedAt = recentHeartbeat,
        });
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        var count = await _service.ReapStaleShardsAsync(default);

        count.Should().Be(0, because: "fresh scheduled_run should not be reaped");
        var run = await _scope.Db.ScheduledRuns.FindAsync(runId);
        run!.Status.Should().Be(ScheduledRunStatus.Running, because: "fresh run must remain Running");
    }

    [Fact]
    public async Task ReapStaleShards_does_not_reclaim_completed_scheduled_run()
    {
        // Arrange: a completed ScheduledRun regardless of heartbeat timestamp
        var schedule = await SeedScheduleAsync();
        var runId = Guid.NewGuid();
        var old = _clock.GetUtcNow().UtcDateTime.AddHours(-1);
        _scope.Db.ScheduledRuns.Add(new ScheduledRun
        {
            Id = runId,
            ScheduleId = schedule.Id,
            Status = ScheduledRunStatus.Completed,
            LastHeartbeatAt = old,
            CreatedAt = old,
            CompletedAt = old,
        });
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        var count = await _service.ReapStaleShardsAsync(default);

        count.Should().Be(0);
        var run = await _scope.Db.ScheduledRuns.FindAsync(runId);
        run!.Status.Should().Be(ScheduledRunStatus.Completed);
    }

    [Fact]
    public async Task ReapStaleShards_sums_shard_and_scheduled_run_counts()
    {
        // Arrange: 1 stale coordinator shard + 2 stale scheduled_runs
        var (jobDto, _, _) = await _service.CreateJobAsync(
            _ownerId, _orgId, new CreateJobRequest { CollectionSha = "sum-test", ShardCount = 1 }, default);
        CoordinatorJobId.TryParse(jobDto!.JobId, out var jobId);
        await _service.ClaimAsync(_ownerId, _orgId, jobId, new ClaimRequest { WorkerId = "w-sum" }, default);

        var schedule = await SeedScheduleAsync();
        var staleHb = _clock.GetUtcNow().UtcDateTime.AddMinutes(-6);
        for (var i = 0; i < 2; i++)
        {
            _scope.Db.ScheduledRuns.Add(new ScheduledRun
            {
                Id = Guid.NewGuid(),
                ScheduleId = schedule.Id,
                Status = ScheduledRunStatus.Running,
                ClaimToken = Guid.NewGuid(),
                ClaimedByWorker = $"w{i}",
                ClaimedAt = staleHb,
                StartedAt = staleHb,
                LastHeartbeatAt = staleHb,
                ClaimDeadline = staleHb.AddSeconds(30),
                CreatedAt = staleHb,
            });
        }
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        // Advance past shard timeout (60s) and scheduled_run timeout (5min)
        _clock.Advance(CoordinatorService.ScheduledRunHeartbeatTimeout + TimeSpan.FromMinutes(1));

        var count = await _service.ReapStaleShardsAsync(default);

        count.Should().Be(3, because: "1 stale shard + 2 stale scheduled_runs = 3 total reaped");
    }

    // ── Schedule seed helper ──────────────────────────────────────────────────

    private async Task<ApiTool.Backend.Data.Entities.Schedule> SeedScheduleAsync()
    {
        var schedule = new ApiTool.Backend.Data.Entities.Schedule
        {
            Id = Guid.NewGuid(),
            OrgId = _orgId,
            Name = $"test-sched-{Guid.NewGuid():N}"[..30],
            CronExpression = "0 2 * * *",
            CollectionRef = "smoke.yaml",
            Enabled = true,
            CreatedBy = _ownerId,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };
        _scope.Db.Schedules.Add(schedule);
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();
        return schedule;
    }

    // ── Fake helpers ──────────────────────────────────────────────────────────

    private sealed class FakeTimeProvider(DateTime initial) : TimeProvider
    {
        private DateTime _now = initial;

        public override DateTimeOffset GetUtcNow() => new DateTimeOffset(_now, TimeSpan.Zero);

        public void Advance(TimeSpan delta) => _now = _now.Add(delta);
    }
}
