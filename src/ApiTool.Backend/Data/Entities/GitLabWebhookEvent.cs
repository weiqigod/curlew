// Refs docs/SPECIFICATION.md:10922-10943 (schema), :9261 (idempotency contract).
namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Idempotency table for inbound GitLab webhook deliveries.
/// Refs docs/SPECIFICATION.md:10922-10943 (schema), :9261 (idempotency contract).
/// </summary>
public sealed class GitLabWebhookEvent
{
    /// <summary>Surrogate primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>Value of the X-Gitlab-Event-UUID header (UNIQUE).</summary>
    public string EventUuid { get; set; } = string.Empty;

    /// <summary>e.g. "Pipeline Hook", "Push Hook", "Merge Request Hook".</summary>
    public string EventType { get; set; } = string.Empty;

    /// <summary>FK gitlab_installations(id). ON DELETE CASCADE.</summary>
    public Guid InstallationId { get; set; }

    /// <summary>UTC moment of receipt.</summary>
    public DateTime ReceivedAt { get; set; }

    /// <summary>UTC moment processing succeeded; null while pending/quarantined.</summary>
    public DateTime? ProcessedAt { get; set; }

    /// <summary>Increments to 5 then quarantines.</summary>
    public int FailureCount { get; set; }

    /// <summary>Set when failure_count reaches 5; events with this set skip dispatch.</summary>
    public DateTime? QuarantinedAt { get; set; }

    /// <summary>Raw JSON event payload — TEXT on SQLite, jsonb on Postgres.</summary>
    public string PayloadJson { get; set; } = "{}";
}
