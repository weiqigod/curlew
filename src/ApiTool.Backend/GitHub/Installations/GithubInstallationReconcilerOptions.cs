namespace ApiTool.Backend.GitHub.Installations;

/// <summary>
/// Configuration options for <see cref="GithubInstallationReconcilerHost"/>.
/// Bound from <c>ApiTool:GitHubApp:Reconciler</c>.
/// </summary>
public sealed class GithubInstallationReconcilerOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:GitHubApp:Reconciler";

    /// <summary>
    /// How many days old <c>last_reconciled_at</c> must be before the reconciler
    /// re-syncs a given installation. Default: 0 (reconcile every tick).
    /// </summary>
    public int StaleDays { get; set; } = 0;
}
