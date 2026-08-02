namespace ApiTool.Backend.Schedules;

/// <summary>Error codes for schedule operations.</summary>
public enum ScheduleError
{
    /// <summary>No error — operation succeeded.</summary>
    None,

    /// <summary>The requesting user does not have permission to perform the action.</summary>
    PermissionDenied,

    /// <summary>The provided cron expression could not be parsed.</summary>
    InvalidCron,

    /// <summary>A schedule with the given name already exists in this organization.</summary>
    ScheduleNameTaken,

    /// <summary>The requested schedule was not found.</summary>
    NotFound,

    /// <summary>The schedule name is null or empty.</summary>
    InvalidName,

    /// <summary>The collection_ref is null or empty.</summary>
    InvalidCollectionRef,

    /// <summary>The timezone is not a recognized IANA identifier.</summary>
    InvalidTimezone,
}
