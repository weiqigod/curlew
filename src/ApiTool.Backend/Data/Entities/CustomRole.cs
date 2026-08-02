using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>A custom permission role defined by an organization.</summary>
public sealed class CustomRole
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>Organization that owns this role.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Human-readable role name, unique within the org.</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>JSON array of permission key strings (e.g. <c>["results.view","results.upload"]</c>).</summary>
    public string PermissionsJson { get; set; } = "[]";

    /// <summary>UTC timestamp when the role was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>
    /// Id of the user who created the role. NULL when the user has been anonymised (M18-006).
    /// </summary>
    [GdprIncluded(GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    public Guid? CreatedBy { get; set; }
}
