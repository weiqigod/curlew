// Spec ref: docs/SPECIFICATION.md:6852 (90-day retention; processed only).
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Webhooks;

/// <summary>
/// Daily cleanup of <c>stripe_webhook_events</c> with status = 'processed' older
/// than <see cref="StripeWebhookCleanupOptions.RetentionDays"/>. Quarantined rows
/// are retained indefinitely for forensic investigation.
/// </summary>
public sealed class StripeWebhookCleanupHost(
    IServiceScopeFactory scopeFactory,
    IOptions<StripeWebhookCleanupOptions> options,
    TimeProvider clock,
    ILogger<StripeWebhookCleanupHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromHours(24);

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation(
            "StripeWebhookCleanupHost started; retention={Days} days; tick={Hours}h",
            options.Value.RetentionDays, _tickInterval.TotalHours);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var store = scope.ServiceProvider.GetRequiredService<StripeWebhookStore>();
                var cutoff = clock.GetUtcNow().UtcDateTime.AddDays(-options.Value.RetentionDays);
                var deleted = await store.DeleteOlderThanAsync(cutoff, stoppingToken);
                if (deleted > 0)
                    logger.LogInformation(
                        "stripe_webhook_cleanup_deleted count={Count} cutoff={Cutoff:o}",
                        deleted, cutoff);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
            catch (Exception ex) { logger.LogError(ex, "StripeWebhookCleanupHost tick failed"); }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }
        logger.LogInformation("StripeWebhookCleanupHost stopped");
    }
}
