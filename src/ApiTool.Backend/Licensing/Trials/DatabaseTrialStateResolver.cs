using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Licensing.Trials;

/// <summary>EF-backed implementation reading the <c>trials</c> table.</summary>
public sealed class DatabaseTrialStateResolver(AppDbContext db, TimeProvider clock) : ITrialStateResolver
{
    private static readonly TrialStateResult None =
        new("none", null, Array.Empty<string>());

    private static readonly TrialStateResult Expired =
        new("expired", null, Array.Empty<string>());

    /// <inheritdoc/>
    public async Task<TrialStateResult> ResolveAsync(Guid userId, string tier, CancellationToken ct = default)
    {
        // Spec row "Active subscription (Solo+)": tier overrides trial state.
        if (!string.Equals(tier, "free", StringComparison.OrdinalIgnoreCase))
            return None;

        var now = clock.GetUtcNow().UtcDateTime;

        // Pull active+unconsumed rows. Bound by |TrialFeatures|, never large.
        // Belt-and-suspenders: ExpiresAt is set to now() at preemption time, so the
        // ExpiresAt > now filter already excludes preempted rows — but the explicit
        // Kind guard makes intent clear at read-time without relying on that side-effect.
        var active = await db.Trials
            .Where(t => t.UserId == userId
                     && t.ConsumedAt == null
                     && t.ExpiresAt > now
                     && t.Kind != TrialKind.PreemptedBySubscription)
            .Select(t => new { t.Feature, t.ExpiresAt })
            .ToListAsync(ct);

        if (active.Count > 0)
        {
            var earliest = active.Min(r => r.ExpiresAt);
            var features = active.Select(r => r.Feature).Distinct().ToArray();
            return new TrialStateResult(
                TrialState: "active",
                TrialExpiryUnixSeconds: new DateTimeOffset(earliest, TimeSpan.Zero).ToUnixTimeSeconds(),
                TrialingFeatures: features);
        }

        // No active rows. If any rows exist at all, the user has consumed/expired
        // their entitlements — spec rows "full trial expired" and "subscription
        // cancelled, period ended". Otherwise the user has never had trials seeded
        // (defensive — should be unreachable post-seeder).
        var anyRow = await db.Trials.AnyAsync(t => t.UserId == userId, ct);
        return anyRow ? Expired : None;
    }
}
