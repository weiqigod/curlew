// Dev/Testing-only endpoint for deterministically triggering one TelemetryAggregatorHost tick.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by the M18-007 e2e observable and integration tests to drive the aggregator synchronously.
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Telemetry;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /api/v1/internal/test-hooks/run-telemetry-aggregator</c> (Development + Testing only).
/// Constructs an ad-hoc <see cref="TelemetryAggregatorHost"/> tick from DI services so the
/// e2e observable can trigger the aggregator deterministically.
/// </summary>
public static class InternalRunTelemetryAggregatorEndpoint
{
    /// <summary>
    /// Conditionally registers the telemetry-aggregator-tick endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalRunTelemetryAggregatorEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/api/v1/internal/test-hooks/run-telemetry-aggregator", async (
            IServiceProvider services,
            CancellationToken ct) =>
        {
            var scopeFactory = services.GetRequiredService<IServiceScopeFactory>();
            var clock = services.GetRequiredService<TimeProvider>();
            var logger = services.GetRequiredService<ILogger<TelemetryAggregatorHost>>();
            var opts = services.GetRequiredService<IOptions<TelemetryAggregatorOptions>>();

            var host = new TelemetryAggregatorHost(scopeFactory, opts, clock, logger);
            await host.TickOnceAsync(ct);

            return HttpResults.Ok(new { ticked = true });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("InternalRunTelemetryAggregator")
        .WithTags("Internal");

        return app;
    }
}
