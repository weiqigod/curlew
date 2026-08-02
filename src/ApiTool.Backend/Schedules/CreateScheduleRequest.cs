namespace ApiTool.Backend.Schedules;

/// <summary>Request body for creating a new cron schedule.</summary>
/// <param name="Name">Human-readable schedule name (unique per org).</param>
/// <param name="Cron">Standard 5-field cron expression (e.g. "0 2 * * *").</param>
/// <param name="CollectionRef">Reference to the collection file to run (e.g. "smoke.yaml").</param>
/// <param name="Timezone">IANA TZ identifier the cron is evaluated in (e.g. "Europe/Stockholm"). Defaults to "UTC".</param>
/// <param name="EnvVars">Optional environment variables injected into the runner at claim time (M18-009). Values are envelope-encrypted at rest.</param>
public sealed record CreateScheduleRequest(
    string? Name,
    string? Cron,
    string? CollectionRef,
    string? Timezone = null,
    Dictionary<string, string>? EnvVars = null);
