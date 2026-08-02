using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Discriminated-union result for <see cref="TrialsService.ActivateOnDemandAsync"/>.
/// Refs docs/SPECIFICATION.md:5800-5860 (on-demand activation).
/// </summary>
public abstract record TrialActivationResult
{
    private TrialActivationResult() { }

    /// <summary>Trial row successfully inserted; user may begin using the feature.</summary>
    /// <param name="Feature">The feature slug that was granted.</param>
    /// <param name="GrantedAt">UTC timestamp when the trial row was inserted.</param>
    /// <param name="ExpiresAt">UTC timestamp after which the trial expires.</param>
    public sealed record Granted(string Feature, DateTime GrantedAt, DateTime ExpiresAt) : TrialActivationResult;

    /// <summary>A trial row already existed; the user has consumed their entitlement.</summary>
    /// <param name="Feature">The feature slug that was requested.</param>
    /// <param name="GrantedAt">UTC timestamp of the existing grant.</param>
    /// <param name="ExpiresAt">UTC timestamp after which the existing trial expires.</param>
    /// <param name="Kind">How the previous grant was created.</param>
    public sealed record AlreadyConsumed(
        string Feature, DateTime GrantedAt, DateTime ExpiresAt, TrialKind Kind) : TrialActivationResult;

    /// <summary>The feature slug is not in <see cref="TrialFeatures.All"/>.</summary>
    /// <param name="Feature">The unrecognised feature slug.</param>
    public sealed record UnknownFeature(string Feature) : TrialActivationResult;
}
