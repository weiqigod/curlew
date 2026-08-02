namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// The set of gated feature slugs eligible for trial.
/// Refs docs/SPECIFICATION.md narrative :5777, :5787, :5827, :5860 (feature names);
/// :5800-5866 (uniqueness model).
/// </summary>
/// <remarks>
/// At user registration, one <c>kind = full_initial</c> row is inserted per slug.
/// On-demand trial activation (M16-007) refuses to grant a slug already present.
/// New slugs added here only seed for users registered after the change — existing
/// users cannot retroactively claim a 14-day window for a newly-shipped feature.
/// </remarks>
public static class TrialFeatures
{
    /// <summary>Read-only list of trialable feature slugs.</summary>
    public static readonly IReadOnlyList<string> All = new[]
    {
        "vault_provider_profiles",
        "shared_vault_templates",
        "schedules",
        "dashboards",
        "custom_roles",
        "audit_log",
        "sso",
        "ai_agents",
    };
}
