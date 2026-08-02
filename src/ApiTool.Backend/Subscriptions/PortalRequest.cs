namespace ApiTool.Backend.Subscriptions;

/// <summary>Request body for POST /api/v1/subscriptions/portal.</summary>
/// <param name="OrgId">Wire-format org id.</param>
/// <param name="ReturnUrl">URL to redirect after the portal session ends.</param>
public sealed record PortalRequest(string OrgId, string ReturnUrl);
