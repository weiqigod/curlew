namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Stable machine-readable error codes for the trials endpoint.
/// Refs docs/SPECIFICATION.md:5800-5860 (on-demand activation).
/// </summary>
internal static class TrialsSentinelErrors
{
    /// <summary>A trial for the requested feature has already been used.</summary>
    public const string AlreadyConsumed = "TRIAL_ALREADY_CONSUMED";

    /// <summary>The requested feature slug is not in <see cref="TrialFeatures.All"/>.</summary>
    public const string FeatureUnknown = "TRIAL_FEATURE_UNKNOWN";
}
