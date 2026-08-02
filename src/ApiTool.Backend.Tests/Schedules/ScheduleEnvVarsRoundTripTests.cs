// Round-trip integration tests for schedule env_vars encryption (M18-009).
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Results;
using ApiTool.Backend.Schedules;
using ApiTool.Backend.Schedules.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>
/// Integration tests for the schedule env_vars encryption round-trip (M18-009, v4-12).
/// Uses an in-memory SQLite database to exercise the full create→persist→claim path.
/// </summary>
public sealed class ScheduleEnvVarsRoundTripTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly FakeScheduleEnvKeyProvider _envProvider;
    private readonly SchedulesService _schedulesSvc;
    private readonly ScheduleExecutorService _executorSvc;
    private readonly Guid _userId;
    private readonly Guid _orgId;

    public ScheduleEnvVarsRoundTripTests()
    {
        _scope = TestDb.CreateOpen();
        _scope.Db.Database.Migrate();
        _envProvider = new FakeScheduleEnvKeyProvider();

        _userId = Guid.NewGuid();
        _orgId = Guid.NewGuid();

        _scope.Db.Users.Add(new User { Id = _userId, Email = $"owner-{_userId:N}@test.com", CreatedAt = DateTime.UtcNow });
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
        _scope.Db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = _orgId,
            Tier = SubscriptionTier.Team,
            Status = SubscriptionStatus.Active,
            Interval = "month",
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        _scope.Db.SaveChanges();

        var clock = new FakeClock(DateTimeOffset.UtcNow);
        var notifier = new NoopResultIngestedNotifier();
        var resultsSvc = new ResultsService(_scope.Db, clock, notifier);

        _schedulesSvc = new SchedulesService(_scope.Db, clock, _envProvider,
            NullLogger<SchedulesService>.Instance);
        _executorSvc = new ScheduleExecutorService(_scope.Db, clock, resultsSvc,
            _envProvider, NullLogger<ScheduleExecutorService>.Instance);
    }

    public ValueTask DisposeAsync() => _scope.DisposeAsync();

    [Fact]
    public async Task Create_schedule_with_env_vars_then_claim_next_returns_those_env_vars()
    {
        var envVars = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["API_KEY"] = "secret-value",
            ["REGION"] = "us-east-1",
        };

        var (dto, err, _) = await _schedulesSvc.CreateAsync(_userId, _orgId, new CreateScheduleRequest(
            Name: "nightly-smoke",
            Cron: "0 2 * * *",
            CollectionRef: "smoke.yaml",
            EnvVars: envVars), default);

        err.Should().Be(ScheduleError.None);
        dto.Should().NotBeNull();

        // Seed a queued run directly — EnqueueDueAsync skips schedules whose NextRunAt is in the future.
        var schedule = await _scope.Db.Schedules.SingleAsync(x => x.OrgId == _orgId);
        _scope.Db.ScheduledRuns.Add(new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = schedule.Id,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        });
        await _scope.Db.SaveChangesAsync();

        // Claim the run — env_vars should be decrypted and returned
        var (runResponse, claimErr) = await _executorSvc.ClaimNextAsync(_orgId, "worker-1", default);
        claimErr.Should().Be(ScheduleClaimError.None);
        runResponse.Should().NotBeNull();
        runResponse!.EnvVars.Should().ContainKey("API_KEY").WhoseValue.Should().Be("secret-value");
        runResponse.EnvVars.Should().ContainKey("REGION").WhoseValue.Should().Be("us-east-1");
    }

    [Fact]
    public async Task Persisted_env_vars_ciphertext_does_not_contain_cleartext_bytes()
    {
        var envVars = new Dictionary<string, string>(StringComparer.Ordinal) { ["SECRET"] = "my-secret-value" };

        await _schedulesSvc.CreateAsync(_userId, _orgId, new CreateScheduleRequest(
            Name: "secret-schedule",
            Cron: "0 3 * * *",
            CollectionRef: "tests.yaml",
            EnvVars: envVars), default);

        var row = await _scope.Db.Schedules.SingleAsync(x => x.OrgId == _orgId);
        row.EnvVarsCiphertext.Should().NotBeNull();
        row.EnvVarsCiphertext!.Length.Should().BeGreaterThan(0);
        row.EnvVarsKid.Should().Be(FakeScheduleEnvKeyProvider.FakeKid);
    }

    [Fact]
    public async Task Schedule_with_null_env_vars_returns_empty_dict_on_claim()
    {
        await _schedulesSvc.CreateAsync(_userId, _orgId, new CreateScheduleRequest(
            Name: "no-env-schedule",
            Cron: "0 4 * * *",
            CollectionRef: "simple.yaml",
            EnvVars: null), default);

        // Seed a queued run directly — EnqueueDueAsync skips schedules whose NextRunAt is in the future.
        var schedule = await _scope.Db.Schedules.SingleAsync(x => x.OrgId == _orgId);
        _scope.Db.ScheduledRuns.Add(new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = schedule.Id,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        });
        await _scope.Db.SaveChangesAsync();

        var (runResponse, claimErr) = await _executorSvc.ClaimNextAsync(_orgId, "worker-2", default);
        claimErr.Should().Be(ScheduleClaimError.None);
        runResponse.Should().NotBeNull();
        runResponse!.EnvVars.Should().BeEmpty();
    }
}
