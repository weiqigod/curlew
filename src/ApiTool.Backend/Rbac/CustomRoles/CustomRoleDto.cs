namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Wire DTO for a custom (or built-in) role.</summary>
/// <param name="Id">Wire-format role id (e.g. <c>role_abc123…</c>), or the built-in name for built-in roles.</param>
/// <param name="Name">Human-readable role name.</param>
/// <param name="Permissions">List of permission key strings belonging to this role.</param>
/// <param name="IsBuiltin">True if this is a built-in (non-deletable) role.</param>
/// <param name="CreatedAt">When the custom role was created. <see langword="null"/> for built-in roles.</param>
public sealed record CustomRoleDto(
    string Id,
    string Name,
    IReadOnlyList<string> Permissions,
    bool IsBuiltin,
    DateTime? CreatedAt);
