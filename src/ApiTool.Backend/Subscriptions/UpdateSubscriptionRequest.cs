namespace ApiTool.Backend.Subscriptions;

/// <summary>Request body for PATCH /api/v1/subscriptions/{id}.</summary>
/// <param name="Tier">Optional new tier; <see langword="null"/> to keep current.</param>
/// <param name="SeatCount">Optional new seat count; <see langword="null"/> to keep current.</param>
/// <param name="Interval">Optional new billing interval; <see langword="null"/> to keep current.</param>
public sealed record UpdateSubscriptionRequest(
    string? Tier,
    int? SeatCount,
    string? Interval);
