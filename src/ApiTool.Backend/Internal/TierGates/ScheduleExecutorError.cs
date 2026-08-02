namespace ApiTool.Backend.Internal.TierGates;

/// <summary>Per-feature error result for schedule-executor endpoints.</summary>
public enum ScheduleExecutorError
{
    /// <summary>Tier check passed.</summary>
    None,

    /// <summary>The organisation row does not exist.</summary>
    OrgNotFound,

    /// <summary>The organisation exists but its tier is below the required minimum.</summary>
    TierIneligible,
}
