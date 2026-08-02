// Refs docs/SPECIFICATION.md:8425 (daily repo-set reconciliation).
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitHub.Installations;

/// <summary>
/// Daily background service that reconciles each active installation's <c>repo_set</c>
/// against the GitHub API's ground truth, correcting any drift.
/// Mirrors the <c>StripeWebhookCleanupHost</c> tick pattern.
/// Refs docs/SPECIFICATION.md:8425.
/// </summary>
public sealed class GithubInstallationReconcilerHost(
    IServiceScopeFactory scopeFactory,
    IGitHubInstallationsApi api,
    IOptions<GithubInstallationReconcilerOptions> options,
    TimeProvider clock,
    ILogger<GithubInstallationReconcilerHost> log,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromHours(24);
    private readonly int _staleDays = options.Value.StaleDays;

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        log.LogInformation(
            "GithubInstallationReconcilerHost started; tick={Hours}h stale_days={StaleDays}",
            _tickInterval.TotalHours, _staleDays);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await ReconcileAllAsync(stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
            catch (Exception ex) { log.LogError(ex, "GithubInstallationReconcilerHost tick failed"); }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }

        log.LogInformation("GithubInstallationReconcilerHost stopped");
    }

    private async Task ReconcileAllAsync(CancellationToken ct)
    {
        await using var scope = scopeFactory.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var svc = scope.ServiceProvider.GetRequiredService<GithubInstallationsService>();

        // Only reconcile non-deleted, non-suspended installations
        var installations = await db.GithubInstallations
            .Where(x => x.DeletedAt == null && x.SuspendedAt == null)
            .ToListAsync(ct);

        log.LogInformation(
            "github_reconciler_tick installations_count={Count}", installations.Count);

        foreach (var install in installations)
        {
            if (ct.IsCancellationRequested) break;

            try
            {
                var remoteRepos = await api.ListInstallationRepositoriesAsync(install.InstallationId, ct);

                // Replace repo_set with the authoritative GitHub list (additions + removals)
                await svc.ReplaceRepoSetAsync(install.InstallationId, remoteRepos, ct);

                install.LastReconciledAt = clock.GetUtcNow().UtcDateTime;
                await db.SaveChangesAsync(ct);

                log.LogInformation(
                    "github_reconciler_synced installation_id={Id} repo_count={Count}",
                    install.InstallationId, remoteRepos.Count);
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested) { break; }
            catch (Exception ex)
            {
                log.LogError(ex,
                    "github_reconciler_failed installation_id={Id}", install.InstallationId);
            }
        }
    }
}
