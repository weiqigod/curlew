// Refs docs/SPECIFICATION.md:5855 (daily 09:00 UTC cron) and :8587 (trial_expiring template + variables).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Notifications.Trials;

/// <summary>
/// Daily background service that queues <c>trial_expiring</c> SendGrid emails for
/// active trial rows approaching the 3-day and 1-day expiry marks. Fires at
/// <see cref="TrialExpiryNotifierOptions.RunAtUtc"/> (default 09:00 UTC).
/// Refs docs/SPECIFICATION.md:5855 (cron schedule) and :8587 (template + variables).
/// </summary>
/// <remarks>
/// Per-row contract: build the <see cref="EmailMessage"/>, await
/// <see cref="IEmailQueue.EnqueueAsync"/>, then set the corresponding
/// <c>notified_*_at</c> column and <c>SaveChangesAsync</c>. If <c>EnqueueAsync</c>
/// throws, the timestamp is <em>not</em> set and the next tick retries.
/// Known trade-off: if the host crashes after enqueue but before <c>SaveChangesAsync</c>,
/// the next tick may send a duplicate email. Acceptable for v1 per spec :5855.
/// </remarks>
public sealed class TrialExpiryNotifier(
    IServiceScopeFactory scopeFactory,
    IOptions<TrialExpiryNotifierOptions> options,
    IOptions<AppOptions> appOptions,
    TimeProvider clock,
    ILogger<TrialExpiryNotifier> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private const string TemplateSlug = "trial_expiring";
    private static readonly TimeSpan ThreeDays = TimeSpan.FromDays(3);
    private static readonly TimeSpan OneDay = TimeSpan.FromDays(1);

    // Test-only override; when null the loop sleeps until the next configured fire time.
    private readonly TimeSpan? _tickInterval = tickInterval;

    // Tracks whether the first tick has fired; used to skip the initial delay in forced-fire mode.
    private bool _firstFireDone;

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var opts = options.Value;
        logger.LogInformation(
            "TrialExpiryNotifier started; RunAtUtc={RunAtUtc} BatchSize={BatchSize}",
            opts.RunAtUtc, opts.BatchSize);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await DelayUntilNextFireAsync(opts, stoppingToken);
                if (stoppingToken.IsCancellationRequested) break;

                await TickOnceAsync(opts, stoppingToken);
                _firstFireDone = true;
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "TrialExpiryNotifier tick failed");
                _firstFireDone = true;
                // On failure, still wait to avoid a tight error loop.
                try { await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken); }
                catch (OperationCanceledException) { break; }
            }
        }

        logger.LogInformation("TrialExpiryNotifier stopped");
    }

    /// <summary>
    /// Sleeps until the next configured fire time. If <see cref="_tickInterval"/>
    /// is set (test mode), sleeps that interval instead. If <c>RunAtUtc == "now"</c>,
    /// returns immediately on the first call and sleeps 1 minute between subsequent calls.
    /// </summary>
    private async Task DelayUntilNextFireAsync(
        TrialExpiryNotifierOptions opts, CancellationToken ct)
    {
        if (_tickInterval is { } interval)
        {
            await Task.Delay(interval, ct);
            return;
        }

        if (opts.IsForcedFire)
        {
            // Fire immediately on the very first call; sleep 1 minute between
            // subsequent ticks so a misconfig in prod does not spin.
            if (_firstFireDone)
                await Task.Delay(TimeSpan.FromMinutes(1), ct);
            return;
        }

        var delay = ComputeNextFireDelay(opts, clock.GetUtcNow());
        if (delay > TimeSpan.Zero)
            await Task.Delay(delay, ct);
    }

    /// <summary>
    /// Computes how long to sleep before the next wall-clock fire. Exposed as
    /// <c>internal</c> so unit tests can verify scheduling accuracy without
    /// starting the hosted service loop.
    /// </summary>
    internal static TimeSpan ComputeNextFireDelay(
        TrialExpiryNotifierOptions opts, DateTimeOffset now)
    {
        var fireAt = opts.ParseRunAtUtc();
        var todayFire = new DateTimeOffset(
            now.Year, now.Month, now.Day,
            fireAt.Hour, fireAt.Minute, fireAt.Second,
            TimeSpan.Zero);
        var next = now < todayFire ? todayFire : todayFire.AddDays(1);
        return next - now;
    }

    /// <summary>Runs the 3-day pass followed by the 1-day pass within one DI scope.</summary>
    internal async Task TickOnceAsync(TrialExpiryNotifierOptions opts, CancellationToken ct)
    {
        await using var scope = scopeFactory.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var queue = scope.ServiceProvider.GetRequiredService<IEmailQueue>();
        var now = clock.GetUtcNow().UtcDateTime;

        var sent3 = await ProcessPassAsync(db, queue, opts, now, ThreeDays, kind3day: true, ct);
        var sent1 = await ProcessPassAsync(db, queue, opts, now, OneDay, kind3day: false, ct);

        if (sent3 + sent1 > 0)
            logger.LogInformation(
                "trial_expiry_notifier_tick sent_3day={Sent3} sent_1day={Sent1}",
                sent3, sent1);
    }

    private async Task<int> ProcessPassAsync(
        AppDbContext db,
        IEmailQueue queue,
        TrialExpiryNotifierOptions opts,
        DateTime now,
        TimeSpan window,
        bool kind3day,
        CancellationToken ct)
    {
        var cutoff = now + window;

        // Query the matching trials. Filter on kind explicitly (defensive — see
        // architectural decision #8 in the plan). The partial index on (expires_at)
        // WHERE notified_*_at IS NULL AND consumed_at IS NULL is engaged when the
        // WHERE clause includes both nullness predicates.
        var query = db.Trials.AsQueryable()
            .Where(t => t.ExpiresAt > now && t.ExpiresAt <= cutoff)
            .Where(t => t.ConsumedAt == null)
            .Where(t => t.Kind == TrialKind.FullInitial || t.Kind == TrialKind.OnDemand);

        query = kind3day
            ? query.Where(t => t.Notified3DayAt == null)
            : query.Where(t => t.Notified1DayAt == null);

        // Fetch user emails alongside trials for the EmailMessage 'To' field.
        var rows = await query
            .OrderBy(t => t.ExpiresAt)
            .Take(opts.BatchSize)
            .Join(
                db.Users,
                t => t.UserId,
                u => u.Id,
                (t, u) => new { Trial = t, UserEmail = u.Email })
            .ToListAsync(ct);

        var upgradeUrl = appOptions.Value.WebAppUrl.TrimEnd('/') + "/billing";
        var sent = 0;

        foreach (var row in rows)
        {
            var msg = new EmailMessage(
                To: row.UserEmail,
                TemplateSlug: TemplateSlug,
                Variables: new Dictionary<string, string>
                {
                    ["user_email"]       = row.UserEmail,
                    ["feature"]          = row.Trial.Feature,
                    ["expires_at_local"] = row.Trial.ExpiresAt
                        .ToString("yyyy-MM-dd HH:mm 'UTC'"),
                    ["upgrade_url"]      = upgradeUrl,
                },
                EnqueuedAt: clock.GetUtcNow());

            try
            {
                await queue.EnqueueAsync(msg, ct);
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested)
            {
                throw;
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex,
                    "trial_expiry_notify_enqueue_failed trial_id={TrialId} user_id={UserId} feature={Feature}",
                    row.Trial.Id, row.Trial.UserId, row.Trial.Feature);
                continue; // do NOT set the notified_*_at column — next tick retries
            }

            if (kind3day)
                row.Trial.Notified3DayAt = now;
            else
                row.Trial.Notified1DayAt = now;
            row.Trial.UpdatedAt = now;

            try
            {
                await db.SaveChangesAsync(ct);
                sent++;
            }
            catch (DbUpdateException ex)
            {
                // Concurrent update on the same row (vanishingly rare — only the
                // notifier writes these columns). Log and continue; next tick
                // will pick the row back up if the timestamp is still null.
                logger.LogWarning(ex,
                    "trial_expiry_notify_save_failed trial_id={TrialId} user_id={UserId}",
                    row.Trial.Id, row.Trial.UserId);
                db.ChangeTracker.Clear();
            }
        }

        return sent;
    }
}
