namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Categorizes a row in the <c>trials</c> table.
/// Refs docs/SPECIFICATION.md:5800-5866 (Trial Persistence) and :11008-11030 (DDL).
/// </summary>
/// <remarks>
/// Persisted as the lowercase snake_case string in the <c>kind</c> column;
/// see <see cref="ApiTool.Backend.Data.AppDbContext.OnModelCreating"/> for
/// the converter that maps each enum value to its spec-mandated string form.
/// </remarks>
public enum TrialKind
{
    /// <summary>14-day full trial granted at registration. One row per gated feature.</summary>
    FullInitial,

    /// <summary>7-day on-demand per-feature trial activated by the user.</summary>
    OnDemand,

    /// <summary>Trial preempted by Stripe checkout success (tier upgrade).</summary>
    PreemptedBySubscription,
}
