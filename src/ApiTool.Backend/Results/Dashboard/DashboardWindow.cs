namespace ApiTool.Backend.Results.Dashboard;

/// <summary>Allowed dashboard time windows per spec ("7d | 30d (default) | 90d"). Open Decision 10.</summary>
public enum DashboardWindow
{
    /// <summary>7-day window.</summary>
    SevenDays,

    /// <summary>30-day window (default).</summary>
    ThirtyDays,

    /// <summary>90-day window.</summary>
    NinetyDays,
}

/// <summary>Parsing and conversion helpers for <see cref="DashboardWindow"/>.</summary>
public static class DashboardWindowExtensions
{
    /// <summary>Default window when <c>?window=</c> is omitted.</summary>
    public const DashboardWindow Default = DashboardWindow.ThirtyDays;

    /// <summary>Allowed wire values, for error messages.</summary>
    public static readonly IReadOnlyList<string> AllowedValues = ["7d", "30d", "90d"];

    /// <summary>
    /// Parses a wire value (<c>"7d"</c>, <c>"30d"</c>, <c>"90d"</c>, or null/empty for default).
    /// Returns <see langword="false"/> for any other value (caller emits 400 unsupported-window).
    /// </summary>
    /// <param name="value">Wire string from <c>?window=</c> query parameter.</param>
    /// <param name="window">Parsed window on success; <see cref="Default"/> on failure.</param>
    /// <returns><see langword="true"/> when parsing succeeded.</returns>
    public static bool TryParse(string? value, out DashboardWindow window)
    {
        if (string.IsNullOrEmpty(value))
        {
            window = Default;
            return true;
        }

        window = value switch
        {
            "7d"  => DashboardWindow.SevenDays,
            "30d" => DashboardWindow.ThirtyDays,
            "90d" => DashboardWindow.NinetyDays,
            _     => default,
        };

        if (value is "7d" or "30d" or "90d")
            return true;

        window = Default;
        return false;
    }

    /// <summary>Returns the window's span as a <see cref="TimeSpan"/> (7, 30, or 90 days).</summary>
    public static TimeSpan ToTimeSpan(this DashboardWindow window) => window switch
    {
        DashboardWindow.SevenDays  => TimeSpan.FromDays(7),
        DashboardWindow.ThirtyDays => TimeSpan.FromDays(30),
        DashboardWindow.NinetyDays => TimeSpan.FromDays(90),
        _ => throw new ArgumentOutOfRangeException(nameof(window)),
    };

    /// <summary>Returns the wire form (e.g. <c>"30d"</c>).</summary>
    public static string ToWire(this DashboardWindow window) => window switch
    {
        DashboardWindow.SevenDays  => "7d",
        DashboardWindow.ThirtyDays => "30d",
        DashboardWindow.NinetyDays => "90d",
        _ => throw new ArgumentOutOfRangeException(nameof(window)),
    };
}
