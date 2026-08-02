// Dev/Testing-only endpoint for deterministically triggering one AuditLogCleanupHost tick.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by the M18-002 e2e observable to trigger the cleanup without waiting for the daily tick.
using ApiTool.Backend.Audit;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /api/v1/internal/test-hooks/run-audit-cleanup</c> (Development + Testing only).
/// Constructs an ad-hoc <see cref="AuditLogCleanupHost"/> tick from DI services so the
/// e2e observable can trigger the cleanup deterministically.
/// </summary>
public static class InternalAuditCleanupTickEndpoint
{
    /// <summary>
    /// Conditionally registers the audit-cleanup-tick endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalAuditCleanupTickEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/api/v1/internal/test-hooks/run-audit-cleanup", async (
            IServiceProvider services,
            CancellationToken ct) =>
        {
            var scopeFactory = services.GetRequiredService<IServiceScopeFactory>();
            var clock = services.GetRequiredService<TimeProvider>();
            var logger = services.GetRequiredService<ILogger<AuditLogCleanupHost>>();
            var opts = services.GetRequiredService<IOptions<AuditLogCleanupOptions>>();

            var host = new AuditLogCleanupHost(scopeFactory, opts, clock, logger);
            await host.TickOnceAsync(ct);

            return HttpResults.Ok(new { ticked = true });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("InternalAuditCleanupTick")
        .WithTags("Internal");

        return app;
    }
}
