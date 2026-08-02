namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Computed trial state for a user, surfaced into the License JWT.
/// Refs docs/SPECIFICATION.md:5806-5816 (state transition table).
/// </summary>
/// <param name="TrialState">One of <c>none</c>, <c>active</c>, <c>expired</c>.</param>
/// <param name="TrialExpiryUnixSeconds">
/// Earliest <c>expires_at</c> across active trial rows, or <see langword="null"/>
/// when <see cref="TrialState"/> is <c>none</c> or <c>expired</c>.
/// </param>
/// <param name="TrialingFeatures">
/// Feature slugs the user is currently trialing (active + unconsumed rows).
/// The License JWT issuer unions these into the <c>features[]</c> claim per spec :5817.
/// </param>
public sealed record TrialStateResult(
    string TrialState,
    long? TrialExpiryUnixSeconds,
    IReadOnlyList<string> TrialingFeatures);
