using Microsoft.EntityFrameworkCore.Storage.ValueConversion;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// EF Core value converter mapping <see cref="TrialKind"/> to and from
/// the spec-mandated lowercase snake_case strings stored in the database.
/// </summary>
internal sealed class TrialKindConverter : ValueConverter<TrialKind, string>
{
    /// <summary>Initializes a new instance of <see cref="TrialKindConverter"/>.</summary>
    public TrialKindConverter()
        : base(
            v => ToDb(v),
            v => FromDb(v))
    {
    }

    private static string ToDb(TrialKind kind) => kind switch
    {
        TrialKind.FullInitial             => "full_initial",
        TrialKind.OnDemand                => "ondemand",
        TrialKind.PreemptedBySubscription => "preempted_by_subscription",
        _ => throw new InvalidOperationException($"Unknown TrialKind: {kind}"),
    };

    private static TrialKind FromDb(string value) => value switch
    {
        "full_initial"              => TrialKind.FullInitial,
        "ondemand"                  => TrialKind.OnDemand,
        "preempted_by_subscription" => TrialKind.PreemptedBySubscription,
        _ => throw new InvalidOperationException($"Unknown trial kind string: {value}"),
    };
}
