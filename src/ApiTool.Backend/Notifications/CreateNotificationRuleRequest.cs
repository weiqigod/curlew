namespace ApiTool.Backend.Notifications;

/// <summary>Request body for creating a notification rule.</summary>
public sealed class CreateNotificationRuleRequest
{
    /// <summary>Delivery channel: <c>"slack"</c> or <c>"email"</c>.</summary>
    public string? Channel { get; set; }

    /// <summary>Webhook URL (slack) or email address (email).</summary>
    public string? Target { get; set; }

    /// <summary>List of event names that trigger delivery (e.g. <c>["run_failed"]</c>).</summary>
    public IReadOnlyList<string>? On { get; set; }
}
