namespace ApiTool.Backend.Data.Entities;

/// <summary>A single enqueued run triggered by the scheduler or a manual run-now.</summary>
public sealed class ScheduledRun
{
    /// <summary>Primary key (serialized as <c>run_&lt;hex&gt;</c>).</summary>
    public Guid Id { get; set; }

    /// <summary>Foreign key to the schedule that triggered this run.</summary>
    public Guid ScheduleId { get; set; }

    /// <summary>Current lifecycle status.</summary>
    public ScheduledRunStatus Status { get; set; } = ScheduledRunStatus.Queued;

    /// <summary>UTC timestamp when this run was enqueued.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp when the run started executing (null if still queued).</summary>
    public DateTime? StartedAt { get; set; }

    /// <summary>UTC timestamp when the run completed (null if not yet done).</summary>
    public DateTime? CompletedAt { get; set; }

    // M16-009: worker-pull claim columns

    /// <summary>Server-minted UUID given to the worker that claimed this run; null when unclaimed.</summary>
    public Guid? ClaimToken { get; set; }

    /// <summary>Identifier of the worker that claimed this run (opaque string from JWT or header).</summary>
    public string? ClaimedByWorker { get; set; }

    /// <summary>UTC timestamp when the run was claimed.</summary>
    public DateTime? ClaimedAt { get; set; }

    /// <summary>UTC timestamp of the most recent heartbeat from the claiming worker.</summary>
    public DateTime? LastHeartbeatAt { get; set; }

    /// <summary>UTC deadline by which the worker must send its first heartbeat.</summary>
    public DateTime? ClaimDeadline { get; set; }

    /// <summary>FK to the result row written when the worker posts its result (set by M16-009 result endpoint).</summary>
    public Guid? ResultId { get; set; }

    /// <summary>Human-readable failure reason written when status transitions to <see cref="ScheduledRunStatus.Failed"/>.</summary>
    public string? FailureReason { get; set; }
}
