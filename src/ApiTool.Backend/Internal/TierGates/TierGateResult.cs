namespace ApiTool.Backend.Internal.TierGates;

/// <summary>Discriminated result of <see cref="ITierGate.EnsureAsync"/>.</summary>
public enum TierGateResult
{
    /// <summary>Tier check passed — the organisation's current tier meets the required minimum.</summary>
    Allowed,

    /// <summary>The organisation row does not exist.</summary>
    OrgNotFound,

    /// <summary>The organisation exists but its tier is below the required minimum.</summary>
    TierIneligible,
}
