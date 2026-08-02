using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Telemetry;

/// <summary>
/// Daily background service that hard-deletes <c>telemetry_events</c> rows older than
/// <see cref="TelemetryPurgeOptions.RetentionDays"/> days (default 90).
/// Raw events are ephemeral; daily aggregates in <c>telemetry_daily_aggregates</c> are
/// unaffected and persist indefinitely.
/// </summary>
/// <remarks>
/// The host is registered outside the Testing environment via
/// <c>AddHostedService&lt;TelemetryPurgeHost&gt;</c> in Program.cs. In tests, the
/// purge is triggered deterministically via
/// <c>POST /api/v1/internal/test-hooks/purge-telemetry-events</c>.
/// </remarks>
public sealed class TelemetryPurgeHost(
    IServiceScopeFactory scopeFactory,
    IOptions<TelemetryPurgeOptions> options,
    TimeProvider clock,
    ILogger<TelemetryPurgeHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? options.Value.TickInterval;
    private readonly int _retentionDays = options.Value.RetentionDays;

    /// <summary>
    /// Deletes all <c>telemetry_events</c> rows whose <c>received_at</c> is older than
    /// <see cref="TelemetryPurgeOptions.RetentionDays"/> days.
    /// Safe to call from tests directly.
    /// </summary>
    public async Task TickOnceAsync(CancellationToken ct)
    {
        await using var scope = scopeFactory.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var cutoff = clock.GetUtcNow().UtcDateTime.AddDays(-_retentionDays);

        var deleted = await db.TelemetryEvents
            .Where(e => e.ReceivedAt < cutoff)
            .ExecuteDeleteAsync(ct);

        if (deleted > 0)
        {
            logger.LogInformation(
                "telemetry_purge deleted={Count} cutoff={Cutoff:o}",
                deleted, cutoff);
        }
        else
        {
            logger.LogDebug("TelemetryPurgeHost: no events older than {Days} days", _retentionDays);
        }
    }

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation(
            "TelemetryPurgeHost started; retention={Days}d tick={Hours}h",
            _retentionDays, _tickInterval.TotalHours);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await TickOnceAsync(stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
            catch (Exception ex) { logger.LogError(ex, "TelemetryPurgeHost tick failed"); }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }

        logger.LogInformation("TelemetryPurgeHost stopped");
    }
}
