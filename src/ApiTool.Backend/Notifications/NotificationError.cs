namespace ApiTool.Backend.Notifications;

/// <summary>Error codes for notification operations.</summary>
public enum NotificationError
{
    /// <summary>No error — operation succeeded.</summary>
    None,

    /// <summary>The requesting user does not have permission to perform the action.</summary>
    PermissionDenied,

    /// <summary>The provided channel value is unknown or invalid.</summary>
    InvalidChannel,

    /// <summary>The provided target value is invalid for the given channel.</summary>
    InvalidTarget,

    /// <summary>The provided events list is empty or contains invalid event names.</summary>
    InvalidEvents,

    /// <summary>The requested resource was not found.</summary>
    NotFound,
}
