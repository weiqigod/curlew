namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Request body for POST /api/v1/organizations/{id}/roles.</summary>
/// <param name="Name">Desired role name (1–100 chars, must not collide with built-in names).</param>
/// <param name="Permissions">List of permission key strings from the catalogue.</param>
public sealed record CreateRoleRequest(string Name, IReadOnlyList<string> Permissions);
