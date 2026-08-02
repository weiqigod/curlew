using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Schedules;

/// <summary>
/// Background service that polls for due schedules every 30 seconds and enqueues runs.
/// Uses a new DI scope per tick so that the scoped <see cref="SchedulesService"/> is
/// resolved fresh each time, avoiding DbContext lifetime issues.
/// </summary>
public sealed class SchedulerHost(
    IServiceScopeFactory scopeFactory,
    ILogger<SchedulerHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromSeconds(30);

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation("SchedulerHost started, polling every {Interval}s", _tickInterval.TotalSeconds);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var enqueuer = scope.ServiceProvider.GetRequiredService<ISchedulerEnqueuer>();
                var count = await enqueuer.EnqueueDueAsync(stoppingToken);

                if (count > 0)
                    logger.LogInformation("scheduler tick {Count} schedules due", count);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "SchedulerHost tick failed");
            }

            try
            {
                await Task.Delay(_tickInterval, stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
        }

        logger.LogInformation("SchedulerHost stopped");
    }
}
