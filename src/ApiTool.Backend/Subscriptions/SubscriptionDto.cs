namespace ApiTool.Backend.Subscriptions;

/// <summary>Wire model for a subscription resource.</summary>
/// <param name="Id">Wire-format subscription id (<c>sub_&lt;hex&gt;</c>).</param>
/// <param name="OrgId">Wire-format organization id (<c>org_&lt;hex&gt;</c>).</param>
/// <param name="Tier">Subscription tier string (e.g. <c>team</c>).</param>
/// <param name="Status">Lifecycle status string (e.g. <c>active</c>).</param>
/// <param name="Interval">Billing interval: <c>month</c> or <c>year</c>.</param>
/// <param name="SeatCount">Number of seats being billed.</param>
/// <param name="SeatLimit">Maximum seat ceiling.</param>
/// <param name="CurrentPeriodStart">Start of the current billing period (UTC).</param>
/// <param name="CurrentPeriodEnd">End of the current billing period (UTC).</param>
/// <param name="CancelAtPeriodEnd">Whether the subscription cancels at period end.</param>
/// <param name="CreatedAt">UTC creation timestamp.</param>
public sealed record SubscriptionDto(
    string Id,
    string OrgId,
    string Tier,
    string Status,
    string Interval,
    int SeatCount,
    int SeatLimit,
    DateTime CurrentPeriodStart,
    DateTime CurrentPeriodEnd,
    bool CancelAtPeriodEnd,
    DateTime CreatedAt);
