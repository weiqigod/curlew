namespace ApiTool.Backend.Schedules;

/// <summary>Response DTO representing a single scheduled run.</summary>
/// <param name="RunId">Wire-format run id (<c>run_&lt;hex&gt;</c>).</param>
/// <param name="Status">Current run status string.</param>
/// <param name="CreatedAt">UTC timestamp when the run was enqueued.</param>
/// <param name="StartedAt">UTC timestamp when execution started, or null.</param>
/// <param name="CompletedAt">UTC timestamp when execution completed, or null.</param>
/// <param name="ResultId">Wire-format result id (<c>res_&lt;hex&gt;</c>) once the run has produced a result; null while queued, running, or for legacy rows.</param>
public sealed record ScheduledRunDto(
    string RunId,
    string Status,
    DateTime CreatedAt,
    DateTime? StartedAt,
    DateTime? CompletedAt,
    string? ResultId);
