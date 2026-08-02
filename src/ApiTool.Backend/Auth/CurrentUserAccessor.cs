using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Scoped service that resolves the current user id from JWT claims, upserting a user row on first visit.
/// </summary>
public sealed class CurrentUserAccessor(
    AppDbContext db,
    IHttpContextAccessor httpContextAccessor,
    TimeProvider clock,
    TrialSeederService seeder)
{
    /// <summary>
    /// Resolves the authenticated user's id, creating a user row if none exists.
    /// Returns <see langword="null"/> when the JWT <c>sub</c> claim is missing or is not a valid GUID —
    /// callers should return 401 in that case.
    /// </summary>
    /// <param name="ct">Cancellation token.</param>
    public async Task<Guid?> ResolveAsync(CancellationToken ct = default)
    {
        var httpContext = httpContextAccessor.HttpContext;
        if (httpContext is null)
            return null;

        var subClaim =
            httpContext.User.FindFirstValue(ClaimTypes.NameIdentifier)
            ?? httpContext.User.FindFirstValue(JwtRegisteredClaimNames.Sub);

        if (subClaim is null || !Guid.TryParse(subClaim, out var userId))
            return null;

        // JWT bearer middleware maps "email" → ClaimTypes.Email by default, so check both
        // forms (mirrors the sub/NameIdentifier lookup above). Falling through to string.Empty
        // here previously caused every upserted user to share an empty email value, which
        // collides with the users.Email UNIQUE index on the second write.
        var email =
            httpContext.User.FindFirstValue(ClaimTypes.Email)
            ?? httpContext.User.FindFirstValue(JwtRegisteredClaimNames.Email)
            ?? string.Empty;

        var existing = await db.Users.FindAsync([userId], ct);
        if (existing is null)
        {
            db.Users.Add(new User
            {
                Id = userId,
                Email = email,
                CreatedAt = clock.GetUtcNow().UtcDateTime,
            });
            try
            {
                await db.SaveChangesAsync(ct);
            }
            catch (DbUpdateException)
            {
                // Another request already inserted this user (race condition on id or email).
                // Clear the failed entity from the change tracker and continue.
                db.ChangeTracker.Clear();
            }

            // Seed full-initial trial rows for the brand-new user (idempotent).
            // Refs docs/SPECIFICATION.md:5811 (14-day full trial at registration).
            var newUser = await db.Users.FindAsync([userId], ct);
            if (newUser is not null)
                await seeder.SeedFullInitialAsync(userId, newUser.CreatedAt, ct);
        }

        return userId;
    }
}
