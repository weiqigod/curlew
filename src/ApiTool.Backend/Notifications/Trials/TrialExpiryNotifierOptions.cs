namespace ApiTool.Backend.Notifications.Trials;

/// <summary>
/// Configuration options for <see cref="TrialExpiryNotifier"/>.
/// Bound from the <c>ApiTool:TrialExpiryNotifier</c> configuration section.
/// Refs docs/SPECIFICATION.md:5855 (daily 09:00 UTC cron).
/// </summary>
public sealed class TrialExpiryNotifierOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:TrialExpiryNotifier";

    /// <summary>
    /// Daily UTC fire time as <c>HH:mm</c> or <c>HH:mm:ss</c>. Defaults to <c>"09:00"</c>.
    /// The literal sentinel <c>"now"</c> (case-insensitive) forces immediate firing on
    /// every tick — used by tests and the observable command in <c>M16-008.yaml</c>.
    /// </summary>
    public string RunAtUtc { get; set; } = "09:00";

    /// <summary>
    /// Maximum number of trial rows queried per pass per tick. Defaults to <c>500</c>.
    /// </summary>
    public int BatchSize { get; set; } = 500;

    /// <summary>
    /// True when <see cref="RunAtUtc"/> equals the sentinel <c>"now"</c>.
    /// </summary>
    public bool IsForcedFire =>
        string.Equals(RunAtUtc, "now", StringComparison.OrdinalIgnoreCase);

    /// <summary>
    /// Parses <see cref="RunAtUtc"/> as a UTC time-of-day. Throws when the
    /// string is neither <c>"now"</c> nor a valid <c>HH:mm</c>/<c>HH:mm:ss</c>.
    /// </summary>
    public TimeOnly ParseRunAtUtc()
    {
        if (IsForcedFire)
            return new TimeOnly(0, 0); // sentinel — never consulted
        if (TimeOnly.TryParseExact(RunAtUtc, "HH:mm", out var hm))
            return hm;
        if (TimeOnly.TryParseExact(RunAtUtc, "HH:mm:ss", out var hms))
            return hms;
        throw new FormatException(
            $"ApiTool:TrialExpiryNotifier:RunAtUtc must be \"now\" or \"HH:mm\"/\"HH:mm:ss\"; got \"{RunAtUtc}\".");
    }
}
