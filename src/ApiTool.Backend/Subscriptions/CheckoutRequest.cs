namespace ApiTool.Backend.Subscriptions;

/// <summary>Request body for POST /api/v1/subscriptions/checkout.</summary>
/// <param name="OrgId">Wire-format org id to subscribe. Required when using the legacy tier/interval/seat_count path.</param>
/// <param name="Tier">Target subscription tier (e.g. <c>team</c>). Required on the legacy path; ignored on the price_id path.</param>
/// <param name="Interval">Billing interval: <c>month</c> or <c>year</c>. Optional; defaults to <c>month</c> when omitted on the legacy path.</param>
/// <param name="SeatCount">Number of seats to purchase. Required on the legacy path; ignored on the price_id path.</param>
/// <param name="SuccessUrl">URL to redirect on successful payment.</param>
/// <param name="CancelUrl">URL to redirect on cancellation.</param>
/// <param name="PriceId">
/// Optional Stripe price id (e.g. <c>price_test_solo_monthly</c>).
/// When supplied, the price_id path is used: org_id/tier/interval/seat_count are derived or ignored,
/// and the bearer-validated org_id is always the one billed.
/// </param>
public sealed record CheckoutRequest(
    string? OrgId,
    string? Tier,
    string? Interval,
    int? SeatCount,
    string SuccessUrl,
    string CancelUrl,
    string? PriceId = null);
