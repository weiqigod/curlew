// Dev/Testing-only endpoint for auditing recently-sent emails.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by the M14-021 convergence e2e spec to assert that billing_receipt emails are queued.
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Notifications.Email;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>GET /internal/test/email-audit</c> (Development + Testing only).
/// Returns a JSON list of recently-sent <see cref="EmailMessage"/> entries recorded
/// by <see cref="IRecentlySentEmailLog"/>. Mirrors the <c>seed-refresh</c> pattern.
/// </summary>
public static class InternalEmailAuditEndpoint
{
    /// <summary>
    /// Conditionally registers the email-audit endpoint when the environment is Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalEmailAuditEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapGet("/internal/test/email-audit",
            (IRecentlySentEmailLog log, int? limit) =>
                HttpResults.Ok(new { entries = log.Recent(limit ?? 50) }))
            .AllowAnonymous()
            .DisableAntiforgery()
            .AddEndpointFilter<InternalAccessFilter>()
            .WithName("InternalEmailAudit")
            .WithTags("Internal");

        return app;
    }
}
