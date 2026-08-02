// Refs docs/SPECIFICATION.md:8409-8427 (lifecycle), :10006-10025 (schema).
using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Registry of customer GitHub App installations. Primary cross-tenant isolation surface.
/// Refs docs/SPECIFICATION.md:8409-8427 (lifecycle), :10006-10025 (schema).
/// </summary>
[GdprTable(GdprTableKind.ExcludedFromBoth)]
public sealed class GithubInstallation
{
    /// <summary>GitHub's installation ID — primary key per spec :10010.</summary>
    public long InstallationId { get; set; }

    /// <summary>Our GitHub App's numeric App ID — one per ApiTool product (spec :8387).</summary>
    public long AppId { get; set; }

    /// <summary>NULL until the customer claims the install (webhook-first path).</summary>
    public Guid? OrgId { get; set; }

    /// <summary>e.g. 'acme-corp' (display only).</summary>
    public string AccountLogin { get; set; } = string.Empty;

    /// <summary>'Organization' or 'User'.</summary>
    public string AccountType { get; set; } = string.Empty;

    /// <summary>'all' or 'selected'.</summary>
    public string RepoSelection { get; set; } = "selected";

    /// <summary>JSON array of {owner, name, id}. JSONB on Postgres, TEXT on SQLite.</summary>
    public string RepoSetJson { get; set; } = "[]";

    /// <summary>Insert timestamp (UTC).</summary>
    public DateTime InstalledAt { get; set; }

    /// <summary>Set when the user claims the install (or instantly on dashboard-initiated path).</summary>
    public DateTime? ClaimedAt { get; set; }

    /// <summary>Non-null while suspended via installation.suspend webhook.</summary>
    public DateTime? SuspendedAt { get; set; }

    /// <summary>Soft-delete marker; never hard-deleted (spec :8423).</summary>
    public DateTime? DeletedAt { get; set; }

    /// <summary>Last time the daily reconciler synced repo_set against the GitHub API.</summary>
    public DateTime LastReconciledAt { get; set; }
}
