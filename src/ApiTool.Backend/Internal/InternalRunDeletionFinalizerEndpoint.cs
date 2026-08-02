// Dev/Testing-only endpoint for deterministically triggering one UserDeletionFinalizerHost tick.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by the M18-005 e2e observable and integration tests to drive the finalizer synchronously.
using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Notifications.Email;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /api/v1/internal/test-hooks/run-deletion-finalizer</c> (Development + Testing only).
/// Constructs an ad-hoc <see cref="UserDeletionFinalizerHost"/> tick from DI services so the
/// e2e observable can trigger the finalizer deterministically.
/// </summary>
public static class InternalRunDeletionFinalizerEndpoint
{
    /// <summary>
    /// Conditionally registers the deletion-finalizer-tick endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalRunDeletionFinalizerEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/api/v1/internal/test-hooks/run-deletion-finalizer", async (
            IServiceProvider services,
            CancellationToken ct) =>
        {
            var scopeFactory = services.GetRequiredService<IServiceScopeFactory>();
            var emailQueue = services.GetRequiredService<IEmailQueue>();
            var clock = services.GetRequiredService<TimeProvider>();
            var logger = services.GetRequiredService<ILogger<UserDeletionFinalizerHost>>();

            var host = new UserDeletionFinalizerHost(scopeFactory, emailQueue, clock, logger);
            await host.TickOnceAsync(ct);

            return HttpResults.Ok(new { ticked = true });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("InternalRunDeletionFinalizer")
        .WithTags("Internal");

        return app;
    }
}
