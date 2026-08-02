using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Bootstrap;

/// <summary>
/// Applies pending EF Core migrations on startup. Must run before the application begins
/// serving requests to ensure the schema is current.
/// </summary>
public sealed class MigrationRunner(
    IServiceScopeFactory scopes,
    ILogger<MigrationRunner> logger)
{
    /// <summary>
    /// Applies all pending EF migrations with a 60-second default timeout. Returns the count
    /// of migrations applied, or throws <see cref="MigrationFailedException"/> on failure.
    /// When <paramref name="run"/> is false, logs "migrations: skipped" and returns 0.
    /// </summary>
    /// <param name="run">Whether to apply migrations. Pass false to skip (e.g., when <c>BACKEND_RUN_MIGRATIONS</c> is unset).</param>
    /// <param name="timeout">Override the default 60-second migration timeout.</param>
    /// <param name="ct">Cancellation token for the outer operation.</param>
    /// <returns>The number of migrations applied.</returns>
    /// <exception cref="MigrationFailedException">Thrown when migrations fail to apply.</exception>
    public async Task<int> RunAsync(bool run, TimeSpan? timeout = null, CancellationToken ct = default)
    {
        if (!run)
        {
            logger.LogInformation("migrations: skipped");
            return 0;
        }

        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(timeout ?? TimeSpan.FromSeconds(60));

        await using var scope = scopes.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        try
        {
            var pending = await db.Database.GetPendingMigrationsAsync(cts.Token);
            var count = pending.Count();
            if (count == 0)
            {
                logger.LogInformation("migrations: applied 0 (up-to-date)");
                return 0;
            }
            await db.Database.MigrateAsync(cts.Token);
            logger.LogInformation("migrations: applied {Count}", count);
            return count;
        }
        catch (Exception ex) when (ex is not OperationCanceledException || !ct.IsCancellationRequested)
        {
            logger.LogError(ex, "migrations: failed");
            throw new MigrationFailedException("Failed to apply EF migrations", ex);
        }
    }
}
