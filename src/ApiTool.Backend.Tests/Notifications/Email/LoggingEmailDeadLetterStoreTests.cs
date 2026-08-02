using ApiTool.Backend.Notifications.Email;
using FluentAssertions;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class LoggingEmailDeadLetterStoreTests
{
    [Fact]
    public async Task RecordAsync_logs_at_error_level_with_dead_letter_token()
    {
        var collector = new TestLogCollector<LoggingEmailDeadLetterStore>();
        var store = new LoggingEmailDeadLetterStore(collector.Logger);
        var msg = new EmailMessage("to@example.com", "email_verification",
            new Dictionary<string, string> { ["first_name"] = "Alex" },
            DateTimeOffset.UtcNow);

        await store.RecordAsync(msg, "SendGrid 503", 3, CancellationToken.None);

        collector.Entries.Should().ContainSingle();
        var entry = collector.Entries[0];
        entry.Level.Should().Be(LogLevel.Error);
        entry.Message.Should().Contain("email_dead_letter");
        entry.Message.Should().Contain("to@example.com");
        entry.Message.Should().Contain("email_verification");
        entry.Message.Should().Contain("SendGrid 503");
    }

    [Fact]
    public async Task RecordAsync_returns_completed_task()
    {
        var collector = new TestLogCollector<LoggingEmailDeadLetterStore>();
        var store = new LoggingEmailDeadLetterStore(collector.Logger);
        var msg = new EmailMessage("to@x.com", "slug", new Dictionary<string, string>(), DateTimeOffset.UtcNow);

        // Should complete without throwing.
        await store.RecordAsync(msg, "err", 1, CancellationToken.None);
    }
}

/// <summary>Captures log entries for assertions in tests.</summary>
internal sealed class TestLogCollector<T>
{
    private readonly List<(LogLevel Level, string Message)> _entries = [];

    public IReadOnlyList<(LogLevel Level, string Message)> Entries => _entries;

    public ILogger<T> Logger { get; }

    public TestLogCollector()
    {
        var factory = LoggerFactory.Create(b =>
            b.AddProvider(new DelegatingLoggerProvider(
                (level, msg) => _entries.Add((level, msg)))));
        Logger = factory.CreateLogger<T>();
    }
}

internal sealed class DelegatingLoggerProvider(Action<LogLevel, string> onLog) : ILoggerProvider
{
    public ILogger CreateLogger(string categoryName) => new DelegatingLogger(onLog);
    public void Dispose() { }
}

internal sealed class DelegatingLogger(Action<LogLevel, string> onLog) : ILogger
{
    public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;
    public bool IsEnabled(LogLevel logLevel) => true;

    public void Log<TState>(LogLevel logLevel, EventId eventId, TState state,
        Exception? exception, Func<TState, Exception?, string> formatter)
    {
        onLog(logLevel, formatter(state, exception));
    }
}
