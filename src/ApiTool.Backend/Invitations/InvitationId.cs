namespace ApiTool.Backend.Invitations;

/// <summary>
/// Helpers for the wire-format invitation identifier (<c>inv_&lt;32-hex-chars&gt;</c>).
/// </summary>
public static class InvitationId
{
    private const string Prefix = "inv_";

    /// <summary>Formats a <see cref="Guid"/> as an invitation wire identifier.</summary>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>Attempts to parse a wire identifier back to a <see cref="Guid"/>.</summary>
    public static bool TryParse(string? value, out Guid id)
    {
        id = Guid.Empty;
        if (string.IsNullOrEmpty(value) || !value.StartsWith(Prefix, StringComparison.Ordinal))
            return false;
        return Guid.TryParseExact(value[Prefix.Length..], "N", out id);
    }
}
