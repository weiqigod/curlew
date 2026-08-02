// Spec ref: docs/SPECIFICATION.md:8595 (90-day retention; processed only).
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Daily cleanup of <c>github_webhook_events</c> with status = 'processed' older
/// than <see cref="GithubWebhookCleanupOptions.RetentionDays"/>. Quarantined rows
/// are retained indefinitely for forensic investigation.
/// </summary>
public sealed class GithubWebhookCleanupHost(
    IServiceScopeFactory scopeFactory,
    IOptions<GithubWebhookCleanupOptions> options,
    TimeProvider clock,
    ILogger<GithubWebhookCleanupHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromHours(24);

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation(
            "GithubWebhookCleanupHost started; retention={Days} days; tick={Hours}h",
            options.Value.RetentionDays, _tickInterval.TotalHours);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var store = scope.ServiceProvider.GetRequiredService<GithubWebhookStore>();
                var cutoff = clock.GetUtcNow().UtcDateTime.AddDays(-options.Value.RetentionDays);
                var deleted = await store.DeleteOlderThanAsync(cutoff, stoppingToken);
                if (deleted > 0)
                    logger.LogInformation(
                        "github_webhook_cleanup_deleted count={Count} cutoff={Cutoff:o}",
                        deleted, cutoff);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
            catch (Exception ex) { logger.LogError(ex, "GithubWebhookCleanupHost tick failed"); }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }
        logger.LogInformation("GithubWebhookCleanupHost stopped");
    }
}
