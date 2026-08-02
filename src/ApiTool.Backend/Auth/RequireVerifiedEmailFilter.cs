// Endpoint filter that gates on users.email_verified.
// Refs docs/SPECIFICATION.md:8576-8580 (verification gating policy).
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Endpoint filter that returns 403 with <c>email-not-verified</c> problem detail
/// when the authenticated user's <c>email_verified</c> column is false.
/// Refs docs/SPECIFICATION.md:8576-8580 (verification gating policy).
/// </summary>
public sealed class RequireVerifiedEmailFilter : IEndpointFilter
{
    /// <inheritdoc/>
    public async ValueTask<object?> InvokeAsync(
        EndpointFilterInvocationContext ctx, EndpointFilterDelegate next)
    {
        var http  = ctx.HttpContext;
        var users = http.RequestServices.GetRequiredService<CurrentUserAccessor>();
        var userId = await users.ResolveAsync(http.RequestAborted);
        if (userId is null)
            return EmailNotVerifiedProblem.Unauthenticated(http);

        var db = http.RequestServices.GetRequiredService<AppDbContext>();
        var verified = await db.Users
            .Where(u => u.Id == userId.Value)
            .Select(u => u.EmailVerified)
            .FirstOrDefaultAsync(http.RequestAborted);

        if (!verified)
            return EmailNotVerifiedProblem.Forbidden(http);

        return await next(ctx);
    }
}

/// <summary>
/// Extension methods that wire <see cref="RequireVerifiedEmailFilter"/> onto a
/// <see cref="RouteHandlerBuilder"/> via a single fluent call.
/// </summary>
public static class RequireVerifiedEmailExtensions
{
    /// <summary>
    /// Gates the endpoint on <c>users.email_verified == true</c>. Returns 403 with
    /// <c>application/problem+json</c> body type <c>.../errors/email-not-verified</c>
    /// when the caller is authenticated but unverified.
    /// </summary>
    public static RouteHandlerBuilder RequireVerifiedEmail(this RouteHandlerBuilder builder)
        => builder.AddEndpointFilter<RequireVerifiedEmailFilter>();
}
