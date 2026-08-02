namespace ApiTool.Backend.Subscriptions;

/// <summary>Result from creating a Stripe Billing Portal session.</summary>
/// <param name="PortalUrl">The URL to redirect the user to for billing management.</param>
public sealed record PortalSession(string PortalUrl);
