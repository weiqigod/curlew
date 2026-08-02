namespace ApiTool.Backend.Data.Entities;

/// <summary>Lifecycle status of an organization.</summary>
public enum OrgStatus
{
    /// <summary>The organization is being provisioned (not yet active).</summary>
    Creating,

    /// <summary>The organization is fully operational.</summary>
    Active,

    /// <summary>The organization is marked for deletion and no longer usable.</summary>
    PendingDeletion,

    /// <summary>The organization has been deleted.</summary>
    Deleted,
}
