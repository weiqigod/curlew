namespace ApiTool.Backend.Internal.TierGates;

/// <summary>Per-feature error result for audit-log bulk export.</summary>
public enum AuditLogExportError
{
    /// <summary>Tier check passed.</summary>
    None,

    /// <summary>The organisation row does not exist.</summary>
    OrgNotFound,

    /// <summary>The organisation exists but its tier is below Enterprise.</summary>
    TierIneligible,
}
