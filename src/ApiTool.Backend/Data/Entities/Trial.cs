using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Per-(user, feature) trial grant.
/// Refs docs/SPECIFICATION.md:5800-5866 (narrative) and :11008-11030 (DDL).
/// </summary>
/// <remarks>
/// Uniqueness is enforced by an index on (user_id, feature) — one trial per
/// feature per user, ever. The 14-day full trial at registration is one row
/// per gated feature with <see cref="Kind"/> = <see cref="TrialKind.FullInitial"/>.
/// On-demand activation inserts <see cref="TrialKind.OnDemand"/>. Stripe
/// checkout success transitions live rows to <see cref="TrialKind.PreemptedBySubscription"/>.
/// </remarks>
[GdprTable(GdprTableKind.NotUserAttributable)]
public sealed class Trial
{
    /// <summary>Primary key — random UUID v4.</summary>
    public Guid Id { get; set; }

    /// <summary>The user this trial belongs to.</summary>
    public Guid UserId { get; set; }

    /// <summary>Gated feature slug — globally unique per user.</summary>
    public string Feature { get; set; } = string.Empty;

    /// <summary>How this trial was granted: full-initial, on-demand, or preempted.</summary>
    public TrialKind Kind { get; set; }

    /// <summary>UTC timestamp when the trial was granted.</summary>
    public DateTime GrantedAt { get; set; }

    /// <summary>UTC timestamp after which the trial no longer entitles the feature.</summary>
    public DateTime ExpiresAt { get; set; }

    /// <summary>UTC timestamp when the trial was consumed (subscription preemption); null while live.</summary>
    public DateTime? ConsumedAt { get; set; }

    /// <summary>UTC timestamp when the 3-day expiry-warning email was queued; null until queued.</summary>
    public DateTime? Notified3DayAt { get; set; }

    /// <summary>UTC timestamp when the 1-day expiry-warning email was queued; null until queued.</summary>
    public DateTime? Notified1DayAt { get; set; }

    /// <summary>UTC timestamp when the row was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp when the row was last updated.</summary>
    public DateTime UpdatedAt { get; set; }
}
