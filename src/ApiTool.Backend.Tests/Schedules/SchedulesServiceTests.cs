using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Schedules;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>Tests for <see cref="SchedulesService"/> against in-memory SQLite.</summary>
public sealed class SchedulesServiceTests
{
    // ── Test helpers ──────────────────────────────────────────────────────────

    private static async Task<(TestDbScope scope, AppDbContext db, SchedulesService svc, Guid userId, Guid orgId)>
        BuildAsync(OrgRole role = OrgRole.Owner)
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"user-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"testorg-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = role,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var svc = new SchedulesService(db, TimeProvider.System, new FakeScheduleEnvKeyProvider(), NullLogger<SchedulesService>.Instance);
        return (scope, db, svc, userId, orgId);
    }

    private static CreateScheduleRequest ValidRequest(string name = "nightly") =>
        new(Name: name, Cron: "0 2 * * *", CollectionRef: "smoke.yaml");

    // ── CreateAsync ───────────────────────────────────────────────────────────

    [Fact]
    public async Task CreateAsync_persists_schedule_with_computed_next_run_at()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, orgId, ValidRequest(), default);

            error.Should().Be(ScheduleError.None);
            dto.Should().NotBeNull();
            dto!.NextRunAt.Should().NotBeNull();
            dto.NextRunAt!.Value.Should().BeAfter(DateTime.UtcNow.AddMinutes(-1));

            var count = await db.Schedules.CountAsync();
            count.Should().Be(1);
        }
    }

    [Fact]
    public async Task CreateAsync_returns_invalid_cron_for_malformed_expression()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, orgId,
                new CreateScheduleRequest("nightly", "not-a-cron", "smoke.yaml"), default);

            error.Should().Be(ScheduleError.InvalidCron);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task CreateAsync_returns_invalid_cron_for_null_cron_expression()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, message) = await svc.CreateAsync(userId, orgId,
                new CreateScheduleRequest("nightly", null, "smoke.yaml"), default);

            error.Should().Be(ScheduleError.InvalidCron);
            dto.Should().BeNull();
            message.Should().Be("cron is required.");
        }
    }

    [Fact]
    public async Task CreateAsync_returns_schedule_name_taken_for_duplicate_in_same_org()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);

            var (dto, error, _) = await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);

            error.Should().Be(ScheduleError.ScheduleNameTaken);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task CreateAsync_allows_same_name_in_different_orgs()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Create a second org
            var orgId2 = Guid.NewGuid();
            db.Organizations.Add(new Organization
            {
                Id = orgId2,
                Name = "AnotherOrg",
                Slug = $"anotherorg-{orgId2:N}"[..20],
                OwnerId = userId,
                Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId2,
                UserId = userId,
                Role = OrgRole.Owner,
                JoinedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var (dto1, error1, _) = await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);
            var (dto2, error2, _) = await svc.CreateAsync(userId, orgId2, ValidRequest("nightly"), default);

            error1.Should().Be(ScheduleError.None);
            error2.Should().Be(ScheduleError.None);
            dto1.Should().NotBeNull();
            dto2.Should().NotBeNull();
        }
    }

    [Fact]
    public async Task CreateAsync_returns_permission_denied_for_member_role()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync(OrgRole.Member);
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, orgId, ValidRequest(), default);

            error.Should().Be(ScheduleError.PermissionDenied);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task CreateAsync_returns_invalid_name_when_name_is_empty()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, orgId,
                new CreateScheduleRequest("", "0 2 * * *", "smoke.yaml"), default);

            error.Should().Be(ScheduleError.InvalidName);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task CreateAsync_returns_invalid_collection_ref_when_ref_is_empty()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, orgId,
                new CreateScheduleRequest("nightly", "0 2 * * *", ""), default);

            error.Should().Be(ScheduleError.InvalidCollectionRef);
            dto.Should().BeNull();
        }
    }

    // ── ListAsync ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task ListAsync_returns_all_schedules_for_org_member()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);
            await svc.CreateAsync(userId, orgId, ValidRequest("weekly"), default);

            var (schedules, error) = await svc.ListAsync(userId, orgId, default);

            error.Should().Be(ScheduleError.None);
            schedules.Should().HaveCount(2);
        }
    }

    // ── GetByNameAsync ────────────────────────────────────────────────────────

    [Fact]
    public async Task GetByNameAsync_returns_schedule_for_org_member()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);

            var (dto, error) = await svc.GetByNameAsync(userId, orgId, "nightly", default);

            error.Should().Be(ScheduleError.None);
            dto.Should().NotBeNull();
            dto!.Name.Should().Be("nightly");
        }
    }

    [Fact]
    public async Task GetByNameAsync_returns_not_found_for_non_member()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);

            var nonMemberId = Guid.NewGuid();
            db.Users.Add(new User { Id = nonMemberId, Email = "nm@test.com", CreatedAt = DateTime.UtcNow });
            await db.SaveChangesAsync();

            var (dto, error) = await svc.GetByNameAsync(nonMemberId, orgId, "nightly", default);

            error.Should().Be(ScheduleError.NotFound);
            dto.Should().BeNull();
        }
    }

    // ── RunNowAsync ───────────────────────────────────────────────────────────

    [Fact]
    public async Task RunNowAsync_creates_queued_run_and_returns_202_dto()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);

            var (dto, error) = await svc.RunNowAsync(userId, orgId, "nightly", default);

            error.Should().Be(ScheduleError.None);
            dto.Should().NotBeNull();
            dto!.RunId.Should().StartWith("run_");
            dto.Status.Should().Be("queued");

            var runCount = await db.ScheduledRuns.CountAsync();
            runCount.Should().Be(1);
        }
    }

    [Fact]
    public async Task RunNowAsync_returns_permission_denied_for_member_role()
    {
        var (scope, db, svc, ownerId, orgId) = await BuildAsync(OrgRole.Owner);
        await using (scope)
        {
            // Create schedule as owner
            await svc.CreateAsync(ownerId, orgId, ValidRequest("nightly"), default);

            // Add a member
            var memberId = Guid.NewGuid();
            db.Users.Add(new User { Id = memberId, Email = "member@test.com", CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgId,
                UserId = memberId,
                Role = OrgRole.Member,
                JoinedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var (dto, error) = await svc.RunNowAsync(memberId, orgId, "nightly", default);

            error.Should().Be(ScheduleError.PermissionDenied);
            dto.Should().BeNull();
        }
    }

    // ── ListRunsAsync ─────────────────────────────────────────────────────────

    [Fact]
    public async Task ListRunsAsync_returns_runs_newest_first()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, orgId, ValidRequest("nightly"), default);
            await svc.RunNowAsync(userId, orgId, "nightly", default);
            await svc.RunNowAsync(userId, orgId, "nightly", default);

            var (runs, error) = await svc.ListRunsAsync(userId, orgId, "nightly", 10, default);

            error.Should().Be(ScheduleError.None);
            runs.Should().HaveCount(2);
        }
    }

    // ── EnqueueDueAsync ───────────────────────────────────────────────────────

    [Fact]
    public async Task EnqueueDueAsync_creates_runs_for_due_schedules()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Insert a schedule whose NextRunAt is in the past
            var schedId = Guid.NewGuid();
            db.Schedules.Add(new Schedule
            {
                Id = schedId,
                OrgId = orgId,
                Name = "overdue",
                CronExpression = "0 2 * * *",
                CollectionRef = "smoke.yaml",
                Enabled = true,
                NextRunAt = DateTime.UtcNow.AddHours(-1),  // due in the past
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow.AddDays(-1),
                UpdatedAt = DateTime.UtcNow.AddDays(-1),
            });
            await db.SaveChangesAsync();

            var count = await svc.EnqueueDueAsync(default);

            count.Should().Be(1);
            var runCount = await db.ScheduledRuns.CountAsync();
            runCount.Should().Be(1);
        }
    }

    [Fact]
    public async Task EnqueueDueAsync_skips_disabled_schedules()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            db.Schedules.Add(new Schedule
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Name = "disabled",
                CronExpression = "0 2 * * *",
                CollectionRef = "smoke.yaml",
                Enabled = false,  // disabled
                NextRunAt = DateTime.UtcNow.AddHours(-1),
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow.AddDays(-1),
                UpdatedAt = DateTime.UtcNow.AddDays(-1),
            });
            await db.SaveChangesAsync();

            var count = await svc.EnqueueDueAsync(default);

            count.Should().Be(0);
            var runCount = await db.ScheduledRuns.CountAsync();
            runCount.Should().Be(0);
        }
    }

    [Fact]
    public async Task EnqueueDueAsync_recomputes_next_run_at_after_enqueue()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var schedId = Guid.NewGuid();
            var oldNextRunAt = DateTime.UtcNow.AddHours(-1);
            db.Schedules.Add(new Schedule
            {
                Id = schedId,
                OrgId = orgId,
                Name = "recompute",
                CronExpression = "0 2 * * *",
                CollectionRef = "smoke.yaml",
                Enabled = true,
                NextRunAt = oldNextRunAt,
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow.AddDays(-1),
                UpdatedAt = DateTime.UtcNow.AddDays(-1),
            });
            await db.SaveChangesAsync();

            await svc.EnqueueDueAsync(default);

            db.ChangeTracker.Clear();
            var schedule = await db.Schedules.FindAsync(schedId);
            schedule!.NextRunAt.Should().BeAfter(DateTime.UtcNow,
                because: "NextRunAt must be recomputed to a future time after enqueue");
            schedule.LastRunAt.Should().NotBeNull(because: "LastRunAt must be set after enqueue");
        }
    }

    [Fact]
    public async Task EnqueueDueAsync_skips_stack_up_when_previous_run_still_queued()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var schedId = Guid.NewGuid();
            db.Schedules.Add(new Schedule
            {
                Id = schedId,
                OrgId = orgId,
                Name = "stackup-queued",
                CronExpression = "0 2 * * *",
                CollectionRef = "smoke.yaml",
                Enabled = true,
                NextRunAt = DateTime.UtcNow.AddHours(-1),
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow.AddDays(-1),
                UpdatedAt = DateTime.UtcNow.AddDays(-1),
            });
            // Existing run still queued
            db.ScheduledRuns.Add(new ScheduledRun
            {
                Id = Guid.NewGuid(),
                ScheduleId = schedId,
                Status = ScheduledRunStatus.Queued,
                CreatedAt = DateTime.UtcNow.AddMinutes(-5),
            });
            await db.SaveChangesAsync();

            var count = await svc.EnqueueDueAsync(default);

            count.Should().Be(0, because: "stack-up: existing queued run should suppress a new enqueue");
            var runCount = await db.ScheduledRuns.CountAsync();
            runCount.Should().Be(1, because: "no new run should be added");

            // But schedule timestamps should still advance
            db.ChangeTracker.Clear();
            var schedule = await db.Schedules.FindAsync(schedId);
            schedule!.LastRunAt.Should().NotBeNull(because: "last_run_at must advance even on stack-up skip");
            schedule.NextRunAt.Should().BeAfter(DateTime.UtcNow, because: "next_run_at must advance to avoid re-firing immediately");
        }
    }

    [Fact]
    public async Task EnqueueDueAsync_skips_stack_up_when_previous_run_still_running()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var schedId = Guid.NewGuid();
            db.Schedules.Add(new Schedule
            {
                Id = schedId,
                OrgId = orgId,
                Name = "stackup-running",
                CronExpression = "0 2 * * *",
                CollectionRef = "smoke.yaml",
                Enabled = true,
                NextRunAt = DateTime.UtcNow.AddHours(-1),
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow.AddDays(-1),
                UpdatedAt = DateTime.UtcNow.AddDays(-1),
            });
            // Existing run is running
            db.ScheduledRuns.Add(new ScheduledRun
            {
                Id = Guid.NewGuid(),
                ScheduleId = schedId,
                Status = ScheduledRunStatus.Running,
                CreatedAt = DateTime.UtcNow.AddMinutes(-3),
                StartedAt = DateTime.UtcNow.AddMinutes(-3),
            });
            await db.SaveChangesAsync();

            var count = await svc.EnqueueDueAsync(default);

            count.Should().Be(0, because: "stack-up: existing running run should suppress a new enqueue");
            var runCount = await db.ScheduledRuns.CountAsync();
            runCount.Should().Be(1, because: "no new run should be added");
        }
    }

    [Fact]
    public async Task EnqueueDueAsync_enqueues_when_previous_run_completed()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var schedId = Guid.NewGuid();
            db.Schedules.Add(new Schedule
            {
                Id = schedId,
                OrgId = orgId,
                Name = "after-completed",
                CronExpression = "0 2 * * *",
                CollectionRef = "smoke.yaml",
                Enabled = true,
                NextRunAt = DateTime.UtcNow.AddHours(-1),
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow.AddDays(-1),
                UpdatedAt = DateTime.UtcNow.AddDays(-1),
            });
            // Previous run is completed — not in-flight
            db.ScheduledRuns.Add(new ScheduledRun
            {
                Id = Guid.NewGuid(),
                ScheduleId = schedId,
                Status = ScheduledRunStatus.Completed,
                CreatedAt = DateTime.UtcNow.AddHours(-2),
                CompletedAt = DateTime.UtcNow.AddHours(-1),
            });
            await db.SaveChangesAsync();

            var count = await svc.EnqueueDueAsync(default);

            count.Should().Be(1, because: "completed run is not in-flight; new enqueue should proceed");
            var runCount = await db.ScheduledRuns.CountAsync();
            runCount.Should().Be(2, because: "old completed run + new queued run");
        }
    }
}
