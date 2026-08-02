namespace ApiTool.Backend.Organizations;

/// <summary>Request body for PATCH /api/v1/organizations/{id}.</summary>
/// <param name="Name">Optional new display name.</param>
/// <param name="AuditLogRetentionDays">
/// Optional audit log retention window in days. Must be &gt; 0.
/// Values &gt; 365 require an Enterprise subscription (v4-2).
/// </param>
public sealed record UpdateOrganizationRequest(string? Name, int? AuditLogRetentionDays);
