// Dev/Testing-only endpoint for seeding a near-expiry trial row for M16-021 e2e assertions.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by scripts/m16-e2e.sh to drive the trial_expiring email assertion.
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /internal/test/seed-near-expiry-trial</c> (Development + Testing only).
/// Inserts a <see cref="Trial"/> row with <c>expires_at = now + DaysUntilExpiry</c>
/// and <c>notified_3day_at = null</c> so <see cref="Notifications.Trials.TrialExpiryNotifier"/>
/// will process it on the next tick. Idempotent: deletes any existing row for the same
/// (user_id, feature) pair before inserting.
/// </summary>
public static class InternalSeedNearExpiryTrialEndpoint
{
    /// <summary>
    /// Conditionally registers the seed-near-expiry-trial endpoint when the environment is Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalSeedNearExpiryTrialEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/internal/test/seed-near-expiry-trial", async (
            SeedNearExpiryTrialRequest? body,
            AppDbContext db,
            TimeProvider clock,
            CancellationToken ct) =>
        {
            if (body is null || body.UserId == Guid.Empty || string.IsNullOrWhiteSpace(body.Feature))
                return HttpResults.BadRequest("user_id and feature are required");

            var now = clock.GetUtcNow().UtcDateTime;

            // Idempotent: remove existing row for (user, feature) to allow re-seeding.
            var existing = await db.Trials
                .Where(t => t.UserId == body.UserId && t.Feature == body.Feature)
                .ToListAsync(ct);
            db.Trials.RemoveRange(existing);

            var trial = new Trial
            {
                Id = Guid.NewGuid(),
                UserId = body.UserId,
                Feature = body.Feature,
                Kind = TrialKind.FullInitial,
                GrantedAt = now.AddDays(-12),
                ExpiresAt = now.AddDays(body.DaysUntilExpiry),
                Notified3DayAt = null,
                Notified1DayAt = null,
                ConsumedAt = null,
                CreatedAt = now,
                UpdatedAt = now,
            };
            db.Trials.Add(trial);
            await db.SaveChangesAsync(ct);

            return HttpResults.Ok(new { trial_id = trial.Id, expires_at = trial.ExpiresAt });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("InternalSeedNearExpiryTrial")
        .WithTags("Internal");

        return app;
    }
}

/// <summary>Request body for the seed-near-expiry-trial endpoint.</summary>
internal sealed record SeedNearExpiryTrialRequest(
    Guid UserId,
    string Feature,
    int DaysUntilExpiry = 2);
