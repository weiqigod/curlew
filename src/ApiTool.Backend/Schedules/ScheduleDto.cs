namespace ApiTool.Backend.Schedules;

/// <summary>Response DTO representing a schedule.</summary>
/// <param name="Id">Wire-format schedule id (<c>sched_&lt;hex&gt;</c>).</param>
/// <param name="Name">Human-readable schedule name.</param>
/// <param name="CronExpression">Standard 5-field cron expression.</param>
/// <param name="Timezone">IANA TZ identifier the cron is evaluated in.</param>
/// <param name="CollectionRef">Reference to the collection file to run.</param>
/// <param name="Enabled">Whether this schedule is active.</param>
/// <param name="NextRunAt">Computed next UTC fire time.</param>
/// <param name="LastRunAt">UTC time of the last queued/completed run.</param>
/// <param name="CreatedAt">UTC creation timestamp.</param>
public sealed record ScheduleDto(
    string Id,
    string Name,
    string CronExpression,
    string Timezone,
    string CollectionRef,
    bool Enabled,
    DateTime? NextRunAt,
    DateTime? LastRunAt,
    DateTime CreatedAt);
