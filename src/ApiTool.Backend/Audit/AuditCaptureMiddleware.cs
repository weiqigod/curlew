namespace ApiTool.Backend.Audit;

/// <summary>
/// Populates the scoped <see cref="AuditContext"/> with the client IP address and
/// User-Agent from every authenticated HTTP request.
/// Must be registered after <c>UseAuthentication</c> / <c>UseAuthorization</c>.
/// </summary>
public sealed class AuditCaptureMiddleware(RequestDelegate next)
{
    private const int MaxUserAgentLength = 500;

    /// <inheritdoc cref="IMiddleware.InvokeAsync"/>
    public async Task InvokeAsync(HttpContext ctx, AuditContext audit)
    {
        audit.IpAddress = ResolveIp(ctx);
        audit.UserAgent = ResolveUserAgent(ctx);
        await next(ctx);
    }

    private static string? ResolveIp(HttpContext ctx)
    {
        var fwd = ctx.Request.Headers["X-Forwarded-For"].ToString();
        if (!string.IsNullOrWhiteSpace(fwd))
            return fwd.Split(',')[0].Trim();
        return ctx.Connection.RemoteIpAddress?.ToString();
    }

    private static string ResolveUserAgent(HttpContext ctx)
    {
        var ua = ctx.Request.Headers.UserAgent.ToString();
        return ua.Length > MaxUserAgentLength ? ua[..MaxUserAgentLength] : ua;
    }
}
