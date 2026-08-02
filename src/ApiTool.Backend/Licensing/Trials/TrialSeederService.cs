using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Seeds one <see cref="TrialKind.FullInitial"/> row per <see cref="TrialFeatures.All"/>
/// slug for a brand-new user. Idempotent — re-invocation on a user with any trial row is
/// a noop. Refs docs/SPECIFICATION.md:5800-5866 (Trial Persistence).
/// </summary>
/// <remarks>
/// The <c>AnyAsync</c> idempotency guard runs on every authenticated request via
/// <c>CurrentUserAccessor</c>. With <c>idx_trials_user</c>, this is a single indexed
/// SELECT — acceptable per-request overhead.
/// </remarks>
public sealed class TrialSeederService(
    AppDbContext db,
    TimeProvider clock,
    ILogger<TrialSeederService> log)
{
    /// <summary>14-day window for the registration trial per spec :5811.</summary>
    public static readonly TimeSpan FullInitialDuration = TimeSpan.FromDays(14);

    /// <summary>
    /// Inserts one full-initial row per <see cref="TrialFeatures.All"/> slug.
    /// Idempotent: returns 0 if any trial row already exists for the user.
    /// Returns the count of rows inserted (0 or |TrialFeatures.All|).
    /// </summary>
    public async Task<int> SeedFullInitialAsync(
        Guid userId, DateTime grantedAt, CancellationToken ct = default)
    {
        if (await db.Trials.AnyAsync(t => t.UserId == userId, ct))
            return 0;

        var now = clock.GetUtcNow().UtcDateTime;
        var expires = grantedAt + FullInitialDuration;

        foreach (var feature in TrialFeatures.All)
        {
            db.Trials.Add(new Trial
            {
                Id = Guid.NewGuid(),
                UserId = userId,
                Feature = feature,
                Kind = TrialKind.FullInitial,
                GrantedAt = grantedAt,
                ExpiresAt = expires,
                CreatedAt = now,
                UpdatedAt = now,
            });
        }

        try
        {
            await db.SaveChangesAsync(ct);
            log.LogInformation(
                "trial_seeded_full_initial user_id={UserId} count={Count}",
                userId, TrialFeatures.All.Count);
            return TrialFeatures.All.Count;
        }
        catch (DbUpdateException)
        {
            // Race: another request already seeded this user. UNIQUE(user_id, feature)
            // rejects the conflict. Clear the change tracker and return 0.
            db.ChangeTracker.Clear();
            log.LogDebug(
                "trial_seed_race_discarded user_id={UserId} — concurrent seed already committed",
                userId);
            return 0;
        }
    }
}
