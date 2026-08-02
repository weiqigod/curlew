namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Wire-format helpers for custom role identifiers (<c>role_&lt;32-hex&gt;</c>).</summary>
public static class RoleId
{
    private const string Prefix = "role_";

    /// <summary>Formats a <see cref="Guid"/> as the wire-format role id string.</summary>
    public static string Format(Guid id) => Prefix + id.ToString("N");

    /// <summary>
    /// Tries to parse a wire-format role id string back to a <see cref="Guid"/>.
    /// Returns <see langword="false"/> and sets <paramref name="id"/> to <see cref="Guid.Empty"/>
    /// if the input is invalid.
    /// </summary>
    public static bool TryParse(string? value, out Guid id)
    {
        id = Guid.Empty;
        if (string.IsNullOrEmpty(value) || !value.StartsWith(Prefix, StringComparison.Ordinal))
            return false;
        return Guid.TryParseExact(value[Prefix.Length..], "N", out id);
    }
}
