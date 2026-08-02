namespace ApiTool.Backend.Notifications;

/// <summary>Wire-format id helper (<c>nrule_&lt;32-hex-chars&gt;</c>).</summary>
public static class NotificationRuleId
{
    private const string Prefix = "nrule_";

    /// <summary>Formats a <see cref="Guid"/> as a <c>nrule_</c>-prefixed hex string.</summary>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>
    /// Attempts to parse a <c>nrule_</c>-prefixed wire-format id back to a <see cref="Guid"/>.
    /// </summary>
    /// <param name="value">The wire-format string.</param>
    /// <param name="id">The parsed <see cref="Guid"/> on success; <see cref="Guid.Empty"/> otherwise.</param>
    /// <returns><see langword="true"/> if parsing succeeded.</returns>
    public static bool TryParse(string? value, out Guid id)
    {
        id = Guid.Empty;
        if (string.IsNullOrEmpty(value) || !value.StartsWith(Prefix, StringComparison.Ordinal))
            return false;
        return Guid.TryParseExact(value[Prefix.Length..], "N", out id);
    }
}
