namespace ApiTool.Backend.Data.Entities;

/// <summary>Lifecycle status of a scheduled run.</summary>
public enum ScheduledRunStatus
{
    /// <summary>The run has been enqueued and is awaiting execution.</summary>
    Queued,

    /// <summary>The run is currently being executed.</summary>
    Running,

    /// <summary>The run completed successfully.</summary>
    Completed,

    /// <summary>The run failed during execution.</summary>
    Failed,

    /// <summary>Reaper reclaimed a stale running row back to queued; terminal-on-row-replacement.</summary>
    Reaped,

    /// <summary>Scheduler skipped this firing because a previous run was still in flight.</summary>
    Skipped,
}
