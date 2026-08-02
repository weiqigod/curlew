namespace ApiTool.Backend.Data.Entities;

/// <summary>Membership role within an organization.</summary>
public enum OrgRole
{
    /// <summary>Full administrative access; the organization creator.</summary>
    Owner,

    /// <summary>Administrative access delegated by the owner.</summary>
    Admin,

    /// <summary>Standard membership with read and run access.</summary>
    Member,
}
