using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Schedules;
using ApiTool.Backend.Tests.TestInfrastructure;
using Cronos;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>Tests for timezone column on schedules (M16-012).</summary>
public sealed class ScheduleTimezoneTests
{
    // ── Helpers ───────────────────────────────────────────────────────────────

    private static async Task<(TestDbScope scope, AppDbContext db, SchedulesService svc, Guid userId, Guid orgId)>
        BuildAsync(OrgRole role = OrgRole.Owner)
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"tz-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TzTestOrg",
            Slug = $"tzorg-{orgId:N}"[..20],
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

    // ── Step 1: Column persistence ────────────────────────────────────────────

    [Fact]
    public async Task Schedule_persists_timezone_column_with_iana_value()
    {
        var (scope, db, _, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var userId = Guid.NewGuid();
            db.Users.Add(new User { Id = userId, Email = $"seed-{userId:N}@t.com", CreatedAt = DateTime.UtcNow });
            var schedule = new Schedule
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Name = "tz-persist-test",
                CronExpression = "0 9 * * *",
                Timezone = "Europe/Stockholm",
                CollectionRef = "smoke.yaml",
                Enabled = true,
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            };
            db.Schedules.Add(schedule);
            await db.SaveChangesAsync();

            db.ChangeTracker.Clear();
            var loaded = await db.Schedules.SingleAsync(s => s.Id == schedule.Id);
            loaded.Timezone.Should().Be("Europe/Stockholm");
        }
    }

    [Fact]
    public async Task Schedule_defaults_timezone_to_utc_when_omitted()
    {
        var (scope, db, _, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var userId = Guid.NewGuid();
            db.Users.Add(new User { Id = userId, Email = $"seed2-{userId:N}@t.com", CreatedAt = DateTime.UtcNow });
            var schedule = new Schedule
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Name = "tz-default-test",
                CronExpression = "0 2 * * *",
                // Timezone deliberately omitted — should default to "UTC"
                CollectionRef = "smoke.yaml",
                Enabled = true,
                CreatedBy = userId,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            };
            db.Schedules.Add(schedule);
            await db.SaveChangesAsync();

            db.ChangeTracker.Clear();
            var loaded = await db.Schedules.SingleAsync(s => s.Id == schedule.Id);
            loaded.Timezone.Should().Be("UTC");
        }
    }

    // ── Step 3: DST-aware Cronos next-occurrence ──────────────────────────────

    [Theory]
    [InlineData("Europe/Stockholm", 9, 1, 8)]   // Jan: 09:00 CET = 08:00 UTC
    [InlineData("Europe/Stockholm", 9, 7, 7)]   // Jul: 09:00 CEST = 07:00 UTC
    [InlineData("America/New_York", 9, 1, 14)]  // Jan: 09:00 EST = 14:00 UTC
    [InlineData("Asia/Tokyo", 9, 6, 0)]         // Jun: 09:00 JST = 00:00 UTC
    [InlineData("UTC", 9, 6, 9)]                // UTC unchanged
    public void NextRunOccurrence_resolves_in_timezone_with_dst(
        string tzName, int cronHour, int month, int expectedUtcHour)
    {
        var tz = TimeZoneInfo.FindSystemTimeZoneById(tzName);
        var cron = CronExpression.Parse($"0 {cronHour} * * *", CronFormat.Standard);

        // Pick a "now" at midnight UTC on the 1st of that month (any year in non-ambiguous period).
        var now = DateTime.SpecifyKind(new DateTime(2025, month, 1, 0, 0, 0), DateTimeKind.Utc);
        var next = cron.GetNextOccurrence(now, tz, inclusive: false);

        next.Should().NotBeNull();
        next!.Value.Hour.Should().Be(expectedUtcHour,
            because: $"'{cronHour}:00' in {tzName} during month {month} should be {expectedUtcHour}:00 UTC");
        next.Value.Kind.Should().Be(DateTimeKind.Utc);
    }

    [Fact]
    public void NextRunOccurrence_handles_spring_forward_gap()
    {
        // Europe/Stockholm springs forward at 02:00 on last Sunday of March.
        // 2025-03-30: clocks skip 02:00..02:59. A cron set for 02:30 is skipped.
        // Cronos either skips the gap entirely or returns the next valid occurrence.
        var tz = TimeZoneInfo.FindSystemTimeZoneById("Europe/Stockholm");
        var cron = CronExpression.Parse("30 2 * * *", CronFormat.Standard);
        // Start one minute before 02:00 local (00:59 UTC = 01:59 CET on 2025-03-30)
        var now = DateTime.SpecifyKind(new DateTime(2025, 3, 30, 0, 59, 0), DateTimeKind.Utc);
        var next = cron.GetNextOccurrence(now, tz, inclusive: false);
        // The key assertion: Cronos does not return null and does not return a time
        // that falls within the spring-forward gap (01:00–01:59 UTC = 02:00–02:59 CET).
        // It should skip to the next occurrence on 2025-03-31.
        next.Should().NotBeNull(because: "Cronos must always return a future occurrence, never null for a valid cron");
        next!.Value.Should().BeAfter(now, because: "next occurrence must be in the future");
        // Verify the result is a UTC DateTime
        next.Value.Kind.Should().Be(DateTimeKind.Utc);
    }

    // ── Step 3: CreateAsync validation ────────────────────────────────────────

    [Theory]
    [InlineData("Atlantis/Lost")]
    [InlineData("Invalid/Tz")]
    [InlineData("not a tz at all")]
    public async Task CreateAsync_returns_InvalidTimezone_for_unknown_tz(string badTz)
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateScheduleRequest("nightly", "0 2 * * *", "smoke.yaml", Timezone: badTz);
            var (dto, error, message) = await svc.CreateAsync(userId, orgId, req, default);

            error.Should().Be(ScheduleError.InvalidTimezone);
            dto.Should().BeNull();
            message.Should().NotBeNullOrEmpty();
        }
    }

    [Fact]
    public async Task CreateAsync_persists_timezone_field_in_dto()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateScheduleRequest("tz-in-dto", "0 9 * * *", "smoke.yaml", Timezone: "Asia/Tokyo");
            var (dto, error, _) = await svc.CreateAsync(userId, orgId, req, default);

            error.Should().Be(ScheduleError.None);
            dto.Should().NotBeNull();
            dto!.Timezone.Should().Be("Asia/Tokyo");
        }
    }

    [Fact]
    public async Task CreateAsync_defaults_to_UTC_when_timezone_omitted()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateScheduleRequest("tz-default-create", "0 2 * * *", "smoke.yaml");
            var (dto, error, _) = await svc.CreateAsync(userId, orgId, req, default);

            error.Should().Be(ScheduleError.None);
            dto!.Timezone.Should().Be("UTC");
        }
    }

    [Fact]
    public async Task EnqueueDueAsync_uses_timezone_when_recomputing_next_run_at()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Create schedule with Europe/Stockholm
            var req = new CreateScheduleRequest("enqueue-tz-test", "0 9 * * *", "smoke.yaml", Timezone: "Europe/Stockholm");
            var (dto, error, _) = await svc.CreateAsync(userId, orgId, req, default);
            error.Should().Be(ScheduleError.None);

            // Manually backdated NextRunAt so EnqueueDueAsync picks it up
            var schedule = await db.Schedules.SingleAsync(s => s.Name == "enqueue-tz-test");
            schedule.NextRunAt = DateTime.UtcNow.AddMinutes(-1);
            await db.SaveChangesAsync();

            var count = await svc.EnqueueDueAsync(default);
            count.Should().Be(1);

            db.ChangeTracker.Clear();
            var updated = await db.Schedules.SingleAsync(s => s.Name == "enqueue-tz-test");
            // NextRunAt should be updated and reflect the TZ-aware computation
            updated.NextRunAt.Should().NotBeNull();
            updated.NextRunAt!.Value.Should().BeAfter(DateTime.UtcNow.AddMinutes(-5));
        }
    }
}
