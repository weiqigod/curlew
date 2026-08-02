namespace ApiTool.Backend.Schedules;

/// <summary>
/// Abstraction for the enqueue-due operation, allowing the <see cref="SchedulerHost"/>
/// to call into a testable seam rather than directly coupling to <see cref="SchedulesService"/>.
/// </summary>
public interface ISchedulerEnqueuer
{
    /// <summary>Finds due schedules and enqueues runs. Returns the count of enqueued schedules.</summary>
    Task<int> EnqueueDueAsync(CancellationToken ct);
}
