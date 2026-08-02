namespace ApiTool.Backend.Coordinator;

/// <summary>Helpers for the wire-format coordinator job identifier (<c>job_&lt;32-hex-chars&gt;</c>).</summary>
public static class CoordinatorJobId
{
    private const string Prefix = "job_";

    /// <summary>
    /// Formats a <see cref="Guid"/> as a coordinator job wire identifier.
    /// </summary>
    /// <param name="id">The internal UUID.</param>
    /// <returns>A string in the form <c>job_&lt;32-hex-chars&gt;</c>.</returns>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>
    /// Attempts to parse a wire identifier back to a <see cref="Guid"/>.
    /// </summary>
    /// <param name="value">The wire identifier (e.g. <c>job_abc123…</c>).</param>
    /// <param name="id">The parsed <see cref="Guid"/> on success.</param>
    /// <returns><see langword="true"/> if parsing succeeded; otherwise <see langword="false"/>.</returns>
    public static bool TryParse(string? value, out Guid id)
    {
        id = Guid.Empty;
        if (string.IsNullOrEmpty(value) || !value.StartsWith(Prefix, StringComparison.Ordinal))
            return false;
        return Guid.TryParseExact(value[Prefix.Length..], "N", out id);
    }
}
