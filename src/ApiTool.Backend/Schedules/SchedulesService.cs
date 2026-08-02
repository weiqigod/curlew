using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results;
using ApiTool.Backend.Schedules.Keys;
using Cronos;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Schedules;

/// <summary>
/// Business logic for creating, listing, and running cron-scheduled test runs.
/// Schedules are scoped to organizations and gated by the RBAC role model.
/// Write operations (create, run-now) require Owner or Admin role;
/// read operations require any org membership.
/// </summary>
public sealed class SchedulesService(
    AppDbContext db,
    TimeProvider clock,
    IScheduleEnvKeyProvider envKeyProvider,
    ILogger<SchedulesService> logger) : ISchedulerEnqueuer
{
    /// <summary>
    /// Creates a new cron schedule for the given organization.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="request">Schedule creation payload.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// Tuple of (dto, error, message). On success, error is <see cref="ScheduleError.None"/>.
    /// </returns>
    public async Task<(ScheduleDto? dto, ScheduleError error, string? message)>
        CreateAsync(Guid userId, Guid orgId, CreateScheduleRequest? request, CancellationToken ct)
    {
        if (!await IsAdminAsync(userId, orgId, ct))
            return (null, ScheduleError.PermissionDenied, "Permission denied.");

        if (string.IsNullOrWhiteSpace(request?.Name))
            return (null, ScheduleError.InvalidName, "name is required.");

        if (string.IsNullOrWhiteSpace(request.CollectionRef))
            return (null, ScheduleError.InvalidCollectionRef, "collection_ref is required.");

        if (string.IsNullOrWhiteSpace(request.Cron))
            return (null, ScheduleError.InvalidCron, "cron is required.");

        CronExpression parsedCron;
        try
        {
            parsedCron = CronExpression.Parse(request.Cron, CronFormat.Standard);
        }
        catch (CronFormatException ex)
        {
            return (null, ScheduleError.InvalidCron, $"Invalid cron expression: {ex.Message}");
        }

        var tzName = string.IsNullOrWhiteSpace(request.Timezone) ? "UTC" : request.Timezone!;
        TimeZoneInfo tz;
        try
        {
            tz = TimeZoneInfo.FindSystemTimeZoneById(tzName);
        }
        catch (TimeZoneNotFoundException)
        {
            return (null, ScheduleError.InvalidTimezone, $"Unknown IANA timezone: {tzName}");
        }
        catch (InvalidTimeZoneException)
        {
            return (null, ScheduleError.InvalidTimezone, $"Invalid IANA timezone: {tzName}");
        }

        var alreadyExists = await db.Schedules.AnyAsync(
            s => s.OrgId == orgId && s.Name == request.Name, ct);
        if (alreadyExists)
            return (null, ScheduleError.ScheduleNameTaken, $"A schedule named '{request.Name}' already exists.");

        var now = DateTime.SpecifyKind(clock.GetUtcNow().UtcDateTime, DateTimeKind.Utc);
        var nextRunAt = parsedCron.GetNextOccurrence(now, tz, inclusive: false);

        // Encrypt env_vars if provided (M18-009).
        byte[]? envVarsCiphertext = null;
        string? envVarsKid = null;
        if (request.EnvVars is { Count: > 0 } envVarsDict)
        {
            var envJson = JsonSerializer.SerializeToUtf8Bytes(envVarsDict);
            var encrypted = await envKeyProvider.EncryptAsync(envJson, ct);
            envVarsCiphertext = encrypted.Ciphertext;
            envVarsKid = encrypted.Kid;
        }

        var schedule = new Schedule
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Name = request.Name,
            CronExpression = request.Cron!,
            Timezone = tzName,
            CollectionRef = request.CollectionRef,
            Enabled = true,
            NextRunAt = nextRunAt,
            CreatedBy = userId,
            CreatedAt = now,
            UpdatedAt = now,
            EnvVarsCiphertext = envVarsCiphertext,
            EnvVarsKid = envVarsKid,
        };

        db.Schedules.Add(schedule);
        await db.SaveChangesAsync(ct);

        return (ToDto(schedule), ScheduleError.None, null);
    }

    /// <summary>
    /// Lists all schedules for the given organization. Requires any org membership.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(IReadOnlyList<ScheduleDto> schedules, ScheduleError error)>
        ListAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return ([], ScheduleError.PermissionDenied);

        var rows = await db.Schedules
            .Where(s => s.OrgId == orgId)
            .OrderBy(s => s.Name)
            .ToListAsync(ct);

        return (rows.Select(ToDto).ToList(), ScheduleError.None);
    }

    /// <summary>
    /// Returns a single schedule by name for the given organization. Requires any org membership.
    /// Returns <see cref="ScheduleError.NotFound"/> if not found or caller has no membership.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="name">Schedule name.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(ScheduleDto? dto, ScheduleError error)>
        GetByNameAsync(Guid userId, Guid orgId, string name, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return (null, ScheduleError.NotFound);

        var schedule = await db.Schedules
            .SingleOrDefaultAsync(s => s.OrgId == orgId && s.Name == name, ct);

        return schedule is null
            ? (null, ScheduleError.NotFound)
            : (ToDto(schedule), ScheduleError.None);
    }

    /// <summary>
    /// Immediately enqueues a run for the named schedule. Requires Owner or Admin role.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="scheduleName">Name of the schedule to run.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(ScheduledRunDto? dto, ScheduleError error)>
        RunNowAsync(Guid userId, Guid orgId, string scheduleName, CancellationToken ct)
    {
        if (!await IsAdminAsync(userId, orgId, ct))
            return (null, ScheduleError.PermissionDenied);

        var schedule = await db.Schedules
            .SingleOrDefaultAsync(s => s.OrgId == orgId && s.Name == scheduleName, ct);

        if (schedule is null)
            return (null, ScheduleError.NotFound);

        var now = clock.GetUtcNow().UtcDateTime;
        var run = new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = schedule.Id,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = now,
        };

        db.ScheduledRuns.Add(run);
        await db.SaveChangesAsync(ct);

        return (ToRunDto(run), ScheduleError.None);
    }

    /// <summary>
    /// Lists runs for the named schedule, newest first. Requires any org membership.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="scheduleName">Name of the schedule.</param>
    /// <param name="limit">Maximum number of runs to return.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(IReadOnlyList<ScheduledRunDto> runs, ScheduleError error)>
        ListRunsAsync(Guid userId, Guid orgId, string scheduleName, int limit, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return ([], ScheduleError.PermissionDenied);

        var schedule = await db.Schedules
            .SingleOrDefaultAsync(s => s.OrgId == orgId && s.Name == scheduleName, ct);

        if (schedule is null)
            return ([], ScheduleError.NotFound);

        var clampedLimit = Math.Clamp(limit, 1, 100);
        var runs = await db.ScheduledRuns
            .Where(r => r.ScheduleId == schedule.Id)
            .OrderByDescending(r => r.CreatedAt)
            .Take(clampedLimit)
            .ToListAsync(ct);

        return (runs.Select(ToRunDto).ToList(), ScheduleError.None);
    }

    /// <summary>
    /// Called by <see cref="SchedulerHost"/> — finds due schedules and enqueues runs.
    /// Updates <see cref="Schedule.LastRunAt"/> and recomputes <see cref="Schedule.NextRunAt"/>
    /// for each schedule that fires. Skips enqueuing a new run when a previous run for the
    /// same schedule is still queued or running (stack-up prevention).
    /// </summary>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>Count of new runs that were enqueued (skipped schedules are not counted).</returns>
    public async Task<int> EnqueueDueAsync(CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;

        var due = await db.Schedules
            .Where(s => s.Enabled && s.NextRunAt != null && s.NextRunAt <= now)
            .ToListAsync(ct);

        if (due.Count == 0)
            return 0;

        // M16-009: load schedule ids that already have an in-flight (queued|running) run
        // so we can skip stack-up firings.
        var dueIds = due.Select(s => s.Id).ToList();
        var inFlight = await db.ScheduledRuns
            .Where(r => dueIds.Contains(r.ScheduleId)
                     && (r.Status == ScheduledRunStatus.Queued || r.Status == ScheduledRunStatus.Running))
            .Select(r => r.ScheduleId)
            .Distinct()
            .ToListAsync(ct);
        var inFlightSet = inFlight.ToHashSet();

        var enqueued = 0;
        foreach (var schedule in due)
        {
            if (!inFlightSet.Contains(schedule.Id))
            {
                db.ScheduledRuns.Add(new ScheduledRun
                {
                    Id = Guid.NewGuid(),
                    ScheduleId = schedule.Id,
                    Status = ScheduledRunStatus.Queued,
                    CreatedAt = now,
                });
                enqueued++;
            }

            // Always update last_run_at + next_run_at for visibility, even on skip.
            schedule.LastRunAt = now;
            TimeZoneInfo schedTz;
            try { schedTz = TimeZoneInfo.FindSystemTimeZoneById(schedule.Timezone); }
            catch
            {
                logger.LogWarning(
                    "Schedule {ScheduleId} has unresolvable timezone '{Timezone}', falling back to UTC",
                    schedule.Id, schedule.Timezone);
                schedTz = TimeZoneInfo.Utc;
            }
            var parsedCron = CronExpression.Parse(schedule.CronExpression, CronFormat.Standard);
            schedule.NextRunAt = parsedCron.GetNextOccurrence(now, schedTz, inclusive: false);
            schedule.UpdatedAt = now;
        }

        await db.SaveChangesAsync(ct);
        return enqueued;
    }

    // ── private helpers ──────────────────────────────────────────────────────

    private async Task<bool> IsMemberAsync(Guid userId, Guid orgId, CancellationToken ct) =>
        await db.OrganizationMembers.AnyAsync(m => m.OrgId == orgId && m.UserId == userId, ct);

    private async Task<bool> IsAdminAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .SingleOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        return member is not null && member.Role is OrgRole.Owner or OrgRole.Admin;
    }

    private static ScheduleDto ToDto(Schedule s) =>
        new(
            Id: ScheduleId.Format(s.Id),
            Name: s.Name,
            CronExpression: s.CronExpression,
            Timezone: s.Timezone,
            CollectionRef: s.CollectionRef,
            Enabled: s.Enabled,
            NextRunAt: s.NextRunAt,
            LastRunAt: s.LastRunAt,
            CreatedAt: s.CreatedAt);

    private static ScheduledRunDto ToRunDto(ScheduledRun r) =>
        new(
            RunId: RunId.Format(r.Id),
            Status: r.Status.ToString().ToLowerInvariant(),
            CreatedAt: r.CreatedAt,
            StartedAt: r.StartedAt,
            CompletedAt: r.CompletedAt,
            ResultId: r.ResultId is { } guid ? ResultId.Format(guid) : null);
}
