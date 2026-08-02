using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Health;

/// <summary>Minimal-API endpoint registrations for health probing.</summary>
public static class HealthEndpoints
{
    /// <summary>Maps <c>GET /health</c> to the HealthService.</summary>
    public static IEndpointRouteBuilder MapHealthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/health", async (HealthService svc, CancellationToken ct) =>
        {
            var r = await svc.CheckAsync(ct);
            var code = r.Status == HealthReport.StatusHealthy ? 200 : 503;
            return HttpResults.Json(r, statusCode: code);
        }).AllowAnonymous();
        return app;
    }
}
