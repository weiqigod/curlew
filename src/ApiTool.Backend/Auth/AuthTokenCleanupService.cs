// Background service that prunes expired/consumed/revoked auth tokens.
// Refs docs/SPECIFICATION.md:8556-8560 (token table housekeeping).
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Hosted service that prunes <c>password_reset_tokens</c> and
/// <c>email_verification_tokens</c> rows older than <see cref="AuthTokenCleanupOptions.RetentionDays"/>
/// on a fixed 1-hour tick. Skipped in the Testing environment.
/// </summary>
public sealed class AuthTokenCleanupService(
    IServiceScopeFactory scopeFactory,
    IOptions<AuthTokenCleanupOptions> options,
    TimeProvider clock,
    ILogger<AuthTokenCleanupService> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private readonly TimeSpan _tickInterval = tickInterval ?? TimeSpan.FromHours(1);

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation(
            "AuthTokenCleanupService started; retention={Days} days; tick={Hours}h",
            options.Value.RetentionDays, _tickInterval.TotalHours);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = scopeFactory.CreateAsyncScope();
                var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
                var cutoff = clock.GetUtcNow().UtcDateTime.AddDays(-options.Value.RetentionDays);

                var prRows = await db.PasswordResetTokens
                    .Where(t => (t.ConsumedAt != null || t.RevokedAt != null || t.ExpiresAt < cutoff)
                                && t.IssuedAt < cutoff)
                    .ToListAsync(stoppingToken);
                db.PasswordResetTokens.RemoveRange(prRows);

                var evRows = await db.EmailVerificationTokens
                    .Where(t => (t.ConsumedAt != null || t.RevokedAt != null || t.ExpiresAt < cutoff)
                                && t.IssuedAt < cutoff)
                    .ToListAsync(stoppingToken);
                db.EmailVerificationTokens.RemoveRange(evRows);

                if (prRows.Count + evRows.Count > 0)
                {
                    await db.SaveChangesAsync(stoppingToken);
                    logger.LogInformation(
                        "auth_token_cleanup_deleted password_reset={Pr} email_verification={Ev} cutoff={Cutoff:o}",
                        prRows.Count, evRows.Count, cutoff);
                }
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "AuthTokenCleanupService tick failed");
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

        logger.LogInformation("AuthTokenCleanupService stopped");
    }
}
