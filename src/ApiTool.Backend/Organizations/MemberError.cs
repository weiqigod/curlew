namespace ApiTool.Backend.Organizations;

/// <summary>Well-known error codes for member management operations.</summary>
public enum MemberError
{
    /// <summary>No error.</summary>
    None,

    /// <summary>The requesting user does not have permission.</summary>
    PermissionDenied,

    /// <summary>The organization was not found or the user is not a member.</summary>
    OrganizationNotFound,

    /// <summary>The member was not found.</summary>
    MemberNotFound,

    /// <summary>The owner cannot be removed.</summary>
    CannotRemoveOwner,

    /// <summary>The owner's role cannot be changed directly. Use transfer ownership.</summary>
    CannotChangeOwnerRole,

    /// <summary>The owner cannot leave the organization.</summary>
    OwnerCannotLeave,

    /// <summary>Transfer target is not an admin or is not a member.</summary>
    InvalidTransferTarget,

    /// <summary>The organization is not in a deletable state.</summary>
    InvalidState,

    /// <summary>audit_log_retention_days exceeds the 365-day cap for non-Enterprise orgs (v4-2).</summary>
    RetentionDaysExceedsCap,

    /// <summary>audit_log_retention_days must be a positive integer.</summary>
    RetentionDaysInvalid,
}
