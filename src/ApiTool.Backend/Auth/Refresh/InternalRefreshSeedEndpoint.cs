// Dev/Testing-only endpoint for seeding a refresh token without going through login.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by scripts/test-token.sh refresh <email> for manual observable verification.
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Licensing.Trials;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Maps <c>POST /internal/test/seed-refresh</c> (Development + Testing only).
/// Inserts a <see cref="RefreshToken"/> row for the given user+device and returns
/// the plaintext token. Used by <c>scripts/test-token.sh</c> for observable verification.
/// </summary>
public static class InternalRefreshSeedEndpoint
{
    /// <summary>
    /// Conditionally registers the seed endpoint when the environment is Development or Testing.
    /// Must be called AFTER <see cref="WebApplication.UseAuthentication"/>.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalRefreshSeedEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/internal/test/seed-refresh", async (
            SeedRefreshRequest? body,
            AppDbContext db,
            TimeProvider clock,
            TrialSeederService seeder,
            CancellationToken ct) =>
        {
            if (body is null || string.IsNullOrWhiteSpace(body.Email))
                return HttpResults.BadRequest("email is required");

            // Upsert user
            var user = await db.Users.FirstOrDefaultAsync(u => u.Email == body.Email, ct);
            if (user is null)
            {
                user = new User { Id = Guid.NewGuid(), Email = body.Email, CreatedAt = clock.GetUtcNow().UtcDateTime };
                db.Users.Add(user);
                await db.SaveChangesAsync(ct);
            }

            // Seed full-initial trials on first creation (idempotent for re-seeded emails).
            // Refs docs/SPECIFICATION.md:5811 (14-day full trial at registration).
            await seeder.SeedFullInitialAsync(user.Id, user.CreatedAt, ct);

            var deviceId = body.DeviceId ?? Guid.NewGuid();
            var familyId = Guid.NewGuid();
            var issuer   = new RefreshTokenIssuer(clock);
            var (plaintext, row) = issuer.Mint(
                user.Id, deviceId, familyId, parentId: null,
                familyRootIssuedAt: clock.GetUtcNow().UtcDateTime,
                clientIp: "seed", userAgent: "test-token.sh");

            db.RefreshTokens.Add(row);
            await db.SaveChangesAsync(ct);

            return HttpResults.Ok(new { plaintext, device_id = deviceId });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("SeedRefreshToken")
        .WithTags("Internal");

        return app;
    }
}

/// <summary>Request body for the seed endpoint.</summary>
internal sealed record SeedRefreshRequest(string? Email, Guid? DeviceId);
