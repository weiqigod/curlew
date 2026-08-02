// Dev/Testing-only endpoint for minting a drto_ reauth token without requiring a password.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by M18-012 e2e convergence to bypass the password check for seed-refresh users
// (which have no password hash set) so the deletion-request → cancel → re-initiate chain
// can execute deterministically. See plan risk-mitigation section for rationale.
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>Record body for the mint-reauth-token hook.</summary>
internal sealed record MintReauthTokenRequest(Guid? UserId);

/// <summary>
/// Maps <c>POST /api/v1/internal/test-hooks/mint-reauth-token</c> (Development + Testing only).
/// Mints a 5-minute single-use <c>drto_</c> token for the given <c>user_id</c> without
/// verifying a password. Intended exclusively for e2e tests where the test user has no
/// password hash (e.g., seed-refresh users). Returns <c>{ "token": "drto_..." }</c>.
/// </summary>
public static class InternalMintReauthTokenEndpoint
{
    /// <summary>
    /// Conditionally registers the mint-reauth-token endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalMintReauthTokenEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/api/v1/internal/test-hooks/mint-reauth-token",
            async (MintReauthTokenRequest? body, AppDbContext db,
                   TimeProvider clock, CancellationToken ct) =>
            {
                if (body?.UserId is null || body.UserId == Guid.Empty)
                    return HttpResults.BadRequest(new { error = "user_id required (non-empty UUID)" });

                // Verify the user exists.
                var exists = await db.Users
                    .AnyAsync(u => u.Id == body.UserId.Value, ct);
                if (!exists)
                    return HttpResults.NotFound(new { error = "user not found" });

                var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.DeletionReauthPrefix);
                var now = clock.GetUtcNow().UtcDateTime;
                var expiresAt = now + TimeSpan.FromMinutes(5);

                db.DeletionReauthTokens.Add(new DeletionReauthToken
                {
                    Id = Guid.NewGuid(),
                    UserId = body.UserId.Value,
                    TokenHash = hash,
                    IssuedAt = now,
                    ExpiresAt = expiresAt,
                });
                await db.SaveChangesAsync(ct);

                return HttpResults.Ok(new { token = plaintext, expires_at = expiresAt });
            })
            .AllowAnonymous()
            .DisableAntiforgery()
            .AddEndpointFilter<InternalAccessFilter>()
            .WithName("InternalMintReauthToken")
            .WithTags("Internal");

        return app;
    }
}
