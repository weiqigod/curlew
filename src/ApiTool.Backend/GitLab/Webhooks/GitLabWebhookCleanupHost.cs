// Refs docs/SPECIFICATION.md:10943 (30-day retention; processed only).
// Mirrors GithubWebhookCleanupHost — see plan Decision H.
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Daily cleanup of <c>gitlab_webhook_events</c> with <c>processed_at IS NOT NULL AND quarantined_at IS NULL</c>
/// older than <see cref="GitLabWebhookCleanupOptions.RetentionDays"/> (default: 30 days per spec :10943).
/// Quarantined rows are retained indefinitely for forensic investigation.
/// Mirrors <c>GithubWebhookCleanupHost</c>.
/// </summary>
public sealed class GitLabWebhookCleanupHost(
    IServiceScopeFactory scopeFactory,
    IOptions<GitLabWebhookCleanupOptions> options,
    TimeProvider clock,
    ILogger<GitLabWebhookCleanupHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromHours(24);

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation(
            "GitLabWebhookCleanupHost started; retention={Days} days; tick={Hours}h",
            options.Value.RetentionDays, _tickInterval.TotalHours);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var store = scope.ServiceProvider.GetRequiredService<GitLabWebhookStore>();
                var cutoff = clock.GetUtcNow().UtcDateTime.AddDays(-options.Value.RetentionDays);
                var deleted = await store.DeleteOlderThanAsync(cutoff, stoppingToken);
                if (deleted > 0)
                    logger.LogInformation(
                        "gitlab_webhook_cleanup_deleted count={Count} cutoff={Cutoff:o}",
                        deleted, cutoff);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
            catch (Exception ex) { logger.LogError(ex, "GitLabWebhookCleanupHost tick failed"); }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }

        logger.LogInformation("GitLabWebhookCleanupHost stopped");
    }
}
