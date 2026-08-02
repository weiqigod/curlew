using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Health;

/// <summary>Composes DB and Redis probes into a single HealthReport.</summary>
public sealed class HealthService(
    IDbHealthProbe db,
    IRedisHealthProbe redis,
    ILogger<HealthService> logger)
{
    /// <summary>Runs both probes and builds a HealthReport.</summary>
    public async Task<HealthReport> CheckAsync(CancellationToken ct)
    {
        var dbTask = db.IsConnectedAsync(ct);
        var redisTask = redis.IsConfigured
            ? redis.IsConnectedAsync(ct)
            : Task.FromResult(true);

        var dbOk = await SafeAsync(dbTask, "db", logger);
        var redisOk = await SafeAsync(redisTask, "redis", logger);

        var dbStr = dbOk ? HealthReport.Connected : HealthReport.Disconnected;
        var redisStr = !redis.IsConfigured
            ? HealthReport.NotConfigured
            : redisOk ? HealthReport.Connected : HealthReport.Disconnected;

        var healthy = dbOk && (redisOk || !redis.IsConfigured);
        var status = healthy ? HealthReport.StatusHealthy : HealthReport.StatusUnhealthy;
        return new HealthReport(status, dbStr, redisStr);
    }

    private static async Task<bool> SafeAsync(Task<bool> t, string name, ILogger logger)
    {
        try
        {
            return await t;
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "health probe {Probe} threw", name);
            return false;
        }
    }
}
