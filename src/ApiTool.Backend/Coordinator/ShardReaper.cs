using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Coordinator;

/// <summary>
/// Background service that periodically reclaims stale coordinator shards.
/// Uses a new DI scope per tick so the scoped <see cref="IShardReaper"/> is
/// resolved fresh each time, avoiding DbContext lifetime issues.
/// </summary>
public sealed class ShardReaper(
    IServiceScopeFactory scopeFactory,
    ILogger<ShardReaper> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromSeconds(15);

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation("ShardReaper started, polling every {Interval}s", _tickInterval.TotalSeconds);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var reaper = scope.ServiceProvider.GetRequiredService<IShardReaper>();
                var count = await reaper.ReapStaleShardsAsync(stoppingToken);
                if (count > 0)
                    logger.LogInformation("shard reaper reclaimed {Count} stale shards", count);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "ShardReaper tick failed");
            }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }

        logger.LogInformation("ShardReaper stopped");
    }
}
