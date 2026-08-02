namespace ApiTool.Backend.Data.Entities;

/// <summary>The delivery channel for a notification rule.</summary>
public enum NotificationChannel
{
    /// <summary>Slack incoming webhook.</summary>
    Slack,

    /// <summary>Email via SMTP.</summary>
    Email,
}
