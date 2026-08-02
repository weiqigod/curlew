// Dev/Testing-only endpoint for deterministically triggering one TelemetryPurgeHost tick.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by the M18-007 e2e observable and integration tests to drive the purge synchronously.
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Telemetry;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /api/v1/internal/test-hooks/purge-telemetry-events</c> (Development + Testing only).
/// Constructs an ad-hoc <see cref="TelemetryPurgeHost"/> tick from DI services so the
/// e2e observable can trigger the purge deterministically.
/// </summary>
public static class InternalPurgeTelemetryEventsEndpoint
{
    /// <summary>
    /// Conditionally registers the telemetry-purge endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalPurgeTelemetryEventsEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/api/v1/internal/test-hooks/purge-telemetry-events", async (
            IServiceProvider services,
            CancellationToken ct) =>
        {
            var scopeFactory = services.GetRequiredService<IServiceScopeFactory>();
            var clock = services.GetRequiredService<TimeProvider>();
            var logger = services.GetRequiredService<ILogger<TelemetryPurgeHost>>();
            var opts = services.GetRequiredService<IOptions<TelemetryPurgeOptions>>();

            var host = new TelemetryPurgeHost(scopeFactory, opts, clock, logger);
            await host.TickOnceAsync(ct);

            return HttpResults.Ok(new { ticked = true });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("InternalPurgeTelemetryEvents")
        .WithTags("Internal");

        return app;
    }
}
