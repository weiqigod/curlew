using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>Persisted notification rule: when to alert and where to send it.</summary>
public sealed class NotificationRule
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>The organization this rule belongs to.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Delivery channel: slack or email.</summary>
    public NotificationChannel Channel { get; set; }

    /// <summary>Webhook URL (Slack) or email address (Email).</summary>
    public string Target { get; set; } = string.Empty;

    /// <summary>Pipe-separated event names, e.g. <c>"run_failed|flaky"</c>.</summary>
    public string OnEvents { get; set; } = string.Empty;

    /// <summary>
    /// The user who created this rule. NULL when the user has been anonymised (M18-006).
    /// </summary>
    [GdprIncluded(GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    public Guid? CreatedBy { get; set; }

    /// <summary>UTC timestamp of rule creation.</summary>
    public DateTime CreatedAt { get; set; }
}
