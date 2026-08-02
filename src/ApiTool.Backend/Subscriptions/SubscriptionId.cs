namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Helpers for the wire-format subscription identifier (<c>sub_&lt;32-hex-chars&gt;</c>).
/// </summary>
public static class SubscriptionId
{
    private const string Prefix = "sub_";

    /// <summary>
    /// Formats a <see cref="Guid"/> as a subscription wire identifier.
    /// </summary>
    /// <param name="id">The internal UUID.</param>
    /// <returns>A string in the form <c>sub_&lt;32-hex-chars&gt;</c>.</returns>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>
    /// Attempts to parse a wire identifier back to a <see cref="Guid"/>.
    /// </summary>
    /// <param name="value">The wire identifier (e.g. <c>sub_abc123…</c>).</param>
    /// <param name="id">The parsed <see cref="Guid"/> on success.</param>
    /// <returns><see langword="true"/> if parsing succeeded; otherwise <see langword="false"/>.</returns>
    public static bool TryParse(string? value, out Guid id)
    {
        id = Guid.Empty;

        if (string.IsNullOrEmpty(value) || !value.StartsWith(Prefix, StringComparison.Ordinal))
            return false;

        var hex = value[Prefix.Length..];
        return Guid.TryParseExact(hex, "N", out id);
    }
}
