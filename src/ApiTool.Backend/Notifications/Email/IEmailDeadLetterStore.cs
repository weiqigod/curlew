namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Seam for recording permanently-failed email delivery attempts.
/// Implementations may write to a structured log, a database, or a monitoring sink.
/// </summary>
public interface IEmailDeadLetterStore
{
    /// <summary>Records a dead-lettered email message along with the last error and attempt count.</summary>
    Task RecordAsync(EmailMessage message, string lastError, int attemptCount, CancellationToken ct);
}
