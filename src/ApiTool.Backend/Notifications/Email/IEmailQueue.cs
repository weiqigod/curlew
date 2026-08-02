namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Asynchronous queue boundary for outbound email messages.
/// The consumer (<c>EmailQueueProcessor</c>) lands in M14-014.
/// </summary>
public interface IEmailQueue
{
    /// <summary>Enqueues <paramref name="message"/> for delivery.</summary>
    ValueTask EnqueueAsync(EmailMessage message, CancellationToken ct = default);
}
