using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Production <see cref="IEmailDeadLetterStore"/> that emits a structured error log entry.
/// The <c>email_dead_letter</c> token is the search anchor for ops alerting.
/// </summary>
public sealed class LoggingEmailDeadLetterStore(ILogger<LoggingEmailDeadLetterStore> log)
    : IEmailDeadLetterStore
{
    /// <inheritdoc/>
    public Task RecordAsync(EmailMessage message, string lastError, int attemptCount, CancellationToken ct)
    {
        log.LogError(
            "email_dead_letter to={To} slug={Slug} attempt_count={AttemptCount} last_error={LastError}",
            message.To, message.TemplateSlug, attemptCount, lastError);
        return Task.CompletedTask;
    }
}
