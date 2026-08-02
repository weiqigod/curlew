// Refs docs/SPECIFICATION.md:8466 + :8631 (token handling rules + redaction patterns).
// Two patterns are scrubbed before any log emission:
//   1. Authorization: Bearer <token> → Authorization: Bearer <redacted>
//   2. ghs_<36+ chars>              → <redacted>
// The filter wraps every ILogger so the scrubbing happens regardless of the
// underlying sink (console, file, ApplicationInsights, etc.).
// Note: the Bearer pattern is case-sensitive, matching the canonical capitalisation only.
// If lower-case 'bearer' appears in log lines in practice, extend the regex options.
using System.Text.RegularExpressions;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Logging;

/// <summary>
/// An <see cref="ILoggerProvider"/> wrapper that intercepts log messages and redacts
/// <c>Authorization: Bearer ...</c> values and <c>ghs_*</c> GitHub installation tokens
/// before forwarding to the inner provider.
/// </summary>
public sealed partial class BearerTokenRedactor : ILoggerProvider
{
    private readonly ILoggerProvider _inner;

    /// <summary>Wraps <paramref name="inner"/> with a redacting layer.</summary>
    public BearerTokenRedactor(ILoggerProvider inner) => _inner = inner;

    /// <inheritdoc/>
    public ILogger CreateLogger(string categoryName)
        => new RedactingLogger(_inner.CreateLogger(categoryName));

    /// <inheritdoc/>
    public void Dispose() => _inner.Dispose();

    /// <summary>Pattern matching <c>Bearer &lt;token&gt;</c> (canonical capitalisation).</summary>
    [GeneratedRegex(@"Bearer\s+[A-Za-z0-9._\-]+", RegexOptions.Compiled)]
    internal static partial Regex BearerPattern();

    /// <summary>Pattern matching GitHub installation tokens of 36+ alphanumeric chars.</summary>
    [GeneratedRegex(@"ghs_[A-Za-z0-9]{36,}", RegexOptions.Compiled)]
    internal static partial Regex GhsPattern();

    /// <summary>
    /// Applies both redaction patterns to <paramref name="input"/>.
    /// Returns the input unchanged if it is null or empty.
    /// </summary>
    public static string Redact(string input)
    {
        if (string.IsNullOrEmpty(input)) return input;
        var s = BearerPattern().Replace(input, "Bearer <redacted>");
        s = GhsPattern().Replace(s, "<redacted>");
        return s;
    }

    private sealed class RedactingLogger(ILogger inner) : ILogger
    {
        public IDisposable? BeginScope<TState>(TState state) where TState : notnull => inner.BeginScope(state);
        public bool IsEnabled(LogLevel logLevel) => inner.IsEnabled(logLevel);

        public void Log<TState>(LogLevel logLevel, EventId eventId, TState state, Exception? exception,
            Func<TState, Exception?, string> formatter)
        {
            // Wrap the formatter so the rendered message is scrubbed before emission.
            string Wrapped(TState s, Exception? e) => Redact(formatter(s, e));
            inner.Log(logLevel, eventId, state, exception, Wrapped);
        }
    }
}
