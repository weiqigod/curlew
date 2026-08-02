namespace ApiTool.Backend.PrChecks;

/// <summary>Helpers for the wire-format pr-check identifier (<c>prc_&lt;32-hex-chars&gt;</c>).</summary>
public static class PrCheckId
{
    private const string Prefix = "prc_";

    /// <summary>
    /// Formats a <see cref="Guid"/> as a pr-check wire identifier.
    /// </summary>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>
    /// Attempts to parse a wire identifier back to a <see cref="Guid"/>.
    /// </summary>
    public static bool TryParse(string? value, out Guid id)
    {
        id = Guid.Empty;
        if (string.IsNullOrEmpty(value) || !value.StartsWith(Prefix, StringComparison.Ordinal))
            return false;
        return Guid.TryParseExact(value[Prefix.Length..], "N", out id);
    }
}
