// Test double that captures enqueued messages for assertion in unit/integration tests.
using ApiTool.Backend.Notifications.Email;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>
/// In-memory <see cref="IEmailQueue"/> test double.
/// Captures all enqueued <see cref="EmailMessage"/> instances into a thread-safe list.
/// Register as a singleton in <see cref="TestInfrastructure.BackendFactory"/>
/// to observe email sends from integration tests.
/// </summary>
public sealed class RecordingEmailQueue : IEmailQueue
{
    private readonly List<EmailMessage> _messages = [];

    /// <summary>All messages enqueued so far, in order.</summary>
    public IReadOnlyList<EmailMessage> Messages
    {
        get
        {
            lock (_messages)
                return [.. _messages];
        }
    }

    /// <inheritdoc/>
    public ValueTask EnqueueAsync(EmailMessage message, CancellationToken ct = default)
    {
        lock (_messages)
            _messages.Add(message);
        return ValueTask.CompletedTask;
    }

    /// <summary>Clears all captured messages. Useful for test reset between test runs.</summary>
    public void Clear()
    {
        lock (_messages)
            _messages.Clear();
    }
}
