// Spec refs: docs/SPECIFICATION.md:8953 (channel-backed processor).
using System.Threading.Channels;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Background service that drains <see cref="EmailMessage"/> items from the in-process channel
/// and delivers them via <see cref="ISmtpSender.SendTemplateAsync"/>.
/// Implements exponential-backoff retry for transient SendGrid errors and dead-letters
/// on exhaustion or permanent failure.
/// Registered only when <c>ApiTool:SendGrid:Mode == "live"</c> (gated in Program.cs).
/// </summary>
public sealed class EmailQueueProcessor(
    Channel<EmailMessage> channel,
    IServiceProvider serviceProvider,
    IOptions<EmailQueueProcessorOptions> options,
    IEmailDeadLetterStore deadLetters,
    ILogger<EmailQueueProcessor> log) : BackgroundService
{
    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var opts = options.Value;
        await foreach (var msg in channel.Reader.ReadAllAsync(stoppingToken).ConfigureAwait(false))
        {
            await ProcessOneAsync(msg, opts, stoppingToken).ConfigureAwait(false);
        }
    }

    private async Task ProcessOneAsync(
        EmailMessage msg, EmailQueueProcessorOptions opts, CancellationToken ct)
    {
        Exception? lastError = null;

        for (var attempt = 1; attempt <= opts.MaxAttempts; attempt++)
        {
            // Resolve the sender through the DI container each attempt (singleton-safe).
            var sender = serviceProvider.GetRequiredService<ISmtpSender>();
            try
            {
                await sender.SendTemplateAsync(
                    msg.To, msg.TemplateSlug,
                    msg.Variables.ToDictionary(kv => kv.Key, kv => kv.Value),
                    ct).ConfigureAwait(false);

                // Record in the test-only audit log when registered (Dev+Testing only).
                serviceProvider.GetService<IRecentlySentEmailLog>()
                    ?.Record(msg, TimeProvider.System.GetUtcNow());

                return; // success — exit retry loop
            }
            catch (EmailTemplateNotFoundException ex)
            {
                // Permanent: no template id configured.
                await DeadLetterAsync(msg, ex, attempt, ct).ConfigureAwait(false);
                return;
            }
            catch (EmailTemplateVariableUnknownException ex)
            {
                // Permanent: variable allowlist violation.
                await DeadLetterAsync(msg, ex, attempt, ct).ConfigureAwait(false);
                return;
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested)
            {
                // Clean server shutdown — abandon in-flight send without dead-lettering.
                return;
            }
            catch (Exception ex) when (IsTransient(ex))
            {
                lastError = ex;
                log.LogWarning(
                    "email delivery attempt {Attempt}/{Max} failed (transient) to={To} slug={Slug}: {Error}",
                    attempt, opts.MaxAttempts, msg.To, msg.TemplateSlug, ex.Message);

                if (attempt < opts.MaxAttempts)
                {
                    var baseDelay = opts.RetryDelays[Math.Min(attempt - 1, opts.RetryDelays.Length - 1)];
                    // Jitter ±20% to prevent thundering-herd on SendGrid after a 5xx wave.
                    var jitteredMs = baseDelay.TotalMilliseconds * (0.8 + (Random.Shared.NextDouble() * 0.4));
                    var delay = TimeSpan.FromMilliseconds(jitteredMs);
                    try { await Task.Delay(delay, ct).ConfigureAwait(false); }
                    catch (OperationCanceledException) { return; }
                }
            }
            catch (Exception ex)
            {
                // Permanent: unexpected 4xx or other non-retryable error.
                await DeadLetterAsync(msg, ex, attempt, ct).ConfigureAwait(false);
                return;
            }
        }

        // All attempts exhausted.
        await DeadLetterAsync(
            msg, lastError ?? new Exception("max attempts exceeded"),
            opts.MaxAttempts, ct).ConfigureAwait(false);
    }

    private Task DeadLetterAsync(EmailMessage msg, Exception ex, int attempt, CancellationToken ct)
    {
        log.LogWarning(
            "email dead-lettering after {Attempt} attempt(s) to={To} slug={Slug}: {Error}",
            attempt, msg.To, msg.TemplateSlug, ex.Message);
        return deadLetters.RecordAsync(msg, ex.Message, attempt, ct);
    }

    private static bool IsTransient(Exception ex)
        => ex is Notifications.SendGridSmtpSender.SendGridRateLimitedException
            or Notifications.SendGridSmtpSender.SendGridUnavailableException;
}
