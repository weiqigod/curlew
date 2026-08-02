using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Audit;

/// <summary>
/// Daily cleanup of <c>organization_audit_log</c> rows older than each org's
/// <c>audit_log_retention_days</c> setting. Mirrors the <c>GithubWebhookCleanupHost</c>
/// pattern with a per-org iteration so retention is honoured individually.
/// </summary>
/// <remarks>
/// The host is registered outside the Testing environment via
/// <c>AddHostedService&lt;AuditLogCleanupHost&gt;</c> in Program.cs. In tests, the
/// cleanup is triggered deterministically via the internal test-only hook
/// (<c>POST /api/v1/internal/test-hooks/run-audit-cleanup</c>) which calls
/// <see cref="TickOnceAsync"/> directly.
/// </remarks>
public sealed class AuditLogCleanupHost(
    IServiceScopeFactory scopeFactory,
    IOptions<AuditLogCleanupOptions> options,
    TimeProvider clock,
    ILogger<AuditLogCleanupHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? options.Value.TickInterval;

    /// <summary>
    /// Runs one cleanup iteration across all organisations, deleting audit log rows
    /// older than each org's configured retention window.
    /// </summary>
    public async Task TickOnceAsync(CancellationToken ct)
    {
        await using var scope = scopeFactory.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var now = clock.GetUtcNow().UtcDateTime;

        var orgs = await db.Organizations
            .Select(o => new { o.Id, o.AuditLogRetentionDays })
            .ToListAsync(ct);

        foreach (var org in orgs)
        {
            var cutoff = now.AddDays(-org.AuditLogRetentionDays);
            var deleted = await db.OrganizationAuditLog
                .Where(e => e.OrgId == org.Id && e.CreatedAt < cutoff)
                .ExecuteDeleteAsync(ct);

            if (deleted > 0)
            {
                AuditLogMetrics.RowsDeleted.Add(
                    deleted,
                    new KeyValuePair<string, object?>("org_id", org.Id));

                logger.LogInformation(
                    "audit_log_cleanup org_id={OrgId} deleted={Count} cutoff={Cutoff:o}",
                    org.Id, deleted, cutoff);
            }
        }
    }

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation(
            "AuditLogCleanupHost started; tick={Hours}h",
            _tickInterval.TotalHours);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await TickOnceAsync(stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
            catch (Exception ex) { logger.LogError(ex, "AuditLogCleanupHost tick failed"); }

            try { await Task.Delay(_tickInterval, stoppingToken); }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested) { break; }
        }

        logger.LogInformation("AuditLogCleanupHost stopped");
    }
}
