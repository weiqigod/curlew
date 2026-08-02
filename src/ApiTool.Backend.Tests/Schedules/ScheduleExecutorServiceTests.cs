using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Results;
using ApiTool.Backend.Schedules;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>Unit tests for <see cref="ScheduleExecutorService"/> against in-memory SQLite.</summary>
public sealed class ScheduleExecutorServiceTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly FakeClock _clock;
    private readonly ScheduleExecutorService _svc;
    private readonly Guid _userId;
    private readonly Guid _orgId;

    public ScheduleExecutorServiceTests()
    {
        _scope = TestDb.CreateOpen();
        _scope.Db.Database.Migrate();

        _clock = new FakeClock(DateTimeOffset.UtcNow);

        var notifier = new NoopResultIngestedNotifier();
        var resultsSvc = new ResultsService(_scope.Db, _clock, notifier);

        _svc = new ScheduleExecutorService(
            _scope.Db, _clock, resultsSvc, new FakeScheduleEnvKeyProvider(), NullLogger<ScheduleExecutorService>.Instance);

        _userId = Guid.NewGuid();
        _orgId = Guid.NewGuid();

        _scope.Db.Users.Add(new User
        {
            Id = _userId,
            Email = $"owner-{_userId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        _scope.Db.Organizations.Add(new Organization
        {
            Id = _orgId,
            Name = "TestOrg",
            Slug = $"testorg-{_userId:N}"[..20],
            OwnerId = _userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        _scope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = _orgId,
            UserId = _userId,
            Role = OrgRole.Owner,
            PermissionsJson = "[]",
            JoinedAt = DateTime.UtcNow,
        });
        _scope.Db.SaveChanges();
        _scope.Db.ChangeTracker.Clear();
    }

    public async ValueTask DisposeAsync() => await _scope.DisposeAsync();

    // ── ClaimNextAsync ────────────────────────────────────────────────────────

    [Fact]
    public async Task ClaimNextAsync_returns_NoRunsAvailable_when_none_queued()
    {
        var (dto, error) = await _svc.ClaimNextAsync(_orgId, "w1", default);

        error.Should().Be(ScheduleClaimError.NoRunsAvailable);
        dto.Should().BeNull();
    }

    [Fact]
    public async Task ClaimNextAsync_returns_NoRunsAvailable_when_run_belongs_to_other_org()
    {
        // Seed a run for a different org
        var otherId = Guid.NewGuid();
        var schedule = await SeedScheduleAsync(orgId: otherId);
        await SeedQueuedRunAsync(schedule.Id);

        var (dto, error) = await _svc.ClaimNextAsync(_orgId, "w1", default);

        error.Should().Be(ScheduleClaimError.NoRunsAvailable, because: "run belongs to a different org");
        dto.Should().BeNull();
    }

    [Fact]
    public async Task ClaimNextAsync_claims_oldest_queued_run_first()
    {
        var schedule = await SeedScheduleAsync();
        var older = await SeedQueuedRunAsync(schedule.Id, createdOffset: TimeSpan.FromMinutes(-10));
        var newer = await SeedQueuedRunAsync(schedule.Id, createdOffset: TimeSpan.FromMinutes(-1));

        var (dto, error) = await _svc.ClaimNextAsync(_orgId, "w1", default);

        error.Should().Be(ScheduleClaimError.None);
        dto!.RunId.Should().Be(RunId.Format(older.Id), because: "oldest run must be claimed first");
    }

    [Fact]
    public async Task ClaimNextAsync_transitions_row_to_running_with_claim_token_and_deadline()
    {
        var schedule = await SeedScheduleAsync();
        var run = await SeedQueuedRunAsync(schedule.Id);

        var (dto, error) = await _svc.ClaimNextAsync(_orgId, "w1", default);

        error.Should().Be(ScheduleClaimError.None);
        dto.Should().NotBeNull();
        dto!.ClaimToken.Should().NotBeNullOrEmpty();
        Guid.TryParse(dto.ClaimToken, out _).Should().BeTrue(because: "claim token must be a valid UUID");
        var expectedDeadline = _clock.GetUtcNow().UtcDateTime.Add(ScheduleExecutorService.ClaimDeadlineDuration);
        dto.Deadline.Should().BeCloseTo(expectedDeadline, TimeSpan.FromSeconds(1),
            because: "deadline must be exactly now + ClaimDeadlineDuration per the task observable");

        _scope.Db.ChangeTracker.Clear();
        var updated = await _scope.Db.ScheduledRuns.FindAsync(run.Id);
        updated!.Status.Should().Be(ScheduledRunStatus.Running);
        updated.ClaimToken.Should().NotBeNull();
        updated.ClaimedByWorker.Should().Be("w1");
        updated.ClaimedAt.Should().NotBeNull();
        updated.StartedAt.Should().NotBeNull();
        updated.LastHeartbeatAt.Should().NotBeNull();
        updated.ClaimDeadline.Should().NotBeNull();
    }

    [Fact]
    public async Task ClaimNextAsync_second_call_returns_NoRunsAvailable_after_first_claims()
    {
        var schedule = await SeedScheduleAsync();
        await SeedQueuedRunAsync(schedule.Id);

        await _svc.ClaimNextAsync(_orgId, "w1", default);
        var (dto2, error2) = await _svc.ClaimNextAsync(_orgId, "w2", default);

        error2.Should().Be(ScheduleClaimError.NoRunsAvailable, because: "in-flight row must not be handed out twice");
        dto2.Should().BeNull();
    }

    [Fact]
    public async Task ClaimNextAsync_returns_collection_ref_from_schedule()
    {
        var schedule = await SeedScheduleAsync(collectionRef: "my-tests/smoke.yaml");
        await SeedQueuedRunAsync(schedule.Id);

        var (dto, error) = await _svc.ClaimNextAsync(_orgId, "w1", default);

        error.Should().Be(ScheduleClaimError.None);
        dto!.CollectionRef.Should().Be("my-tests/smoke.yaml");
    }

    // ── HeartbeatAsync ────────────────────────────────────────────────────────

    [Fact]
    public async Task HeartbeatAsync_updates_last_heartbeat_when_token_matches()
    {
        var schedule = await SeedScheduleAsync();
        var run = await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        Guid.TryParse(dto!.ClaimToken, out var token);

        _clock.Advance(TimeSpan.FromSeconds(10));
        RunId.TryParse(dto.RunId, out var runId);
        var error = await _svc.HeartbeatAsync(_orgId, runId, token, default);

        error.Should().Be(ScheduleClaimError.None);
        _scope.Db.ChangeTracker.Clear();
        var updated = await _scope.Db.ScheduledRuns.FindAsync(run.Id);
        updated!.LastHeartbeatAt.Should().BeCloseTo(_clock.GetUtcNow().UtcDateTime, TimeSpan.FromSeconds(1));
    }

    [Fact]
    public async Task HeartbeatAsync_returns_ClaimReaped_when_row_back_to_queued()
    {
        var schedule = await SeedScheduleAsync();
        var run = await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        Guid.TryParse(dto!.ClaimToken, out var token);
        RunId.TryParse(dto.RunId, out var runId);

        // Simulate reaper: reset to Queued and clear token
        _scope.Db.ChangeTracker.Clear();
        var fresh = await _scope.Db.ScheduledRuns.FindAsync(run.Id);
        fresh!.Status = ScheduledRunStatus.Queued;
        fresh.ClaimToken = null;
        fresh.ClaimedByWorker = null;
        fresh.ClaimedAt = null;
        fresh.LastHeartbeatAt = null;
        fresh.ClaimDeadline = null;
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        var error = await _svc.HeartbeatAsync(_orgId, runId, token, default);

        error.Should().Be(ScheduleClaimError.ClaimReaped, because: "row status is back to Queued with null token");
    }

    [Fact]
    public async Task HeartbeatAsync_returns_StaleClaim_when_token_mismatch()
    {
        var schedule = await SeedScheduleAsync();
        var run = await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        var wrongToken = Guid.NewGuid();
        var error = await _svc.HeartbeatAsync(_orgId, runId, wrongToken, default);

        error.Should().Be(ScheduleClaimError.StaleClaim, because: "token does not match the stored claim");
    }

    [Fact]
    public async Task HeartbeatAsync_returns_NotFound_when_run_not_in_org()
    {
        var error = await _svc.HeartbeatAsync(_orgId, Guid.NewGuid(), Guid.NewGuid(), default);

        error.Should().Be(ScheduleClaimError.NotFound);
    }

    // ── SubmitResultAsync ─────────────────────────────────────────────────────

    [Fact]
    public async Task SubmitResultAsync_persists_result_and_sets_result_id_on_completed()
    {
        var schedule = await SeedScheduleAsync();
        var run = await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        var req = new ScheduleResultRequest
        {
            ClaimToken = dto.ClaimToken,
            CollectionName = "smoke",
            RunAt = DateTime.UtcNow,
            DurationMs = 500,
            PassCount = 3,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };

        var error = await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);

        error.Should().Be(ScheduleClaimError.None);

        _scope.Db.ChangeTracker.Clear();
        var updated = await _scope.Db.ScheduledRuns.FindAsync(run.Id);
        updated!.Status.Should().Be(ScheduledRunStatus.Completed);
        updated.ResultId.Should().NotBeNull(because: "result FK must be set after successful submission");
        updated.CompletedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task SubmitResultAsync_transitions_to_failed_when_fail_count_gt_zero()
    {
        var schedule = await SeedScheduleAsync();
        var run = await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        var req = new ScheduleResultRequest
        {
            ClaimToken = dto.ClaimToken,
            CollectionName = "smoke",
            RunAt = DateTime.UtcNow,
            DurationMs = 500,
            PassCount = 2,
            FailCount = 1,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };

        var error = await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);

        error.Should().Be(ScheduleClaimError.None);
        _scope.Db.ChangeTracker.Clear();
        var updated = await _scope.Db.ScheduledRuns.FindAsync(run.Id);
        updated!.Status.Should().Be(ScheduledRunStatus.Failed);
        updated.FailureReason.Should().NotBeNullOrEmpty();
    }

    [Fact]
    public async Task SubmitResultAsync_returns_AlreadyCompleted_on_retry_with_different_token()
    {
        var schedule = await SeedScheduleAsync();
        await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        var req = new ScheduleResultRequest
        {
            ClaimToken = dto.ClaimToken,
            CollectionName = "smoke",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };

        await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);

        // Retry with a DIFFERENT token — must still be 409 AlreadyCompleted
        var req2 = new ScheduleResultRequest
        {
            ClaimToken = Guid.NewGuid().ToString(),
            CollectionName = req.CollectionName,
            RunAt = req.RunAt,
            DurationMs = req.DurationMs,
            PassCount = req.PassCount,
            FailCount = req.FailCount,
            SkippedCount = req.SkippedCount,
            TriggeredBy = req.TriggeredBy,
            Items = req.Items,
        };
        var error = await _svc.SubmitResultAsync(_userId, _orgId, runId, req2, default);

        error.Should().Be(ScheduleClaimError.AlreadyCompleted);
    }

    [Fact]
    public async Task SubmitResultAsync_idempotent_retry_with_same_claim_token_returns_None_and_does_not_double_insert()
    {
        var schedule = await SeedScheduleAsync();
        await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        var req = new ScheduleResultRequest
        {
            ClaimToken = dto.ClaimToken,
            CollectionName = "smoke-idem",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };

        // First post — succeeds
        var first = await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);
        first.Should().Be(ScheduleClaimError.None);

        _scope.Db.ChangeTracker.Clear();
        var afterFirst = await _scope.Db.ScheduledRuns.FindAsync((object)runId);
        var firstResultId = afterFirst!.ResultId;

        // Second post — same claim_token (network retry)
        var second = await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);

        second.Should().Be(ScheduleClaimError.None,
            because: "same claim_token retry must return None (200) — not AlreadyCompleted (409)");

        // Exactly one results row for this collection_name
        _scope.Db.ChangeTracker.Clear();
        var resultCount = _scope.Db.Results.Count(r => r.CollectionName == "smoke-idem" && r.OrgId == _orgId);
        resultCount.Should().Be(1,
            because: "idempotent retry must not insert a duplicate results row");

        // result_id on the run unchanged
        var afterSecond = await _scope.Db.ScheduledRuns.FindAsync((object)runId);
        afterSecond!.ResultId.Should().Be(firstResultId,
            because: "result_id must not change on an idempotent retry");
    }

    [Fact]
    public async Task SubmitResultAsync_terminal_row_with_DIFFERENT_token_still_returns_AlreadyCompleted()
    {
        var schedule = await SeedScheduleAsync();
        await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        var req = new ScheduleResultRequest
        {
            ClaimToken = dto.ClaimToken,
            CollectionName = "smoke-diff-token",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };
        await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);

        // Post again with a DIFFERENT token (adversarial case — different worker)
        var adversarialReq = new ScheduleResultRequest
        {
            ClaimToken = Guid.NewGuid().ToString(),
            CollectionName = req.CollectionName,
            RunAt = req.RunAt,
            DurationMs = req.DurationMs,
            PassCount = req.PassCount,
            FailCount = req.FailCount,
            SkippedCount = req.SkippedCount,
            TriggeredBy = req.TriggeredBy,
            Items = req.Items,
        };
        var error = await _svc.SubmitResultAsync(_userId, _orgId, runId, adversarialReq, default);

        error.Should().Be(ScheduleClaimError.AlreadyCompleted,
            because: "a terminal row with a mismatched claim_token must not be treated as an idempotent retry");
    }

    [Fact]
    public async Task SubmitResultAsync_returns_StaleClaim_on_token_mismatch()
    {
        var schedule = await SeedScheduleAsync();
        await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        var req = new ScheduleResultRequest
        {
            ClaimToken = Guid.NewGuid().ToString(),   // wrong token
            CollectionName = "smoke",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };

        var error = await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);

        error.Should().Be(ScheduleClaimError.StaleClaim);
    }

    [Fact]
    public async Task SubmitResultAsync_returns_ClaimReaped_after_reap()
    {
        var schedule = await SeedScheduleAsync();
        var run = await SeedQueuedRunAsync(schedule.Id);
        var (dto, _) = await _svc.ClaimNextAsync(_orgId, "w1", default);
        RunId.TryParse(dto!.RunId, out var runId);

        // Simulate reaper
        _scope.Db.ChangeTracker.Clear();
        var fresh = await _scope.Db.ScheduledRuns.FindAsync(run.Id);
        fresh!.Status = ScheduledRunStatus.Queued;
        fresh.ClaimToken = null;
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();

        var req = new ScheduleResultRequest
        {
            ClaimToken = dto.ClaimToken,
            CollectionName = "smoke",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };

        var error = await _svc.SubmitResultAsync(_userId, _orgId, runId, req, default);

        error.Should().Be(ScheduleClaimError.ClaimReaped);
    }

    [Theory]
    [InlineData(null)]
    [InlineData("not-a-guid")]
    [InlineData("")]
    public async Task SubmitResultAsync_returns_InvalidRequest_when_claim_token_missing_or_malformed(string? token)
    {
        var req = new ScheduleResultRequest
        {
            ClaimToken = token,
            CollectionName = "smoke",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = null,
        };

        var error = await _svc.SubmitResultAsync(_userId, _orgId, Guid.NewGuid(), req, default);

        error.Should().Be(ScheduleClaimError.InvalidRequest);
    }

    // ── Seed helpers ──────────────────────────────────────────────────────────

    private async Task<Schedule> SeedScheduleAsync(
        Guid? orgId = null,
        string collectionRef = "smoke.yaml")
    {
        var targetOrgId = orgId ?? _orgId;
        var createdBy = _userId;

        if (orgId.HasValue && orgId != _orgId)
        {
            // Need a separate user/org for the foreign-org case.
            var otherId = Guid.NewGuid();
            createdBy = otherId;
            _scope.Db.Users.Add(new User { Id = otherId, Email = $"other-{otherId:N}@t.com", CreatedAt = DateTime.UtcNow });
            _scope.Db.Organizations.Add(new Organization
            {
                Id = targetOrgId,
                Name = $"Other-{targetOrgId:N}"[..20],
                Slug = $"other-{targetOrgId:N}"[..20],
                OwnerId = otherId,
                Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            await _scope.Db.SaveChangesAsync();
            _scope.Db.ChangeTracker.Clear();
        }

        var schedule = new Schedule
        {
            Id = Guid.NewGuid(),
            OrgId = targetOrgId,
            Name = $"sched-{Guid.NewGuid():N}"[..30],
            CronExpression = "0 2 * * *",
            CollectionRef = collectionRef,
            Enabled = true,
            CreatedBy = createdBy,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };
        _scope.Db.Schedules.Add(schedule);
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();
        return schedule;
    }

    private async Task<ScheduledRun> SeedQueuedRunAsync(Guid scheduleId, TimeSpan? createdOffset = null)
    {
        var now = DateTime.UtcNow + (createdOffset ?? TimeSpan.Zero);
        var run = new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = scheduleId,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = now,
        };
        _scope.Db.ScheduledRuns.Add(run);
        await _scope.Db.SaveChangesAsync();
        _scope.Db.ChangeTracker.Clear();
        return run;
    }
}
