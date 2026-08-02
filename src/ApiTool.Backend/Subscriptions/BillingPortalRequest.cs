namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Request body for <c>POST /api/v1/subscriptions/billing-portal</c>.
/// All fields optional — when <see cref="ReturnUrl"/> is null the endpoint defaults to
/// <c>App:WebAppUrl + "/billing"</c> per Open Decision #2 (milestone-mapping.md).
/// </summary>
public sealed record BillingPortalRequest(string? ReturnUrl = null);

/// <summary>Response body for <c>POST /api/v1/subscriptions/billing-portal</c>. The <c>url</c> field carries the Stripe billing portal session URL.</summary>
public sealed record BillingPortalResponse(string Url);
