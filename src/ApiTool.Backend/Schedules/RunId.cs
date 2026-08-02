namespace ApiTool.Backend.Schedules;

/// <summary>Wire-format id helper (<c>run_&lt;32-hex-chars&gt;</c>).</summary>
public static class RunId
{
    private const string Prefix = "run_";

    /// <summary>Formats a <see cref="Guid"/> as a <c>run_</c>-prefixed hex string.</summary>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>
    /// Attempts to parse a <c>run_</c>-prefixed wire-format id back to a <see cref="Guid"/>.
    /// </summary>
    /// <param name="value">The wire-format string (e.g. <c>run_abc123…</c>).</param>
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
