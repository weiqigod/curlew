namespace ApiTool.Backend.Data.Entities;

/// <summary>Status of a single notification delivery attempt.</summary>
public enum NotificationDeliveryStatus
{
    /// <summary>Delivery is pending (not yet attempted or in-flight).</summary>
    Pending,

    /// <summary>Delivery succeeded — the target accepted the payload.</summary>
    Delivered,

    /// <summary>Delivery failed after exhausting all retries.</summary>
    Failed,
}
