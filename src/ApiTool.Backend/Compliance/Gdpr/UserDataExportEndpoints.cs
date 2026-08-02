using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Storage;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Maps the per-user GDPR data export endpoints (M18-004, v4-4):
/// <list type="bullet">
///   <item><c>POST /api/v1/users/me/export-requests</c> — queue a new export request (202/429)</item>
///   <item><c>GET /api/v1/users/me/export-requests/{id}</c> — poll status + signed URL (200/404)</item>
/// </list>
/// </summary>
public static class UserDataExportEndpoints
{
    private static readonly TimeSpan RateLimitWindow = TimeSpan.FromHours(24);
    private static readonly TimeSpan SignedUrlTtl = TimeSpan.FromHours(24);

    /// <summary>Registers both export endpoints.</summary>
    public static IEndpointRouteBuilder MapUserDataExportEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/users/me/export-requests", Create)
            .RequireAuthorization()
            .DisableAntiforgery()
            .Produces<UserExportRequestDto>(StatusCodes.Status202Accepted)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status429TooManyRequests)
            .WithName("CreateUserExportRequest")
            .WithTags("UserDataExport");

        app.MapGet("/api/v1/users/me/export-requests/{id:guid}", Get)
            .RequireAuthorization()
            .Produces<UserExportRequestDto>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status404NotFound)
            .WithName("GetUserExportRequest")
            .WithTags("UserDataExport");

        return app;
    }

    private static async Task<IResult> Create(
        CurrentUserAccessor users,
        AppDbContext db,
        TimeProvider clock,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return HttpResults.Unauthorized();

        var now = clock.GetUtcNow().UtcDateTime;
        var windowStart = now - RateLimitWindow;

        // Rate-limit: 1 request per user per 24h.
        var existing = await db.UserExportRequests
            .Where(r => r.UserId == userId.Value && r.CreatedAt > windowStart)
            .OrderByDescending(r => r.CreatedAt)
            .FirstOrDefaultAsync(ct);

        if (existing is not null)
        {
            var retryAfterSeconds = (int)(existing.CreatedAt.Add(RateLimitWindow) - now).TotalSeconds;
            return Microsoft.AspNetCore.Http.TypedResults.Problem(
                statusCode: StatusCodes.Status429TooManyRequests,
                title: "Export rate limited",
                detail: $"You can request one export per 24 hours. Retry after {retryAfterSeconds} seconds.",
                extensions: new Dictionary<string, object?> { ["code"] = "export_rate_limited" })
                .WithHeader("Retry-After", Math.Max(0, retryAfterSeconds).ToString());
        }

        var request = new UserExportRequest
        {
            Id = Guid.NewGuid(),
            UserId = userId.Value,
            Status = UserExportStatus.Queued,
            CreatedAt = now,
        };
        db.UserExportRequests.Add(request);
        await db.SaveChangesAsync(ct);

        return HttpResults.Accepted(
            uri: null,
            value: ToDto(request, signedUrl: null));
    }

    private static async Task<IResult> Get(
        Guid id,
        CurrentUserAccessor users,
        AppDbContext db,
        IObjectStore store,
        TimeProvider clock,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return HttpResults.Unauthorized();

        var request = await db.UserExportRequests
            .Where(r => r.Id == id && r.UserId == userId.Value)
            .FirstOrDefaultAsync(ct);

        if (request is null)
            return HttpResults.NotFound();

        string? signedUrl = null;
        if (request.Status == UserExportStatus.Ready && request.ObjectKey is not null && request.ExpiresAt.HasValue)
        {
            var now = clock.GetUtcNow().UtcDateTime;
            if (request.ExpiresAt.Value > now)
            {
                var ttl = request.ExpiresAt.Value - now;
                try
                {
                    var uri = await store.GetSignedUrlAsync(request.ObjectKey, ttl, ct);
                    signedUrl = uri.ToString();
                }
                catch (Exception)
                {
                    // Signed URL generation is best-effort; callers can retry.
                }
            }
            else
            {
                // TTL elapsed — mark row as Expired.
                request.Status = UserExportStatus.Expired;
                request.Version = Guid.NewGuid();
                try
                {
                    await db.SaveChangesAsync(ct);
                }
                catch (Microsoft.EntityFrameworkCore.DbUpdateConcurrencyException)
                {
                    // A concurrent GET already transitioned this row to Expired.
                    // Both requests agree on the outcome — swallow and continue.
                }
            }
        }

        return HttpResults.Ok(ToDto(request, signedUrl));
    }

    private static UserExportRequestDto ToDto(UserExportRequest request, string? signedUrl) =>
        new(
            Id: request.Id.ToString(),
            Status: request.Status.ToString().ToLowerInvariant(),
            CreatedAt: request.CreatedAt.ToString("O"),
            ReadyAt: request.ReadyAt?.ToString("O"),
            ExpiresAt: request.ExpiresAt?.ToString("O"),
            SignedUrl: signedUrl,
            FailureReason: request.FailureReason);
}

/// <summary>Extension to add a header to an <see cref="IResult"/>.</summary>
file static class ResultExtensions
{
    public static IResult WithHeader(this IResult result, string name, string value) =>
        new HeaderResult(result, name, value);
}

/// <summary>Wraps an <see cref="IResult"/> and appends a response header.</summary>
file sealed class HeaderResult(IResult inner, string name, string value) : IResult
{
    public async Task ExecuteAsync(HttpContext context)
    {
        context.Response.Headers[name] = value;
        await inner.ExecuteAsync(context);
    }
}
