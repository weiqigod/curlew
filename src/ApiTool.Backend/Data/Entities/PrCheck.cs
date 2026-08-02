namespace ApiTool.Backend.Data.Entities;

/// <summary>A PR check posted by the CLI after uploading a result.</summary>
public sealed class PrCheck
{
    /// <summary>Primary key (serialized as <c>prc_&lt;hex&gt;</c>).</summary>
    public Guid Id { get; set; }

    /// <summary>Owning organization foreign key.</summary>
    public Guid OrgId { get; set; }

    /// <summary>The GitHub/GitLab repo slug (e.g. owner/name).</summary>
    public string Repo { get; set; } = string.Empty;

    /// <summary>Pull-request number.</summary>
    public int Pr { get; set; }

    /// <summary>Legacy v4.2 state (success | failure). Kept for backward compat with /organizations/{orgId}/pr-checks.</summary>
    public string State { get; set; } = string.Empty;

    /// <summary>Optional foreign key to the result that was uploaded alongside this check.</summary>
    public Guid? ResultId { get; set; }

    /// <summary>Server-side creation timestamp (UTC).</summary>
    public DateTime CreatedAt { get; set; }

    // ── M14-018 additions ──

    /// <summary>FK to github_installations.installation_id; null until poster starts.</summary>
    public long? InstallationId { get; set; }

    /// <summary>GitHub check_run id returned by /repos/{o}/{r}/check-runs.</summary>
    public long? CheckRunId { get; set; }

    /// <summary>UUID generated on insert; sent to GitHub as external_id.</summary>
    public Guid ExternalId { get; set; }

    /// <summary>One of {success, failure, neutral, cancelled, skipped, timed_out}.</summary>
    public string? Conclusion { get; set; }

    /// <summary>Optional URL surfaced as 'Details' on the GitHub check run.</summary>
    public string? DetailsUrl { get; set; }

    /// <summary>Markdown summary; truncated to 60 000 chars defensively.</summary>
    public string? OutputSummary { get; set; }

    /// <summary>Markdown text; truncated to 60 000 chars defensively.</summary>
    public string? OutputText { get; set; }

    /// <summary>JSON array of annotations (up to 50 per POST per spec :8525).</summary>
    public string? AnnotationsJson { get; set; }

    /// <summary>The git commit SHA for this check run (40-char hex).</summary>
    public string HeadSha { get; set; } = string.Empty;

    /// <summary>Timestamp when posting began; set before the outbound POST to GitHub.</summary>
    public DateTime? PostingStartedAt { get; set; }

    /// <summary>Timestamp when posting completed successfully.</summary>
    public DateTime? PostedAt { get; set; }

    /// <summary>Number of outbound POST attempts made.</summary>
    public int AttemptCount { get; set; }

    /// <summary>Last error message if posting failed.</summary>
    public string? LastError { get; set; }

    /// <summary>Lifecycle status: pending | queued | posting | posted | failed | REPO_NOT_COVERED.</summary>
    public string Status { get; set; } = "pending";

    // ── M16-014 additions ──

    /// <summary>
    /// Provider discriminator. One of: 'github' | 'gitlab'. Defaults to 'github' for
    /// back-compat with M14-018 rows. CHECK constraint enforced at DDL level (M16-014).
    /// </summary>
    public string Provider { get; set; } = "github";

    /// <summary>FK to gitlab_installations(id). Null for github-provider rows.</summary>
    public Guid? GitLabInstallationId { get; set; }

    /// <summary>GitLab commit-status row id returned by POST /projects/{id}/statuses/{sha}. Null for github rows.</summary>
    public long? GitLabStatusId { get; set; }
}
