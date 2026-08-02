// Dev/Testing-only endpoint for backdating a user's pending_deletion_at past the 30-day cooldown.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by M18-012 e2e convergence to advance the GDPR deletion state machine deterministically
// without requiring a 30-day wall-clock wait.
using ApiTool.Backend.Data;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>Record body for the backdate-deletion-request hook.</summary>
internal sealed record BackdateDeletionRequest(Guid? UserId, int DaysAgo = 31);

/// <summary>
/// Maps <c>POST /api/v1/internal/test-hooks/backdate-deletion-request</c> (Development + Testing only).
/// Sets <c>users.pending_deletion_at</c> to <c>UtcNow - DaysAgo</c> so the
/// <see cref="ApiTool.Backend.Compliance.Gdpr.UserDeletionFinalizerHost"/> picks the row up immediately
/// without a 30-day wall-clock wait.
/// </summary>
public static class InternalBackdateDeletionEndpoint
{
    /// <summary>
    /// Conditionally registers the backdate-deletion-request endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalBackdateDeletionEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/api/v1/internal/test-hooks/backdate-deletion-request",
            async (BackdateDeletionRequest? body, AppDbContext db,
                   TimeProvider clock, CancellationToken ct) =>
            {
                if (body?.UserId is null || body.UserId == Guid.Empty)
                    return HttpResults.BadRequest(new { error = "user_id required (non-empty UUID)" });

                var newTs = clock.GetUtcNow().UtcDateTime - TimeSpan.FromDays(body.DaysAgo);

                var user = await db.Users
                    .Where(u => u.Id == body.UserId.Value && u.PendingDeletionAt != null)
                    .FirstOrDefaultAsync(ct);

                if (user is null)
                    return HttpResults.NotFound(new { error = "no pending deletion for user" });

                user.PendingDeletionAt = newTs;
                await db.SaveChangesAsync(ct);

                return HttpResults.Ok(new { user_id = user.Id, pending_deletion_at = newTs });
            })
            .AllowAnonymous()
            .DisableAntiforgery()
            .AddEndpointFilter<InternalAccessFilter>()
            .WithName("InternalBackdateDeletion")
            .WithTags("Internal");

        return app;
    }
}
