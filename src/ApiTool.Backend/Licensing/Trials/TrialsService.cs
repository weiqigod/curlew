using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Handles on-demand trial activation logic.
/// Refs docs/SPECIFICATION.md:5800-5860 (on-demand activation).
/// </summary>
public sealed class TrialsService(AppDbContext db, TimeProvider clock, ILogger<TrialsService> log)
{
    /// <summary>7-day window for on-demand trials per spec :5800.</summary>
    public static readonly TimeSpan OnDemandDuration = TimeSpan.FromDays(7);

    /// <summary>
    /// Attempts to grant an on-demand trial of <paramref name="feature"/> for
    /// <paramref name="userId"/>. Returns one of three discriminated-union cases:
    /// <list type="bullet">
    ///   <item><see cref="TrialActivationResult.UnknownFeature"/> — slug not registered.</item>
    ///   <item><see cref="TrialActivationResult.AlreadyConsumed"/> — row already exists.</item>
    ///   <item><see cref="TrialActivationResult.Granted"/> — new row inserted successfully.</item>
    /// </list>
    /// </summary>
    public async Task<TrialActivationResult> ActivateOnDemandAsync(
        Guid userId, string feature, CancellationToken ct)
    {
        if (!TrialFeatures.All.Contains(feature))
            return new TrialActivationResult.UnknownFeature(feature);

        var existing = await db.Trials
            .Where(t => t.UserId == userId && t.Feature == feature)
            .Select(t => new { t.GrantedAt, t.ExpiresAt, t.Kind })
            .FirstOrDefaultAsync(ct);

        if (existing is not null)
            return new TrialActivationResult.AlreadyConsumed(
                feature, existing.GrantedAt, existing.ExpiresAt, existing.Kind);

        var now = clock.GetUtcNow().UtcDateTime;
        var row = new Trial
        {
            Id        = Guid.NewGuid(),
            UserId    = userId,
            Feature   = feature,
            Kind      = TrialKind.OnDemand,
            GrantedAt = now,
            ExpiresAt = now + OnDemandDuration,
            CreatedAt = now,
            UpdatedAt = now,
        };
        db.Trials.Add(row);

        try
        {
            await db.SaveChangesAsync(ct);
        }
        catch (DbUpdateException)
        {
            // Race with a concurrent activation — the UNIQUE(user_id, feature) constraint won.
            // Re-read to return the winning row's details.
            db.ChangeTracker.Clear();
            var winner = await db.Trials.FirstAsync(
                t => t.UserId == userId && t.Feature == feature, ct);
            return new TrialActivationResult.AlreadyConsumed(
                feature, winner.GrantedAt, winner.ExpiresAt, winner.Kind);
        }

        log.LogInformation(
            "trial_activated_ondemand user_id={UserId} feature={Feature}",
            userId, feature);

        return new TrialActivationResult.Granted(feature, row.GrantedAt, row.ExpiresAt);
    }
}
