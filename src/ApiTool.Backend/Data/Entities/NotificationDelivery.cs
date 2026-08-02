namespace ApiTool.Backend.Data.Entities;

/// <summary>Record of a single notification dispatch attempt.</summary>
public sealed class NotificationDelivery
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>The rule that triggered this delivery.</summary>
    public Guid RuleId { get; set; }

    /// <summary>Denormalized org id for efficient org-scoped queries.</summary>
    public Guid OrgId { get; set; }

    /// <summary>The result that triggered the dispatch, if applicable.</summary>
    public Guid? ResultId { get; set; }

    /// <summary>Channel used for this delivery.</summary>
    public NotificationChannel Channel { get; set; }

    /// <summary>Final delivery status.</summary>
    public NotificationDeliveryStatus Status { get; set; }

    /// <summary>HTTP response code from the webhook (Slack), or null for email/error.</summary>
    public int? ResponseCode { get; set; }

    /// <summary>Number of attempts made (1 = no retries needed).</summary>
    public int AttemptCount { get; set; }

    /// <summary>Error message describing the last failure, if any.</summary>
    public string? ErrorMessage { get; set; }

    /// <summary>UTC timestamp of the final attempt.</summary>
    public DateTime AttemptedAt { get; set; }
}
