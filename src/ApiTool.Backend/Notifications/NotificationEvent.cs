namespace ApiTool.Backend.Notifications;

/// <summary>Events that can trigger a notification dispatch.</summary>
public enum NotificationEvent
{
    /// <summary>A test run finished with at least one failure.</summary>
    RunFailed,

    /// <summary>A test run contained flaky tests.</summary>
    Flaky,
}

/// <summary>Helpers for parsing and formatting <see cref="NotificationEvent"/> values.</summary>
internal static class NotificationEventHelper
{
    private const string RunFailedWire = "run_failed";
    private const string FlakyWire = "flaky";
    private const char Separator = '|';

    /// <summary>
    /// Tries to parse a wire-format string (e.g. <c>"run_failed"</c>) to a
    /// <see cref="NotificationEvent"/>.
    /// </summary>
    internal static bool TryParse(string value, out NotificationEvent evt)
    {
        evt = default;
        switch (value)
        {
            case RunFailedWire:
                evt = NotificationEvent.RunFailed;
                return true;
            case FlakyWire:
                evt = NotificationEvent.Flaky;
                return true;
            default:
                return false;
        }
    }

    /// <summary>Formats a <see cref="NotificationEvent"/> to its wire-format string.</summary>
    internal static string Format(NotificationEvent evt) => evt switch
    {
        NotificationEvent.RunFailed => RunFailedWire,
        NotificationEvent.Flaky => FlakyWire,
        _ => evt.ToString().ToLowerInvariant(),
    };

    /// <summary>
    /// Parses a pipe-separated stored string (e.g. <c>"run_failed|flaky"</c>) into a
    /// list of <see cref="NotificationEvent"/> values. Returns null on any invalid token.
    /// </summary>
    internal static IReadOnlyList<NotificationEvent>? ParseStoredEvents(string stored)
    {
        if (string.IsNullOrWhiteSpace(stored))
            return null;

        var parts = stored.Split(Separator);
        var result = new List<NotificationEvent>(parts.Length);
        foreach (var part in parts)
        {
            if (!TryParse(part.Trim(), out var evt))
                return null;
            result.Add(evt);
        }
        return result.Count > 0 ? result : null;
    }

    /// <summary>
    /// Formats a list of events as a pipe-separated stored string.
    /// </summary>
    internal static string FormatStoredEvents(IEnumerable<NotificationEvent> events) =>
        string.Join(Separator, events.Select(Format));
}
