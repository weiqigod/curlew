// Refs docs/SPECIFICATION.md:10943 (30-day retention for GitLab), plan Decision H.
namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Options for <see cref="GitLabWebhookCleanupHost"/>. Bound from
/// <c>ApiTool:GitLab:Webhook:Cleanup</c> in appsettings.
/// Spec :10943 specifies 30 days for GitLab (vs 90 days for GitHub).
/// </summary>
public sealed class GitLabWebhookCleanupOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:GitLab:Webhook:Cleanup";

    /// <summary>
    /// Number of days to retain processed (non-quarantined) rows.
    /// Defaults to 30 per spec :10943. Quarantined rows are retained indefinitely.
    /// </summary>
    public int RetentionDays { get; set; } = 30;
}
