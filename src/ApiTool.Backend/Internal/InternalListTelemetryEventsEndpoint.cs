// Dev/Testing-only endpoint for listing telemetry events by install_id.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by M18-012 e2e convergence to assert cross-cluster integration of the telemetry pipeline.
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>GET /api/v1/internal/test-hooks/list-telemetry-events</c> (Development + Testing only).
/// Returns <c>{ "count": N, "rows": [ { "install_id": "...", "event_type": "...", "received_at": "..." } ] }</c>
/// filtered by <c>?install_id=</c>. Used by M18-012's e2e to assert cross-cluster integration
/// of the telemetry pipeline without inspecting raw DB rows.
/// </summary>
public static class InternalListTelemetryEventsEndpoint
{
    /// <summary>
    /// Conditionally registers the list-telemetry-events endpoint when the environment is
    /// Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalListTelemetryEventsEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapGet("/api/v1/internal/test-hooks/list-telemetry-events",
            async (string? install_id, AppDbContext db, CancellationToken ct) =>
            {
                if (string.IsNullOrWhiteSpace(install_id) ||
                    !Guid.TryParse(install_id, out var id))
                    return HttpResults.BadRequest(new { error = "install_id query param required (UUID)" });

                var rows = await db.Set<TelemetryEvent>()
                    .Where(e => e.InstallId == id)
                    .OrderByDescending(e => e.ReceivedAt)
                    .Select(e => new
                    {
                        install_id = e.InstallId,
                        event_type = e.EventType,
                        received_at = e.ReceivedAt,
                    })
                    .Take(500)
                    .ToListAsync(ct);

                return HttpResults.Ok(new { count = rows.Count, rows });
            })
            .AllowAnonymous()
            .AddEndpointFilter<InternalAccessFilter>()
            .WithName("InternalListTelemetryEvents")
            .WithTags("Internal");

        return app;
    }
}
