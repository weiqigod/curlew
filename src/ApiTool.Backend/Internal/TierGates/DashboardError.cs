namespace ApiTool.Backend.Internal.TierGates;

/// <summary>Per-feature error result for dashboard endpoints.</summary>
public enum DashboardError
{
    /// <summary>Tier check passed.</summary>
    None,

    /// <summary>The organisation row does not exist.</summary>
    OrgNotFound,

    /// <summary>The organisation exists but its tier is below the required minimum.</summary>
    TierIneligible,
}
