using ApiTool.Backend.Logging;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Tests.Logging;

/// <summary>
/// Tests for BearerTokenRedactor — behavior #6: redact Authorization headers and ghs_ tokens.
/// </summary>
public sealed class BearerTokenRedactorTests
{
    [Theory]
    [InlineData("Authorization: Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig",
                "Authorization: Bearer <redacted>")]
    [InlineData("token=ghs_abcdefghijklmnopqrstuvwxyz0123456789",
                "token=<redacted>")]
    [InlineData("Bearer eyJ.x.y and Bearer abc.def.ghi",  // multi-occurrence
                "Bearer <redacted> and Bearer <redacted>")]
    [InlineData("plain text no bearer no ghs", "plain text no bearer no ghs")]
    [InlineData("ghs_short_31chars_aaaaaaaaaaaaa", "ghs_short_31chars_aaaaaaaaaaaaa")]  // < 36 chars: not matched
    public void Redact_replaces_known_patterns(string input, string expected)
    {
        BearerTokenRedactor.Redact(input).Should().Be(expected);
    }

    [Fact]
    public void Logger_wraps_formatter_so_emitted_message_is_redacted()
    {
        var inner = new RecordingLogger();
        var redactor = new BearerTokenRedactor(new SingleLoggerProvider(inner));
        var wrapped = redactor.CreateLogger("test");
        wrapped.LogInformation("Authorization header was 'Bearer eyJabc.def.ghi'");

        inner.Lines.Should().ContainSingle()
            .Which.Should().Be("Authorization header was 'Bearer <redacted>'");
    }

    /// <summary>An ILogger that records all formatted message strings.</summary>
    private sealed class RecordingLogger : ILogger
    {
        public List<string> Lines { get; } = new();
        public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;
        public bool IsEnabled(LogLevel logLevel) => true;
        public void Log<TState>(LogLevel logLevel, EventId eventId, TState state, Exception? exception,
            Func<TState, Exception?, string> formatter)
            => Lines.Add(formatter(state, exception));
    }

    /// <summary>An ILoggerProvider that returns the same ILogger for every category.</summary>
    private sealed class SingleLoggerProvider(ILogger logger) : ILoggerProvider
    {
        public ILogger CreateLogger(string categoryName) => logger;
        public void Dispose() { }
    }
}
