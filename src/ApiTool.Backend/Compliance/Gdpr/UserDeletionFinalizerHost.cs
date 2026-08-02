using ApiTool.Backend.Notifications.Email;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Background service that daily (03:00 UTC) finalises GDPR account-deletion requests
/// whose 30-day cooldown has elapsed. Invokes <see cref="IUserAnonymiser"/> for each
/// eligible user, then enqueues an <c>account_deletion_completed</c> email.
/// <para>
/// State machine managed by this host: <c>PendingDeletionAt set → AnonymisedAt set</c>.
/// The <see cref="IUserAnonymiser"/> stub (M18-005) sets <c>anonymised_at</c>; the
/// concrete anonymiser (M18-006) will additionally scrub all PII columns.
/// </para>
/// Refs docs/SPECIFICATION.md v4-5.
/// </summary>
public sealed class UserDeletionFinalizerHost(
    IServiceScopeFactory scopeFactory,
    IEmailQueue emailQueue,
    TimeProvider clock,
    ILogger<UserDeletionFinalizerHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private static readonly TimeSpan CooldownDuration = TimeSpan.FromDays(30);
    private static readonly TimeSpan DefaultInterval = TimeSpan.FromHours(24);

    /// <summary>
    /// Processes all users whose <c>pending_deletion_at</c> is more than 30 days ago
    /// and whose <c>anonymised_at</c> is still null.
    /// Safe to call from tests directly (bypasses the daily schedule logic).
    /// </summary>
    public async Task TickOnceAsync(CancellationToken ct)
    {
        using var scope = scopeFactory.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var anonymiser = scope.ServiceProvider.GetRequiredService<IUserAnonymiser>();

        var now = clock.GetUtcNow().UtcDateTime;
        var threshold = now - CooldownDuration;

        var eligibleUsers = await db.Users
            .Where(u => u.PendingDeletionAt != null
                     && u.PendingDeletionAt <= threshold
                     && u.AnonymisedAt == null)
            .Select(u => new { u.Id, u.Email })
            .ToListAsync(ct);

        foreach (var user in eligibleUsers)
        {
            try
            {
                // Snapshot email BEFORE anonymisation (M18-006 will scrub the email column).
                var email = user.Email;

                await anonymiser.AnonymiseAsync(user.Id, ct);

                await emailQueue.EnqueueAsync(new EmailMessage(
                    To: email,
                    TemplateSlug: "account_deletion_completed",
                    Variables: new Dictionary<string, string>
                    {
                        ["user_email"]       = email,
                        ["deleted_at_local"] = now.ToString("yyyy-MM-dd HH:mm 'UTC'"),
                    },
                    EnqueuedAt: clock.GetUtcNow()), ct);

                logger.LogInformation(
                    "account_deletion_finalized userId={UserId}", user.Id);
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                // Log per-user failure; continue processing remaining users.
                logger.LogError(ex,
                    "account_deletion_finalizer_failed userId={UserId}", user.Id);
            }
        }
    }

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        // Delay to the next 03:00 UTC before starting the loop.
        var now = clock.GetUtcNow();
        var next03 = now.UtcDateTime.Date.AddHours(3);
        if (next03 <= now.UtcDateTime)
            next03 = next03.AddDays(1);
        var initialDelay = next03 - now.UtcDateTime;

        try
        {
            await Task.Delay(initialDelay, stoppingToken).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
        {
            return;
        }

        var interval = tickInterval ?? DefaultInterval;
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await TickOnceAsync(stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Unhandled exception in UserDeletionFinalizerHost tick loop.");
            }

            try
            {
                await Task.Delay(interval, stoppingToken).ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
        }
    }
}
