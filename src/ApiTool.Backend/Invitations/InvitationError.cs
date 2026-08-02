namespace ApiTool.Backend.Invitations;

/// <summary>Well-known error codes for invitation operations.</summary>
public enum InvitationError
{
    /// <summary>No error.</summary>
    None,

    /// <summary>The requesting user does not have permission for this action.</summary>
    PermissionDenied,

    /// <summary>The organization was not found or the user is not a member.</summary>
    OrganizationNotFound,

    /// <summary>The invitation was not found.</summary>
    InvitationNotFound,

    /// <summary>A pending invitation for this email already exists.</summary>
    InvitationPending,

    /// <summary>The invitation token has expired.</summary>
    InvitationExpired,

    /// <summary>The user is already a member of the organization.</summary>
    AlreadyMember,

    /// <summary>The organization's seat limit has been reached.</summary>
    SeatLimitReached,

    /// <summary>The invitation has exceeded the maximum resend count.</summary>
    ResendLimitReached,

    /// <summary>The resend cooldown period has not elapsed.</summary>
    ResendCooldown,

    /// <summary>The provided email address is empty or whitespace.</summary>
    InvalidEmail,
}
