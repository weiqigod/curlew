namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Full-replacement body for PATCH /api/v1/organizations/{id}/roles/{roleId}.</summary>
/// <param name="Name">Replacement role name.</param>
/// <param name="Permissions">Complete replacement permission set.</param>
public sealed record UpdateRoleRequest(string Name, IReadOnlyList<string> Permissions);
