namespace ApiTool.Backend.Notifications;

/// <summary>Wire representation of a notification rule.</summary>
/// <param name="Id">Wire-format id (e.g. <c>nrule_abc…</c>).</param>
/// <param name="Channel">Channel name in snake_case.</param>
/// <param name="Target">Webhook URL or email address.</param>
/// <param name="On">List of event names that trigger this rule.</param>
/// <param name="CreatedAt">UTC creation timestamp.</param>
public sealed record NotificationRuleDto(
    string Id,
    string Channel,
    string Target,
    IReadOnlyList<string> On,
    DateTime CreatedAt);
