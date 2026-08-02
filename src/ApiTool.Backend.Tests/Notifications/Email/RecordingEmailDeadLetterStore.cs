using ApiTool.Backend.Notifications.Email;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>Test double for <see cref="IEmailDeadLetterStore"/>. Records dead-letter events for assertion.</summary>
public sealed class RecordingEmailDeadLetterStore : IEmailDeadLetterStore
{
    private readonly List<(EmailMessage Message, string LastError, int AttemptCount)> _entries = [];

    /// <summary>All recorded dead-letter events in order.</summary>
    public IReadOnlyList<(EmailMessage Message, string LastError, int AttemptCount)> Entries => _entries;

    /// <inheritdoc/>
    public Task RecordAsync(EmailMessage message, string lastError, int attemptCount, CancellationToken ct)
    {
        _entries.Add((message, lastError, attemptCount));
        return Task.CompletedTask;
    }
}
