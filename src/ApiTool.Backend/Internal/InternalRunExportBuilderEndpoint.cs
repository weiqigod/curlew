// Dev/Testing-only endpoint for deterministically triggering one UserExportBuilderHost tick.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by the M18-004 e2e observable and integration tests to drive the builder synchronously.
using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Storage;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /api/v1/internal/test-hooks/run-export-builder</c> (Development + Testing only).
/// Constructs an ad-hoc <see cref="UserExportBuilderHost"/> tick from DI services so the
/// e2e observable can trigger the builder deterministically.
/// </summary>
public static class InternalRunExportBuilderEndpoint
{
    /// <summary>
    /// Conditionally registers the export-builder-tick endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalRunExportBuilderEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/api/v1/internal/test-hooks/run-export-builder", async (
            IServiceProvider services,
            CancellationToken ct) =>
        {
            var scopeFactory = services.GetRequiredService<IServiceScopeFactory>();
            var store = services.GetRequiredService<IObjectStore>();
            var clock = services.GetRequiredService<TimeProvider>();
            var logger = services.GetRequiredService<ILogger<UserExportBuilderHost>>();

            var host = new UserExportBuilderHost(scopeFactory, store, clock, logger);
            await host.TickOnceAsync(ct);

            return HttpResults.Ok(new { ticked = true });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("InternalRunExportBuilder")
        .WithTags("Internal");

        return app;
    }
}
