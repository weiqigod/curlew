namespace ApiTool.Backend.Subscriptions;

/// <summary>Request body for POST /api/v1/subscriptions/preview-proration.</summary>
/// <param name="NewPriceId">The Stripe price id to swap the active subscription to.</param>
/// <param name="SeatCount">Optional new seat count; defaults to the current subscription's seat count when null.</param>
public sealed record PreviewProrationRequest(string NewPriceId, int? SeatCount);
