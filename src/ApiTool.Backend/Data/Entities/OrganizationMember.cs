using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>Records a user's membership and role in an organization.</summary>
public sealed class OrganizationMember
{
    /// <summary>Composite key part: the organization.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Composite key part: the member user.</summary>
    [GdprIncluded(GdprDisposition.InExportInDeletionHard)]
    [GdprUserAttribution]
    public Guid UserId { get; set; }

    /// <summary>The role this user holds in the organization.</summary>
    public OrgRole Role { get; set; }

    /// <summary>UTC timestamp when the user joined (or was added).</summary>
    public DateTime JoinedAt { get; set; }

    /// <summary>Id of the user who invited this member, or <see langword="null"/> for the owner.</summary>
    [GdprIncluded(GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    public Guid? InvitedBy { get; set; }

    /// <summary>Serialized permission list (JSON array of strings). Defaults to empty array.</summary>
    public string PermissionsJson { get; set; } = "[]";

    /// <summary>
    /// Optional FK to an <c>organization_custom_roles</c> row. When set, the member's
    /// effective permissions come from the custom role (plus <see cref="PermissionsJson"/>
    /// overrides); when null, the built-in <see cref="Role"/> applies.
    /// </summary>
    public Guid? RoleId { get; set; }
}
