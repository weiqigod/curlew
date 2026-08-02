namespace ApiTool.Backend.Notifications;

/// <summary>Wire representation of a notification delivery attempt.</summary>
/// <param name="Id">Wire-format delivery id (e.g. <c>ndel_abc…</c>).</param>
/// <param name="RuleId">Wire-format rule id.</param>
/// <param name="Channel">Channel name in snake_case.</param>
/// <param name="Status">Delivery status: <c>"pending"</c>, <c>"delivered"</c>, or <c>"failed"</c>.</param>
/// <param name="ResponseCode">HTTP response code from the target, or null.</param>
/// <param name="AttemptCount">Number of attempts made.</param>
/// <param name="ErrorMessage">Error message from last failure, or null.</param>
/// <param name="AttemptedAt">UTC timestamp of the final attempt.</param>
public sealed record NotificationDeliveryDto(
    string Id,
    string RuleId,
    string Channel,
    string Status,
    int? ResponseCode,
    int AttemptCount,
    string? ErrorMessage,
    DateTime AttemptedAt);
