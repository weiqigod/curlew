using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>A logger that records all messages for assertion in tests.</summary>
public sealed class RecordingLogger<T> : ILogger<T>
{
    private readonly List<string> _messages = [];

    /// <summary>Gets all recorded log messages.</summary>
    public IReadOnlyList<string> Messages => _messages;

    /// <inheritdoc/>
    public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;

    /// <inheritdoc/>
    public bool IsEnabled(LogLevel logLevel) => true;

    /// <inheritdoc/>
    public void Log<TState>(LogLevel logLevel, EventId eventId, TState state,
        Exception? exception, Func<TState, Exception?, string> formatter)
    {
        _messages.Add(formatter(state, exception));
    }
}
