// Dev/Testing-only endpoint that forces one TrialExpiryNotifier tick for e2e assertions.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by scripts/m16-e2e.sh to trigger the trial_expiring email during the e2e scenario.
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Notifications.Trials;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /internal/test/trial-expiry-tick</c> (Development + Testing only).
/// Resolves the registered <see cref="TrialExpiryNotifier"/> from the hosted-service list
/// and invokes <c>TickOnceAsync</c> with <c>RunAtUtc=now</c> so the e2e script can
/// trigger a trial-expiry email without waiting for the 09:00 UTC wall-clock fire.
/// Returns 503 when the notifier is absent (e.g. in Testing where it is intentionally
/// excluded).
/// </summary>
public static class InternalTrialTickEndpoint
{
    /// <summary>
    /// Conditionally registers the trial-expiry-tick endpoint when the environment is Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalTrialTickEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/internal/test/trial-expiry-tick", async (
            IServiceProvider services,
            CancellationToken ct) =>
        {
            // TrialExpiryNotifier is registered via AddHostedService; resolve from the
            // hosted-service enumeration. Returns null in Testing (excluded from DI).
            var notifier = services
                .GetServices<IHostedService>()
                .OfType<TrialExpiryNotifier>()
                .FirstOrDefault();

            if (notifier is null)
                return HttpResults.StatusCode(StatusCodes.Status503ServiceUnavailable);

            var opts = new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 500 };
            await notifier.TickOnceAsync(opts, ct);

            return HttpResults.Ok(new { ticked = true });
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("InternalTrialExpiryTick")
        .WithTags("Internal");

        return app;
    }
}
