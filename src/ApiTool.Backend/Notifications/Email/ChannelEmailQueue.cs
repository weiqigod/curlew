using System.Threading.Channels;

namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// <see cref="IEmailQueue"/> backed by an unbounded <see cref="Channel{T}"/>.
/// Register as a singleton alongside <c>Channel&lt;EmailMessage&gt;</c>.
/// The consumer (<c>EmailQueueProcessor</c>) lands in M14-014.
/// </summary>
public sealed class ChannelEmailQueue(Channel<EmailMessage> channel) : IEmailQueue
{
    /// <inheritdoc/>
    public ValueTask EnqueueAsync(EmailMessage message, CancellationToken ct = default)
        => channel.Writer.WriteAsync(message, ct);
}
