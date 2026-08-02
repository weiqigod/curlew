namespace ApiTool.Backend.Organizations;

/// <summary>
/// Helpers for the wire-format organization identifier (<c>org_&lt;32-hex-chars&gt;</c>).
/// </summary>
public static class OrgId
{
    private const string Prefix = "org_";

    /// <summary>
    /// Formats a <see cref="Guid"/> as an org wire identifier.
    /// </summary>
    /// <param name="id">The internal UUID.</param>
    /// <returns>A string in the form <c>org_&lt;32-hex-chars&gt;</c>.</returns>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>
    /// Attempts to parse a wire identifier back to a <see cref="Guid"/>.
    /// </summary>
    /// <param name="value">The wire identifier (e.g. <c>org_abc123…</c>).</param>
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

    /// <summary>
    /// Converts a wire identifier back to its raw <see cref="Guid"/> string (format "D").
    /// Intended for test use where a <see cref="Guid"/> is needed from a wire id.
    /// </summary>
    /// <param name="wireId">The wire identifier.</param>
    /// <returns>The <see cref="Guid"/> formatted with dashes.</returns>
    public static string ToGuidString(string wireId)
    {
        if (!TryParse(wireId, out var id))
            throw new ArgumentException($"Invalid org id: '{wireId}'", nameof(wireId));
        return id.ToString("D");
    }
}
