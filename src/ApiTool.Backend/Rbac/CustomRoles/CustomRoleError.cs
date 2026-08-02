namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Sentinel error codes returned by <see cref="CustomRolesService"/>.</summary>
public enum CustomRoleError
{
    /// <summary>No error.</summary>
    None,

    /// <summary>The requestor lacks the required role (owner only for write operations).</summary>
    PermissionDenied,

    /// <summary>A custom role with the same name already exists in this organization.</summary>
    RoleNameTaken,

    /// <summary>One or more permission keys in the request are not in the catalogue.</summary>
    InvalidPermission,

    /// <summary>The role name is empty, too long, or collides with a built-in role name.</summary>
    InvalidName,

    /// <summary>The referenced role does not exist in this organization.</summary>
    RoleNotFound,

    /// <summary>DELETE was blocked because one or more members still reference this role.</summary>
    RoleInUse,

    /// <summary>The organization could not be found (or the requestor is not a member).</summary>
    OrganizationNotFound,
}
