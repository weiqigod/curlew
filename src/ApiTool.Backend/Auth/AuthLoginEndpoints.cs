using ApiTool.Backend.Data;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Sso;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>Local-password login endpoint — a narrow escape hatch for the bootstrap admin on a self-hosted install.</summary>
public static class AuthLoginEndpoints
{
    // A pre-computed argon2id hash of a random string used on the miss path
    // to prevent user-enumeration via timing differences.
    private static readonly string PlaceholderHash =
        new PasswordHasher().Hash("placeholder-constant-dummy-value-12345");

    /// <summary>Maps the <c>POST /api/v1/auth/login</c> endpoint.</summary>
    public static IEndpointRouteBuilder MapAuthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/auth/login", async (
            LoginRequest? body,
            AppDbContext db,
            PasswordHasher hasher,
            SessionTokenIssuer issuer,
            CancellationToken ct) =>
        {
            if (body is null
                || string.IsNullOrWhiteSpace(body.Email)
                || string.IsNullOrWhiteSpace(body.Password))
            {
                return HttpResults.Json(
                    new ErrorResponse("invalid_credentials", "Email and password are required."),
                    statusCode: StatusCodes.Status400BadRequest);
            }

            var user = await db.Users.FirstOrDefaultAsync(u => u.Email == body.Email, ct);

            // Always run Verify to prevent user-enumeration via timing.
            var hashToCheck = user?.PasswordHash ?? PlaceholderHash;
            var ok = hasher.Verify(body.Password, hashToCheck);

            if (user is null || user.PasswordHash is null || !ok)
            {
                return HttpResults.Json(
                    new ErrorResponse("invalid_credentials", "Incorrect email or password."),
                    statusCode: StatusCodes.Status401Unauthorized);
            }

            var ttl   = TimeSpan.FromHours(8);
            var token = issuer.IssueForUser(user.Id, user.Email, ttl);
            var role  = user.IsAdmin ? "admin" : "member";
            return HttpResults.Ok(new LoginResponse(token, "Bearer", (int)ttl.TotalSeconds, role));
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .WithName("AuthLogin")
        .WithTags("Auth")
        .Accepts<LoginRequest>("application/json")
        .Produces<LoginResponse>()
        .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
        .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized);

        return app;
    }
}
