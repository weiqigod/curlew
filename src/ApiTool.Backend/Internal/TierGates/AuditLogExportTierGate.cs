namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data.Entities;
using System.Diagnostics;

/// <summary>
/// Per-feature adapter for audit-log bulk-export endpoints. Requires Enterprise tier (v4.4 decision v4-1).
/// Spec: docs/SPECIFICATION.md "Audit Log Export &amp; Retention" → Bulk Export Endpoint.
/// </summary>
public static class AuditLogExportTierGate
{
    /// <summary>The minimum subscription tier required to bulk-export audit-log entries.</summary>
    public const SubscriptionTier RequiredMinimum = SubscriptionTier.Enterprise;

    /// <summary>
    /// Delegates to <see cref="ITierGate.EnsureAsync"/> with <see cref="RequiredMinimum"/>
    /// and maps the result to <see cref="AuditLogExportError"/>.
    /// </summary>
    public static async Task<AuditLogExportError> EnsureEnterpriseAsync(
        ITierGate gate, Guid orgId, CancellationToken ct) =>
        await gate.EnsureAsync(orgId, RequiredMinimum, ct) switch
        {
            TierGateResult.Allowed        => AuditLogExportError.None,
            TierGateResult.OrgNotFound    => AuditLogExportError.OrgNotFound,
            TierGateResult.TierIneligible => AuditLogExportError.TierIneligible,
            _ => throw new UnreachableException(),
        };
}
